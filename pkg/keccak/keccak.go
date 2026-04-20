// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package keccak provides legacy Keccak-256 (Ethereum-compatible) hashing
// with SIMD acceleration via XKCP.
//
// Sum256x4 uses AVX2 (4-way parallel). Sum256x8 uses AVX-512 (8-way parallel).
// The caller is responsible for dispatching to the correct function based on
// CPU capabilities; Sum256x8 will crash on non-AVX-512 hardware.
//
// # Platform availability
//
// Sum256x4 and Sum256x8 are only compiled on linux/amd64 (with the !purego
// build tag). On every other platform (darwin, windows, linux/arm64, or any
// build configured with -tags=purego) the symbols do not exist; callers must
// route around them using the HasSIMD / BatchWidth helpers below, which are
// available on all platforms and return false / 0 where no SIMD hasher is
// linked in.
//
// # Equal-length (or nil) input constraint
//
// All non-nil inputs passed to Sum256x4 / Sum256x8 in the same call MUST have
// the same length. This is an intrinsic limit of the batched XKCP primitive:
// KeccakP1600timesN_PermuteAll_24rounds advances the sponge state of every
// lane in lockstep, so any lane that has already absorbed its final padding
// block gets extra, unwanted permutations every time a longer lane absorbs
// another block — silently corrupting its output.
//
// Nil (or zero-length) lanes are allowed for partial batches: they are
// treated as no-op fillers so the caller can populate N_real < N lanes with
// real data and ignore the remaining N - N_real output digests. The digests
// produced for nil lanes are not meaningful and must not be consumed. Mixing
// distinct non-zero lengths within one call is unsupported and will produce
// wrong digests for the shorter lanes, even when every individual length
// would work on its own in an all-same-length call.
package keccak

import "encoding/hex"

// Hash256 represents a 32-byte Keccak-256 hash
type Hash256 [32]byte

// HexString returns the hash as a hexadecimal string
func (h Hash256) HexString() string {
	return hex.EncodeToString(h[:])
}

// HasAVX512 reports whether the CPU supports AVX-512 (F + VL) and the
// AVX-512 code path is available.
func HasAVX512() bool {
	return hasAVX512
}

// HasSIMD reports whether any SIMD-accelerated Keccak path is available
// (AVX2 or AVX-512).
func HasSIMD() bool {
	return hasAVX2 || hasAVX512
}

// BatchWidth returns the SIMD batch width: 8 for AVX-512, 4 for AVX2, or 0
// if no SIMD acceleration is available.
func BatchWidth() int {
	if hasAVX512 {
		return 8
	}
	if hasAVX2 {
		return 4
	}
	return 0
}
