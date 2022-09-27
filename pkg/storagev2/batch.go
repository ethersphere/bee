// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import "context"

// Batch provides set of operations that are batched.
type Batch interface {
	// Put adds a new item to the batch.
	Put(Item) error

	// Delete adds a new delete operation to the batch.
	Delete(Key) error

	// Commit commits the batch.
	Commit() error
}

// BatchedStore is a store that provides batch operations.
type BatchedStore interface {
	Store

	// Batch returns a new Batch.
	Batch(context.Context) (Batch, error)
}
