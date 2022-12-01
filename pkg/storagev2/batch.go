// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"context"
	"errors"
)

// ErrBatchCommitted is returned by Batch.Commit
// call when a batch has already been committed.
var ErrBatchCommitted = errors.New("storage: batch has already been committed")

// ErrBatchNotSupported is returned by BatchedStore.Batch call when batching
// is not supported.
var ErrBatchNotSupported = errors.New("storage: batch operations not supported")

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
	Batch(context.Context) (Batch, error)
}
