// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import (
	"context"
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
	fetcher    storage.Getter  // network retrieval interface to fetch chunks
	putter     storage.Putter  // interface to local storage to save reconstructed chunks
	addrs      []swarm.Address // all addresses of the intermediate chunk
	inflight   []atomic.Bool   // locks to protect wait channels and RS buffer
	cache      map[string]int  // map from chunk address shard position index
	waits      []chan struct{} // wait channels for each chunk
	rsbuf      [][]byte        // RS buffer of data + parity shards for erasure decoding
	ready      chan struct{}   // signal channel for successful retrieval of shardCnt chunks
	lastLen    int             // length of the last data chunk in the RS buffer
	shardCnt   int             // number of data shards
	parityCnt  int             // number of parity shards
	wg         sync.WaitGroup  // wait group to wait for all goroutines to finish
	mu         sync.Mutex      // mutex to protect buffer
	err        error           // error of the last erasure decoding
	fetchedCnt atomic.Int32    // count successful retrievals
	cancel     func()          // cancel function for RS decoding
	remove     func()          // callback to remove decoder from decoders cache
	config     Config          // configuration
}

type Getter interface {
	storage.Getter
	io.Closer
}

// New returns a decoder object used to retrieve children of an intermediate chunk
func New(addrs []swarm.Address, shardCnt int, g storage.Getter, p storage.Putter, remove func(), conf Config) Getter {
	ctx, cancel := context.WithCancel(context.Background())
	size := len(addrs)

	rsg := &decoder{
		fetcher:   g,
		putter:    p,
		addrs:     addrs,
		inflight:  make([]atomic.Bool, size),
		cache:     make(map[string]int, size),
		waits:     make([]chan struct{}, shardCnt),
		rsbuf:     make([][]byte, size),
		ready:     make(chan struct{}, 1),
		cancel:    cancel,
		remove:    remove,
		shardCnt:  shardCnt,
		parityCnt: size - shardCnt,
		config:    conf,
	}

	// after init, cache and wait channels are immutable, need no locking
	for i := 0; i < shardCnt; i++ {
		rsg.cache[addrs[i].ByteString()] = i
		rsg.waits[i] = make(chan struct{})
	}

	// prefetch chunks according to strategy
	if !conf.Strict || conf.Strategy != NONE {
		rsg.wg.Add(1)
		go func() {
			rsg.err = rsg.prefetch(ctx)
			rsg.wg.Done()
		}()
	}
	return rsg
}

// Get will call parities and other sibling chunks if the chunk address cannot be retrieved
// assumes it is called for data shards only
func (g *decoder) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
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
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return swarm.NewChunk(addr, g.getData(i)), nil
}

// setData sets the data shard in the RS buffer
func (g *decoder) setData(i int, chdata []byte) {
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
	if i == g.shardCnt-1 && g.lastLen > 0 {
		return g.rsbuf[i][:g.lastLen] // cut padding
	}
	return g.rsbuf[i]
}

// fly commits to retrieve the chunk (fly and land)
// it marks a chunk as inflight and returns true unless it is already inflight
// the atomic bool implements a singleflight pattern
func (g *decoder) fly(i int, up bool) (success bool) {
	return g.inflight[i].CompareAndSwap(!up, up)
}

// fetch retrieves a chunk from the underlying storage
// it must be called asynchonously and only once for each chunk (singleflight pattern)
// it races with erasure recovery which takes precedence even if it started later
// due to the fact that erasure recovery could only implement global locking on all shards
func (g *decoder) fetch(ctx context.Context, i int) {
	fctx, cancel := context.WithTimeout(ctx, g.config.FetchTimeout)
	defer cancel()
	ch, err := g.fetcher.Get(fctx, g.addrs[i])
	if err != nil {
		_ = g.fly(i, false) // unset inflight
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if i < len(g.waits) {
		select {
		case <-g.waits[i]: // if chunk is retrieved, ignore
			return
		default:
		}
	}

	select {
	case <-ctx.Done(): // if context is cancelled, ignore
		_ = g.fly(i, false) // unset inflight
		return
	default:
	}

	//  write chunk to rsbuf and signal waiters
	g.setData(i, ch.Data()) // save the chunk in the RS buffer
	if i < len(g.waits) {   // if the chunk is a data shard
		close(g.waits[i]) // signal that the chunk is retrieved
	}

	// if all chunks are retrieved, signal ready
	n := g.fetchedCnt.Add(1)
	if n == int32(g.shardCnt) {
		close(g.ready) // signal that just enough chunks are retrieved for decoding
	}
}

// missing gathers missing data shards not yet retrieved
// it sets the chunk as inflight and returns the index of the missing data shards
func (g *decoder) missing() (m []int) {
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
func (g *decoder) recover(ctx context.Context) error {
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
func (g *decoder) save(ctx context.Context, missing []int) error {
	for _, i := range missing {
		if err := g.putter.Put(ctx, swarm.NewChunk(g.addrs[i], g.rsbuf[i])); err != nil {
			return err
		}
	}
	return nil
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
