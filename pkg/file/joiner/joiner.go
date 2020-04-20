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
	"io/ioutil"

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
	if spanLength < swarm.ChunkSize {
		return bytes.NewReader(rootChunk.Data()[8:]), int64(spanLength), nil
	}


	r := internal.NewSimpleJoinerJob(s.store, int64(spanLength), rootChunk.Data()[8:])
	return r, int64(spanLength), nil
}

func NewSimpleJoiner(store storage.Storer) file.Joiner {
	return &simpleJoiner{
		store: store,
		logger: logging.New(ioutil.Discard, 0),

	}
}
