// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package bmt

import (
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Hasher is a reusable hasher for fixed maximum size chunks representing a BMT.
// This implementation buffers all data and defers hashing to Hash(),
// using SIMD-accelerated Keccak for multi-level trees.
//
// The same hasher instance must not be called concurrently on more than one chunk.
//
// The same hasher instance is synchronously reusable.
type Hasher struct {
	*Conf        // configuration
	bmt   *tree  // prebuilt BMT resource for flowcontrol and proofs
	size  int    // bytes written to Hasher since last Reset()
	span  []byte // The span of the data subsumed under the chunk
}

// NewHasher gives back an instance of a Hasher struct
func NewHasher() *Hasher {
	return newHasherWithConf(NewConf(swarm.BmtBranches, 32))
}

// NewPrefixHasher gives back an instance of a Hasher struct with the given prefix
// prepended to every hash operation.
func NewPrefixHasher(prefix []byte) *Hasher {
	return newHasherWithConf(NewConfWithPrefix(prefix, swarm.BmtBranches, 32))
}

func newHasherWithConf(conf *Conf) *Hasher {
	return &Hasher{
		Conf: conf,
		span: make([]byte, SpanSize),
		bmt:  newTree(conf.maxSize, conf.depth, conf.baseHasher, conf.prefix),
	}
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

// Hash returns the BMT root hash of the buffer and an error
// using Hash presupposes sequential synchronous writes (io.Writer interface).
func (h *Hasher) Hash(b []byte) ([]byte, error) {
	if h.size == 0 {
		return doHash(h.baseHasher(), h.span, h.zerohashes[h.depth])
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
		return doHash(h.baseHasher(), h.span, rootHash)
	}
	rootHash, err := h.hashSIMD()
	if err != nil {
		return nil, err
	}
	return doHash(h.baseHasher(), h.span, rootHash)
}

// Reset prepares the Hasher for reuse
func (h *Hasher) Reset() {
	h.size = 0
	copy(h.span, zerospan)
}
