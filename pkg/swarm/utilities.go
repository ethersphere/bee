// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

func AddressSliceContains(addrs []Address, a Address) bool {
	return FindAddressIdx(addrs, a) != -1
}

func AddressSliceRemove(addrs []Address, a Address) []Address {
	i := FindAddressIdx(addrs, a)
	if i == -1 {
		return addrs
	}

	return append(addrs[:i], addrs[i+1:]...)
}

func FindAddressIdx(addrs []Address, a Address) int {
	for i, v := range addrs {
		if v.Equal(a) {
			return i
		}
	}
	return -1
}

func FindChunkIdxWithAddress(chunks []Chunk, a Address) int {
	for i, c := range chunks {
		if c != nil && a.Equal(c.Address()) {
			return i
		}
	}
	return -1
}

func ChunksSliceContainsAddress(chunks []Chunk, a Address) bool {
	return FindChunkIdxWithAddress(chunks, a) != -1
}
