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

/// ERRORS

type cannotRecoverError struct {
	missingChunks int
}

func (e cannotRecoverError) Error() string {
	return fmt.Sprintf("redundancy getter: there are %d missing chunks in order to do recovery", e.missingChunks)
}

func IsCannotRecoverError(err error, missingChunks int) bool {
	return errors.Is(err, cannotRecoverError{missingChunks})
}

type isNotRecoveredError struct {
	chAddress string
}

func (e isNotRecoveredError) Error() string {
	return fmt.Sprintf("redundancy getter: chunk with address %s is not recovered", e.chAddress)
}

func IsNotRecoveredError(err error, chAddress string) bool {
	return errors.Is(err, isNotRecoveredError{chAddress})
}

type noDataAddressIncludedError struct {
	chAddress string
}

func (e noDataAddressIncludedError) Error() string {
	return fmt.Sprintf("redundancy getter: no data shard address given with chunk address %s", e.chAddress)
}

func IsNoDataAddressIncludedError(err error, chAddress string) bool {
	return errors.Is(err, noDataAddressIncludedError{chAddress})
}

type noRedundancyError struct {
	chAddress string
}

func (e noRedundancyError) Error() string {
	return fmt.Sprintf("redundancy getter: cannot get chunk %s because no redundancy added", e.chAddress)
}

func IsNoRedundancyError(err error, chAddress string) bool {
	return errors.Is(err, noRedundancyError{chAddress})
}

/// TYPES

// inflightChunk is initialized if recovery happened already
type inflightChunk struct {
	pos  int           // chunk index in the erasureData/intermediate chunk
	wait chan struct{} // chunk is under retrieval
}

// getter retrieves children of an intermediate chunk
// it caches sibling chunks if erasure decoding was called on the level already
type getter struct {
	storage.Getter
	storage.Putter
	mu          sync.Mutex
	sAddresses  []swarm.Address          // shard addresses
	pAddresses  []swarm.Address          // parity addresses
	cache       map[string]inflightChunk // map from chunk address to cached shard chunk data
	erasureData [][]byte                 // data + parity shards for erasure decoding; TODO mutex
	encrypted   bool                     // swarm datashards are encrypted
}

// New returns a getter object which is used to retrieve children of an intermediate chunk
func New(sAddresses, pAddresses []swarm.Address, g storage.Getter, p storage.Putter) storage.Getter {
	encrypted := len(sAddresses[0].Bytes()) == swarm.HashSize*2
	shards := len(sAddresses)
	parities := len(pAddresses)
	n := shards + parities
	erasureData := make([][]byte, n)
	cache := make(map[string]inflightChunk, n)
	// init cache
	for i, addr := range sAddresses {
		cache[addr.String()] = inflightChunk{
			pos: i,
			// wait channel initialization is needed when recovery starts
		}
	}
	for i, addr := range pAddresses {
		cache[addr.String()] = inflightChunk{
			pos: len(sAddresses) + i,
			// no wait channel initialization is needed
		}
	}

	return &getter{
		Getter:      g,
		Putter:      p,
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
	g.mu.Lock()
	cValue, ok := g.cache[addr.String()]
	g.mu.Unlock()
	if !ok || cValue.pos >= len(g.sAddresses) {
		return nil, noDataAddressIncludedError{addr.String()}
	}

	if cValue.wait != nil { // equals to g.processing but does not need lock again
		return g.getAfterProcessed(ctx, addr)
	}

	ch, err := g.Getter.Get(ctx, addr)
	if err == nil {
		return ch, nil
	}
	if errors.Is(storage.ErrNotFound, err) && len(g.pAddresses) == 0 {
		return nil, noRedundancyError{addr.String()}
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
	g.mu.Lock()
	defer g.mu.Unlock()
	iCh := g.cache[addr.String()]
	return iCh.wait != nil
}

// getAfterProcessed returns chunk from the cache
func (g *getter) getAfterProcessed(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	g.mu.Lock()
	c, ok := g.cache[addr.String()]
	// sanity check
	if !ok {
		return nil, fmt.Errorf("redundancy getter: chunk %s should have been in the cache", addr.String())
	}

	cacheData := g.erasureData[c.pos]
	g.mu.Unlock()
	if cacheData != nil {
		return g.cacheDataToChunk(addr, cacheData)
	}

	select {
	case <-c.wait:
		return g.cacheDataToChunk(addr, cacheData)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// executeStrategies executes recovery strategies from redundancy for the given swarm address
func (g *getter) executeStrategies(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	g.initWaitChannels()
	err := g.cautiousStrategy(ctx)
	if err != nil {
		g.closeWaitChannels()
		return nil, err
	}

	return g.getAfterProcessed(ctx, addr)
}

// initWaitChannels initializes the wait channels in the cache mapping which indicates the start of the recovery process as well
func (g *getter) initWaitChannels() {
	g.mu.Lock()
	for _, addr := range g.sAddresses {
		iCh := g.cache[addr.String()]
		iCh.wait = make(chan struct{})
		g.cache[addr.String()] = iCh
	}
	g.mu.Unlock()
}

// closeChannls closes all pending channels
func (g *getter) closeWaitChannels() {
	for _, addr := range g.sAddresses {
		c := g.cache[addr.String()]
		if !channelIsClosed(c.wait) {
			close(c.wait)
		}
	}
}

// cautiousStrategy requests all chunks (data and parity) on the level
// and if it has enough data for erasure decoding then it cancel other requests
func (g *getter) cautiousStrategy(ctx context.Context) error {
	requiredChunks := len(g.sAddresses)
	subContext, cancelContext := context.WithCancel(ctx)
	retrievedCh := make(chan struct{}, requiredChunks+len(g.pAddresses))
	var wg sync.WaitGroup

	addresses := append(g.sAddresses, g.pAddresses...)
	for _, a := range addresses {
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
			if c.pos < len(g.sAddresses) && !channelIsClosed(c.wait) {
				close(c.wait)
			}
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
		return cannotRecoverError{requiredChunks - retrieved}
	}

	return g.erasureDecode(ctx)
}

// erasureDecode perform Reed-Solomon recovery on data
// assumes it is called after filling up cache with the required amount of shards and parities
func (g *getter) erasureDecode(ctx context.Context) error {
	enc, err := reedsolomon.New(len(g.sAddresses), len(g.pAddresses))
	if err != nil {
		return err
	}

	// missing chunks
	var missingIndices []int
	for i := range g.sAddresses {
		if g.erasureData[i] == nil {
			missingIndices = append(missingIndices, i)
		}
	}

	g.mu.Lock()
	err = enc.ReconstructData(g.erasureData)
	g.mu.Unlock()
	if err != nil {
		return err
	}

	g.closeWaitChannels()
	// save missing chunks
	for _, index := range missingIndices {
		data := g.erasureData[index]
		addr := g.sAddresses[index]
		err := g.Putter.Put(ctx, swarm.NewChunk(addr, data))
		if err != nil {
			return err
		}
	}
	return nil
}

// cacheDataToChunk transforms passed chunk data to legit swarm chunk
func (g *getter) cacheDataToChunk(addr swarm.Address, chData []byte) (swarm.Chunk, error) {
	if chData == nil {
		return nil, isNotRecoveredError{addr.String()}
	}
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
	case _, ok := <-wait:
		return !ok
	default:
		return false
	}
}
