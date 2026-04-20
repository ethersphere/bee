// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package keccak provides legacy Keccak-256 (Ethereum-compatible) hashing
// with SIMD acceleration via XKCP.
//
// On amd64, the package automatically selects between AVX-512 (8-way parallel)
// and AVX2 (4-way parallel) based on the CPU's capabilities.
package keccak

import (
	"encoding/hex"

	"golang.org/x/crypto/sha3"
)

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
	return hasAVX2
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

// Sum256 computes a single Keccak-256 hash (legacy, Ethereum-compatible).
// Uses the best available implementation.
func Sum256(data []byte) Hash256 {
	var out Hash256
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	copy(out[:], h.Sum(nil))
	return out
}

// Sum256x4 computes 4 Keccak-256 hashes in parallel using AVX2.
// Callers must check HasSIMD() first; invoking without AVX2 panics.
func Sum256x4(inputs [4][]byte) [4]Hash256 {
	if !hasAVX2 {
		panic("keccak: Sum256x4 requires AVX2; call HasSIMD() first")
	}
	var outputs [4]Hash256
	var inputsCopy [4][]byte
	copy(inputsCopy[:], inputs[:])
	keccak256x4(&inputsCopy, &outputs)
	return outputs
}

// Sum256x8 computes 8 Keccak-256 hashes in parallel using AVX-512.
// Callers must check HasAVX512() first; invoking without AVX-512 panics.
func Sum256x8(inputs [8][]byte) [8]Hash256 {
	if !hasAVX512 {
		panic("keccak: Sum256x8 requires AVX-512; call HasAVX512() first")
	}
	var outputs [8]Hash256
	var inputsCopy [8][]byte
	copy(inputsCopy[:], inputs[:])
	keccak256x8(&inputsCopy, &outputs)
	return outputs
}
