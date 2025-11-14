// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package combinator

import (
	"iter"
	"math/bits"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// IterateReplicaAddresses returns an iterator (iter.Seq) that yields bit
// combinations of an address, starting from replication level 1. The original
// address is not returned. This approach allows for memory-efficient iteration
// over a large set of combinations. The combination with the one flipped bit of
// the original address will be returned at the end.
//
// # Performance and Memory Considerations
//
// To ensure safe use of the yielded addresses, this function returns a new copy
// of the address on each iteration. This prevents accidental modification of
// previously yielded addresses.
//
// The iterator terminates if the replication level exceeds passed maxLevel or if
// the input data slice is not long enough for the bit manipulations required at
// the next replication level.
func IterateReplicaAddresses(addr swarm.Address, maxLevel int) iter.Seq[swarm.Address] {
	// State variables for the iterator closure.
	// A single buffer is used and mutated in each iteration, and a copy is yielded.
	// It is initialized with a copy of the original address data.
	currentSlice := append([]byte{}, addr.Bytes()...)

	var currentLevel int
	var bytesNeeded int
	// nextLevelIndex marks the combination index at which the replication level increases
	// (e.g., 1, 2, 4, 8, ...).
	nextLevelIndex := 1
	// prevCombinationIndex is used to calculate the bitwise difference for
	// efficient state transitions.
	var prevCombinationIndex int

	return func(yield func(swarm.Address) bool) {
		// combinationIndex iterates through all possible combinations, but skip the original address.
		for combinationIndex := 1; ; combinationIndex++ {
			// When the combinationIndex reaches the next power of two, the replication level
			// of bit combinations is increased for subsequent iterations.
			if combinationIndex >= nextLevelIndex {
				// The replication level is determined by the number of bits in the combinationIndex.
				// combinationIndex=1 -> replication level=1
				// combinationIndex=2 -> replication level=2
				// combinationIndex=4 -> replication level=3
				// combinationIndex=8 -> replication level=4
				currentLevel = bits.Len(uint(combinationIndex))
				// Set the threshold for the next replication level increase.
				// For replication level=1 (idx=1), next threshold is 2.
				// For replication level=2 (idx=2,3), next threshold is 4.
				// For replication level=3 (idx=4..7), next threshold is 8.
				nextLevelIndex = 1 << currentLevel

				// Boundary checks are performed only when the replication level changes.
				if currentLevel > maxLevel {
					if maxLevel <= 0 {
						// Do not return the bit flip address of replication level 0,
						// because replication level 0 should have no replicas. Negative
						// replication levels are invalid and should not return any
						// replicas, as well.
						return
					}
					// Create a new slice based on the original address.
					originalAddrBytes := addr.Bytes()
					flippedAddrBytes := make([]byte, len(originalAddrBytes))
					copy(flippedAddrBytes, originalAddrBytes)

					// Calculate the byte index for the bit to flip.
					bitIndexToFlip := maxLevel
					byteIndex := bitIndexToFlip / 8

					// Ensure the flippedAddrBytes is long enough to flip this bit.
					if len(flippedAddrBytes) <= byteIndex {
						return // Cannot flip bit, slice is too short.
					}

					// Flip the level bit in the new slice.
					bitPositionInByte := 7 - (bitIndexToFlip % 8)
					bitMask := byte(1 << bitPositionInByte)
					flippedAddrBytes[byteIndex] ^= bitMask

					// Yield this modified address
					if !yield(swarm.NewAddress(flippedAddrBytes)) {
						return // Consumer-requested stop.
					}
					return // Iteration completed up to the defined maximum replication level.
				}

				bytesNeeded = (currentLevel + 7) / 8 // Ceiling of integer division.

				if len(addr.Bytes()) < bytesNeeded {
					// The data slice is too short for the current replication level.
					return
				}
			}

			// The generation logic is optimized to flip only the bits that
			// differ from the previous combination. For subsequent indices,
			// the buffer is XORed with the difference between the current and
			// previous combination indices.
			bitsToFlip := combinationIndex ^ prevCombinationIndex
			for bitIndex := 0; bitIndex < currentLevel; bitIndex++ {
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

			// Yield a copy of the mutated slice. If yield returns false, the
			// consumer has requested to stop the iteration.
			if !yield(swarm.NewAddress(append([]byte(nil), currentSlice...))) {
				return // Consumer-requested stop.
			}

			// Check for integer overflow on the combinationIndex.
			if combinationIndex < 0 {
				return // Integer overflow; terminate iteration.
			}
		}
	}
}
