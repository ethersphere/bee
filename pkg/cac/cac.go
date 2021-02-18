package cac

import (
	"encoding/binary"
	"errors"

	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	errTooShortChunkData = errors.New("short chunk data")
	errTooLargeChunkData = errors.New("data too large")
)

// New creates a new content address chunk by initializing a span and appending the data to it.
func New(data []byte) (swarm.Chunk, error) {
	if len(data) > swarm.ChunkSize+swarm.SpanSize {
		return nil, errTooLargeChunkData
	}

	if len(data) < swarm.SpanSize {
		return nil, errTooShortChunkData
	}

	span := make([]byte, swarm.SpanSize)
	binary.LittleEndian.PutUint64(span, uint64(len(data)))
	return NewWithSpan(data, span)
}

// NewWithDataSpan creates a new chunk assuming that the span precedes the actual data.
func NewWithDataSpan(data []byte) (swarm.Chunk, error) {
	if len(data) > swarm.ChunkSize+swarm.SpanSize {
		return nil, errTooLargeChunkData
	}

	if len(data) < swarm.SpanSize {
		return nil, errTooShortChunkData
	}
	span := data[:swarm.SpanSize]
	content := data[swarm.SpanSize:]
	return NewWithSpan(content, span)
}

// NewWithSpan creates a new chunk prepending the given span to the data.
func NewWithSpan(data []byte, span []byte) (swarm.Chunk, error) {
	h := hasher(data)
	hash, err := h(span)
	if err != nil {
		return nil, err
	}
	return swarm.NewChunk(swarm.NewAddress(hash), append(span, data...)), nil
}

// hasher is a helper function to hash a given data based on the given span.
func hasher(data []byte) func([]byte) ([]byte, error) {
	return func(span []byte) ([]byte, error) {
		hasher := bmtpool.Get()
		defer bmtpool.Put(hasher)

		if err := hasher.SetSpanBytes(span); err != nil {
			return nil, err
		}
		if _, err := hasher.Write(data); err != nil {
			return nil, err
		}
		return hasher.Sum(nil), nil
	}
}
