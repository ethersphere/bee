// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package keccak

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"

	"golang.org/x/crypto/sha3"
)

// referenceHash returns the legacy Keccak-256 digest using golang.org/x/crypto,
// which is the same function the BMT hasher uses in its goroutine path.
func referenceHash(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	return h.Sum(nil)
}

// testInputLengths covers a wide spread: sub-rate (<136), one-byte-before the
// rate (135 — where Keccak padding forces the 0x81 collision), exact rate,
// just over, and multi-block sizes out to 4096 bytes. The BMT caller only
// uses 64/96-byte inputs; the larger sizes exercise the lockstep multi-block
// absorption in the C wrapper.
var testInputLengths = []int{0, 1, 31, 32, 63, 64, 65, 95, 96, 97, 128, 134, 135, 136, 137, 200, 272, 1024, 4096}

func TestSum256x4(t *testing.T) {
	if !HasSIMD() {
		t.Skip("AVX2 not available on this CPU")
	}

	for _, length := range testInputLengths {
		t.Run(fmt.Sprintf("len_%d", length), func(t *testing.T) {
			var inputs [4][]byte
			var expected [4][]byte
			for i := range inputs {
				buf := make([]byte, length)
				_, _ = rand.Read(buf)
				inputs[i] = buf
				expected[i] = referenceHash(buf)
			}

			got := Sum256x4(inputs)
			for i := range got {
				if !bytes.Equal(got[i][:], expected[i]) {
					t.Errorf("lane %d mismatch:\n  got  %x\n  want %x", i, got[i], expected[i])
				}
			}
		})
	}
}

// TestSum256x4_PartialBatch mirrors the BMT caller's usage when the last batch
// is partial: some lanes carry real data, the rest are nil. The wrapper
// produces a (meaningless) digest for the nil lanes and correct digests for
// the real lanes — only the latter are asserted.
func TestSum256x4_PartialBatch(t *testing.T) {
	if !HasSIMD() {
		t.Skip("AVX2 not available on this CPU")
	}

	for _, realLen := range []int{64, 96, 128} {
		for realLanes := 1; realLanes < 4; realLanes++ {
			t.Run(fmt.Sprintf("real_%d_of_4_len_%d", realLanes, realLen), func(t *testing.T) {
				var inputs [4][]byte
				var expected [4][]byte
				for i := range realLanes {
					buf := make([]byte, realLen)
					_, _ = rand.Read(buf)
					inputs[i] = buf
					expected[i] = referenceHash(buf)
				}
				// inputs[realLanes..3] remain nil

				got := Sum256x4(inputs)
				for i := range realLanes {
					if !bytes.Equal(got[i][:], expected[i]) {
						t.Errorf("real lane %d mismatch:\n  got  %x\n  want %x", i, got[i], expected[i])
					}
				}
			})
		}
	}
}

func TestSum256x8(t *testing.T) {
	if !HasAVX512() {
		t.Skip("AVX-512 not available on this CPU")
	}

	for _, length := range testInputLengths {
		t.Run(fmt.Sprintf("len_%d", length), func(t *testing.T) {
			var inputs [8][]byte
			var expected [8][]byte
			for i := range inputs {
				buf := make([]byte, length)
				_, _ = rand.Read(buf)
				inputs[i] = buf
				expected[i] = referenceHash(buf)
			}

			got := Sum256x8(inputs)
			for i := range got {
				if !bytes.Equal(got[i][:], expected[i]) {
					t.Errorf("lane %d mismatch:\n  got  %x\n  want %x", i, got[i], expected[i])
				}
			}
		})
	}
}

func TestSum256x8_PartialBatch(t *testing.T) {
	if !HasAVX512() {
		t.Skip("AVX-512 not available on this CPU")
	}

	for _, realLen := range []int{64, 96, 128} {
		for realLanes := 1; realLanes < 8; realLanes++ {
			t.Run(fmt.Sprintf("real_%d_of_8_len_%d", realLanes, realLen), func(t *testing.T) {
				var inputs [8][]byte
				var expected [8][]byte
				for i := range realLanes {
					buf := make([]byte, realLen)
					_, _ = rand.Read(buf)
					inputs[i] = buf
					expected[i] = referenceHash(buf)
				}

				got := Sum256x8(inputs)
				for i := range realLanes {
					if !bytes.Equal(got[i][:], expected[i]) {
						t.Errorf("real lane %d mismatch:\n  got  %x\n  want %x", i, got[i], expected[i])
					}
				}
			})
		}
	}
}
