// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package file provides interfaces for file-oriented operations .
package file

import (
	"io"
	"hash"

	"github.com/ethersphere/bee/pkg/swarm"
)


// SwarmHash represents the hasher used to operate on files.
type SwarmHash hash.Hash


// Joiner returns file data referenced by the given Swarm Address to the given io.Reader.
//
// The call returns when the chunk for the given Swarm Address is found,
// returning the length of the data which will be returned.
// The called can then read the data on the io.Reader that was provided.
type Joiner interface {
	Join(address swarm.Address, dataInput io.Reader) (dataLength int64, err error)
}

// Splitter starts a new file splitting job.
//
// Data is read from the provided reader.
// If the dataLength parameter is 0, data is read until io.EOF is encountered.
// When EOF is received and splitting is done, the resulting Swarm Address is returned.
type Splitter interface {
	Split(data io.Reader, dataLength int64) (addr swarm.Address, err error)
}
