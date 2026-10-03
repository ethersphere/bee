// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package keccak

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"testing"
)

// stubOverhead is the stack the stubs themselves use below the call-site SP
// before their frame starts: the return address and the saved frame pointer.
const stubOverhead = 16

// stackTestLengths extends testInputLengths with lengths that straddle block
// boundaries and multi-block inputs, to cover every absorb/squeeze path.
var stackTestLengths = append([]int{271, 273, 408, 4095, 4097, 4104, 8192}, testInputLengths...)

// TestStackUsage is a regression test for the XKCP C code overflowing the
// goroutine stack. The C code has no stack checks, so the assembly stubs must
// run it inside their reserved frame; if it uses more than that (or runs below
// the frame, as the original stubs did) it silently corrupts whatever memory
// lies below the goroutine stack, which surfaced as GC crashes in production.
//
// The probe paints the stack below the stub with a sentinel, calls the stub at
// every SP alignment the C code realigns for, and checks that nothing below the
// stub's frame was touched.
func TestStackUsage(t *testing.T) {
	t.Run("x4", func(t *testing.T) {
		if !HasSIMD() {
			t.Skip("AVX2 not available on this CPU")
		}
		testStackUsage(t, 4, keccak256x4FrameSize, 32, func(inputs [][]byte, off uintptr) (uintptr, [][32]byte) {
			var in [4][]byte
			var out [4]Hash256
			copy(in[:], inputs)
			used := stackProbe4(&in, &out, off)
			return used, hashesOf(out[:])
		})
	})
	t.Run("x8", func(t *testing.T) {
		if !HasAVX512() {
			t.Skip("AVX-512 not available on this CPU")
		}
		testStackUsage(t, 8, keccak256x8FrameSize, 64, func(inputs [][]byte, off uintptr) (uintptr, [][32]byte) {
			var in [8][]byte
			var out [8]Hash256
			copy(in[:], inputs)
			used := stackProbe8(&in, &out, off)
			return used, hashesOf(out[:])
		})
	})
}

func testStackUsage(t *testing.T, lanes, frameSize int, align uintptr, probe func([][]byte, uintptr) (uintptr, [][32]byte)) {
	t.Helper()

	limit := uintptr(stubOverhead + frameSize)
	var maxUsed uintptr
	for _, length := range stackTestLengths {
		data := make([]byte, length)
		if _, err := rand.Read(data); err != nil {
			t.Fatal(err)
		}
		want := referenceHash(data)

		// every lane filled, and every partial batch with the tail lanes nil
		for filled := lanes; filled >= 1; filled-- {
			inputs := make([][]byte, lanes)
			for i := range filled {
				inputs[i] = data
			}
			for off := uintptr(0); off < align; off += 8 {
				name := fmt.Sprintf("len=%d filled=%d off=%d", length, filled, off)

				used, got := probe(inputs, off)
				if used <= stubOverhead {
					t.Fatalf("%s: probe saw %d bytes of stack used; the probe is not measuring the C code", name, used)
				}
				if used > limit {
					t.Fatalf("%s: C code used %d bytes of stack, stub reserves %d: it overflows the stub frame and corrupts memory below the goroutine stack",
						name, used-stubOverhead, frameSize)
				}
				for i := range filled {
					if !bytes.Equal(got[i][:], want) {
						t.Fatalf("%s: lane %d digest mismatch", name, i)
					}
				}
				maxUsed = max(maxUsed, used)
			}
		}
	}
	t.Logf("max C stack usage %d of %d reserved bytes", maxUsed-stubOverhead, frameSize)
}

func hashesOf(out []Hash256) [][32]byte {
	hs := make([][32]byte, len(out))
	for i, h := range out {
		hs[i] = h
	}
	return hs
}

// TestStubFrameSizes checks that the literal frame sizes in the TEXT directives
// (which go vet requires to be literals) match the Go constants the stubs use to
// place SP at the top of the frame. A TEXT frame smaller than the constant would
// make the C code overwrite the stub's saved frame pointer and return address.
func TestStubFrameSizes(t *testing.T) {
	for _, tc := range []struct {
		file, fn string
		want     int
	}{
		{"keccak_times4_linux_amd64.s", "keccak256x4", keccak256x4FrameSize},
		{"keccak_times8_linux_amd64.s", "keccak256x8", keccak256x8FrameSize},
	} {
		src, err := os.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		m := regexp.MustCompile(`TEXT ·` + tc.fn + `\(SB\), \$(\d+)-16`).FindSubmatch(src)
		if m == nil {
			t.Fatalf("%s: TEXT directive for %s not found", tc.file, tc.fn)
		}
		got, err := strconv.Atoi(string(m[1]))
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s: TEXT frame size %d does not match Go constant %d", tc.file, got, tc.want)
		}
	}
}
