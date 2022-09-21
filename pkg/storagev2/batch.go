// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import "context"

type Batch interface {
	// We will only need to batch write operations
	Put(Item) error
	Delete(Key) error

	Commit() error
}

type BatchedStore interface {
	Store
	Batch(context.Context) (Batch, error)
}
