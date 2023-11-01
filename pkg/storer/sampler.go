// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Sampler interface providing the iterator to iterate through the reserve
type Sampler interface {
	Iterate(depth uint8, f func(Chunk) (bool, error)) error
	StorageRadius() uint8
}

// Chunk serves to reify the partial info about a chunk coming from indexstore
// the Chunk and Stamp functions allow lazy loading of chunk data and/or postage stamp
type Chunk struct {
	Address swarm.Address
	BatchID []byte
	Type    swarm.ChunkType
	db      *DB
}

// Chunk returns the swarm.Chunk
func (c *Chunk) Chunk(ctx context.Context) (swarm.Chunk, error) {
	return c.db.ChunkStore().Get(ctx, c.Address)
}

// Stamp returns the postage stamp for the chunk
func (c *Chunk) Stamp() (*postage.Stamp, error) {
	s, err := chunkstamp.LoadWithBatchID(c.db.repo.IndexStore(), "reserve", c.Address, c.BatchID)
	if err != nil {
		return nil, err
	}
	return postage.NewStamp(c.BatchID, s.Index(), s.Timestamp(), s.Sig()), nil
}

// Iterate iterates through the reserve and applies f to all chunks at and above the PO depth
func (db *DB) Iterate(depth uint8, f func(Chunk) (bool, error)) error {
	return db.reserve.IterateChunksItems(db.repo, depth, func(chi reserve.ChunkItem) (bool, error) {
		return f(Chunk{chi.ChunkAddress, chi.BatchID, chi.Type, db})
	})
}
