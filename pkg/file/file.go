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

// Joiner returns file data referenced by the given Swarm Address to the given io.Reader.
//
// The call returns when the chunk for the given Swarm Address is found,
// returning the length of the data which will be returned.
// The called can then read the data on the io.Reader that was provided.
type Joiner interface {
	Join(ctx context.Context, address swarm.Address) (dataOut io.ReadCloser, dataLength int64, err error)
}
