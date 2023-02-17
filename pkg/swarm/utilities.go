// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import "bytes"

// ContainsAddress reports whether a is present in addrs.
func ContainsAddress(addrs []Address, a Address) bool {
	return IndexOfAddress(addrs, a) != -1
}

// RemoveAddress removes first occurrence of a in addrs, returning the modified slice.
func RemoveAddress(addrs []Address, a Address) []Address {
	i := IndexOfAddress(addrs, a)
	if i == -1 {
		return addrs
	}

	return append(addrs[:i], addrs[i+1:]...)
}

// IndexOfAddress returns the index of the first occurrence of a in addrs,
// or -1 if not present.
func IndexOfAddress(addrs []Address, a Address) int {
	for i, v := range addrs {
		if v.Equal(a) {
			return i
		}
	}
	return -1
}

// IndexOfChunkWithAddress returns the index of the first occurrence of
// Chunk with Address a in chunks, or -1 if not present.
func IndexOfChunkWithAddress(chunks []Chunk, a Address) int {
	for i, c := range chunks {
		if c != nil && a.Equal(c.Address()) {
			return i
		}
	}
	return -1
}

// ContainsChunkWithAddress reports whether Chunk with Address a is present in chunks.
func ContainsChunkWithAddress(chunks []Chunk, a Address) bool {
	return IndexOfChunkWithAddress(chunks, a) != -1
}

// ContainsChunkWithAddress reports whether Chunk with data d is present in chunks.
func ContainsChunkWithData(chunks []Chunk, d []byte) bool {
	for _, c := range chunks {
		if c != nil && bytes.Equal(c.Data(), d) {
			return true
		}
	}
	return false
}

// FindStampWithBatchID returns the first occurrence of Stamp having the same batchID.
func FindStampWithBatchID(stamps []Stamp, batchID []byte) (Stamp, bool) {
	for _, s := range stamps {
		if s != nil && bytes.Equal(s.BatchID(), batchID) {
			return s, true
		}
	}
	return nil, false
}
