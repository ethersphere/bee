// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/encryption/store"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/klauspost/reedsolomon"
)

// inflightChunk is initialized if recovery happened already
type inflightChunk struct {
	pos  int           // chunk index in the erasureData/intermediate chunk
	wait chan struct{} // chunk is under retrieval
}

// getter retrieves children of an intermediate chunk
// it caches sibling chunks if erasure decoding was called on the level already
type getter struct {
	storage.Getter
	mu          sync.Mutex
	sAddresses  []swarm.Address          // shard addresses
	pAddresses  []swarm.Address          // parity addresses
	cache       map[string]inflightChunk // map from chunk address to cached shard chunk data
	erasureData [][]byte                 // data + parity shards for erasure decoding; TODO mutex
	encrypted   bool                     // swarm datashards are encrypted
}

// New returns a getter object which is used to retrieve children of an intermediate chunk
func New(sAddresses, pAddresses []swarm.Address, g storage.Getter) storage.Getter {
	encrypted := len(sAddresses[0].Bytes()) == swarm.HashSize*2
	shards := len(sAddresses)
	parities := len(pAddresses)
	n := shards + parities
	erasureData := make([][]byte, n)
	cache := make(map[string]inflightChunk, n)

	return &getter{
		Getter:      g,
		sAddresses:  sAddresses,
		pAddresses:  pAddresses,
		cache:       cache,
		encrypted:   encrypted,
		erasureData: erasureData,
	}
}

// Get will call parities and other sibling chunks if the chunk address cannot be retrieved
// assumes it is called for data shards only
func (g *getter) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	if g.processing(addr) {
		return g.getAfterProcessed(ctx, addr)
	}

	ch, err := g.Getter.Get(ctx, addr)
	if err == nil {
		return ch, nil
	}
	if errors.Is(storage.ErrNotFound, err) && len(g.pAddresses) == 0 {
		return nil, fmt.Errorf("redundancy getter: cannot get chunk %s because no redundancy added", addr.ByteString())
	}

	// during the get, the recovery may have started by other process
	if g.processing(addr) {
		return g.getAfterProcessed(ctx, addr)
	}

	return g.executeStrategies(ctx, addr)
}

// Inc increments the counter for the given key.
func (g *getter) setErasureData(index int, data []byte) {
	g.mu.Lock()
	g.erasureData[index] = data
	g.mu.Unlock()
}

// processing returns whether the recovery workflow has been started
func (g *getter) processing(addr swarm.Address) bool {
	_, ok := g.cache[addr.String()]
	return ok
}

// getAfterProcessed returns chunk from the cache
func (g *getter) getAfterProcessed(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	c, ok := g.cache[addr.String()]
	// sanity check
	if !ok {
		return nil, fmt.Errorf("redundancy getter: chunk %s should have been in the cache", addr.String())
	}

	if g.erasureData[c.pos] != nil {
		return g.cacheDataToChunk(addr, g.erasureData[c.pos])
	}

	select {
	case <-c.wait:
		return g.cacheDataToChunk(addr, g.erasureData[c.pos])
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// executeStrategies executes recovery strategies from redundancy for the given swarm address
func (g *getter) executeStrategies(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	g.initCache()
	err := g.cautiousStrategy(ctx)
	if err != nil {
		return nil, err
	}

	return g.getAfterProcessed(ctx, addr)
}

// initCache initializes the cache mapping values for chunks with which indicating the start of the recovery process as well
func (g *getter) initCache() {
	for i, addr := range g.sAddresses {
		g.cache[addr.String()] = inflightChunk{
			pos:  i,
			wait: make(chan struct{}),
		}
	}
	for i, addr := range g.pAddresses {
		g.cache[addr.String()] = inflightChunk{
			pos: len(g.sAddresses) + i,
			// no wait channel initialization is needed
		}
	}
}

// cautiousStrategy requests all chunks (data and parity) on the level
// and if it has enough data for erasure decoding then it cancel other requests
func (g *getter) cautiousStrategy(ctx context.Context) error {
	requiredChunks := len(g.sAddresses)
	subContext, cancelContext := context.WithCancel(ctx)
	retrievedCh := make(chan struct{}, requiredChunks)
	var wg sync.WaitGroup

	for _, a := range g.sAddresses {
		wg.Add(1)
		c := g.cache[a.String()]
		go func(a swarm.Address, c inflightChunk) {
			defer wg.Done()
			// enrypted chunk data should remain encrypted
			address := swarm.NewAddress(a.Bytes()[:swarm.HashSize])
			ch, err := g.Getter.Get(subContext, address)
			if err != nil {
				return
			}
			g.setErasureData(c.pos, ch.Data())
			close(c.wait)
			retrievedCh <- struct{}{}
		}(a, c)
	}
	for _, a := range g.pAddresses {
		wg.Add(1)
		c := g.cache[a.String()]
		go func(address swarm.Address, c inflightChunk) {
			defer wg.Done()
			ch, err := g.Getter.Get(subContext, address)
			if err != nil {
				return
			}
			g.setErasureData(c.pos, ch.Data())
			retrievedCh <- struct{}{}
		}(a, c)
	}

	// Goroutine to wait for WaitGroup completion
	go func() {
		wg.Wait()
		close(retrievedCh)
	}()
	retrieved := 0
	for retrieved < requiredChunks {
		_, ok := <-retrievedCh
		if !ok {
			break
		}
		retrieved++
	}
	cancelContext()

	if retrieved < requiredChunks {
		return fmt.Errorf("redundancy getter: there are %d missing chunks in order to do recovery", requiredChunks-retrieved)
	}

	return g.erasureDecode()
}

// erasureDecode perform Reed-Solomon recovery on data
// assumes it is called after filling up cache with the required amount of shards and parities
func (g *getter) erasureDecode() error {
	enc, err := reedsolomon.New(len(g.sAddresses), len(g.pAddresses))
	if err != nil {
		return err
	}

	err = enc.ReconstructData(g.erasureData)
	if err != nil {
		return err
	}

	// close wait channels
	for _, addr := range g.sAddresses {
		c, ok := g.cache[addr.String()]
		if ok && channelIsClosed(c.wait) {
			close(c.wait)
		}
	}
	return nil
}

// cacheDataToChunk transforms passed chunk data to legit swarm chunk
func (g *getter) cacheDataToChunk(addr swarm.Address, chData []byte) (swarm.Chunk, error) {
	if g.encrypted {
		data, err := store.DecryptChunkData(chData, addr.Bytes()[swarm.HashSize:])
		if err != nil {
			return nil, err
		}
		chData = data
	}

	return swarm.NewChunk(addr, chData), nil
}

func channelIsClosed(wait <-chan struct{}) bool {
	select {
	case <-wait:
		return true
	default:
		return false
	}
}
