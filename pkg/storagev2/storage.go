// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"context"
	"errors"
	"fmt"
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
type IterateFn func(Result) (bool, error)

// Filter subtracts entries from the iteration.
type Filter func(string, []byte) bool

// ItemAttribute tells the Query which Item
// attribute should be loaded from the store.
type ItemAttribute int

const (
	// QueryItem indicates interest in the whole Item.
	QueryItem ItemAttribute = iota

	// QueryItemID indicates interest in the Result.ID.
	// No data will be unmarshalled.
	QueryItemID

	// QueryItemSize indicates interest in the Result.Size.
	// No data will be unmarshalled.
	QueryItemSize
)

// Order represents order of the iteration
type Order bool

const (
	// AscendingOrder indicates a forward iteration.
	AscendingOrder Order = false

	// DescendingOrder denotes the backward iteration.
	DescendingOrder Order = true
)

// ErrInvalidQuery indicates that the query is not a valid query.
var ErrInvalidQuery = errors.New("invalid query")

// Query denotes the iteration attributes.
type Query struct {
	// Factory is a constructor passed by client
	// to construct new object for the result.
	Factory func() Item

	// ItemAttribute indicates a specific interest of an Item attribute.
	ItemAttribute ItemAttribute

	// Order denotes the order of iteration.
	Order Order

	// Filters represent further constraints on the iteration.
	Filters []Filter
}

// Validate checks if the query is a valid query.
func (q Query) Validate() error {
	if q.ItemAttribute == QueryItem && q.Factory == nil {
		return fmt.Errorf("missing Factory: %w", ErrInvalidQuery)
	}
	return nil
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

// Key represents the item identifiers.
type Key interface {
	fmt.Stringer

	// ID is the unique identifier of Item.
	ID() string

	// Namespace is used to separate similar items.
	// E.g.: can be seen as a table construct.
	Namespace() string
}

// Item represents an item which can be used in the Store.
type Item interface {
	Key
	Marshaler
	Unmarshaler
}

// Store contains the interfaces required for the Data Abstraction Layer.
type Store interface {
	// Get unmarshalls object with the given Item.Key.ID into the given Item.
	Get(Item) error

	// Has reports whether the Item with the given Key.ID exists in the store.
	Has(Key) (bool, error)

	// GetSize returns the size of Item with the given Key.ID.
	GetSize(Key) (int, error)

	// Iterate executes the given IterateFn on this store.
	// The Result of the iteration will be affected by the given Query.
	Iterate(Query, IterateFn)

	// Count returns the count of items in the
	// store that are in the same Key.Namespace.
	Count(Key) (int, error)

	// Put inserts or updates the the given Item identified by its Key.ID.
	Put(Item) error

	// Delete removes the Item with the given Key.ID form the store.
	Delete(Key) error
}

// Tx represents an in-progress store transaction.
// A transaction must end with a call to Commit or Rollback.
type Tx interface {
	Store

	// Commit commits the transaction.
	Commit(context.Context) error

	// Rollback aborts the transaction.
	Rollback(context.Context) error
}
