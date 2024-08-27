// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/klauspost/reedsolomon"
)

var (
	errStrategyNotAllowed = errors.New("strategy not allowed")
	errStrategyFailed     = errors.New("strategy failed")
)

// decoder is a private implementation of storage.Getter
// if retrieves children of an intermediate chunk potentially using erasure decoding
// it caches sibling chunks if erasure decoding started already
type decoder struct {
	fetcher      storage.Getter  // network retrieval interface to fetch chunks
	putter       storage.Putter  // interface to local storage to save reconstructed chunks
	addrs        []swarm.Address // all addresses of the intermediate chunk
	inflight     []atomic.Bool   // locks to protect wait channels and RS buffer
	cache        map[string]int  // map from chunk address shard position index
	waits        []chan error    // wait channels for each chunk
	rsbuf        [][]byte        // RS buffer of data + parity shards for erasure decoding
	goodRecovery chan struct{}   // signal channel for successful retrieval of shardCnt chunks
	badRecovery  chan struct{}   // signals that either the recovery has failed or not allowed to run
	initRecovery chan struct{}   // signals that the recovery has been initialized
	lastLen      int             // length of the last data chunk in the RS buffer
	shardCnt     int             // number of data shards
	parityCnt    int             // number of parity shards
	mu           sync.Mutex      // mutex to protect buffer
	fetchedCnt   atomic.Int32    // count successful retrievals
	failedCnt    atomic.Int32    // count successful retrievals
	remove       func(error)     // callback to remove decoder from decoders cache
	config       Config          // configuration
	logger       log.Logger
}

// New returns a decoder object used to retrieve children of an intermediate chunk
func New(addrs []swarm.Address, shardCnt int, g storage.Getter, p storage.Putter, remove func(error), conf Config) storage.Getter {
	size := len(addrs)

	d := &decoder{
		fetcher:      g,
		putter:       p,
		addrs:        addrs,
		inflight:     make([]atomic.Bool, size),
		cache:        make(map[string]int, size),
		waits:        make([]chan error, size),
		rsbuf:        make([][]byte, size),
		goodRecovery: make(chan struct{}),
		badRecovery:  make(chan struct{}),
		initRecovery: make(chan struct{}),
		remove:       remove,
		shardCnt:     shardCnt,
		parityCnt:    size - shardCnt,
		config:       conf,
		logger:       conf.Logger.WithName("redundancy").Build(),
	}

	// after init, cache and wait channels are immutable, need no locking
	for i := 0; i < shardCnt; i++ {
		d.cache[addrs[i].ByteString()] = i
	}

	// after init, cache and wait channels are immutable, need no locking
	for i := 0; i < size; i++ {
		d.waits[i] = make(chan error)
	}

	go d.prefetch()

	return d
}

// Get will call parities and other sibling chunks if the chunk address cannot be retrieved
// assumes it is called for data shards only
func (g *decoder) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	i, ok := g.cache[addr.ByteString()]
	if !ok {
		return nil, storage.ErrNotFound
	}
	err := g.fetch(ctx, i, true)
	if err != nil {
		return nil, err
	}
	return swarm.NewChunk(addr, g.getData(i)), nil
}

