// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cac

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/bmtpool"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	ErrChunkSpanShort = fmt.Errorf("chunk span must have exactly length of %d", swarm.SpanSize)
	ErrChunkDataLarge = fmt.Errorf("chunk data exceeds maximum allowed length")
)

// New creates a new content address chunk by initializing a span and appending the data to it.
func New(data []byte) (swarm.Chunk, error) {
	dataLength := len(data)

	if err := validateDataLength(dataLength); err != nil {
		return nil, err
	}

	span := make([]byte, swarm.SpanSize)
	binary.LittleEndian.PutUint64(span, uint64(dataLength))

	return newWithSpan(data, span)
}

// NewWithDataSpan creates a new chunk assuming that the span precedes the actual data.
func NewWithDataSpan(data []byte) (swarm.Chunk, error) {
	dataLength := len(data)

	if err := validateDataLength(dataLength - swarm.SpanSize); err != nil {
		return nil, err
	}

	return newWithSpan(data[swarm.SpanSize:], data[:swarm.SpanSize])
}

// validateDataLength validates if data length (without span) is correct.
func validateDataLength(dataLength int) error {
	if dataLength < 0 { // dataLength could be negative when span size is subtracted
		spanLength := swarm.SpanSize + dataLength
		return fmt.Errorf("invalid CAC span length %d: %w", spanLength, ErrChunkSpanShort)
	}
	if dataLength > swarm.ChunkSize {
		return fmt.Errorf("invalid CAC data length %d: %w", dataLength, ErrChunkDataLarge)
	}
	return nil
}

// newWithSpan creates a new chunk prepending the given span to the data.
func newWithSpan(data, span []byte) (swarm.Chunk, error) {
	hash, err := DoHash(data, span)
	if err != nil {
		return nil, err
	}

	cacData := make([]byte, len(data)+len(span))
	copy(cacData, span)
	copy(cacData[swarm.SpanSize:], data)

	return swarm.NewChunk(swarm.NewAddress(hash), cacData), nil
}

// Valid checks whether the given chunk is a valid content-addressed chunk.
func Valid(c swarm.Chunk) bool {
	data := c.Data()

	if validateDataLength(len(data)-swarm.SpanSize) != nil {
		return false
	}

	hash, _ := DoHash(data[swarm.SpanSize:], data[:swarm.SpanSize])

	return bytes.Equal(hash, c.Address().Bytes())
}

func DoHash(data, span []byte) ([]byte, error) {
	hasher := bmtpool.Get()
	defer bmtpool.Put(hasher)

	hasher.SetHeader(span)
	if _, err := hasher.Write(data); err != nil {
		return nil, err
	}

	return hasher.Hash(nil)
}
