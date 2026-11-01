// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"bytes"
	"errors"
	"math"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var zeroAddress = [32]byte{}

// ChunkPayloadSize returns the effective byte length of an intermediate chunk
// assumes data is always chunk size (without span)
func ChunkPayloadSize(data []byte) (int, error) {
	l := len(data)
	for l >= swarm.HashSize {
		if !bytes.Equal(data[l-swarm.HashSize:l], zeroAddress[:]) {
			return l, nil
		}

		l -= swarm.HashSize
	}

	return 0, errors.New("redundancy getter: intermediate chunk does not have at least a child")
}

// ChunkAddresses returns data shards and parities of the intermediate chunk
// assumes data is truncated by ChunkPayloadSize
func ChunkAddresses(data []byte, parities, reflen int) (addrs []swarm.Address, shardCnt int) {
	shardCnt = (len(data) - parities*swarm.HashSize) / reflen
	for offset := 0; offset < len(data); offset += reflen {
		addrs = append(addrs, swarm.NewAddress(data[offset:offset+swarm.HashSize]))
		if len(addrs) == shardCnt && reflen != swarm.HashSize {
			reflen = swarm.HashSize
			offset += reflen
		}
	}
	return addrs, shardCnt
}

// ReferenceCount brute-forces the data shard count from which identify the parity count as well in a substree
// assumes span > swarm.chunkSize
// returns data and parity shard number
//
// span is attacker-influenced on the decryption path, so the arithmetic below
// saturates instead of wrapping: a span larger than any real trie can hold
// yields the shard count for the widest representable subtree rather than
// looping forever. Results for every non-overflowing span are unchanged.
func ReferenceCount(span uint64, level redundancy.Level, encrytedChunk bool) (int, int) {
	// assume we have a trie of size `span` then we can assume that all of
	// the forks except for the last one on the right are of equal size
	// this is due to how the splitter wraps levels.
	// first the algorithm will search for a BMT level where span can be included
	// then identify how large data one reference can hold on that level
	// then count how many references can satisfy span
	// and finally how many parity shards should be on that level
	maxShards := level.GetMaxShards()
	if encrytedChunk {
		maxShards = level.GetMaxEncShards()
	}
	var (
		// branching factor is how many data shard references can fit into one
		// intermediate chunk. It is >= 2 for every Level value including the
		// invalid ones, which fall back to swarm.Branches / 2; the saturation
		// guards below divide by it and rely on that. See
		// TestReferenceCountBranchingFactor.
		branching  = uint64(maxShards)
		branchSize = uint64(swarm.ChunkSize)
	)
	// search for branch level big enough to include span
	branchLevel := 1
	for branchSize < span {
		// the next multiplication would overflow: span is larger than any
		// representable trie, so stop here rather than wrapping to 0 and
		// looping forever. branchSize is not read after this loop.
		if branchSize > math.MaxUint64/branching {
			branchLevel++
			break
		}
		branchSize *= branching
		branchLevel++
	}
	// span in one full reference
	referenceSize := uint64(swarm.ChunkSize)
	// referenceSize = branching ** (branchLevel - 1)
	// this loop performs exactly as many multiplications as the loop above
	// actually carried out, so the guard here is defensive only.
	for i := 1; i < branchLevel-1; i++ {
		if referenceSize > math.MaxUint64/branching {
			referenceSize = math.MaxUint64
			break
		}
		referenceSize *= branching
	}

	dataShardAddresses := 1
	spanOffset := referenceSize
	for spanOffset < span {
		if spanOffset > math.MaxUint64-referenceSize {
			break
		}
		spanOffset += referenceSize
		dataShardAddresses++
	}

	parityAddresses := level.GetParities(dataShardAddresses)
	if encrytedChunk {
		parityAddresses = level.GetEncParities(dataShardAddresses)
	}

	return dataShardAddresses, parityAddresses
}
