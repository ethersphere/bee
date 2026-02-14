// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/keccak"
)

// hashSIMD computes the BMT root hash using a bitvector cascade with SIMD-
// accelerated Keccak hashing. Leaf sections are hashed in parallel goroutines.
// Completing a SIMD-width group at one level immediately triggers hashing at the
// next level via atomic bitvector race resolution, overlapping work across tree
// levels.
func (h *Hasher) hashSIMD() ([]byte, error) {
	t := h.bmt
	bw := h.batchWidth
	segSize := h.segmentSize
	secSize := 2 * segSize
	leafCount := len(t.levels[0])

	// Reset done bitvectors for this hash operation.
	for i := range t.done {
		atomic.StoreUint64(&t.done[i], 0)
	}

	// Spawn one goroutine per bw-aligned group of leaf sections.
	var wg sync.WaitGroup
	for start := 0; start < leafCount; start += bw {
		end := start + bw
		if end > leafCount {
			end = leafCount
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			h.hashLeafGroup(start, end, secSize, segSize)
			h.cascade(0, start, end-start)
		}(start, end)
	}
	wg.Wait()

	// Populate node left/right from flat hashes for proof compatibility.
	h.populateNodesFromHashes()

	lastLevel := len(t.hashes) - 1
	return t.hashes[lastLevel][:segSize], nil
}

// hashLeafGroup hashes leaf sections [start, end) from the buffer into hashes[0].
func (h *Hasher) hashLeafGroup(start, end, secSize, segSize int) {
	t := h.bmt
	bw := h.batchWidth
	buf := t.buffer
	dst := t.hashes[0]

	if bw == 8 {
		var inputs [8][]byte
		for i := start; i < end; i += 8 {
			batch := 8
			if i+batch > end {
				batch = end - i
			}
			for j := range batch {
				offset := (i + j) * secSize
				inputs[j] = buf[offset : offset+secSize]
			}
			for j := batch; j < 8; j++ {
				inputs[j] = nil
			}
			outputs := keccak.Sum256x8(inputs)
			for j := range batch {
				copy(dst[(i+j)*segSize:(i+j+1)*segSize], outputs[j][:])
			}
		}
	} else {
		var inputs [4][]byte
		for i := start; i < end; i += 4 {
			batch := 4
			if i+batch > end {
				batch = end - i
			}
			for j := range batch {
				offset := (i + j) * secSize
				inputs[j] = buf[offset : offset+secSize]
			}
			for j := batch; j < 4; j++ {
				inputs[j] = nil
			}
			outputs := keccak.Sum256x4(inputs)
			for j := range batch {
				copy(dst[(i+j)*segSize:(i+j+1)*segSize], outputs[j][:])
			}
		}
	}
}

// cascade propagates completed hashes up the tree using atomic bitvector
// race resolution. When a SIMD-width group completes at a level, exactly one
// goroutine wins the race and hashes the pairs to produce the next level.
func (h *Hasher) cascade(level, start, count int) {
	t := h.bmt
	bw := h.batchWidth
	segSize := h.segmentSize
	numLevels := len(t.hashes)

	for level < numLevels-1 {
		countAtLevel := len(t.hashes[level]) / segSize

		// Set bits for the hashes we just computed at [start, start+count).
		var setBits uint64
		for i := start; i < start+count; i++ {
			setBits |= 1 << uint(i)
		}

		old := atomic.OrUint64(&t.done[level], setBits)
		newVal := old | setBits

		// Determine the group that these hashes belong to.
		// A group of 2*bw consecutive hashes produces bw parent hashes.
		groupSize := 2 * bw
		if groupSize > countAtLevel {
			groupSize = countAtLevel
		}
		groupStart := (start / groupSize) * groupSize
		groupEnd := groupStart + groupSize
		if groupEnd > countAtLevel {
			groupEnd = countAtLevel
		}

		// Build the group completion mask.
		var groupMask uint64
		for i := groupStart; i < groupEnd; i++ {
			groupMask |= 1 << uint(i)
		}

		wasComplete := (old & groupMask) == groupMask
		isComplete := (newVal & groupMask) == groupMask

		if !isComplete || wasComplete {
			return // group not yet complete, or already handled
		}

		// This goroutine completed the group — hash pairs to next level.
		actualGroupSize := groupEnd - groupStart
		pairCount := actualGroupSize / 2
		h.simdHashPairs(level, groupStart, pairCount, segSize)

		// Continue cascading at the next level.
		level++
		start = groupStart / 2
		count = pairCount
	}
}

// simdHashPairs hashes pairCount consecutive pairs from hashes[level] starting
// at groupStart, writing results into hashes[level+1]. Each pair is two adjacent
// 32-byte hashes (64 bytes total) — zero-copy from the flat array.
func (h *Hasher) simdHashPairs(level, groupStart, pairCount, segSize int) {
	t := h.bmt
	bw := h.batchWidth
	pairSize := 2 * segSize
	src := t.hashes[level]
	dst := t.hashes[level+1]
	dstBase := groupStart / 2

	if bw == 8 {
		var inputs [8][]byte
		for i := 0; i < pairCount; i += 8 {
			batch := 8
			if i+batch > pairCount {
				batch = pairCount - i
			}
			for j := range batch {
				offset := (groupStart + (i+j)*2) * segSize
				inputs[j] = src[offset : offset+pairSize]
			}
			for j := batch; j < 8; j++ {
				inputs[j] = nil
			}
			outputs := keccak.Sum256x8(inputs)
			for j := range batch {
				dstIdx := dstBase + i + j
				copy(dst[dstIdx*segSize:(dstIdx+1)*segSize], outputs[j][:])
			}
		}
	} else {
		var inputs [4][]byte
		for i := 0; i < pairCount; i += 4 {
			batch := 4
			if i+batch > pairCount {
				batch = pairCount - i
			}
			for j := range batch {
				offset := (groupStart + (i+j)*2) * segSize
				inputs[j] = src[offset : offset+pairSize]
			}
			for j := batch; j < 4; j++ {
				inputs[j] = nil
			}
			outputs := keccak.Sum256x4(inputs)
			for j := range batch {
				dstIdx := dstBase + i + j
				copy(dst[dstIdx*segSize:(dstIdx+1)*segSize], outputs[j][:])
			}
		}
	}
}

// populateNodesFromHashes fills in node left/right fields from the flat hashes
// arrays so that Proof() can traverse the tree. hashes[L][i] maps to the parent
// of levels[L][i]: parent.left if i is even, parent.right if i is odd.
func (h *Hasher) populateNodesFromHashes() {
	t := h.bmt
	segSize := h.segmentSize

	for L := range len(t.hashes) - 1 {
		hashCount := len(t.hashes[L]) / segSize
		for i := range hashCount {
			src := t.hashes[L][i*segSize : (i+1)*segSize]
			n := t.levels[L][i]
			if n.isLeft {
				n.parent.left = src
			} else {
				n.parent.right = src
			}
		}
	}
}
