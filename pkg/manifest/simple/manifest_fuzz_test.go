// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package simple_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/manifest/simple"
)

// FuzzManifestUnmarshalBinary drives the persisted simple-manifest decoder
// (simple.Manifest.UnmarshalBinary) on arbitrary bytes, asserting it never
// panics and that a successful decode round-trips through MarshalBinary to a
// stable canonical form.
func FuzzManifestUnmarshalBinary(f *testing.F) {
	// Valid seeds built with the real API.
	type seedEntry struct {
		path      string
		reference string
		metadata  map[string]string
	}
	seeds := [][]seedEntry{
		nil, // empty manifest
		{
			{path: "entry-1", reference: "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"},
		},
		{
			{path: "text/robots.txt", reference: "aa"},
			{path: "img/1.png", reference: "bb"},
			{path: "readme.md", reference: "cc"},
			{
				path:     "/",
				metadata: map[string]string{"index-document": "readme.md", "error-document": "404.html"},
			},
		},
	}

	for _, entries := range seeds {
		m := simple.NewManifest()
		for _, en := range entries {
			if err := m.Add(en.path, en.reference, en.metadata); err != nil {
				f.Fatal(err)
			}
		}
		b, err := m.MarshalBinary()
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}

	// Raw JSON literals to prime edge coverage.
	f.Add([]byte("{}"))
	f.Add([]byte(`{"entries":{}}`))
	f.Add([]byte(`{"entries":{"a":{"reference":"x"}}}`))
	f.Add([]byte(`{"entries":{"a":{"reference":"x","metadata":{"k":"v"}}}}`))
	f.Add([]byte("{"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, b []byte) {
		m := simple.NewManifest()
		if err := m.UnmarshalBinary(b); err != nil {
			return
		}

		// Invariant: a successful decode must re-marshal without error and
		// yield a non-nil result.
		b1, err := m.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary after successful UnmarshalBinary: %v", err)
		}
		if b1 == nil {
			t.Fatal("MarshalBinary returned nil bytes after successful UnmarshalBinary")
		}

		// Invariant: canonical round-trip stability. JSON map marshaling
		// sorts keys, so re-decoding then re-marshaling yields identical bytes.
		m2 := simple.NewManifest()
		if err := m2.UnmarshalBinary(b1); err != nil {
			t.Fatalf("UnmarshalBinary of canonical bytes: %v", err)
		}
		b2, err := m2.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary of re-decoded manifest: %v", err)
		}
		if !bytes.Equal(b1, b2) {
			t.Fatalf("canonical round-trip mismatch:\n b1=%q\n b2=%q", b1, b2)
		}
	})
}
