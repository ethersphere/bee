// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"io"
)

// StateIterFunc is used when iterating through StateStorer key/value pairs
type StateIterFunc func(key, val []byte) (stop bool, err error)

// StateStorer is a storage interface for storing and retrieving key/value pairs.
type StateStorer interface {
	io.Closer

	// Get unmarshalls object with the given key into the given obj.
	Get(key string, obj interface{}) error

	// Put inserts or updates the given obj stored under the given key.
	Put(key string, obj interface{}) error

	// Delete removes object form the store stored under the given key.
	Delete(key string) error

	// Iterate iterates over all keys with the given prefix and calls iterFunc.
	Iterate(prefix string, iterFunc StateIterFunc) error
}

// StateStorerCleaner is the interface for cleaning the store.
type StateStorerCleaner interface {
	// Nuke the store so that only the bare essential entries are left.
	Nuke() error
	// ClearForHopping removes all data not required in a new neighborhood
	ClearForHopping() error
}

// StateStorerManager defines all external methods of the state storage
type StateStorerManager interface {
	StateStorer
	StateStorerCleaner
}
