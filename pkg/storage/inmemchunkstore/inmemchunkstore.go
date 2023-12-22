// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemchunkstore

import (
	"context"
	"sync"

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type ChunkStore struct {
	mu     sync.Mutex
	chunks map[string]swarm.Chunk
}

func New() *ChunkStore {
	return &ChunkStore{
		chunks: make(map[string]swarm.Chunk),
	}
}

func (c *ChunkStore) Get(_ context.Context, addr swarm.Address) (swarm.Chunk, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	chunk, ok := c.chunks[addr.ByteString()]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return chunk, nil
}

func (c *ChunkStore) Put(_ context.Context, ch swarm.Chunk, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	chunk, ok := c.chunks[ch.Address().ByteString()]
	if !ok {
		chunk = swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(ch.Stamp())
	}
	c.chunks[ch.Address().ByteString()] = chunk

	return nil
}

func (c *ChunkStore) GetRefCnt(ctx context.Context, addr swarm.Address) (uint32, error) {
	h, e := c.Has(ctx, addr)
	if h {
		return 1, e
	} else {
		return 0, e
	}
}

func (c *ChunkStore) IncRefCnt(ctx context.Context, addr swarm.Address, _ uint32) (uint32, error) {
	return c.GetRefCnt(ctx, addr)
}

func (c *ChunkStore) Has(_ context.Context, addr swarm.Address) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, exists := c.chunks[addr.ByteString()]

	return exists, nil
}

func (c *ChunkStore) Delete(_ context.Context, addr swarm.Address, why string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.chunks, addr.ByteString())

	return nil
}

func (c *ChunkStore) Iterate(_ context.Context, fn storage.IterateChunkFn) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, chunk := range c.chunks {
		stop, err := fn(chunk)
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
