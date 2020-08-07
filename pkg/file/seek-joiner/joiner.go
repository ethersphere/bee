// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package joiner provides implementations of the file.Joiner interface
package joiner

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/seek-joiner/internal"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// simpleJoiner wraps a non-optimized implementation of file.Joiner.
type simpleJoiner struct {
	getter storage.Getter
}

// NewSimpleJoiner creates a new simpleJoiner.
func NewSimpleJoiner(getter storage.Getter) file.Joiner {
	return &simpleJoiner{
		getter: getter,
	}
}

func (s *simpleJoiner) Size(ctx context.Context, address swarm.Address) (dataSize int64, err error) {
	// retrieve the root chunk to read the total data length the be retrieved
	rootChunk, err := s.getter.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		return 0, err
	}

	chunkLength := rootChunk.Data()
	if len(chunkLength) < 8 {
		return 0, fmt.Errorf("invalid chunk content of %d bytes", chunkLength)
	}

	chunkData := rootChunk.Data()
	if toDecrypt {
		originalData, err := internal.DecryptChunkData(rootChunk.Data(), key)
		if err != nil {
			return 0, err
		}
		chunkData = originalData
	}
	dataLength := binary.LittleEndian.Uint64(chunkData[:8])
	return int64(dataLength), nil
}

// Join implements the file.Joiner interface.
//
// It uses a non-optimized internal component that only retrieves a data chunk
// after the previous has been read.
func (s *simpleJoiner) Join(ctx context.Context, address swarm.Address) (dataOut io.ReadCloser, dataSize int64, err error) {
	var addr = address.Bytes()

	// retrieve the root chunk to read the total data length the be retrieved
	rootChunk, err := s.getter.Get(ctx, storage.ModeGetRequest, swarm.NewAddress(addr))
	if err != nil {
		return nil, 0, err
	}

	var chunkData = rootChunk.Data()

	// if this is a single chunk, short circuit to returning just that chunk
	spanLength := binary.LittleEndian.Uint64(chunkData[:8])
	chunkToSend := rootChunk
	if spanLength <= swarm.ChunkSize {
		data := chunkData[8:]
		return file.NewSimpleReadCloser(data), int64(spanLength), nil
	}
	r := internal.NewSimpleJoinerJob(ctx, s.getter, chunkToSend)
	return r, int64(spanLength), nil
}
