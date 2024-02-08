// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy

type erasureTable struct {
	shards   []int
	parities []int
}

// newErasureTable initializes a shards<->parities table
//
//	 the value order must be strictly descending in both arrays
//		example usage:
//			shards := []int{94, 68, 46, 28, 14, 5, 1}
//			parities := []int{9, 8, 7, 6, 5, 4, 3}
//			var et = newErasureTable(shards, parities)
func newErasureTable(shards, parities []int) erasureTable {
	if len(shards) != len(parities) {
		panic("redundancy table: shards and parities arrays must be of equal size")
	}

	maxShards := shards[0]
	maxParities := parities[0]
	for k := 1; k < len(shards); k++ {
		s := shards[k]
		if maxShards <= s {
			panic("redundancy table: shards should be in strictly descending order")
		}
		p := parities[k]
		if maxParities <= p {
			panic("redundancy table: parities should be in strictly descending order")
		}
		maxShards, maxParities = s, p
	}

	return erasureTable{
		shards:   shards,
		parities: parities,
	}
}

// getParities gives back the optimal parity number for a given shard
func (et *erasureTable) getParities(maxShards int) int {
	for k, s := range et.shards {
		if maxShards >= s {
			return et.parities[k]
		}
	}
	return 0
}
