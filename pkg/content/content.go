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

var (
	bmtPool = bmtlegacy.NewTreePool(swarm.NewHasher, swarm.Branches, bmtlegacy.PoolSize)
)

var _ swarm.ChunkValidator = (*ContentAddressValidator)(nil)

// ContentAddressValidator validates that the address of a given chunk
// is the content address of its contents.
type ContentAddressValidator struct {
}

// NewContentAddressValidator constructs a new ContentAddressValidator
func NewContentAddressValidator() swarm.ChunkValidator {
	return &ContentAddressValidator{}
}

// Validate performs the validation check.
func (v *ContentAddressValidator) Validate(ch swarm.Chunk) (valid bool) {
	chunkData := ch.Data()
	rch, err := contentChunkFromBytes(chunkData)
	if err != nil {
		return false
	}

	address := ch.Address()
	return address.Equal(rch.Address())
}

// NewContentChunk creates a new content-addressed single-span chunk.
// The length of the chunk data is set as the span.
func NewContentChunk(data []byte) (swarm.Chunk, error) {
	return NewContentChunkWithSpan(data, int64(len(data)))
}

// NewContentChunkWithSpan creates a new content-addressed chunk from given data and span.
func NewContentChunkWithSpan(data []byte, span int64) (swarm.Chunk, error) {
	if len(data) > swarm.ChunkSize {
		return nil, errors.New("max chunk size exceeded")
	}
	if span < swarm.ChunkSize && span != int64(len(data)) {
		return nil, fmt.Errorf("single-span chunk size mismatch; span is %d, chunk data length %d", span, len(data))
	}
	spanBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(spanBytes, uint64(span))
	return newContentChunk(data, spanBytes, span)
}

// contentChunkFromBytes deserializes a content-addressed chunk.
func contentChunkFromBytes(chunkData []byte) (swarm.Chunk, error) {
	if len(chunkData) < swarm.SpanSize {
		return nil, errors.New("shorter than minimum length")
	}
	return contentChunkFromBytesAndSpan(chunkData[8:], chunkData[:8])
}

// contentChunkFromBytesAndSpan deserializes a content-addressed chunk from separate
// data and span byte slices.
func contentChunkFromBytesAndSpan(data []byte, spanBytes []byte) (swarm.Chunk, error) {
	span := binary.LittleEndian.Uint64(spanBytes)
	return newContentChunk(data, spanBytes, int64(span))
}

// newContentChunk is the lowest level handler of deserialization of content-addressed chunks.
func newContentChunk(data []byte, spanBytes []byte, span int64) (swarm.Chunk, error) {
	hasher := bmtlegacy.New(bmtPool)

	// execute hash, compare and return result
	err := hasher.SetSpan(span)
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
