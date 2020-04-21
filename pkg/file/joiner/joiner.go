// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package joiner provides implementations of the file.Joiner interface
package joiner

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner/internal"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// simpleJoiner wraps a non-optimized implementation of file.Joiner.
type simpleJoiner struct {
	store  storage.Storer
	logger logging.Logger
}

// NewSimpleJoiner creates a new simpleJoiner.
func NewSimpleJoiner(store storage.Storer) file.Joiner {
	return &simpleJoiner{
		store:  store,
		logger: logging.New(os.Stderr, 6),
	}
}

// Join implements the file.Joiner interface
//
// It uses a non-optimized internal component that only retrieves a data chunk
// after the previous has been read.
func (s *simpleJoiner) Join(ctx context.Context, address swarm.Address) (dataOut io.Reader, dataSize int64, err error) {

	// retrieve the root chunk to read the total data length the be retrieved
	rootChunk, err := s.store.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		s.logger.Debugf("unable to find root chunk for '%s'", address)
		return nil, 0, err
	}

	// if this is a single chunk, short circuit to returning just that chunk
	spanLength := binary.LittleEndian.Uint64(rootChunk.Data())
	if spanLength < swarm.ChunkSize {
		s.logger.Tracef("root chunk %v is single chunk, short circuit", rootChunk)
		return bytes.NewReader(rootChunk.Data()[8:]), int64(spanLength), nil
	}

	s.logger.Tracef("joining root chunk %v", rootChunk)
	r := internal.NewSimpleJoinerJob(ctx, s.store, rootChunk)
	return r, int64(spanLength), nil
}
