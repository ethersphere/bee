// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/ethersphere/bee/pkg/swarm"
)

// Getter is the interface that wraps the basic Get method.
type Getter interface {
	// Get a chunk by its swarm.Address. Returns the chunk associated with
	// the address alongside with its postage stamp, or a storage.ErrNotFound
	// if the chunk is not found.
	Get(context.Context, swarm.Address) (swarm.Chunk, error)
}

// Putter is the interface that wraps the basic Put method.
type Putter interface {
	// Put a chunk into the store alongside with its postage stamp. No duplicates
	// are allowed. It returns `exists=true` In case the chunk already exists.
	Put(context.Context, swarm.Chunk) (exists bool, err error)
}

// Deleter is the interface that wraps the basic Delete method.
type Deleter interface {
	// Delete a chunk by the given swarm.Address.
	Delete(context.Context, swarm.Address) error
}

// PutterFunc type is an adapter to allow the use of
// ChunkStore as Putter interface. If f is a function
// with the appropriate signature, PutterFunc(f) is a
// Putter that calls f.
type PutterFunc func(context.Context, swarm.Chunk) (bool, error)

// Put calls f(ctx, chunk).
func (f PutterFunc) Put(ctx context.Context, chunk swarm.Chunk) (bool, error) {
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
	io.Closer
	Getter
	Putter
	Deleter

	// Has checks whether a chunk exists in the store.
	Has(context.Context, swarm.Address) (bool, error)
	// Iterate over chunks in no particular order.
	Iterate(context.Context, IterateChunkFn) error
}

type SizeReporter interface {
	Size() (uint64, error)
	Capacity() uint64
}

// Descriptor holds information required for Pull syncing. This struct
// is provided by subscribing to pull index.
type Descriptor struct {
	Address swarm.Address
	BinID   uint64
}

func (d *Descriptor) String() string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("%s bin id %v", d.Address, d.BinID)
}

type PullSubscriber interface {
	SubscribePull(ctx context.Context, bin uint8, since, until uint64) (c <-chan Descriptor, closed <-chan struct{}, stop func())
}

type PushSubscriber interface {
	SubscribePush(ctx context.Context, skipFn func([]byte) bool) (c <-chan swarm.Chunk, repeat, stop func())
}
