// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package file provides interfaces for file-oriented operations.
package file

import (
	"context"
	"io"

	"github.com/ethersphere/bee/pkg/swarm"
)

type Reader interface {
	io.ReadSeeker
	io.ReaderAt
}

// Joiner provides the inverse functionality of the Splitter.
type Joiner interface {
	Reader
	// IterateChunkAddresses is used to iterate over chunks addresses of some root hash.
	IterateChunkAddresses(swarm.AddressIterFunc) error
	// Size returns the span of the hash trie represented by the joiner's root hash.
	Size() int64
}

// Splitter starts a new file splitting job.
//
// Data is read from the provided reader.
// If the dataLength parameter is 0, data is read until io.EOF is encountered.
// When EOF is received and splitting is done, the resulting Swarm Address is returned.
type Splitter interface {
	Split(ctx context.Context, dataIn io.ReadCloser, dataLength int64, toEncrypt bool) (addr swarm.Address, err error)
}
