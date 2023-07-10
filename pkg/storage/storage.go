// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"errors"
	"fmt"
	"io"
)

var (
	ErrOverwriteNewerChunk       = errors.New("overwriting chunk with newer timestamp")
	ErrOverwriteOfImmutableBatch = errors.New("overwrite of existing immutable batch")
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

// BatchedStore is a store that supports batching of Writer method calls.
type BatchedStore interface {
	Store
	Batcher
}

// Recoverer allows store to recover from a failure when
// the transaction was not committed or rolled back.
type Recoverer interface {
	Recover() error
}
