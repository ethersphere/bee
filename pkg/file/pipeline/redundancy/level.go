// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy

import "github.com/ethersphere/bee/pkg/swarm"

type Level uint8

const (
	NONE Level = iota
	MEDIUM
	STRONG
	INSANE
	PARANOID
)

const maxLevel = 8

// GetParities returns number of parities based on appendix F table 5
func (l Level) GetParities(shards int) int {
	switch l {
	case PARANOID:
		return 91
	case INSANE:
		return 31
	case STRONG:
		return strongEt.GetParities(shards)
	case MEDIUM:
		return mediumEt.GetParities(shards)
	default:
		return 0
	}
}

// GetMaxShards returns back the maximum number of effective data chunks
func (l Level) GetMaxShards() int {
	p := l.GetParities(swarm.Branches)
	return swarm.Branches - p
}

// GetEncParities returns number of parities for encrypted chunks based on appendix F table 6
func (l Level) GetEncParities(shards int) int {
	switch l {
	case PARANOID:
		return 56
	case INSANE:
		return 27
	case STRONG:
		return 19
	case MEDIUM:
		return 9
	default:
		return 0
	}
}

// GetMaxEncShards returns back the maximum number of effective encrypted data chunks
func (l Level) GetMaxEncShards() int {
	p := l.GetParities(swarm.EncryptedBranches)
	return (swarm.Branches - p) / 2
}

// TABLE INITS

var strongEt = NewErasureTable(
	[]int{104, 95, 86, 77, 69, 61, 53, 46, 39, 32, 26, 20, 15, 10, 6, 3, 1},
	[]int{21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5},
)

var mediumEt = NewErasureTable(
	[]int{94, 68, 46, 28, 14, 5, 1},
	[]int{9, 8, 7, 6, 5, 4, 3},
)
