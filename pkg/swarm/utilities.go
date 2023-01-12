// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

// AddressSliceContains reports whether a is present in addrs.
func AddressSliceContains(addrs []Address, a Address) bool {
	return FindAddressIdx(addrs, a) != -1
}

// AddressSliceRemove removes first occurrence of a in addrs, returning the modified slice.
func AddressSliceRemove(addrs []Address, a Address) []Address {
	i := FindAddressIdx(addrs, a)
	if i == -1 {
		return addrs
	}

	return append(addrs[:i], addrs[i+1:]...)
}

// FindAddressIdx returns the index of the first occurrence of a in addrs,
// or -1 if not present.
func FindAddressIdx(addrs []Address, a Address) int {
	for i, v := range addrs {
		if v.Equal(a) {
			return i
		}
	}
	return -1
}

// FindChunkIdxWithAddress returns the index of the first occurrence of
// Chunk with Address a in chunks, or -1 if not present.
func FindChunkIdxWithAddress(chunks []Chunk, a Address) int {
	for i, c := range chunks {
		if c != nil && a.Equal(c.Address()) {
			return i
		}
	}
	return -1
}

// ChunksSliceContainsAddress reports whether Chunk with Address a is present in chunks.
func ChunksSliceContainsAddress(chunks []Chunk, a Address) bool {
	return FindChunkIdxWithAddress(chunks, a) != -1
}
