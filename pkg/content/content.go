// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package contains convenience methods and validator for content-addressed chunks
package content

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/swarm"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
)

// NewChunk creates a new content-addressed single-span chunk.
// The length of the chunk data is set as the span.
func NewChunk(data []byte) (swarm.Chunk, error) {
	return NewChunkWithSpan(data, int64(len(data)))
}

// NewChunkWithSpan creates a new content-addressed chunk from given data and span.
func NewChunkWithSpan(data []byte, span int64) (swarm.Chunk, error) {
	if len(data) > swarm.ChunkSize {
		return nil, errors.New("max chunk size exceeded")
	}
	if span < swarm.ChunkSize && span != int64(len(data)) {
		return nil, fmt.Errorf("single-span chunk size mismatch; span is %d, chunk data length %d", span, len(data))
	}

	bmtPool := bmtlegacy.NewTreePool(swarm.NewHasher, swarm.Branches, bmtlegacy.PoolSize)
	hasher := bmtlegacy.New(bmtPool)

	// execute hash, compare and return result
	spanBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(spanBytes, uint64(span))
	err := hasher.SetSpanBytes(spanBytes)
	if err != nil {
		return nil, err
	}
	_, err = hasher.Write(data)
	if err != nil {
		return nil, err
	}
	s := hasher.Sum(nil)

	payload := append(spanBytes, data...)
	address := swarm.NewAddress(s)
	return swarm.NewChunk(address, payload), nil
}

// NewChunkWithSpanBytes deserializes a content-addressed chunk from separate
// data and span byte slices.
func NewChunkWithSpanBytes(data, spanBytes []byte) (swarm.Chunk, error) {
	bmtPool := bmtlegacy.NewTreePool(swarm.NewHasher, swarm.Branches, bmtlegacy.PoolSize)
	hasher := bmtlegacy.New(bmtPool)

	// execute hash, compare and return result
	err := hasher.SetSpanBytes(spanBytes)
	if err != nil {
		return nil, err
	}
	_, err = hasher.Write(data)
	if err != nil {
		return nil, err
	}
	s := hasher.Sum(nil)

	payload := append(spanBytes, data...)
	address := swarm.NewAddress(s)
	return swarm.NewChunk(address, payload), nil
}

// contentChunkFromBytes deserializes a content-addressed chunk.
func contentChunkFromBytes(chunkData []byte) (swarm.Chunk, error) {
	if len(chunkData) < swarm.SpanSize {
		return nil, errors.New("shorter than minimum length")
	}

	return NewChunkWithSpanBytes(chunkData[8:], chunkData[:8])
}
