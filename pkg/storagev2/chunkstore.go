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

type Getter interface {
	// Get a chunk by its swarm.Address. Returns the chunk associated with
	// the address alongside with its postage stamp, or a storage.ErrNotFound
	// if the chunk is not found.
	Get(context.Context, swarm.Address) (swarm.Chunk, error)
}

type Putter interface {
	// Put a chunk into the store alongside with its postage stamp. No duplicates
	// are allowed. It returns `exists=true` In case the chunk already exists.
	Put(context.Context, swarm.Chunk) (exists bool, err error)
}

type IterateChunkFn func(swarm.Chunk) (stop bool, err error)

type ChunkStore interface {
	io.Closer
	Getter
	Putter

	// Iterate over chunks in no particular order.
	Iterate(context.Context, IterateChunkFn) error
	// Has checks whether a chunk exists in the store.
	Has(context.Context, swarm.Address) (bool, error)
	// Delete a chunk from the store.
	Delete(context.Context, swarm.Address) error
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
