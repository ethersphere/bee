// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"encoding/binary"
	"hash"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var _ Hash = (*Hasher)(nil)

var (
	zerospan    = make([]byte, 8)
	zerosection = make([]byte, 64)
)

// Capacity returns the maximum amount of bytes that will be processed by this hasher implementation.
// since BMT assumes a balanced binary tree, capacity it is always a power of 2
func (h *Hasher) Capacity() int {
	return h.maxSize
}

// LengthToSpan creates a binary data span size representation.
// It is required for calculating the BMT hash.
func LengthToSpan(length int64) []byte {
	span := make([]byte, SpanSize)
	binary.LittleEndian.PutUint64(span, uint64(length))
	return span
}

// LengthFromSpan returns length from span.
func LengthFromSpan(span []byte) uint64 {
	return binary.LittleEndian.Uint64(span)
}

// SetHeaderInt64 sets the metadata preamble to the little endian binary representation of int64 argument for the current hash operation.
func (h *Hasher) SetHeaderInt64(length int64) {
	binary.LittleEndian.PutUint64(h.span, uint64(length))
}

// SetHeader sets the metadata preamble to the span bytes given argument for the current hash operation.
func (h *Hasher) SetHeader(span []byte) {
	copy(h.span, span)
}

// Size returns the digest size of the hash
func (h *Hasher) Size() int {
	return h.segmentSize
}

// BlockSize returns the optimal write size to the Hasher
func (h *Hasher) BlockSize() int {
	return 2 * h.segmentSize
}

// Sum returns the BMT root hash of the buffer, unsafe version of Hash
func (h *Hasher) Sum(b []byte) []byte {
	s, _ := h.Hash(b)
	return s
}

// calculates the Keccak256 SHA3 hash of the data
func sha3hash(data ...[]byte) ([]byte, error) {
	return doHash(swarm.NewHasher(), data...)
}

// calculates Hash of the data
func doHash(h hash.Hash, data ...[]byte) ([]byte, error) {
	h.Reset()
	for _, v := range data {
		if _, err := h.Write(v); err != nil {
			return nil, err
		}
	}
	return h.Sum(nil), nil
}
