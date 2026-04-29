// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package bmt

import (
	"github.com/ethersphere/bee/v2/pkg/keccak"
)

// hashSIMD computes the BMT root hash using SIMD-accelerated Keccak hashing.
// It processes the tree level by level from leaves to root, using batched
// SIMD calls instead of goroutine-per-section. A single thread handles all
// levels since SIMD already provides intra-call parallelism (4-way or 8-way).
func (h *simdHasher) hashSIMD() ([]byte, error) {
	secsize := 2 * h.segmentSize
	bw := h.batchWidth
	prefixLen := len(h.prefix)

	// Leaf level: hash each section and write results to parent nodes.
	h.hashLeavesBatch(0, len(h.bmt.levels[0]), bw, secsize, prefixLen)

	// Internal levels: process each level single-threaded (diminishing work).
	for lvl := 1; lvl < len(h.bmt.levels)-1; lvl++ {
		h.hashNodesBatch(h.bmt.levels[lvl], bw, prefixLen)
	}

	// Root level: hash using scalar hasher.
	root := h.bmt.levels[len(h.bmt.levels)-1][0]
	return doHash(root.hasher, root.left, root.right)
}

// hashLeavesBatch hashes leaf sections in the range [start, end) using SIMD batches.
func (h *simdHasher) hashLeavesBatch(start, end, bw, secsize, prefixLen int) {
	// the chunk buffer holds raw leaf-level bytes back-to-back: section i lives at [i*secsize, (i+1)*secsize).
	buf := h.bmt.buffer

	if bw == 8 {
		// AVX-512 path: each SIMD call hashes up to 8 sections in lockstep.
		var inputs [8][]byte
		for i := start; i < end; i += 8 {
			// the trailing batch may be short — clamp so we never read past `end`.
			batch := 8
			if i+batch > end {
				batch = end - i
			}
			// stage each lane's input. With a configured prefix we must materialise
			// prefix||section into a scratch buffer because the prefix bytes are not
			// stored in `buf` itself; without a prefix we hand the section slice
			// straight to the SIMD primitive (zero-copy).
			for j := 0; j < batch; j++ {
				offset := (i + j) * secsize
				if prefixLen > 0 {
					copy(h.bmt.leafConcat[j][prefixLen:], buf[offset:offset+secsize])
					inputs[j] = h.bmt.leafConcat[j][:prefixLen+secsize]
				} else {
					inputs[j] = buf[offset : offset+secsize]
				}
			}
			// nil out unused lanes so XKCP treats them as no-op fillers, see keccak.Sum256x8 docs.
			for j := batch; j < 8; j++ {
				inputs[j] = nil
			}
			// single 8-way SIMD permutation produces all 8 digests at once.
			outputs := keccak.Sum256x8(inputs)
			// each digest is the value of the parent's left or right child slot;
			// write it directly into the parent so the next level can read it without copying.
			for j := 0; j < batch; j++ {
				leaf := h.bmt.levels[0][i+j]
				if leaf.isLeft {
					copy(leaf.parent.left, outputs[j][:])
				} else {
					copy(leaf.parent.right, outputs[j][:])
				}
			}
		}
	} else {
		// AVX2 path: identical structure to the AVX-512 branch but at width 4.
		var inputs [4][]byte
		for i := start; i < end; i += 4 {
			// clamp the trailing batch so we never read past `end`.
			batch := 4
			if i+batch > end {
				batch = end - i
			}
			// stage prefix||section per lane (or a zero-copy slice when no prefix is configured).
			for j := 0; j < batch; j++ {
				offset := (i + j) * secsize
				if prefixLen > 0 {
					copy(h.bmt.leafConcat[j][prefixLen:], buf[offset:offset+secsize])
					inputs[j] = h.bmt.leafConcat[j][:prefixLen+secsize]
				} else {
					inputs[j] = buf[offset : offset+secsize]
				}
			}
			// nil out unused lanes so XKCP treats them as no-op fillers.
			for j := batch; j < 4; j++ {
				inputs[j] = nil
			}
			// 4-way SIMD permutation produces all digests in one call.
			outputs := keccak.Sum256x4(inputs)
			// place each digest directly into the parent's left/right child slot.
			for j := 0; j < batch; j++ {
				leaf := h.bmt.levels[0][i+j]
				if leaf.isLeft {
					copy(leaf.parent.left, outputs[j][:])
				} else {
					copy(leaf.parent.right, outputs[j][:])
				}
			}
		}
	}
}

// hashNodesBatch hashes a level of internal nodes using SIMD batches.
// Each node's left||right (64 bytes) is hashed to produce the input for its parent.
func (h *simdHasher) hashNodesBatch(nodes []*simdNode, bw, prefixLen int) {
	count := len(nodes)
	segSize := h.segmentSize
	// `concat` is the per-lane scratch buffer reused across calls; its leading prefixLen
	// bytes already hold the configured prefix (set up at tree construction time), so
	// we only ever rewrite the trailing left||right region.
	concat := &h.bmt.concat

	if bw == 8 {
		// AVX-512 path: 8 nodes hashed per SIMD call.
		var inputs [8][]byte
		for i := 0; i < count; i += 8 {
			// clamp the trailing batch so we never read past `count`.
			batch := 8
			if i+batch > count {
				batch = count - i
			}
			// for each active lane, stage prefix||left||right into its scratch slot.
			// the `prefixLen` bytes at the head are already populated, so we only
			// overwrite the [prefixLen, prefixLen+2*segSize) range.
			for j := 0; j < batch; j++ {
				n := nodes[i+j]
				copy(concat[j][prefixLen:prefixLen+segSize], n.left)
				copy(concat[j][prefixLen+segSize:], n.right)
				inputs[j] = concat[j][:prefixLen+2*segSize]
			}
			// nil out unused lanes so XKCP treats them as no-op fillers.
			for j := batch; j < 8; j++ {
				inputs[j] = nil
			}
			// one 8-way permutation produces all parent digests for this batch.
			outputs := keccak.Sum256x8(inputs)
			// drop each digest into its parent's left/right slot for the next level.
			for j := 0; j < batch; j++ {
				n := nodes[i+j]
				if n.isLeft {
					copy(n.parent.left, outputs[j][:])
				} else {
					copy(n.parent.right, outputs[j][:])
				}
			}
		}
	} else {
		// AVX2 path: same shape as the AVX-512 branch but at width 4.
		var inputs [4][]byte
		for i := 0; i < count; i += 4 {
			// clamp trailing batch to the remaining nodes.
			batch := 4
			if i+batch > count {
				batch = count - i
			}
			// stage prefix||left||right per active lane in the per-lane scratch buffer.
			for j := 0; j < batch; j++ {
				n := nodes[i+j]
				copy(concat[j][prefixLen:prefixLen+segSize], n.left)
				copy(concat[j][prefixLen+segSize:], n.right)
				inputs[j] = concat[j][:prefixLen+2*segSize]
			}
			// nil out unused lanes so XKCP treats them as no-op fillers.
			for j := batch; j < 4; j++ {
				inputs[j] = nil
			}
			// 4-way SIMD permutation produces all parent digests for this batch.
			outputs := keccak.Sum256x4(inputs)
			// place each digest directly into the parent's left/right child slot.
			for j := 0; j < batch; j++ {
				n := nodes[i+j]
				if n.isLeft {
					copy(n.parent.left, outputs[j][:])
				} else {
					copy(n.parent.right, outputs[j][:])
				}
			}
		}
	}
}
