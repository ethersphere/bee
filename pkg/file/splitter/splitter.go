// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package splitter provides implementations of the file.Splitter interface
package splitter

import (
	"context"
	"fmt"
	"io"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/splitter/internal"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// simpleSplitter wraps a non-optimized implementation of file.Splitter
type simpleSplitter struct {
	putter storage.Putter
}

// NewSimpleSplitter creates a new SimpleSplitter
func NewSimpleSplitter(putter storage.Putter) file.Splitter {
	return &simpleSplitter{
		putter: putter,
	}
}

// Split implements the file.Splitter interface
//
// It uses a non-optimized internal component that blocks when performing
// multiple levels of hashing when building the file hash tree.
//
// It returns the Swarmhash of the data.
func (s *simpleSplitter) Split(ctx context.Context, r io.ReadCloser, dataLength int64) (addr swarm.Address, err error) {
	j := internal.NewSimpleSplitterJob(ctx, s.putter, dataLength)

	var total int64
	data := make([]byte, swarm.ChunkSize)
	var eof bool
	for !eof {
		c, err := r.Read(data)
		total += int64(c)
		if err != nil {
			if err == io.EOF {
				if total < dataLength {
					return swarm.ZeroAddress, fmt.Errorf("splitter only received %d bytes of data, expected %d bytes", total+int64(c), dataLength)
				}
				eof = true
			} else {
				return swarm.ZeroAddress, err
			}
		}
		cc, err := j.Write(data[:c])
		if err != nil {
			return swarm.ZeroAddress, err
		}
		if cc < c {
			return swarm.ZeroAddress, fmt.Errorf("write count to file hasher component %d does not match read count %d", cc, c)
		}
	}

	sum := j.Sum(nil)
	return swarm.NewAddress(sum), nil
}
