// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package combinator

import (
	"iter"
	"math/bits"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// IterateAddressCombinations returns an iterator (iter.Seq) that yields bit
// combinations of an address. The combinations are produced in order of
// increasing 'depth', starting from depth 0. This approach allows for
// memory-efficient iteration over a large set of combinations.
//
// The maxDepth parameter defines the maximum depth of the combination
// generation, serving as a safeguard against excessive memory allocation and
// computation time. A depth of 24 results in approximately 16.7 million
// combinations.
//
// # Performance and Memory Considerations
//
// For optimal performance, this function yields the same byte slice on each
// iteration, modifying its content in place. This avoids memory allocations
// within the loop.
//
// Consequently, it is unsafe to retain a reference to the yielded slice after
// the loop advances. If the slice needs to be stored, a copy must be created.
//
// Example of correct usage:
//
//	// Safe: A copy of the slice is created and stored.
//	var allCombinations [][]byte
//	for combo := range IterateAddressCombinations(data, 8) {
//	    allCombinations = append(allCombinations, slices.Clone(combo))
//	}
//
// Example of incorrect usage:
//
//	// Unsafe: This will result in a slice where all elements point to the
//	// same underlying byte slice, which will hold the value of the last
//	// combination generated.
//	var allCombinationsBad [][]byte
//	for combo := range IterateAddressCombinations(data, 8) {
//	    allCombinationsBad = append(allCombinationsBad, combo)
//	}
//
// The iterator terminates if the depth exceeds maxDepth or if the input data
// slice is not long enough for the bit manipulations required at the next
// depth level.
func IterateAddressCombinations(addr swarm.Address, maxDepth int) iter.Seq[swarm.Address] {
	// State variables for the iterator closure.
	// A single buffer is used, mutated, and yielded in each iteration.
	// It is initialized with a copy of the original address data.
	currentSlice := append([]byte{}, addr.Bytes()...)

	var currentDepth int
	var bytesNeeded int
	// nextDepthIndex marks the combination index at which the depth increases
	// (e.g., 1, 2, 4, 8, ...).
	nextDepthIndex := 1
	// prevCombinationIndex is used to calculate the bitwise difference for
	// efficient state transitions.
	var prevCombinationIndex int

	return func(yield func(swarm.Address) bool) {
		// combinationIndex iterates through all possible combinations.
		for combinationIndex := 0; ; combinationIndex++ {
			// When the combinationIndex reaches the next power of two, the depth
			// of bit combinations is increased for subsequent iterations.
			if combinationIndex >= nextDepthIndex {
				// The depth is determined by the number of bits in the combinationIndex.
				// combinationIndex=1 -> depth=1
				// combinationIndex=2 -> depth=2
				// combinationIndex=4 -> depth=3
				// combinationIndex=8 -> depth=4
				currentDepth = bits.Len(uint(combinationIndex))
				// Set the threshold for the next depth increase.
				// For depth=1 (idx=1), next threshold is 2.
				// For depth=2 (idx=2,3), next threshold is 4.
				// For depth=3 (idx=4..7), next threshold is 8.
				nextDepthIndex = 1 << currentDepth

				// Boundary checks are performed only when the depth changes.
				if currentDepth > maxDepth {
					return // Iteration completed up to the defined maximum depth.
				}

				bytesNeeded = (currentDepth + 7) / 8 // Ceiling of integer division.

				if len(addr.Bytes()) < bytesNeeded {
					// The data slice is too short for the current depth.
					return
				}
			}

			// The generation logic is optimized to flip only the bits that
			// differ from the previous combination. For combinationIndex=0,
			// (0^0) is 0, so no bits are flipped. For subsequent indices,
			// the buffer is XORed with the difference between the current and
			// previous combination indices.
			bitsToFlip := combinationIndex ^ prevCombinationIndex
			for bitIndex := 0; bitIndex < currentDepth; bitIndex++ {
				// Check if the bit at bitIndex is set in the difference.
				if (bitsToFlip>>bitIndex)&1 == 1 {
					// If set, flip the corresponding bit in the buffer.
					byteIndex := bitIndex / 8
					bitPositionInByte := 7 - (bitIndex % 8)
					bitMask := byte(1 << bitPositionInByte)
					currentSlice[byteIndex] ^= bitMask
				}
			}
			prevCombinationIndex = combinationIndex // Update for the next iteration.

			// Yield the mutated slice. If yield returns false, the consumer
			// has requested to stop the iteration.
			if !yield(swarm.NewAddress(currentSlice)) {
				return // Consumer-requested stop.
			}

			// Check for integer overflow on the combinationIndex.
			if combinationIndex < 0 {
				return // Integer overflow; terminate iteration.
			}
		}
	}
}
