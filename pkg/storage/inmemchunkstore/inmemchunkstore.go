// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemchunkstore

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type ChunkStore struct {
	mu     sync.Mutex
	chunks map[string]chunkCount
}

type chunkCount struct {
	chunk swarm.Chunk
	count int
}

func New() *ChunkStore {
	return &ChunkStore{
		chunks: make(map[string]chunkCount),
	}
}

func (c *ChunkStore) Get(_ context.Context, addr swarm.Address) (swarm.Chunk, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	chunk, ok := c.chunks[c.key(addr)]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return chunk.chunk, nil
}

func (c *ChunkStore) Put(_ context.Context, ch swarm.Chunk) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	chunkCount, ok := c.chunks[c.key(ch.Address())]
	if !ok {
		chunkCount.chunk = swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(ch.Stamp())
	}
	chunkCount.count++
	c.chunks[c.key(ch.Address())] = chunkCount

	return nil
}

func (c *ChunkStore) Has(_ context.Context, addr swarm.Address) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, exists := c.chunks[c.key(addr)]

	return exists, nil
}

func (c *ChunkStore) Delete(_ context.Context, addr swarm.Address) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	chunkCount := c.chunks[c.key(addr)]
	chunkCount.count--
	if chunkCount.count <= 0 {
		delete(c.chunks, addr.ByteString())
	} else {
		c.chunks[c.key(addr)] = chunkCount
	}

	return nil
}

func (c *ChunkStore) Replace(_ context.Context, ch swarm.Chunk, emplace bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	chunkCount := c.chunks[c.key(ch.Address())]
	chunkCount.chunk = ch
	if emplace {
		chunkCount.count++
	}
	c.chunks[c.key(ch.Address())] = chunkCount

	return nil
}

func (c *ChunkStore) Iterate(_ context.Context, fn storage.IterateChunkFn) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, chunkCount := range c.chunks {
		stop, err := fn(chunkCount.chunk)
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

func (c *ChunkStore) key(addr swarm.Address) string {
	if len(addr.Bytes()) < swarm.HashSize {
		return addr.ByteString()
	}
	return string(addr.Bytes()[:swarm.HashSize])
}
