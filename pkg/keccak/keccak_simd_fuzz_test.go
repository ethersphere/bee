// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package keccak

import (
	"bytes"
	"testing"
)

// fuzzMaxLen caps lane lengths so each iteration stays fast while still
// covering many multi-block inputs (136-byte rate).
const fuzzMaxLen = 2048

func FuzzSum256x4(f *testing.F) {
	addSIMDSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte, lenA, lenB uint16, nilMask, altMask uint8) {
		if !HasSIMD() {
			t.Skip("AVX2 not available on this CPU")
		}
		lanes := fuzzLanes(4, data, lenA, lenB, nilMask, altMask)
		var in [4][]byte
		copy(in[:], lanes)
		checkSIMD(t, lanes, keccak256x4FrameSize,
			func() []Hash256 { out := Sum256x4(in); return out[:] },
			func(pattern uintptr) uintptr {
				probeIn := in
				var out [4]Hash256
				prepareLanes(probeIn[:])
				return stackProbe4(&probeIn, &out, 0, pattern)
			})
	})
}

func FuzzSum256x8(f *testing.F) {
	addSIMDSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte, lenA, lenB uint16, nilMask, altMask uint8) {
		if !HasAVX512() {
			t.Skip("AVX-512 not available on this CPU")
		}
		lanes := fuzzLanes(8, data, lenA, lenB, nilMask, altMask)
		var in [8][]byte
		copy(in[:], lanes)
		checkSIMD(t, lanes, keccak256x8FrameSize,
			func() []Hash256 { out := Sum256x8(in); return out[:] },
			func(pattern uintptr) uintptr {
				probeIn := in
				var out [8]Hash256
				prepareLanes(probeIn[:])
				return stackProbe8(&probeIn, &out, 0, pattern)
			})
	})
}

func addSIMDSeeds(f *testing.F) {
	f.Helper()
	seed := []byte("swarm keccak simd fuzz seed")
	// all lanes equal, BMT-sized inputs
	f.Add(seed, uint16(64), uint16(64), uint8(0), uint8(0))
	f.Add(seed, uint16(96), uint16(96), uint8(0), uint8(0))
	// partial batches below and above the 136-byte rate (the latter is the
	// negative-index case: nil lanes next to multi-block lanes)
	f.Add(seed, uint16(64), uint16(64), uint8(0xF0), uint8(0))
	f.Add(seed, uint16(4095%fuzzMaxLen), uint16(0), uint8(0x08), uint8(0))
	f.Add(seed, uint16(136), uint16(136), uint8(0x0A), uint8(0))
	// mixed lengths, rejected before reaching C
	f.Add(seed, uint16(100), uint16(200), uint8(0), uint8(0x02))
	f.Add(seed, uint16(140), uint16(200), uint8(0x04), uint8(0x01))
	// all lanes empty
	f.Add([]byte{}, uint16(0), uint16(0), uint8(0xFF), uint8(0))
}

// fuzzLanes builds n lanes: lanes whose nilMask bit is set are nil, lanes whose
// altMask bit is set get length lenB, all others length lenA. Lane contents are
// derived from data and differ per lane, so a digest landing in the wrong lane
// is detected.
func fuzzLanes(n int, data []byte, lenA, lenB uint16, nilMask, altMask uint8) [][]byte {
	lanes := make([][]byte, n)
	for i := range lanes {
		if nilMask&(1<<i) != 0 {
			continue
		}
		length := int(lenA) % fuzzMaxLen
		if altMask&(1<<i) != 0 {
			length = int(lenB) % fuzzMaxLen
		}
		lane := make([]byte, length)
		for j := range lane {
			b := byte(j)
			if len(data) > 0 {
				b = data[(j+i*7)%len(data)]
			}
			lane[j] = b ^ byte(i)
		}
		lanes[i] = lane
	}
	return lanes
}

// checkSIMD asserts the Sum256xN contract for one set of lanes: mixed non-empty
// lengths panic; otherwise every non-empty lane gets the reference digest and
// the C code stays inside the stub's frame for both probe patterns.
func checkSIMD(t *testing.T, lanes [][]byte, frameSize int, sum func() []Hash256, probe func(pattern uintptr) uintptr) {
	t.Helper()

	length := -1
	for _, lane := range lanes {
		if len(lane) == 0 {
			continue
		}
		if length >= 0 && len(lane) != length {
			assertPanics(t, func() { sum() })
			return
		}
		length = len(lane)
	}

	got := sum()
	for i, lane := range lanes {
		// empty lanes are fillers whose digest is meaningless
		if len(lane) == 0 {
			continue
		}
		if want := referenceHash(lane); !bytes.Equal(got[i][:], want) {
			t.Fatalf("lane %d (len %d) digest mismatch:\n  got  %x\n  want %x", i, len(lane), got[i], want)
		}
	}

	limit := uintptr(stubOverhead + frameSize)
	for _, pattern := range probePatterns {
		if used := probe(pattern); used > limit {
			t.Fatalf("pattern %#x: C code used %d bytes of stack, stub reserves %d", pattern, used-stubOverhead, frameSize)
		}
	}
}
