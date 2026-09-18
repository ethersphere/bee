// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter

import (
	"bytes"
	"testing"
)

// fuzzStruct is a plain struct with exported fields and no custom
// (Un)Marshal methods, so proxyItem.Unmarshal falls through to json.Unmarshal.
type fuzzStruct struct {
	Name  string
	Value int
	Data  []byte
}

func FuzzProxyItemUnmarshal(f *testing.F) {
	// Valid seed: marshal a real proxyItem (yields json bytes).
	seedItem := newProxyItem("k", &fuzzStruct{Name: "hello", Value: 42, Data: []byte("bytes")})
	if b, err := seedItem.Marshal(); err == nil {
		f.Add(b)
	}

	f.Add([]byte(nil))
	f.Add([]byte(""))
	f.Add([]byte("{"))
	f.Add([]byte(`{"Name":"x"`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Fresh proxyItem with a fresh obj each iteration.
		pi := newProxyItem("k", &fuzzStruct{})
		// NO-PANIC is the invariant. json.Unmarshal may return an error on
		// malformed input; that is fine, do not assert error == nil.
		_ = pi.Unmarshal(data)
	})
}

func FuzzRawItemUnmarshal(f *testing.F) {
	// Valid seed: marshal a real rawItem backed by a []byte (returns bytes verbatim).
	seedItem := &rawItem{newProxyItem("", []byte("seedbytes"))}
	if b, err := seedItem.Marshal(); err == nil {
		f.Add(b)
	}

	f.Add([]byte(nil))
	f.Add([]byte(""))
	f.Add([]byte("short"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Fresh rawItem mirroring Iterate's Factory.
		ri := &rawItem{newProxyItem("", []byte(nil))}
		if err := ri.Unmarshal(data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Round-trip holds for the []byte path on non-empty input.
		// len(data)==0 early-returns leaving obj unchanged, so skip it.
		if len(data) > 0 {
			got, err := ri.Marshal()
			if err != nil {
				t.Fatalf("marshal after unmarshal: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("round-trip mismatch: got %x want %x", got, data)
			}
		}
	})
}
