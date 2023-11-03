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
	case MEDIUM:
		return mediumEt.GetParities(shards)
	case STRONG:
		return strongEt.GetParities(shards)
	case INSANE:
		return insaneEt.GetParities(shards)
	case PARANOID:
		return paranoidEt.GetParities(shards)
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
	case MEDIUM:
		return encMediumEt.GetParities(shards)
	case STRONG:
		return encStrongEt.GetParities(shards)
	case INSANE:
		return encInsaneEt.GetParities(shards)
	case PARANOID:
		return encParanoidEt.GetParities(shards)
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

var mediumEt = NewErasureTable(
	[]int{94, 68, 46, 28, 14, 5, 1},
	[]int{9, 8, 7, 6, 5, 4, 3},
)

var encMediumEt = NewErasureTable(
	[]int{47, 34, 23, 14, 7, 2},
	[]int{9, 8, 7, 6, 5, 4},
)

var strongEt = NewErasureTable(
	[]int{104, 95, 86, 77, 69, 61, 53, 46, 39, 32, 26, 20, 15, 10, 6, 3, 1},
	[]int{21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5},
)
var encStrongEt = NewErasureTable(
	[]int{52, 47, 43, 38, 34, 30, 26, 23, 19, 16, 13, 10, 7, 5, 3, 1},
	[]int{21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6},
)

var insaneEt = NewErasureTable(
	[]int{92, 87, 82, 77, 73, 68, 63, 59, 54, 50, 45, 41, 37, 33, 29, 26, 22, 19, 16, 13, 10, 8, 5, 3, 2, 1},
	[]int{31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6},
)
var encInsaneEt = NewErasureTable(
	[]int{46, 43, 41, 38, 36, 34, 31, 29, 27, 25, 22, 20, 18, 16, 14, 13, 11, 9, 8, 6, 5, 4, 2, 1},
	[]int{31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 7},
)

var paranoidEt = NewErasureTable(
	[]int{
		37, 36, 35, 34, 33, 32, 31, 30, 29, 28,
		27, 26, 25, 24, 23, 22, 21, 20, 19, 18,
		17, 16, 15, 14, 13, 12, 11, 10, 9, 8,
		7, 6, 5, 4, 3, 2, 1,
	},
	[]int{
		90, 88, 87, 85, 84, 82, 81, 79, 77, 76,
		74, 72, 71, 69, 67, 66, 64, 62, 60, 59,
		57, 55, 53, 51, 49, 48, 46, 44, 41, 39,
		37, 35, 32, 30, 27, 24, 20,
	},
)
var encParanoidEt = NewErasureTable(
	[]int{
		18, 17, 16, 15, 14, 13, 12, 11, 10, 9,
		8, 7, 6, 5, 4, 3, 2, 1,
	},
	[]int{
		88, 85, 82, 79, 76, 72, 69, 66, 62, 59,
		55, 51, 48, 44, 39, 35, 30, 24,
	},
)
