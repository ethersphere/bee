// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/multiformats/go-multiaddr"
)

// fuzzMaxUnderlays mirrors the unexported maxUnderlaysPerPeer limit in the bzz
// package. If that constant changes, this must be updated to match.
const fuzzMaxUnderlays = 20

// FuzzDeserializeUnderlays feeds arbitrary bytes to the underlay wire decoder,
// which runs on peer-supplied data in the hive gossip and handshake paths. It
// asserts the decoder never panics, honours its documented bounds, and that any
// set it accepts survives a serialize/deserialize round-trip unchanged.
func FuzzDeserializeUnderlays(f *testing.F) {
	single := []string{
		"/ip4/127.0.0.1/tcp/1634",
		"/ip4/1.2.3.4/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA",
	}
	mas := make([]multiaddr.Multiaddr, 0, len(single))
	for _, s := range single {
		ma, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			f.Fatal(err)
		}
		mas = append(mas, ma)
		b, err := bzz.SerializeUnderlays([]multiaddr.Multiaddr{ma})
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}

	// list format (2+ addresses uses the custom prefixed encoding)
	list, err := bzz.SerializeUnderlays(mas)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(list)

	// hand-crafted edge cases
	f.Add([]byte{})                 // empty
	f.Add([]byte{0x99})             // prefix only, empty list
	f.Add([]byte{0x99, 0xff, 0x7f}) // prefix + oversized varint length
	f.Add([]byte{0x99, 0x05, 0x00}) // prefix + length exceeding remaining bytes
	f.Add([]byte{0x00})             // invalid single multiaddr

	f.Fuzz(func(t *testing.T, data []byte) {
		addrs, err := bzz.DeserializeUnderlays(data)
		if err != nil {
			if addrs != nil {
				t.Fatalf("expected nil addrs on error, got %d entries", len(addrs))
			}
			return
		}

		// Zero underlays is a valid decode (the empty-list encoding); callers
		// guard len==0 downstream, so only the upper bound is asserted here.
		if len(addrs) > fuzzMaxUnderlays {
			t.Fatalf("returned %d underlays, exceeds documented max %d", len(addrs), fuzzMaxUnderlays)
		}
		for i, a := range addrs {
			if a == nil {
				t.Fatalf("nil multiaddr at index %d", i)
			}
			_ = a.String()
			_ = a.Bytes()
		}

		// Round-trip stability: an accepted set must re-encode and decode to the
		// same set. A mismatch would indicate a lossy or ambiguous encoding.
		ser, err := bzz.SerializeUnderlays(addrs)
		if err != nil {
			t.Fatalf("re-serialize accepted underlays: %v", err)
		}
		got, err := bzz.DeserializeUnderlays(ser)
		if err != nil {
			t.Fatalf("re-deserialize accepted underlays: %v", err)
		}
		if !bzz.AreUnderlaysEqual(addrs, got) {
			t.Fatal("underlay round-trip produced a different set")
		}
	})
}
