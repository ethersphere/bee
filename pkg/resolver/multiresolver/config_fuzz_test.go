// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multiresolver_test

import (
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/resolver/multiresolver"
)

// FuzzParseConnectionStrings fuzzes the resolver connection-string parser,
// which runs on operator-supplied "tld:address@endpoint" configuration
// strings. Parsing uses strings.Index results to byte-slice the input
// (endpoint[:i], endpoint[i+1:i+3], endpoint[i+1:]) and feeds a segment to
// common.HexToAddress; the target asserts the parser never panics on
// arbitrary input (out-of-range byte slice, negative index, nil deref) and
// that the only error it may return is ErrTLDTooLong.
func FuzzParseConnectionStrings(f *testing.F) {
	// Valid canonical forms that round-trip through the parser.
	f.Add("eth:0x314159265dD8dbb310642f98f50C066173C1259b@https://foo.example")
	f.Add("eth:https://cloudflare-eth.com")
	f.Add("0xabc@http://localhost:8545")

	// Benign edge inputs to keep the seed corpus green.
	f.Add("")
	f.Add(":")
	f.Add("@")
	f.Add("://")
	f.Add("http://x")
	f.Add("a:b")
	f.Add("ü:x")

	f.Fuzz(func(t *testing.T, cs string) {
		cfgs, err := multiresolver.ParseConnectionStrings([]string{cs})
		if err != nil {
			// The parser only ever fails on an over-long TLD.
			if !errors.Is(err, multiresolver.ErrTLDTooLong) {
				t.Fatalf("unexpected error for %q: %v", cs, err)
			}
			return
		}
		// On success the output length must match the input length.
		if len(cfgs) != 1 {
			t.Fatalf("expected 1 config for %q, got %d", cs, len(cfgs))
		}
	})
}
