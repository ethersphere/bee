// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/swarm"
)

// Level is the redundancy level
// which carries information about how much redundancy should be added to data to remain retrievable with a 1-10^(-6) certainty
// in different groups of expected chunk retrival error rates (level values)
type Level uint8

const (
	// no redundancy will be added
	NONE Level = iota
	// expected 1% chunk retrieval error rate
	MEDIUM
	// expected 5% chunk retrieval error rate
	STRONG
	// expected 10% chunk retrieval error rate
	INSANE
	// expected 50% chunk retrieval error rate
	PARANOID
)

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
	case NONE:
		return erasureTable{}, errors.New("redundancy: level NONE does not have erasure table")
	case MEDIUM:
		return mediumEt, nil
	case STRONG:
		return strongEt, nil
	case INSANE:
		return insaneEt, nil
	case PARANOID:
		return paranoidEt, nil
	default:
		return erasureTable{}, fmt.Errorf("redundancy: level value %d is not a legit redundancy level", l)
	}
}

func (l Level) getEncErasureTable() (erasureTable, error) {
	switch l {
	case NONE:
		return erasureTable{}, errors.New("redundancy: level NONE does not have erasure table")
	case MEDIUM:
		return encMediumEt, nil
	case STRONG:
		return encStrongEt, nil
	case INSANE:
		return encInsaneEt, nil
	case PARANOID:
		return encParanoidEt, nil
	default:
		return erasureTable{}, fmt.Errorf("redundancy: level value %d is not a legit redundancy level", l)
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

// GetReplicaCounts returns back the ascending dispersed replica counts for all redundancy levels
func GetReplicaCounts() [5]int {
	c := replicaCounts
	return c
}

// the actual number of replicas needed to keep the error rate below 1/10^6
// for the five levels of redundancy are 0, 2, 4, 5, 19
// we use an approximation as the successive powers of 2
var replicaCounts = [5]int{0, 2, 4, 8, 16}

type levelKey struct{}

// SetLevelInContext sets the redundancy level in the context
func SetLevelInContext(ctx context.Context, level Level) context.Context {
	return context.WithValue(ctx, levelKey{}, level)
}

// GetLevelFromContext is a helper function to extract the redundancy level from the context
func GetLevelFromContext(ctx context.Context) Level {
	rlevel := PARANOID
	if val := ctx.Value(levelKey{}); val != nil {
		rlevel = val.(Level)
	}
	return rlevel
}
