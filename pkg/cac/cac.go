package cac

import (
	"bytes"
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
	dataLength := len(data)
	if dataLength > swarm.ChunkSize {
		return nil, errTooLargeChunkData
	}

	if dataLength == 0 {
		return nil, errTooShortChunkData
	}

	span := make([]byte, swarm.SpanSize)
	binary.LittleEndian.PutUint64(span, uint64(dataLength))
	return newWithSpan(data, span)
}

// NewWithDataSpan creates a new chunk assuming that the span precedes the actual data.
func NewWithDataSpan(data []byte) (swarm.Chunk, error) {
	dataLength := len(data)
	if dataLength > swarm.ChunkSize+swarm.SpanSize {
		return nil, errTooLargeChunkData
	}

	if dataLength < swarm.SpanSize {
		return nil, errTooShortChunkData
	}
	return newWithSpan(data[swarm.SpanSize:], data[:swarm.SpanSize])
}

// newWithSpan creates a new chunk prepending the given span to the data.
func newWithSpan(data, span []byte) (swarm.Chunk, error) {
	h := hasher(data)
	hash, err := h(span)
	if err != nil {
		return nil, err
	}

	cdata := make([]byte, len(data)+len(span))
	copy(cdata[:swarm.SpanSize], span)
	copy(cdata[swarm.SpanSize:], data)
	return swarm.NewChunk(swarm.NewAddress(hash), cdata), nil
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

// Valid checks whether the given chunk is a valid content-addressed chunk.
func Valid(c swarm.Chunk) bool {
	data := c.Data()
	if len(data) < swarm.SpanSize {
		return false
	}

	if len(data) > swarm.ChunkSize+swarm.SpanSize {
		return false
	}

	h := hasher(data[swarm.SpanSize:])
	hash, _ := h(data[:swarm.SpanSize])
	return bytes.Equal(hash, c.Address().Bytes())
}
