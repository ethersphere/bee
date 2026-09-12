// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func FuzzParseHexAddress(f *testing.F) {
	// Valid 64-char hex seed produced the production way: round-trip a real address.
	f.Add(swarm.MustParseHexAddress("aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899").String())
	f.Add(swarm.RandAddress(f).String())
	// Benign edges.
	f.Add("")
	f.Add("abc")                      // odd-length hex
	f.Add("zz")                       // non-hex chars
	f.Add("0xdead")                   // 0x prefix (non-hex 'x')
	f.Add("a")                        // single char
	f.Add("AABBCC")                   // uppercase hex
	f.Add(strings.Repeat("ab", 4096)) // very long hex

	f.Fuzz(func(t *testing.T, s string) {
		a, err := swarm.ParseHexAddress(s)
		if err != nil {
			return
		}
		// Invariant: bytes equal the hex.DecodeString output.
		want, derr := hex.DecodeString(s)
		if derr != nil {
			t.Fatalf("ParseHexAddress accepted %q that hex.DecodeString rejected: %v", s, derr)
		}
		if string(a.Bytes()) != string(want) {
			t.Fatalf("bytes mismatch for %q", s)
		}
		// Round-trip: re-parsing String() yields an Equal address.
		b, err := swarm.ParseHexAddress(a.String())
		if err != nil {
			t.Fatalf("re-parse of %q failed: %v", a.String(), err)
		}
		if !b.Equal(a) {
			t.Fatalf("round-trip mismatch for %q", s)
		}
	})
}

func FuzzParseBitStrAddress(f *testing.F) {
	// Valid binary-string seeds.
	f.Add("")
	f.Add("0")
	f.Add("1")
	f.Add(strings.Repeat("1", 256))  // exact 32-byte width
	f.Add(strings.Repeat("0", 256))  // exact 32-byte width
	f.Add(strings.Repeat("10", 128)) // exact 32-byte width
	f.Add(strings.Repeat("1", 300))  // over-long, exercises copy truncation
	// Benign bad-char inputs (must error cleanly, not panic).
	f.Add("2")
	f.Add("1a1")
	f.Add("111x")
	f.Add("1é") // multibyte rune

	f.Fuzz(func(t *testing.T, s string) {
		a, err := swarm.ParseBitStrAddress(s)
		if err != nil {
			return
		}
		// bytesToAddr always yields exactly HashSize bytes.
		if !a.IsValidLength() {
			t.Fatalf("ParseBitStrAddress(%q) produced invalid-length address", s)
		}
	})
}
