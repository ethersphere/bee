// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/pkg/encryption/store"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/klauspost/reedsolomon"
)

// chunk is initialized if recovery happened already
type chunk struct {
	data []byte        // chunk data, can be encrypted
	wait chan struct{} // chunk is under retrieval
}

// getter retrieves children of an intermediate chunk
// it caches sibling chunks if erasure decoding was called on the level already
type getter struct {
	storage.Getter
	sAddresses []swarm.Address  // shard addresses
	pAddresses []swarm.Address  // parity addresses
	cache      map[string]chunk // map from chunk address to cached shard chunk data; TODO mutex
	encrypted  bool             // swarm datashards are encrypted
}

// New returns a getter object which is used to retrieve children of an intermediate chunk
func New(sAddresses, pAddresses []swarm.Address, g storage.Getter) storage.Getter {
	cache := make(map[string]chunk)
	encrypted := len(sAddresses[0].Bytes()) == swarm.HashSize*2

	return &getter{
		Getter:     g,
		sAddresses: sAddresses,
		pAddresses: pAddresses,
		cache:      cache,
		encrypted:  encrypted,
	}
}

// Get will call parities and other sibling chunks if the chunk address cannot be retrieved
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

// processing returns whether the recovery workflow has been started
func (g getter) processing(addr swarm.Address) bool {
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

	if c.data != nil {
		return g.cacheDataToChunk(addr, c.data)
	}

	select {
	case <-c.wait:
		return g.cacheDataToChunk(addr, c.data)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// executeStrategies executes recovery strategies from redundancy for the given swarm address
func (g *getter) executeStrategies(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	err := g.explorerStrategy(ctx, addr)
	if err != nil {
		// TODO cautious strategy, retrieve all chunks, stop when it is enough
	}

	return g.getAfterProcessed(ctx, addr)
}

// explorerStrategy identifies how many shards are missing outside of the current missing address
// also fills up cache the first time
func (g *getter) explorerStrategy(ctx context.Context, addr swarm.Address) error {
	var addresses []swarm.Address
	for _, a := range g.sAddresses {
		if a.Compare(addr) != 0 {
			addresses = append(addresses, a)
		}
	}
	// add one parity chunk since addr cannot be retrieved
	addresses = append(addresses, g.pAddresses[0])

	var (
		wg            sync.WaitGroup
		missingChunks uint32
	)
	wg.Add(len(addresses))
	for _, a := range addresses {
		go func(a swarm.Address) {
			defer wg.Done()
			c := &chunk{
				wait: make(chan struct{}),
			}
			g.cache[a.String()] = *c
			// enrypted chunk data should remain encrypted
			address := swarm.NewAddress(a.Bytes()[:swarm.HashSize])
			ch, err := g.Getter.Get(ctx, address)
			if err != nil {
				// available map holds which chunks exactly
				atomic.AddUint32(&missingChunks, 1)
				return
			}
			c.data = ch.Data()
			c.wait <- struct{}{}
		}(a)
	}
	wg.Wait()

	if missingChunks > 0 {
		return fmt.Errorf("redundancy getter: there are %d missing chunks in order to do recovery", missingChunks)
	}

	return g.erasureDecode()
}

// erasureDecode perform Reed-Solomon recovery on data
// assumes it is called after filling up cache with the required amount of shards and parities
func (g *getter) erasureDecode() error {
	shards := len(g.sAddresses)
	parities := len(g.pAddresses)
	n := shards + parities
	data := make([][]byte, n)
	enc, err := reedsolomon.New(shards, parities)
	if err != nil {
		return err
	}

	var missingShardIndices []int
	for i, addr := range g.sAddresses {
		c, ok := g.cache[addr.String()]
		if !ok {
			missingShardIndices = append(missingShardIndices, i)
		}
		data[i] = c.data
	}
	for i := shards; i < n; i++ {
		addr := g.pAddresses[i-shards]
		c, ok := g.cache[addr.String()]
		if ok {
			data[i] = c.data // later strategies may require parity data
		}
	}

	err = enc.ReconstructData(data)
	if err != nil {
		return err
	}

	for _, i := range missingShardIndices {
		addr := g.sAddresses[i]
		c, ok := g.cache[addr.String()]
		if ok {
			c.data = data[i]
			c.wait <- struct{}{}
			// not necessary to update `available` since decoding was successful
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
