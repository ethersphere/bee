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
		switch s := shards; {
		case s >= 104:
			return 21
		case s >= 95:
			return 20
		case s >= 86:
			return 19
		case s >= 77:
			return 18
		case s >= 69:
			return 17
		case s >= 61:
			return 16
		case s >= 53:
			return 15
		case s >= 46:
			return 14
		case s >= 39:
			return 13
		case s >= 32:
			return 12
		case s >= 26:
			return 11
		case s >= 20:
			return 10
		case s >= 15:
			return 9
		case s >= 10:
			return 8
		case s >= 6:
			return 7
		case s >= 3:
			return 6
		default:
			return 5
		}
	case MEDIUM:
		switch s := shards; {
		case s >= 94:
			return 9
		case s >= 68:
			return 8
		case s >= 46:
			return 7
		case s >= 28:
			return 6
		case s >= 14:
			return 5
		case s >= 5:
			return 4
		default:
			return 3
		}
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
