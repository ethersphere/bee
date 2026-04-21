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

// Reset prepares the Hasher for reuse. The internal data buffer is not zeroed;
// any stale bytes past h.size are overwritten on the next Write and zero-filled
// on demand by Hash, so post-Reset inspection of the buffer may still see
// previous-chunk contents.
func (h *simdHasher) Reset() {
	h.size = 0
	copy(h.span, zerospan)
}
