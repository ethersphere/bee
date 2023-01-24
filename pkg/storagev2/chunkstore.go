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

var (
	// ErrNoStampsForChunk is returned when chunk was found but there were no associated stamps.
	ErrNoStampsForChunk = fmt.Errorf("chunk found but no stamps found: %w", ErrNotFound)

	// ErrStampNotFound is returned when chunk with desired stamp was not fund.
	ErrStampNotFound = fmt.Errorf("chunk with stamp was not found: %w", ErrNotFound)
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

// GetterWithStamp is the interface that provides a way to specify a postage
// stamp to filter the Get query. This is required as we will support multiple stamps
// on a chunk. As a result some components would need to query chunk and provide
// the stamp to return by using the argument.
type GetterWithStamp interface {
	GetWithStamp(context.Context, swarm.Address, []byte) (swarm.Chunk, error)
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
	io.Closer
	Getter
	GetterWithStamp
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

type ChunkState = int

const (
	// ChunkSent is used by the pusher component to notify about successful push of chunk from
	// the node. A chunk could be retried on failure so, this sent count is maintained to
	// understand how many attempts were made by the node while pushing. The attempts are
	// registered only when an actual request was sent from this node.
	ChunkSent ChunkState = iota
	// ChunkStored is used by the pusher component to notify that the uploader node is
	// the closest node and has stored the chunk.
	ChunkStored
	// ChunkSynced is used by the pusher component to notify that the chunk is synced to the
	// network. This is reported when a valid receipt was received after the chunk was
	// pushed.
	ChunkSynced
)

// PushReporter is used to report chunk state.
type PushReporter interface {
	Report(context.Context, swarm.Chunk, ChunkState) error
}
