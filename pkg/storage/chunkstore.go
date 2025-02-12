// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Getter is the interface that wraps the basic Get method.
type Getter interface {
	// Get a chunk by its swarm.Address. Returns the chunk associated with
	// the address alongside with its postage stamp, or a storage.ErrNotFound
	// if the chunk is not found.
	// If the chunk has multiple stamps, then the first stamp is returned in this
	// query. In order to deterministically get the stamp, use the GetterWithStamp
	// interface. If the chunk is not found storage.ErrNotFound will be returned.
	Get(context.Context, swarm.Address) (swarm.Chunk, error)
}

// Putter is the interface that wraps the basic Put method.
type Putter interface {
	// Put a chunk into the store alongside with its postage stamp.
	Put(context.Context, swarm.Chunk) error
}

// Deleter is the interface that wraps the basic Delete method.
type Deleter interface {
	// Delete a chunk by the given swarm.Address.
	Delete(context.Context, swarm.Address) error
}

// Hasser is the interface that wraps the basic Has method.
type Hasser interface {
	// Has checks whether a chunk exists in the store.
	Has(context.Context, swarm.Address) (bool, error)
}

// Replacer is the interface that wraps the basic Replace method.
type Replacer interface {
	// Replace a chunk in the store.
	Replace(context.Context, swarm.Chunk, bool) error
}

// PutterFunc type is an adapter to allow the use of
// ChunkStore as Putter interface. If f is a function
// with the appropriate signature, PutterFunc(f) is a
// Putter that calls f.
type PutterFunc func(context.Context, swarm.Chunk) error

// Put calls f(ctx, chunk).
func (f PutterFunc) Put(ctx context.Context, chunk swarm.Chunk) error {
	return f(ctx, chunk)
}

type GetterFunc func(context.Context, swarm.Address) (swarm.Chunk, error)

func (f GetterFunc) Get(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
	return f(ctx, address)
}

type IterateChunkFn func(swarm.Chunk) (stop bool, err error)

// ChunkGetterDeleter is a storage that provides
// only read and delete operations for chunks.
type ChunkGetterDeleter interface {
	Getter
	Deleter
}

type ChunkStore interface {
	Getter
	Putter
	Deleter
	Hasser
	Replacer

	// Iterate over chunks in no particular order.
	Iterate(context.Context, IterateChunkFn) error
}

type ReadOnlyChunkStore interface {
	Getter
	Hasser
}
