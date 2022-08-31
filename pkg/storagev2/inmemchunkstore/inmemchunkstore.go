// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemchunkstore

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

type ChunkStore struct {
	chunks sync.Map
}

func New() *ChunkStore {
	return &ChunkStore{}
}

func (c *ChunkStore) Get(_ context.Context, addr swarm.Address) (swarm.Chunk, error) {
	val, found := c.chunks.Load(addr.ByteString())
	if !found {
		return nil, storage.ErrNotFound
	}
	return val.(swarm.Chunk), nil
}

func (c *ChunkStore) Put(_ context.Context, ch swarm.Chunk) (bool, error) {
	_, loaded := c.chunks.LoadOrStore(ch.Address().ByteString(), ch)
	return loaded, nil
}

func (c *ChunkStore) Has(_ context.Context, addr swarm.Address) (bool, error) {
	_, exists := c.chunks.Load(addr.ByteString())
	return exists, nil
}

func (c *ChunkStore) Delete(_ context.Context, addr swarm.Address) error {
	c.chunks.Delete(addr.ByteString())
	return nil
}

func (c *ChunkStore) Iterate(_ context.Context, fn storage.IterateChunkFn) error {
	var retErr error
	c.chunks.Range(func(_, val interface{}) bool {
		stop, err := fn(val.(swarm.Chunk))
		if err != nil {
			retErr = err
			return false
		}
		return !stop
	})
	return retErr
}

func (c *ChunkStore) Close() error {
	return nil
}
