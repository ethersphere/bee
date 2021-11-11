package storage

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/pkg/swarm"
)

var ErrNoContext = errors.New("no such context")

type ContextStorer interface {
	ContextByName(Context, string) (string, SimpleChunkStorer, func(), error)
	LookupContext() SimpleChunkGetter
	DiscContext() SimpleChunkPutGetter
	PinContext() PinStore
}

type PinStore interface {
	Pins() ([]swarm.Address, error)
	Unpin(swarm.Address) error

	NestedChunkStorer
}

type NestedChunkStorer interface {
	SimpleChunkGetter

	Delete(root string) error
	EachStore(cb EachStoreFunc) error
	GetByName(root string) (SimpleChunkStorer, func(), error)
}

type EachStoreFunc func(string, SimpleChunkStorer) (stop bool, err error)

type Context int

const (
	ContextSync Context = iota + 1
	ContextPin
	ContextUpload
	ContextDisc
)

type SimpleChunkPutGetter interface {
	SimpleChunkPutter
	SimpleChunkGetter
}

type SimpleChunkStorer interface {
	SimpleChunkPutter
	SimpleChunkGetter
	// Iterate over chunks in no particular order.
	Iterate(IterateChunkFn) error
	// Delete a chunk from the store.
	Delete(context.Context, swarm.Address) error
	// Has checks whether a chunk exists in the store.
	Has(context.Context, swarm.Address) (bool, error)
	// Count returns the number of chunks in the store.
	Count() (int, error)
}

type SimpleChunkGetter interface {
	// Get a chunk by its swarm.Address. Returns the chunk associated with
	// the address alongside with its postage stamp, or a storage.ErrNotFound
	// if the chunk is not found.
	Get(context.Context, swarm.Address) (swarm.Chunk, error)
}

type SimpleChunkPutter interface {
	// Put a chunk into the store alongside with its postage stamp. No duplicates
	// are allowed. It returns `exists=true` In case the chunk already exists.
	Put(context.Context, swarm.Chunk) (exists bool, err error)
}

type IterateChunkFn func(swarm.Chunk) (stop bool, err error)
