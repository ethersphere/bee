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
	// empty input: no data was ever written, so the BMT root is the all-zero
	// subtree hash at depth h.depth. Still prepend the span so the output
	// shape matches a normal chunk hash.
	if h.size == 0 {
		return doHash(h.baseHasher(), h.span, h.zerohashes[h.depth])
	}
	// zero-fill the tail of the buffer: every leaf section must carry
	// deterministic bytes, because SIMD batches hash whole sections at a time
	// without a "is this section occupied?" check. clear() lowers to memclr.
	clear(h.bmt.buffer[h.size:])
	// degenerate single-level tree: the whole buffer is already one section,
	// so there is nothing for SIMD to batch — just run the scalar hasher.
	if len(h.bmt.levels) == 1 {
		secsize := 2 * h.segmentSize
		root := h.bmt.levels[0][0]
		rootHash, err := doHash(root.hasher, h.bmt.buffer[:secsize])
		if err != nil {
			return nil, err
		}
		return doHash(h.baseHasher(), h.span, rootHash)
	}
	// general case: fan out through the tree via hashSIMD, which processes
	// each level bottom-up in batches of 4 or 8 hashes per SIMD call.
	rootHash, err := h.hashSIMD()
	if err != nil {
		return nil, err
	}
	// prepend the span and hash once more to produce the chunk address.
	// When a prefix is configured, baseHasher() returns a PrefixHasher whose
	// Reset re-absorbs the prefix, so the final digest is
	// keccak(prefix || span || rootHash) — the same wrap-per-level rule used
	// by hashSIMD at every internal level.
	return doHash(h.baseHasher(), h.span, rootHash)
}

// HashPadded is equivalent to Hash for the SIMD hasher since Hash already
// zero-fills the buffer. Exposed so the Hasher interface contract is satisfied.
func (h *simdHasher) HashPadded(b []byte) ([]byte, error) {
	return h.Hash(b)
}

// Reset prepares the Hasher for reuse. The internal data buffer is not zeroed;
// any stale bytes past h.size are overwritten on the next Write and zero-filled
// on demand by Hash, so post-Reset inspection of the buffer may still see
// previous-chunk contents.
func (h *simdHasher) Reset() {
	h.size = 0
	copy(h.span, zerospan)
}

// Proof returns the inclusion proof of the i-th data segment.
func (h *simdHasher) Proof(i int) Proof {
	// keep the original (segment-level) index around: callers identify the
	// segment by its 0..127 position; we divide down to a leaf-node (section)
	// index below for tree navigation.
	index := i
	if i < 0 || i > 127 {
		panic("segment index can only lie between 0-127")
	}
	// two segments share one leaf section (32B + 32B hashed together),
	// so the leaf-node index is i/2.
	i = i / 2
	n := h.bmt.leaves[i]
	isLeft := n.isLeft
	// walk from the leaf's parent to the root, collecting the sister hash
	// at each level along the way.
	var sisters [][]byte
	for n = n.parent; n != nil; n = n.parent {
		sisters = append(sisters, n.getSister(isLeft))
		isLeft = n.isLeft
	}

	// copy out the two segments that live in this leaf section. One is the
	// segment being proven, the other is its immediate sister (the very first
	// entry in the proof path, below the leaf level).
	secsize := 2 * h.segmentSize
	offset := i * secsize
	section := make([]byte, secsize)
	copy(section, h.bmt.buffer[offset:offset+secsize])
	segment, firstSegmentSister := section[:h.segmentSize], section[h.segmentSize:]
	// if the original index is odd, the proven segment sits on the right,
	// so swap which half is the segment and which is the sister.
	if index%2 != 0 {
		segment, firstSegmentSister = firstSegmentSister, segment
	}
	// prepend the in-section sister so the proof list reads leaf-up: the
	// first hop combines segment with firstSegmentSister, then walks upward.
	sisters = append([][]byte{firstSegmentSister}, sisters...)
	return Proof{segment, sisters, h.span, index}
}

// Verify reconstructs the BMT root from a proof for the i-th segment.
func (h *simdHasher) Verify(i int, proof Proof) (root []byte, err error) {
	// rebuild the leaf section (two 32B segments concatenated) in the same
	// order they were originally hashed. Even index → proven segment on the
	// left, odd index → proven segment on the right.
	var section []byte
	if i%2 == 0 {
		section = append(append(section, proof.ProveSegment...), proof.ProofSegments[0]...)
	} else {
		section = append(append(section, proof.ProofSegments[0]...), proof.ProveSegment...)
	}
	// derive the leaf-node index from the segment index, mirroring Proof.
	i = i / 2
	n := h.bmt.leaves[i]
	hasher := h.baseHasher()
	isLeft := n.isLeft
	// start by hashing the reconstructed section: this is the leaf-level
	// hash that the proof path will lift up to the root.
	root, err = doHash(hasher, section)
	if err != nil {
		return nil, err
	}
	n = n.parent

	// climb level by level, pairing the running hash with the sister from
	// the proof. Orientation alternates based on whether we arrived at this
	// node from its left or right child.
	for _, sister := range proof.ProofSegments[1:] {
		if isLeft {
			root, err = doHash(hasher, root, sister)
		} else {
			root, err = doHash(hasher, sister, root)
		}
		if err != nil {
			return nil, err
		}
		// advance to the next level; record which side we came from so the
		// next hash pairs the operands in the correct order.
		isLeft = n.isLeft
		n = n.parent
	}
	// finally mix in the span to produce the chunk address, matching what
	// Hash does at the top of the tree.
	return doHash(hasher, proof.Span, root)
}
