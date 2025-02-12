// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	ErrOverwriteNewerChunk = errors.New("overwriting chunk with newer timestamp")
	ErrUnknownChunkType    = errors.New("unknown chunk type")
)

// Result represents the item returned by the read operation, which returns
// the item as the result. Or Key and/or Size in case the whole Item is not
// needed.
type Result struct {
	// ID is the Key.ID of the result Item.
	ID string

	// Size is the size of the result Item.
	Size int

	// Entry in the result Item.
	Entry Item
}

// IterateFn iterates through the Items of the store in the Key.Namespace.
// The function returns a boolean to indicate if the iteration should stop.
type IterateFn func(Result) (bool, error)

// Filter subtracts entries from the iteration. Filters would not construct the
// Item from the serialized bytes. Instead, users can add logic to check the entries
// directly in byte format or partially or fully unmarshal the data and check.
type Filter func(string, []byte) bool

// QueryItemProperty tells the Query which Item
// property should be loaded from the store to the result.
type QueryItemProperty int

const (
	// QueryItem indicates interest in the whole Item.
	QueryItem QueryItemProperty = iota

	// QueryItemID indicates interest in the Result.ID.
	// No data will be unmarshalled.
	QueryItemID

	// QueryItemSize indicates interest in the Result.Size.
	// No data will be unmarshalled.
	QueryItemSize
)

// Order represents order of the iteration
type Order int

const (
	// KeyAscendingOrder indicates a forward iteration based on ordering of keys.
	KeyAscendingOrder Order = iota

	// KeyDescendingOrder denotes the backward iteration based on ordering of keys.
	KeyDescendingOrder
)

// ErrInvalidQuery indicates that the query is not a valid query.
var (
	ErrInvalidQuery    = errors.New("storage: invalid query")
	ErrNotFound        = errors.New("storage: not found")
	ErrReferenceLength = errors.New("storage: invalid reference length")
	ErrInvalidChunk    = errors.New("storage: invalid chunk")
)

// Query denotes the iteration attributes.
type Query struct {
	// Factory is a constructor passed by client
	// to construct new object for the result.
	Factory func() Item

	// Prefix indicates interest in an item
	// that contains this prefix in its ID.
	Prefix string

	// PrefixAtStart indicates that the
	// iteration should start at the prefix.
	PrefixAtStart bool

	// SkipFirst skips the first element in the iteration.
	SkipFirst bool

	// ItemProperty indicates a specific interest of an Item property.
	ItemProperty QueryItemProperty

	// Order denotes the order of iteration.
	Order Order

	// Filters represent further constraints on the iteration.
	Filters []Filter
}

// Validate checks if the query is a valid query.
func (q Query) Validate() error {
	if q.ItemProperty == QueryItem && q.Factory == nil {
		return fmt.Errorf("missing Factory: %w", ErrInvalidQuery)
	}
	return nil
}

// Key represents the item identifiers.
type Key interface {
	// ID is the unique identifier of Item.
	ID() string

	// Namespace is used to separate similar items.
	// E.g.: can be seen as a table construct.
	Namespace() string
}

// Marshaler is the interface implemented by types
// that can marshal themselves into valid Item.
type Marshaler interface {
	Marshal() ([]byte, error)
}

// Unmarshaler is the interface implemented by types
// that can unmarshal a JSON description of themselves.
// The input can be assumed to be a valid encoding of
// a Item value.
type Unmarshaler interface {
	Unmarshal([]byte) error
}

// Cloner makes a deep copy of the Item.
type Cloner interface {
	Clone() Item
}

// Item represents an item which can be used in the Store.
type Item interface {
	Key
	Marshaler
	Unmarshaler
	Cloner
	fmt.Stringer
}

// Store contains the methods required for the Data Abstraction Layer.
type Store interface {
	io.Closer

	Reader
	Writer
}

// Reader groups methods that read from the store.
type Reader interface {
	// Get unmarshalls object with the given Item.Key.ID into the given Item.
	Get(Item) error

	// Has reports whether the Item with the given Key.ID exists in the store.
	Has(Key) (bool, error)

	// GetSize returns the size of Item with the given Key.ID.
	GetSize(Key) (int, error)

	// Iterate executes the given IterateFn on this store.
	// The Result of the iteration will be affected by the given Query.
	Iterate(Query, IterateFn) error

	// Count returns the count of items in the
	// store that are in the same Key.Namespace.
	Count(Key) (int, error)
}

// Writer groups methods that change the state of the store.
type Writer interface {
	// Put inserts or updates the given Item identified by its Key.ID.
	Put(Item) error

	// Delete removes the given Item form the store.
	// It will not return error if the key doesn't exist.
	Delete(Item) error
}

// BatchStore is a store that supports batching of Writer method calls.
type BatchStore interface {
	Store
	Batcher
}

// Recoverer allows store to recover from a failure when
// the transaction was not committed or rolled back.
type Recoverer interface {
	Recover() error
}

type IndexStore interface {
	Reader
	Writer
}

type Sharky interface {
	Read(context.Context, sharky.Location, []byte) error
	Write(context.Context, []byte) (sharky.Location, error)
	Release(context.Context, sharky.Location) error
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
	SubscribePush(ctx context.Context) (c <-chan swarm.Chunk, stop func())
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
	ChunkCouldNotSync
)

// PushReporter is used to report chunk state.
type PushReporter interface {
	Report(context.Context, swarm.Chunk, ChunkState) error
}

// ErrBatchCommitted is returned by Batch.Commit
// call when a batch has already been committed.
var ErrBatchCommitted = errors.New("storage: batch has already been committed")

// Batch provides set of operations that are batched.
type Batch interface {
	// Put adds a new item to the batch.
	Put(Item) error

	// Delete adds a new delete operation to the batch.
	Delete(Item) error

	// Commit commits the batch.
	Commit() error
}

// Batcher specifies a constructor for creating new batches.
type Batcher interface {
	// Batch returns a new Batch.
	Batch(context.Context) Batch
}

func ChunkType(ch swarm.Chunk) swarm.ChunkType {
	if cac.Valid(ch) {
		return swarm.ChunkTypeContentAddressed
	} else if soc.Valid(ch) {
		return swarm.ChunkTypeSingleOwner
	}
	return swarm.ChunkTypeUnspecified
}

// IdentityAddress returns the internally used address for the chunk
// since the single owner chunk address is not a unique identifier for the chunk,
// but hashing the soc address and the wrapped chunk address is.
// it is used in the reserve sampling and other places where a key is needed to represent a chunk.
func IdentityAddress(chunk swarm.Chunk) (swarm.Address, error) {

	if cac.Valid(chunk) {
		return chunk.Address(), nil
	}

	// check the chunk is single owner chunk or cac
	if sch, err := soc.FromChunk(chunk); err == nil {
		socAddress, err := sch.Address()
		if err != nil {
			return swarm.ZeroAddress, err
		}
		h := swarm.NewHasher()
		_, err = h.Write(socAddress.Bytes())
		if err != nil {
			return swarm.ZeroAddress, err
		}
		_, err = h.Write(sch.WrappedChunk().Address().Bytes())
		if err != nil {
			return swarm.ZeroAddress, err
		}

		return swarm.NewAddress(h.Sum(nil)), nil
	}

	return swarm.ZeroAddress, fmt.Errorf("identity address failed on chunk %s: %w", chunk, ErrUnknownChunkType)
}
