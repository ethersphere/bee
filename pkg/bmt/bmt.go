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

// Hasher is a reusable hasher for fixed maximum size chunks representing a BMT
// It reuses a pool of trees for amortised memory allocation and resource control,
// and supports order-agnostic concurrent segment writes and section (double segment) writes
// as well as sequential read and write.
//
// The same hasher instance must not be called concurrently on more than one chunk.
//
// The same hasher instance is synchronously reusable.
//
// Sum gives back the tree to the pool and guaranteed to leave
// the tree and itself in a state reusable for hashing a new chunk.
type Hasher struct {
	*Conf          // configuration
	bmt  *tree     // prebuilt BMT resource for flowcontrol and proofs
	size int       // bytes written to Hasher since last Reset()
	span []byte    // The span of the data subsumed under the chunk
}

// NewHasher gives back an instance of a Hasher struct
func NewHasher(hasherFact func() hash.Hash) *Hasher {
	conf := NewConf(hasherFact, swarm.BmtBranches, 32)

	return &Hasher{
		Conf: conf,
		span: make([]byte, SpanSize),
		bmt:  newTree(conf.maxSize, conf.depth, conf.hasher),
	}
}

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

// Hash returns the BMT root hash of the buffer and an error
// using Hash presupposes sequential synchronous writes (io.Writer interface).
func (h *Hasher) Hash(b []byte) ([]byte, error) {
	if h.size == 0 {
		return doHash(h.hasher(), h.span, h.zerohashes[h.depth])
	}
	// zero-fill remainder so all sections have deterministic input
	for i := h.size; i < h.maxSize; i++ {
		h.bmt.buffer[i] = 0
	}
	if len(h.bmt.levels) == 1 {
		// single-level tree: hash the only section directly
		secsize := 2 * h.segmentSize
		root := h.bmt.levels[0][0]
		rootHash, err := doHash(root.hasher, h.bmt.buffer[:secsize])
		if err != nil {
			return nil, err
		}
		return doHash(h.hasher(), h.span, rootHash)
	}
	rootHash, err := h.hashSIMD()
	if err != nil {
		return nil, err
	}
	return doHash(h.hasher(), h.span, rootHash)
}

// Sum returns the BMT root hash of the buffer, unsafe version of Hash
func (h *Hasher) Sum(b []byte) []byte {
	s, _ := h.Hash(b)
	return s
}

// Write calls sequentially add to the buffer to be hashed.
// All hashing is deferred to Hash().
func (h *Hasher) Write(b []byte) (int, error) {
	l := len(b)
	maxVal := h.maxSize - h.size
	if l > maxVal {
		l = maxVal
	}
	copy(h.bmt.buffer[h.size:], b)
	h.size += l
	return l, nil
}

// Reset prepares the Hasher for reuse
func (h *Hasher) Reset() {
	h.size = 0
	copy(h.span, zerospan)
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
