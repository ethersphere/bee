// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package joiner provides implementations of the file.Joiner interface
package joiner

import (
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

// simpleJoinerReadCloser wraps a byte slice in a io.ReadCloser implementation.
type simpleReadCloser struct {
	buffer []byte
	cursor int
}

// Read implements io.Reader.
func (s *simpleReadCloser) Read(b []byte) (int, error) {
	if s.cursor == len(s.buffer) {
		return 0, io.EOF
	}
	copy(b, s.buffer)
	s.cursor += len(s.buffer)
	return len(s.buffer), nil
}

// Close implements io.Closer.
func (s *simpleReadCloser) Close() error {
	return nil
}

// NewSimpleJoiner creates a new simpleJoiner.
func NewSimpleJoiner(store storage.Storer) file.Joiner {
	return &simpleJoiner{
		store:  store,
		logger: logging.New(os.Stderr, 6),
	}
}

// Join implements the file.Joiner interface.
//
// It uses a non-optimized internal component that only retrieves a data chunk
// after the previous has been read.
func (s *simpleJoiner) Join(ctx context.Context, address swarm.Address) (dataOut io.ReadCloser, dataSize int64, err error) {

	// retrieve the root chunk to read the total data length the be retrieved
	rootChunk, err := s.store.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		return nil, 0, err
	}

	// if this is a single chunk, short circuit to returning just that chunk
	spanLength := binary.LittleEndian.Uint64(rootChunk.Data())
	if spanLength < swarm.ChunkSize {
		s.logger.Tracef("simplejoiner root chunk %v is single chunk, skipping join and returning directly", rootChunk)
		return &simpleReadCloser{
			buffer: (rootChunk.Data()[8:]),
		}, int64(spanLength), nil
	}

	s.logger.Tracef("simplejoiner joining root chunk %v", rootChunk)
	r := internal.NewSimpleJoinerJob(ctx, s.store, rootChunk)
	return r, int64(spanLength), nil
}
