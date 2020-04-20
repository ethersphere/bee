// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package joiner provides implemenations of the file.Joiner interface
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


type simpleJoiner struct {
	store storage.Storer
	logger logging.Logger
}

func (s *simpleJoiner) Join(ctx context.Context, address swarm.Address) (dataOut io.Reader, dataSize int64, err error) {
	rootChunk, err := s.store.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		s.logger.Debugf("unable to find root chunk for '%s'", address)
		return nil, 0, err
	}
	spanLength := binary.LittleEndian.Uint64(rootChunk.Data())

	// if this is a single chunk, short circuit to returning just that chunk
	if spanLength < swarm.ChunkSize {
		s.logger.Tracef("root chunk %v is single chunk, short circuit", rootChunk)
		return bytes.NewReader(rootChunk.Data()[8:]), int64(spanLength), nil
	}


	s.logger.Tracef("joining root chunk %v", rootChunk)
	r := internal.NewSimpleJoinerJob(ctx, s.store, rootChunk)
	return r, int64(spanLength), nil
}

func NewSimpleJoiner(store storage.Storer) file.Joiner {
	return &simpleJoiner{
		store: store,
		logger: logging.New(os.Stderr, 6),
	}
}
