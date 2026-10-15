// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz_test

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/multiformats/go-multiaddr"
)

// FuzzParseAddress fuzzes the signed-address wire decoder used to ingest peers
// from hive gossip and the handshake. Every field crosses the trust boundary,
// so the target asserts the parser never panics and that any address it accepts
// satisfies the invariants the rest of the node relies on (correct nonce and
// chequebook lengths, positive timestamp, and an overlay that matches the bytes
// on the wire — i.e. a peer cannot get an address accepted for an overlay it did
// not sign).
func FuzzParseAddress(f *testing.F) {
	const networkID = uint64(3)

	priv, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		f.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(priv)

	nonce := common.HexToHash("0x2").Bytes()
	overlay, err := crypto.NewOverlayAddress(priv.PublicKey, networkID, nonce)
	if err != nil {
		f.Fatal(err)
	}

	ma, err := multiaddr.NewMultiaddr("/ip4/1.2.3.4/tcp/1634")
	if err != nil {
		f.Fatal(err)
	}
	underlay, err := bzz.SerializeUnderlays([]multiaddr.Multiaddr{ma})
	if err != nil {
		f.Fatal(err)
	}

	chequebook := common.HexToAddress("0xabc0000000000000000000000000000000000123")
	addr, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{ma}, overlay, networkID, nonce, 1, chequebook)
	if err != nil {
		f.Fatal(err)
	}

	// a fully valid record, so the fuzzer has a signed corpus entry to mutate
	f.Add(underlay, overlay.Bytes(), addr.Signature, nonce, int64(1), chequebook.Bytes())
	// empty everything
	f.Add([]byte{}, []byte{}, []byte{}, []byte{}, int64(0), []byte{})

	f.Fuzz(func(t *testing.T, underlay, overlay, signature, nonce []byte, timestamp int64, chequebook []byte) {
		got, err := bzz.ParseAddress(underlay, overlay, signature, nonce, timestamp, networkID, chequebook)
		if err != nil {
			return
		}

		if len(nonce) != bzz.NonceLength {
			t.Fatalf("accepted nonce of length %d, want %d", len(nonce), bzz.NonceLength)
		}
		if timestamp <= 0 {
			t.Fatalf("accepted non-positive timestamp %d", timestamp)
		}
		if l := len(chequebook); l != 0 && l != common.AddressLength {
			t.Fatalf("accepted chequebook of length %d", l)
		}
		if !bytes.Equal(got.Overlay.Bytes(), overlay) {
			t.Fatal("returned overlay does not match the overlay on the wire")
		}
		if len(got.Underlays) == 0 {
			t.Fatal("accepted address with no underlays")
		}
		for i, u := range got.Underlays {
			if u == nil {
				t.Fatalf("nil underlay at index %d", i)
			}
		}
	})
}
