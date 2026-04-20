// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package bmt

import (
	"encoding/binary"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var _ Hasher = (*simdHasher)(nil)

// simdHasher is a BMT hasher implementation that buffers all data and defers hashing
// to Hash(), using SIMD-accelerated Keccak for multi-level trees.
//
// Single-threaded: SIMD provides intra-call parallelism (4-way or 8-way), replacing
// the goroutine-per-section model of the goroutine hasher.
//
// The same hasher instance must not be called concurrently on more than one chunk.
// The same hasher instance is synchronously reusable after calling Reset.
type simdHasher struct {
	*simdConf
	bmt  *simdTree
	size int
	span []byte
}

func newSIMDHasher() *simdHasher {
	sc := newSIMDConf(nil, swarm.BmtBranches, 32)
	return &simdHasher{
		simdConf: sc,
		span:     make([]byte, SpanSize),
		bmt:      newSIMDTree(sc.maxSize, sc.depth, sc.baseHasher, sc.prefix),
	}
}

func newSIMDPrefixHasher(prefix []byte) *simdHasher {
	sc := newSIMDConf(prefix, swarm.BmtBranches, 32)
	return &simdHasher{
		simdConf: sc,
		span:     make([]byte, SpanSize),
		bmt:      newSIMDTree(sc.maxSize, sc.depth, sc.baseHasher, sc.prefix),
	}
}

// Capacity returns the maximum number of bytes this hasher can process.
func (h *simdHasher) Capacity() int { return h.maxSize }

// Size returns the digest size.
func (h *simdHasher) Size() int { return h.segmentSize }

// BlockSize returns the optimal write block size.
func (h *simdHasher) BlockSize() int { return 2 * h.segmentSize }

// Sum is the unsafe Hash wrapper.
func (h *simdHasher) Sum(b []byte) []byte { s, _ := h.Hash(b); return s }

// SetHeaderInt64 sets the span preamble from an int64.
func (h *simdHasher) SetHeaderInt64(length int64) {
	binary.LittleEndian.PutUint64(h.span, uint64(length))
}

// SetHeader copies the span preamble from the argument.
func (h *simdHasher) SetHeader(span []byte) { copy(h.span, span) }

// Write buffers input to be hashed. All hashing is deferred to Hash().
func (h *simdHasher) Write(b []byte) (int, error) {
	l := len(b)
	maxVal := h.maxSize - h.size
	if l > maxVal {
		l = maxVal
	}
	copy(h.bmt.buffer[h.size:], b)
	h.size += l
	return l, nil
}

// Hash returns the BMT root hash of the buffer written so far.
func (h *simdHasher) Hash(b []byte) ([]byte, error) {
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

// HashPadded is equivalent to Hash for the SIMD hasher since Hash already
// zero-fills the buffer. Exposed so the Hasher interface contract is satisfied.
func (h *simdHasher) HashPadded(b []byte) ([]byte, error) {
	return h.Hash(b)
}

// Reset prepares the Hasher for reuse.
func (h *simdHasher) Reset() {
	h.size = 0
	copy(h.span, zerospan)
}

// Proof returns the inclusion proof of the i-th data segment.
func (h *simdHasher) Proof(i int) Proof {
	index := i
	if i < 0 || i > 127 {
		panic("segment index can only lie between 0-127")
	}
	i = i / 2
	n := h.bmt.leaves[i]
	isLeft := n.isLeft
	var sisters [][]byte
	for n = n.parent; n != nil; n = n.parent {
		sisters = append(sisters, n.getSister(isLeft))
		isLeft = n.isLeft
	}

	secsize := 2 * h.segmentSize
	offset := i * secsize
	section := make([]byte, secsize)
	copy(section, h.bmt.buffer[offset:offset+secsize])
	segment, firstSegmentSister := section[:h.segmentSize], section[h.segmentSize:]
	if index%2 != 0 {
		segment, firstSegmentSister = firstSegmentSister, segment
	}
	sisters = append([][]byte{firstSegmentSister}, sisters...)
	return Proof{segment, sisters, h.span, index}
}

// Verify reconstructs the BMT root from a proof for the i-th segment.
func (h *simdHasher) Verify(i int, proof Proof) (root []byte, err error) {
	var section []byte
	if i%2 == 0 {
		section = append(append(section, proof.ProveSegment...), proof.ProofSegments[0]...)
	} else {
		section = append(append(section, proof.ProofSegments[0]...), proof.ProveSegment...)
	}
	i = i / 2
	n := h.bmt.leaves[i]
	hasher := h.baseHasher()
	isLeft := n.isLeft
	root, err = doHash(hasher, section)
	if err != nil {
		return nil, err
	}
	n = n.parent

	for _, sister := range proof.ProofSegments[1:] {
		if isLeft {
			root, err = doHash(hasher, root, sister)
		} else {
			root, err = doHash(hasher, sister, root)
		}
		if err != nil {
			return nil, err
		}
		isLeft = n.isLeft
		n = n.parent
	}
	return doHash(hasher, proof.Span, root)
}
