// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package keccak

import "fmt"

// Sum256x4 computes 4 Keccak-256 hashes in parallel using AVX2.
//
// All non-nil inputs MUST have the same length; nil (or zero-length) lanes
// are allowed as partial-batch fillers whose output must be ignored. It panics
// if two non-empty inputs differ in length. See the package doc for the
// rationale.
//
// Only compiled on linux/amd64 (the only platform where the XKCP .syso is
// linkable). Call sites that may run on other platforms must be gated on the
// same build tag or on keccak.HasSIMD() at runtime.
func Sum256x4(inputs [4][]byte) [4]Hash256 {
	var outputs [4]Hash256
	var inputsCopy [4][]byte
	copy(inputsCopy[:], inputs[:])
	prepareLanes(inputsCopy[:])
	keccak256x4(&inputsCopy, &outputs)
	return outputs
}

// Sum256x8 computes 8 Keccak-256 hashes in parallel using AVX-512.
// Must only be called on AVX-512-capable hardware.
//
// All non-nil inputs MUST have the same length; nil (or zero-length) lanes
// are allowed as partial-batch fillers whose output must be ignored. It panics
// if two non-empty inputs differ in length. See the package doc for the
// rationale.
//
// Only compiled on linux/amd64 (the only platform where the XKCP .syso is
// linkable). Call sites that may run on other platforms must be gated on the
// same build tag or on keccak.HasAVX512() at runtime.
func Sum256x8(inputs [8][]byte) [8]Hash256 {
	var outputs [8]Hash256
	var inputsCopy [8][]byte
	copy(inputsCopy[:], inputs[:])
	prepareLanes(inputsCopy[:])
	keccak256x8(&inputsCopy, &outputs)
	return outputs
}

// prepareLanes enforces the input contract of the XKCP wrappers before the
// lanes are handed to C, where nothing is bounds checked.
//
// The wrappers absorb max_full = max(len/136) full blocks in lockstep and then
// write each lane's padding marker at padded[len - max_full*136] in a 136-byte
// stack buffer. For any lane with fewer full blocks than the longest one that
// index is negative, and the write lands up to several KB below the buffer,
// outside the stub's frame and possibly outside the goroutine stack.
//
// Empty lanes (the documented partial-batch fillers) are therefore replaced
// with an alias of a non-empty lane, so every lane has the same length and the
// index stays in range; their digests are discarded by the caller anyway.
// Lanes of different non-empty lengths are a caller bug that would also
// corrupt memory, so they panic instead of reaching C.
func prepareLanes(lanes [][]byte) {
	var filler []byte
	for _, lane := range lanes {
		if len(lane) == 0 {
			continue
		}
		if filler == nil {
			filler = lane
			continue
		}
		if len(lane) != len(filler) {
			panic(fmt.Sprintf("keccak: SIMD lanes must be empty or of equal length, got %d and %d bytes", len(filler), len(lane)))
		}
	}
	// all lanes empty: max_full is 0, so every padding index is 0 and in range.
	if filler == nil {
		return
	}
	for i, lane := range lanes {
		if len(lane) == 0 {
			lanes[i] = filler
		}
	}
}
