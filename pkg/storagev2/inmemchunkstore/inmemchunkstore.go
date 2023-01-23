// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemchunkstore

import (
	"bytes"
	"context"
	"sync"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

type ChunkStore struct {
	chunks map[string]chunkData
	lock   sync.Mutex
}

type chunkData struct {
	chunk  swarm.Chunk
	stamps []swarm.Stamp
}

func New() *ChunkStore {
	return &ChunkStore{
		chunks: make(map[string]chunkData),
	}
}

func (c *ChunkStore) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	return c.GetWithStamp(ctx, addr, nil)
}

func (c *ChunkStore) GetWithStamp(ctx context.Context, addr swarm.Address, batchID []byte) (swarm.Chunk, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	data, ok := c.chunks[addr.ByteString()]
	if !ok {
		return nil, storage.ErrNotFound
	}

	// when batchID is not specified, chunk with first stamp is returned
	if batchID == nil {
		if st := firstStamp(data.stamps); st != nil {
			return makeChunk(data.chunk, st), nil
		}

		return nil, storage.ErrNoStampsForChunk
	}

	// when batchID is specified, we need to search stamps by batchID
	if st, found := findStampWithBatchID(data.stamps, batchID); found {
		return makeChunk(data.chunk, st), nil
	}

	return nil, storage.ErrStampNotFound
}

func (c *ChunkStore) Put(_ context.Context, ch swarm.Chunk) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	data, ok := c.chunks[ch.Address().ByteString()]
	if !ok {
		data = chunkData{
			chunk:  makeChunk(ch, nil),
			stamps: make([]swarm.Stamp, 0, 1),
		}
	}

	// append new stamp only if it doesn't exist
	if st := ch.Stamp(); st != nil && st.BatchID() != nil {
		if _, found := findStampWithBatchID(data.stamps, ch.Stamp().BatchID()); !found {
			data.stamps = append(data.stamps, ch.Stamp())
		}
	}

	c.chunks[ch.Address().ByteString()] = data

	return nil
}

func (c *ChunkStore) Has(_ context.Context, addr swarm.Address) (bool, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	_, exists := c.chunks[addr.ByteString()]

	return exists, nil
}

func (c *ChunkStore) Delete(_ context.Context, addr swarm.Address) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	delete(c.chunks, addr.ByteString())

	return nil
}

func (c *ChunkStore) Iterate(_ context.Context, fn storage.IterateChunkFn) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, data := range c.chunks {
		stop, err := fn(makeChunk(data.chunk, firstStamp(data.stamps)))
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}

	return nil
}

func (c *ChunkStore) Close() error {
	return nil
}

func firstStamp(stamps []swarm.Stamp) swarm.Stamp {
	if len(stamps) > 0 {
		return stamps[0]
	}
	return nil
}

func makeChunk(ch swarm.Chunk, st swarm.Stamp) swarm.Chunk {
	return swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(st)
}

// note: this should be probably moved to swarm package with other utilities (rebase needed)
func findStampWithBatchID(stamps []swarm.Stamp, batchID []byte) (swarm.Stamp, bool) {
	for _, s := range stamps {
		if bytes.Equal(s.BatchID(), batchID) {
			return s, true
		}
	}
	return nil, false
}
