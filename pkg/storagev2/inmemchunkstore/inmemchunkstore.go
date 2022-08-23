// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemchunkstore

import (
	"context"
	"sync"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

type chunkStore struct {
	chunks sync.Map
}

func New() storage.ChunkStore {
	return &chunkStore{}
}

func (c *chunkStore) Get(_ context.Context, addr swarm.Address) (swarm.Chunk, error) {
	val, found := c.chunks.Load(addr.ByteString())
	if !found {
		return nil, storage.ErrNotFound
	}
	return val.(swarm.Chunk), nil
}

func (c *chunkStore) Put(_ context.Context, ch swarm.Chunk) (bool, error) {
	_, loaded := c.chunks.LoadOrStore(ch.Address().ByteString(), ch)
	return loaded, nil
}

func (c *chunkStore) Has(_ context.Context, addr swarm.Address) (bool, error) {
	_, exists := c.chunks.Load(addr.ByteString())
	return exists, nil
}

func (c *chunkStore) Delete(_ context.Context, addr swarm.Address) error {
	c.chunks.Delete(addr.ByteString())
	return nil
}

func (c *chunkStore) Iterate(_ context.Context, fn storage.IterateChunkFn) error {
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

func (c *chunkStore) Close() error {
	return nil
}
