// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	"math/big"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/postage"
)

// FuzzStampIssuerUnmarshalBinary fuzzes (*postage.StampIssuer).UnmarshalBinary,
// the msgpack decoder for persisted StampIssuer state on disk. msgpack is
// self-describing/self-bounding, so this is defensive coverage: the decoder
// must never panic on corrupt or truncated bytes, and on a successful decode
// re-marshaling must not error. A byte-for-byte round-trip is intentionally
// not asserted because msgpack field ordering and nil-vs-empty slice
// normalization make exact equality unreliable.
func FuzzStampIssuerUnmarshalBinary(f *testing.F) {
	issuer := postage.NewStampIssuer("label", "keyid", make([]byte, 32), big.NewInt(3), 24, 16, 7, true)
	valid, err := issuer.MarshalBinary()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte{})
	f.Add([]byte{0xff})
	f.Add(make([]byte, 32))

	f.Fuzz(func(t *testing.T, data []byte) {
		si := new(postage.StampIssuer)
		if err := si.UnmarshalBinary(data); err != nil { // must not panic on any input
			return
		}
		if _, err := si.MarshalBinary(); err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
	})
}

// FuzzStampIssuerItemUnmarshal fuzzes (*postage.StampIssuerItem).Unmarshal, the
// persisted storage.Item wrapper that delegates to StampIssuer.UnmarshalBinary
// (the same msgpack path). Same invariants: never panic on arbitrary bytes, and
// on success the decoded issuer is non-nil and re-marshals cleanly.
func FuzzStampIssuerItemUnmarshal(f *testing.F) {
	issuer := postage.NewStampIssuer("label", "keyid", make([]byte, 32), big.NewInt(3), 24, 16, 7, true)
	item := &postage.StampIssuerItem{Issuer: issuer}
	valid, err := item.Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte{})
	f.Add([]byte{0xff})
	f.Add(make([]byte, 32))

	f.Fuzz(func(t *testing.T, data []byte) {
		it := new(postage.StampIssuerItem)
		if err := it.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if it.Issuer == nil {
			t.Fatal("decoded StampIssuerItem has nil Issuer")
		}
		if _, err := it.Marshal(); err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
	})
}
