// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/klauspost/reedsolomon"
)

var (
	RetryInterval = 200 * time.Millisecond // retry interval for failed retrievals
)

// getter is the private implementation of storage.Getter
// if retrieves children of an intermediate chunk potentially using erasure decoding
// it caches sibling chunks if erasure decoding started already
type getter struct {
	fetcher   storage.Getter  // network retrieval interface to fetch chunks
	putter    storage.Putter  // interface to local storage to save reconstructed chunks
	addrs     []swarm.Address // all addresses of the intermediate chunk
	inflight  []atomic.Bool   // locks to protect wait channels and RS buffer
	cache     map[string]int  // map from chunk address shard position index
	waits     []chan struct{} // wait channels for each chunk
	rsbuf     [][]byte        // RS buffer of data + parity shards for erasure decoding
	fetched   atomic.Int32    // count successful retrievals
	ready     chan struct{}   // signal channel for successful retrieval of shardCnt chunks
	shardCnt  int             // number of data shards
	parityCnt int             // number of parity shards
	wg        sync.WaitGroup  // wait group to wait for all goroutines to finish
	mu        sync.Mutex      // mutex to protect buffer
	cancel    func()          // cancel function for RS decoding
}

type Getter interface {
	storage.Getter
	io.Closer
}

// New returns a getter object which is used to retrieve children of an intermediate chunk
func New(addrs []swarm.Address, shardCnt int, g storage.Getter, p storage.Putter, strategy Strategy, strict bool) Getter {
	ctx, cancel := context.WithCancel(context.Background())
	size := len(addrs)

	rsg := &getter{
		fetcher:   g,
		putter:    p,
		addrs:     addrs,
		inflight:  make([]atomic.Bool, size),
		cache:     make(map[string]int, size),
		waits:     make([]chan struct{}, shardCnt),
		rsbuf:     make([][]byte, size),
		ready:     make(chan struct{}, 1),
		cancel:    cancel,
		shardCnt:  shardCnt,
		parityCnt: size - shardCnt,
	}

	// after init, cache and wait channels are immutable, need no locking
	for i := 0; i < shardCnt; i++ {
		rsg.cache[addrs[i].ByteString()] = i
		rsg.waits[i] = make(chan struct{})
	}

	// prefetch chunks according to strategy
	go rsg.prefetch(ctx, int(strategy), strict)
	return rsg
}

// Get will call parities and other sibling chunks if the chunk address cannot be retrieved
// assumes it is called for data shards only
func (g *getter) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	i, ok := g.cache[addr.ByteString()]
	if !ok {
		return nil, storage.ErrNotFound
	}
	if g.fly(i, true) {
		g.wg.Add(1)
		go func() {
			g.fetch(ctx, i)
			g.wg.Done()
		}()
	}
	select {
	case <-g.waits[i]:
		return swarm.NewChunk(addr, g.rsbuf[i]), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// fly commits to retrieve the chunk (fly and land)
// it marks a chunk as inflight and returns true unless it is already inflight
// the atomic bool implements a singleflight pattern
func (g *getter) fly(i int, up bool) (success bool) {
	return g.inflight[i].CompareAndSwap(!up, up)
}

// fetch retrieves a chunk from the underlying storage
// it must be called asynchonously and only once for each chunk (singleflight pattern)
// it races with erasure recovery which takes precedence even if it started later
// due to the fact that erasure recovery could only implement global locking on all shards
func (g *getter) fetch(ctx context.Context, i int) {
	ch, err := g.fetcher.Get(ctx, g.addrs[i])
	var wait chan struct{}
	if i < len(g.waits) {
		wait = g.waits[i]
	}

	if err != nil {
		select {
		case <-wait: // if chunk is retrieved, ignore
		case <-ctx.Done(): // if context is cancelled, ignore
		case <-time.After(RetryInterval): // retry after retry interval
			g.fetch(ctx, i)
		}
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	select {
	case <-wait: // if chunk is retrieved, ignore
		return
	case <-ctx.Done(): // if context is cancelled, ignore
		_ = g.fly(i, false) // unset inflight
		return
	default:
	}
	g.rsbuf[i] = ch.Data() // save the chunk in the RS buffer
	if wait != nil {
		close(wait) // signal that the chunk is retrieved
	}
	n := g.fetched.Add(1)
	if n == int32(g.shardCnt) {
		g.ready <- struct{}{} // signal that just enough chunks are retrieved for decoding
	}
}

// missing gathers missing data shards not yet retrieved
// it sets the chunk as inflight and returns the index of the missing data shards
func (g *getter) missing() (m []int) {
	for i := 0; i < g.shardCnt; i++ {
		select {
		case <-g.waits[i]: // if chunk is retrieved, ignore
			continue
		default:
		}
		_ = g.fly(i, true) // commit (RS) or will commit to retrieve the chunk
		m = append(m, i)   // remember the missing chunk
	}
	return m
}

// decode uses Reed-Solomon erasure coding decoder to recover data shards
// it must be called after shqrdcnt shards are retrieved
// it must be called under g.mu mutex protection
func (g *getter) decode(ctx context.Context) error {
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
func (g *getter) recover(ctx context.Context) error {
	// buffer lock acquired
	g.mu.Lock()
	defer g.mu.Unlock()

	// gather missing shards
	m := g.missing()
	if len(m) == 0 {
		return nil
	}

	// decode using Reed-Solomon decoder
	if err := g.decode(ctx); err != nil {
		return err
	}

	// close wait channels for missing chunks
	for _, i := range m {
		close(g.waits[i])
	}

	// save chunks
	return g.save(ctx, m)
}

// save iterate over reconstructed shards and puts the corresponding chunks to local storage
func (g *getter) save(ctx context.Context, missing []int) error {
	for _, i := range missing {
		if err := g.putter.Put(ctx, swarm.NewChunk(g.addrs[i], g.rsbuf[i])); err != nil {
			return err
		}
	}
	return nil
}

func (g *getter) Close() error {
	g.cancel()
	g.wg.Wait()
	return nil
}
