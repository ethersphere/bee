// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy

import (
	"errors"

	"github.com/ethersphere/bee/pkg/swarm"
)

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
	et, err := l.getErasureTable()
	if err != nil {
		return 0
	}
	return et.getParities(shards)
}

// GetMaxShards returns back the maximum number of effective data chunks
func (l Level) GetMaxShards() int {
	p := l.GetParities(swarm.Branches)
	return swarm.Branches - p
}

// GetEncParities returns number of parities for encrypted chunks based on appendix F table 6
func (l Level) GetEncParities(shards int) int {
	et, err := l.getEncErasureTable()
	if err != nil {
		return 0
	}
	return et.getParities(shards)
}

func (l Level) getErasureTable() (erasureTable, error) {
	switch l {
	case MEDIUM:
		return *mediumEt, nil
	case STRONG:
		return *strongEt, nil
	case INSANE:
		return *insaneEt, nil
	case PARANOID:
		return *paranoidEt, nil
	default:
		return erasureTable{}, errors.New("redundancy: level NONE does not have erasure table")
	}
}

func (l Level) getEncErasureTable() (erasureTable, error) {
	switch l {
	case MEDIUM:
		return *encMediumEt, nil
	case STRONG:
		return *encStrongEt, nil
	case INSANE:
		return *encInsaneEt, nil
	case PARANOID:
		return *encParanoidEt, nil
	default:
		return erasureTable{}, errors.New("redundancy: level NONE does not have erasure table")
	}
}

// GetMaxEncShards returns back the maximum number of effective encrypted data chunks
func (l Level) GetMaxEncShards() int {
	p := l.GetEncParities(swarm.EncryptedBranches)
	return (swarm.Branches - p) / 2
}

// GetReplicaCount returns back the dispersed replica number
func (l Level) GetReplicaCount() int {
	return replicaCounts[int(l)]
}

// GetReplicaIndexBase returns back the dispersed replica index base of the level
func (l Level) GetReplicaIndexBase() int {
	return replicaIndexBases[int(l)-1]
}

// Decrement returns a weaker redundancy level compare to the current one
func (l Level) Decrement() Level {
	return Level(uint8(l) - 1)
}

// TABLE INITS

var mediumEt = newErasureTable(
	[]int{94, 68, 46, 28, 14, 5, 1},
	[]int{9, 8, 7, 6, 5, 4, 3},
)
var encMediumEt = newErasureTable(
	[]int{47, 34, 23, 14, 7, 2},
	[]int{9, 8, 7, 6, 5, 4},
)

var strongEt = newErasureTable(
	[]int{104, 95, 86, 77, 69, 61, 53, 46, 39, 32, 26, 20, 15, 10, 6, 3, 1},
	[]int{21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5},
)
var encStrongEt = newErasureTable(
	[]int{52, 47, 43, 38, 34, 30, 26, 23, 19, 16, 13, 10, 7, 5, 3, 1},
	[]int{21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6},
)

var insaneEt = newErasureTable(
	[]int{92, 87, 82, 77, 73, 68, 63, 59, 54, 50, 45, 41, 37, 33, 29, 26, 22, 19, 16, 13, 10, 8, 5, 3, 2, 1},
	[]int{31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6},
)
var encInsaneEt = newErasureTable(
	[]int{46, 43, 41, 38, 36, 34, 31, 29, 27, 25, 22, 20, 18, 16, 14, 13, 11, 9, 8, 6, 5, 4, 2, 1},
	[]int{31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 7},
)

var paranoidEt = newErasureTable(
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
var encParanoidEt = newErasureTable(
	[]int{
		18, 17, 16, 15, 14, 13, 12, 11, 10, 9,
		8, 7, 6, 5, 4, 3, 2, 1,
	},
	[]int{
		88, 85, 82, 79, 76, 72, 69, 66, 62, 59,
		55, 51, 48, 44, 39, 35, 30, 24,
	},
)

// DISPERSED REPLICAS INIT

// GetReplicaCounts returns back the ascending dispersed replica counts for all redundancy levels
func GetReplicaCounts() [5]int {
	c := replicaCounts
	return c
}

// the actual number of replicas needed to keep the error rate below 1/10^6
// for the five levels of redundancy are 0, 2, 4, 5, 19
// we use an approximation as the successive powers of 2
var replicaCounts = [5]int{0, 2, 4, 8, 16}

// index bases needed to keep track how many addresses were mined for a level.
var replicaIndexBases = [5]int{0, 2, 6, 14}
