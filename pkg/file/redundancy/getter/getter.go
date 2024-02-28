// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/klauspost/reedsolomon"
)

// decoder is a private implementation of storage.Getter
// if retrieves children of an intermediate chunk potentially using erasure decoding
// it caches sibling chunks if erasure decoding started already
type decoder struct {
	fetcher   storage.Getter  // network retrieval interface to fetch chunks
	putter    storage.Putter  // interface to local storage to save reconstructed chunks
	cache     map[string]int  // map from chunk address shard position index
	addrs     []swarm.Address // all addresses of the intermediate chunk
	inflight  []atomic.Bool   // locks to protect wait channels and RS buffer
	waits     []chan struct{} // wait channels for each chunk
	derrs     []error         // decoding errors
	ferrs     []error         // fetch errors
	decoded   chan struct{}   // signal that the decoding is finished
	err       error           // error of the last erasure decoding
	rsbuf     [][]byte        // RS buffer of data + parity shards for erasure decoding
	chunks    [][]byte        // chunks fetched
	lastLen   int             // length of the last data chunk in the RS buffer
	shardCnt  int             // number of data shards
	parityCnt int             // number of parity shards
	fetched   *counter        // count number of fetched chunks
	failed    *counter        // count number of failed retrievals
	mu        sync.Mutex      // mutex to protect the decoder state
	wg        sync.WaitGroup  // wait group to wait for all goroutines to finish
	cancel    func()          // cancel function for RS decoding
	remove    func()          // callback to remove decoder from decoders cache
	config    Config          // configuration
}

type Getter interface {
	storage.Getter
	io.Closer
}

// New returns a decoder object used to retrieve children of an intermediate chunk
func New(addrs []swarm.Address, shardCnt int, g storage.Getter, p storage.Putter, remove func(), conf Config) Getter {
	ctx, cancel := context.WithCancel(context.Background())
	size := len(addrs)

	d := &decoder{
		fetcher:   g,
		putter:    p,
		addrs:     addrs,
		cache:     make(map[string]int, size),
		inflight:  make([]atomic.Bool, shardCnt),
		waits:     make([]chan struct{}, shardCnt),
		ferrs:     make([]error, shardCnt),
		derrs:     make([]error, shardCnt),
		decoded:   make(chan struct{}),
		rsbuf:     make([][]byte, size),
		chunks:    make([][]byte, size),
		fetched:   newCounter(shardCnt - 1),
		failed:    newCounter(size - shardCnt),
		cancel:    cancel,
		remove:    remove,
		shardCnt:  shardCnt,
		parityCnt: size - shardCnt,
		config:    conf,
	}

	if conf.Strategy == RACE || !conf.Strict {
		go func() { // if not enough shards are retrieved, signal that decoding is finished
			select {
			case <-ctx.Done():
			case <-d.failed.c:
				d.close(ErrNotEnoughShards)
			}
		}()
	} else {
		d.fetched.cancel()
		d.failed.cancel()
		d.close(ErrRecoveryUnavailable)
	}

	// init cache and wait channels
	// after init, they are immutable, need no locking
	for i := 0; i < shardCnt; i++ {
		d.cache[addrs[i].ByteString()] = i
		d.waits[i] = make(chan struct{})
	}

	// prefetch chunks according to strategy
	d.wg.Add(1)
	go d.run(ctx)
	return d
}

// Get will call parities and other sibling chunks if the chunk address cannot be retrieved
// assumes it is called for data shards only
func (g *decoder) Get(ctx context.Context, addr swarm.Address) (c swarm.Chunk, err error) {
	i, ok := g.cache[addr.ByteString()]
	if !ok {
		return nil, storage.ErrNotFound
	}
	if g.fly(i) {
		g.wg.Add(1)
		go g.fetch(ctx, i)
	}

	fetched := g.waits[i]
	decoded := g.decoded
	for {
		select {
		case <-fetched:
			// if the chunk is retrieval is completed and there is no error, return the chunk
			if g.ferrs[i] == nil {
				return swarm.NewChunk(addr, g.getData(i, g.chunks)), nil
			}
			fetched = nil

		case <-decoded:
			// if the RS decoding is completed
			// if there was no error, return the chunk from the RS buffer (recovery)
			if g.err == nil {
				return swarm.NewChunk(addr, g.getData(i, g.rsbuf)), nil
			}

			// otherwise (if there was an error), and chunk retrieval had already been attempted,
			// return the combined error of fetching and the decoding
			if fetched == nil {
				return nil, errors.Join(g.err, g.ferrs[i])
			}
			// continue waiting for retrieval to complete
			// disable this case and enable the case waiting for retrieval to complete
			decoded = nil

		case <-ctx.Done():
			// if the context is cancelled, return the error
			return nil, errors.Join(g.err, fmt.Errorf("Get: %w", ctx.Err()))
		}
	}
}

