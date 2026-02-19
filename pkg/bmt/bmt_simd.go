// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"github.com/ethersphere/bee/v2/pkg/keccak"
)

// hashSIMD computes the BMT root hash using SIMD-accelerated Keccak hashing.
// It processes the tree level by level from leaves to root, using batched
// SIMD calls instead of goroutine-per-section. A single thread handles all
// levels since SIMD already provides intra-call parallelism (4-way or 8-way).
func (h *Hasher) hashSIMD() ([]byte, error) {
	secsize := 2 * h.segmentSize
	bw := h.batchWidth
	prefixLen := len(h.prefix)

	// Leaf level: hash each section and write results to parent nodes.
	// Single-threaded: SIMD batching (4 or 8 hashes per call) replaces goroutine parallelism.
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
func (h *Hasher) hashLeavesBatch(start, end, bw, secsize, prefixLen int) {
	buf := h.bmt.buffer

	if bw == 8 {
		var inputs [8][]byte
		for i := start; i < end; i += 8 {
			batch := 8
			if i+batch > end {
				batch = end - i
			}
			for j := 0; j < batch; j++ {
				offset := (i + j) * secsize
				if prefixLen > 0 {
					copy(h.bmt.leafConcat[j][prefixLen:], buf[offset:offset+secsize])
					inputs[j] = h.bmt.leafConcat[j][:prefixLen+secsize]
				} else {
					inputs[j] = buf[offset : offset+secsize]
				}
			}
			for j := batch; j < 8; j++ {
				inputs[j] = nil
			}
			outputs := keccak.Sum256x8(inputs)
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
		var inputs [4][]byte
		for i := start; i < end; i += 4 {
			batch := 4
			if i+batch > end {
				batch = end - i
			}
			for j := 0; j < batch; j++ {
				offset := (i + j) * secsize
				if prefixLen > 0 {
					copy(h.bmt.leafConcat[j][prefixLen:], buf[offset:offset+secsize])
					inputs[j] = h.bmt.leafConcat[j][:prefixLen+secsize]
				} else {
					inputs[j] = buf[offset : offset+secsize]
				}
			}
			for j := batch; j < 4; j++ {
				inputs[j] = nil
			}
			outputs := keccak.Sum256x4(inputs)
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
func (h *Hasher) hashNodesBatch(nodes []*node, bw, prefixLen int) {
	count := len(nodes)
	segSize := h.segmentSize
	concat := &h.bmt.concat

	if bw == 8 {
		var inputs [8][]byte
		for i := 0; i < count; i += 8 {
			batch := 8
			if i+batch > count {
				batch = count - i
			}
			for j := 0; j < batch; j++ {
				n := nodes[i+j]
				copy(concat[j][prefixLen:prefixLen+segSize], n.left)
				copy(concat[j][prefixLen+segSize:], n.right)
				inputs[j] = concat[j][:prefixLen+2*segSize]
			}
			for j := batch; j < 8; j++ {
				inputs[j] = nil
			}
			outputs := keccak.Sum256x8(inputs)
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
		var inputs [4][]byte
		for i := 0; i < count; i += 4 {
			batch := 4
			if i+batch > count {
				batch = count - i
			}
			for j := 0; j < batch; j++ {
				n := nodes[i+j]
				copy(concat[j][prefixLen:prefixLen+segSize], n.left)
				copy(concat[j][prefixLen+segSize:], n.right)
				inputs[j] = concat[j][:prefixLen+2*segSize]
			}
			for j := batch; j < 4; j++ {
				inputs[j] = nil
			}
			outputs := keccak.Sum256x4(inputs)
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
