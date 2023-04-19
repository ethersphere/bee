// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"io"
)

// StateStorer defines methods required to get, set, delete values for different keys
// and close the underlying resources.
type StateStorer interface {
	Get(key string, i interface{}) (err error)
	Put(key string, i interface{}) (err error)
	Delete(key string) (err error)
	Iterate(prefix string, iterFunc StateIterFunc) (err error)
	// DB returns the underlying DB storage.
	// The returned interface is a gross hack until
	// we can refactor the kademlia to not use the shed.
	DB() interface{}
	io.Closer
}

// StateIterFunc is used when iterating through StateStorer key/value pairs
type StateIterFunc func(key, value []byte) (stop bool, err error)
