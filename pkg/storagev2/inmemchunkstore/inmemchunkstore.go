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
	chunks map[string][]swarm.Chunk
	lock   sync.Mutex
}

func New() *ChunkStore {
	return &ChunkStore{}
}

func (c *ChunkStore) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	return c.GetWithStamp(ctx, addr, nil)
}

func (c *ChunkStore) GetWithStamp(ctx context.Context, addr swarm.Address, batchID []byte) (swarm.Chunk, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	chunks, ok := c.chunks[addr.ByteString()]
	if !ok {
		return nil, storage.ErrNotFound
	}

	// when batchID is not specifed, first chunk is returned
	if batchID == nil {
		if len(chunks) > 0 {
			return chunks[0], nil
		}
		return nil, storage.ErrNotFound
	}

	// when batchID is specified, we need to search chunks by batchID
	if ch, found := findChunkWithBatchID(chunks, batchID); found {
		return ch, nil
	}

	return nil, storage.ErrNotFound
}

func (c *ChunkStore) Put(_ context.Context, ch swarm.Chunk) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	chunks, ok := c.chunks[ch.Address().ByteString()]
	if !ok {
		chunks = make([]swarm.Chunk, 0, 1)
	}

	// append new chunk only if it doesn't exist
	if _, found := findChunkWithBatchID(chunks, ch.Stamp().BatchID()); !found {
		chunks = append(chunks, ch)
		c.chunks[ch.Address().ByteString()] = chunks
	}

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

	for _, chunks := range c.chunks {
		for _, chunk := range chunks {
			stop, err := fn(chunk)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
		}
	}

	return nil
}

func (c *ChunkStore) Close() error {
	return nil
}

// note: this should be probabbly moved to swarm package with other utilities (rebase needed)
func findChunkWithBatchID(chunks []swarm.Chunk, batchID []byte) (swarm.Chunk, bool) {
	for _, chunk := range chunks {
		if bytes.Equal(chunk.Stamp().BatchID(), batchID) {
			return chunk, true
		}
	}
	return nil, false
}