// setData sets the data shard in the chunks slice
func (g *decoder) setData(i int, chdata []byte) {
	data := chdata
	// pad the chunk with zeros if it is smaller than swarm.ChunkSize
	if len(data) < swarm.ChunkWithSpanSize {
		g.lastLen = len(data)
		data = make([]byte, swarm.ChunkWithSpanSize)
		copy(data, chdata)
	}
	g.chunks[i] = data
}

// getData returns the data shard from the RS buffer
func (g *decoder) getData(i int, s [][]byte) []byte {
	if i == g.shardCnt-1 && g.lastLen > 0 {
		return s[i][:g.lastLen] // cut padding
	}
	return s[i]
}

// fly commits to retrieve the chunk (fly and land)
// it marks a chunk as inflight and returns true unless it is already inflight
// the atomic bool implements the signal for a singleflight pattern
func (g *decoder) fly(i int) (success bool) {
	return g.inflight[i].CompareAndSwap(false, true)
}

// fetch retrieves a chunk from the underlying storage
// it must be called asynchonously and only once for each chunk (singleflight pattern)
// it races with erasure recovery; the latter takes precedence even if it started later
// due to the fact that erasure recovery could only implement global locking on all shards
func (g *decoder) fetch(ctx context.Context, i int) {
	defer g.wg.Done()
	// set the timeout context for the fetch
	fctx, cancel := context.WithTimeout(ctx, g.config.FetchTimeout)
	defer cancel()

	// retrieve the chunk using the underlying storage
	ch, err := g.fetcher.Get(fctx, g.addrs[i])
	// if there was an error, the error and return
	g.mu.Lock()
	defer g.mu.Unlock()
	// whatever happens, signal that the chunk retrieval is finished
	if err != nil {
		if i < g.shardCnt {
			g.ferrs[i] = err
		}
		g.failed.inc()
		return
	}
	if i < g.shardCnt {
		defer close(g.waits[i])
	}

	//  write chunk to rsbuf and signal waiters
	g.setData(i, ch.Data()) // save the chunk in the RS buffer

	// if all chunks are retrieved, signal ready
	g.fetched.inc()
}

// missing gathers missing data shards not yet retrieved or retrieved with an error
// it sets the chunk as inflight and returns the index of the missing data shards
func (g *decoder) missing() (m []int) {
	// initialize RS buffer
	for i, ch := range g.chunks {
		if len(ch) > 0 {
			g.rsbuf[i] = ch
			if i < g.shardCnt {
				_ = g.fly(i) // commit (RS) or will commit to retrieve the chunk
			}
		} else {
			if i < g.shardCnt {
				g.derrs[i] = g.ferrs[i]
				m = append(m, i)
			}
		}
	}
	return m
}

// decode uses Reed-Solomon erasure coding decoder to recover data shards
// it must be called after shardcnt shards are retrieved
// it must be called under mutex protection
func (g *decoder) decode(ctx context.Context) error {
	enc, err := reedsolomon.New(g.shardCnt, g.parityCnt)
	if err != nil {
		return err
	}

	// decode data
	return enc.ReconstructData(g.rsbuf)
}

// recover wraps the stages of data shard recovery:
// 1. gather missing data shards
// 2. decode using Reed-Solomon decoder
// 3. save reconstructed chunks
func (g *decoder) recover(ctx context.Context) (err error) {

	defer func() {
		g.close(err)
	}()
	g.mu.Lock()
	defer g.mu.Unlock()

	// gather missing shards
	m := g.missing()

	// decode using Reed-Solomon decoder
	err = g.decode(ctx)
	if err != nil {
		return err
	}

	// save chunks
	err = g.save(ctx, m)
	return err
}

// save iterate over reconstructed shards and puts the corresponding chunks to local storage
func (g *decoder) save(ctx context.Context, missing []int) error {
	for _, i := range missing {
		if err := g.putter.Put(ctx, swarm.NewChunk(g.addrs[i], g.getData(i, g.rsbuf))); err != nil {
			return err
		}
	}
	return nil
}

func (g *decoder) close(err error) {
	g.err = err
	close(g.decoded)
}

// Close terminates the prefetch loop, waits for all goroutines to finish and
// removes the decoder from the cache
// it implements the io.Closer interface
func (g *decoder) Close() error {
	g.cancel()
	g.wg.Wait()
	g.remove()
	return nil
}