// fetch retrieves a chunk from the netstore if it is the first time the chunk is fetched.
// If the fetch fails and waiting for the recovery is allowed, the function will wait
// for either a good or bad recovery signal.
func (g *decoder) fetch(ctx context.Context, i int, waitForRecovery bool) (err error) {

	waitRecovery := func(err error) error {
		if !waitForRecovery {
			return err
		}

		select {
		case <-g.badRecovery:
			return storage.ErrNotFound
		case <-g.goodRecovery:
			g.logger.Debug("recovered chunk", "address", g.addrs[i])
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// recovery has started, wait for result instead of fetching from the network
	select {
	case <-g.initRecovery:
		return waitRecovery(nil)
	default:
	}

	// first time
	if g.fly(i) {

		fctx, cancel := context.WithTimeout(ctx, g.config.FetchTimeout)
		defer cancel()

		// when the recovery is triggered, we can terminate any inflight requests.
		// we do the extra bool check to not fire an unnecessary goroutine
		if waitForRecovery {
			go func() {
				defer cancel()
				select {
				case <-g.initRecovery:
				case <-fctx.Done():
				}
			}()
		}

		// retrieval
		ch, err := g.fetcher.Get(fctx, g.addrs[i])
		if err != nil {
			g.failedCnt.Add(1)
			close(g.waits[i])
			return waitRecovery(err)
		}

		g.setData(i, ch.Data())
		close(g.waits[i])
		g.fetchedCnt.Add(1)
		return nil
	}

	select {
	case <-g.waits[i]:
	case <-ctx.Done():
		return ctx.Err()
	}

	if g.getData(i) != nil {
		return nil
	}

	return waitRecovery(storage.ErrNotFound)
}

func (g *decoder) prefetch() {

	var err error
	defer func() {
		if err != nil {
			close(g.badRecovery)
		} else {
			close(g.goodRecovery)
		}
		g.remove(err)
	}()

	s := g.config.Strategy
	for ; s < strategyCnt; s++ {

		err = g.runStrategy(s)
		if err != nil && s == DATA || s == RACE {
			g.logger.Debug("failed strategy", "strategy", s)
		}

		if err == nil || g.config.Strict {
			break
		}
	}

	if err != nil {
		return
	}

	close(g.initRecovery)

	err = g.recover()
	if err == nil && s > DATA {
		g.logger.Debug("successful recovery", "strategy", s)
	}

}

func (g *decoder) runStrategy(s Strategy) error {

	// across the different strategies, the common goal is to fetch at least as many chunks
	// as the number of data shards.
	// DATA strategy has a max error tolerance of zero.
	// RACE strategy has a max error tolerance of number of parity chunks.
	var allowedErrs int
	var m []int

	switch s {
	case NONE:
		return errStrategyNotAllowed
	case DATA:
		// only retrieve data shards
		m = g.unattemptedDataShards()
		allowedErrs = 0
	case PROX:
		// proximity driven selective fetching
		// NOT IMPLEMENTED
		return errStrategyNotAllowed
	case RACE:
		allowedErrs = g.parityCnt
		// retrieve all chunks at once enabling race among chunks
		m = g.unattemptedDataShards()
		for i := g.shardCnt; i < len(g.addrs); i++ {
			m = append(m, i)
		}
	}

	if len(m) == 0 {
		return nil
	}

	c := make(chan error, len(m))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, i := range m {
		go func(i int) {
			c <- g.fetch(ctx, i, false)
		}(i)
	}

	for range c {
		if g.fetchedCnt.Load() >= int32(g.shardCnt) {
			return nil
		}
		if g.failedCnt.Load() > int32(allowedErrs) {
			return errStrategyFailed
		}
	}

	return nil
}

// recover wraps the stages of data shard recovery:
// 1. gather missing data shards
// 2. decode using Reed-Solomon decoder
// 3. save reconstructed chunks
func (g *decoder) recover() error {
	// gather missing shards
	m := g.missingDataShards()
	if len(m) == 0 {
		return nil // recovery is not needed as there are no missing data chunks
	}

	// decode using Reed-Solomon decoder
	if err := g.decode(); err != nil {
		return err
	}

	// save chunks
	return g.save(m)
}

// decode uses Reed-Solomon erasure coding decoder to recover data shards
// it must be called after shqrdcnt shards are retrieved
func (g *decoder) decode() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	enc, err := reedsolomon.New(g.shardCnt, g.parityCnt)
	if err != nil {
		return err
	}

	// decode data
	return enc.ReconstructData(g.rsbuf)
}

func (g *decoder) unattemptedDataShards() (m []int) {
	for i := 0; i < g.shardCnt; i++ {
		select {
		case <-g.waits[i]: // attempted
			continue
		default:
			m = append(m, i) // remember the missing chunk
		}
	}
	return m
}

// it must be called under mutex protection
func (g *decoder) missingDataShards() (m []int) {
	for i := 0; i < g.shardCnt; i++ {
		if g.getData(i) == nil {
			m = append(m, i)
		}
	}
	return m
}

// setData sets the data shard in the RS buffer
func (g *decoder) setData(i int, chdata []byte) {
	g.mu.Lock()
	defer g.mu.Unlock()

	data := chdata
	// pad the chunk with zeros if it is smaller than swarm.ChunkSize
	if len(data) < swarm.ChunkWithSpanSize {
		g.lastLen = len(data)
		data = make([]byte, swarm.ChunkWithSpanSize)
		copy(data, chdata)
	}
	g.rsbuf[i] = data
}

// getData returns the data shard from the RS buffer
func (g *decoder) getData(i int) []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	if i == g.shardCnt-1 && g.lastLen > 0 {
		return g.rsbuf[i][:g.lastLen] // cut padding
	}
	return g.rsbuf[i]
}

// fly commits to retrieve the chunk (fly and land)
// it marks a chunk as inflight and returns true unless it is already inflight
// the atomic bool implements a singleflight pattern
func (g *decoder) fly(i int) (success bool) {
	return g.inflight[i].CompareAndSwap(false, true)
}

// save iterate over reconstructed shards and puts the corresponding chunks to local storage
func (g *decoder) save(missing []int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, i := range missing {
		if err := g.putter.Put(context.Background(), swarm.NewChunk(g.addrs[i], g.rsbuf[i])); err != nil {
			return err
		}
	}
	return nil
}
