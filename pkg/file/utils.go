// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"bytes"
	"errors"

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

// ChunkAddresses returns the child (data + parity) addresses of the
// intermediate chunk and the data shard count. Trailing stamp-carrier
// references are excluded — see CarrierAddresses.
// assumes data is truncated by ChunkPayloadSize
func ChunkAddresses(data []byte, parities, carrierRefs, reflen int) (addrs []swarm.Address, shardCnt int) {
	shardCnt = (len(data) - (parities+carrierRefs)*swarm.HashSize) / reflen
	dataLen := shardCnt * reflen
	// a malformed parent may advertise more parity and carrier references than
	// its payload can hold; do not index outside of it
	if dataLen < 0 || dataLen+parities*swarm.HashSize > len(data) {
		return nil, shardCnt
	}
	for offset := 0; offset < dataLen; offset += reflen {
		addrs = append(addrs, swarm.NewAddress(data[offset:offset+swarm.HashSize]))
	}
	for i := range parities {
		offset := dataLen + i*swarm.HashSize
		addrs = append(addrs, swarm.NewAddress(data[offset:offset+swarm.HashSize]))
	}
	return addrs, shardCnt
}

// CarrierAddresses returns the trailing stamp-carrier group references of the
// intermediate chunk, carriers first then carrier parities.
// assumes data is truncated by ChunkPayloadSize
func CarrierAddresses(data []byte, carrierRefs int) []swarm.Address {
	if carrierRefs <= 0 || carrierRefs*swarm.HashSize > len(data) {
		return nil
	}
	addrs := make([]swarm.Address, 0, carrierRefs)
	for i := carrierRefs; i > 0; i-- {
		offset := len(data) - i*swarm.HashSize
		addrs = append(addrs, swarm.NewAddress(data[offset:offset+swarm.HashSize]))
	}
	return addrs
}

// ReferenceCount brute-forces the data shard count from which identify the parity count as well in a substree
// assumes span > swarm.chunkSize
// returns data shard, parity and stamp-carrier reference counts
func ReferenceCount(span uint64, level redundancy.Level, encrytedChunk bool) (int, int, int) {
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
		branching  = uint64(maxShards) // branching factor is how many data shard references can fit into one intermediate chunk
		branchSize = uint64(swarm.ChunkSize)
	)
	// search for branch level big enough to include span
	branchLevel := 1
	for branchSize < span {

		branchSize *= branching
		branchLevel++
	}
	// span in one full reference
	referenceSize := uint64(swarm.ChunkSize)
	// referenceSize = branching ** (branchLevel - 1)
	for i := 1; i < branchLevel-1; i++ {
		referenceSize *= branching
	}

	dataShardAddresses := 1
	spanOffset := referenceSize
	for spanOffset < span {
		spanOffset += referenceSize
		dataShardAddresses++
	}

	parityAddresses := level.GetParities(dataShardAddresses)
	if encrytedChunk {
		parityAddresses = level.GetEncParities(dataShardAddresses)
	}

	return dataShardAddresses, parityAddresses, level.CarrierRefs(dataShardAddresses + parityAddresses)
}
