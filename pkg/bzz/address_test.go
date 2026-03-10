// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"

	"github.com/multiformats/go-multiaddr"
)

func TestBzzAddress(t *testing.T) {
	t.Parallel()

	node1ma, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}

	nonce := common.HexToHash("0x2").Bytes()

	privateKey1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	overlay, err := crypto.NewOverlayAddress(privateKey1.PublicKey, 3, nonce)
	if err != nil {
		t.Fatal(err)
	}
	signer1 := crypto.NewDefaultSigner(privateKey1)

	bzzAddress, err := bzz.NewAddress(signer1, []multiaddr.Multiaddr{node1ma}, overlay, 3, nonce)
	if err != nil {
		t.Fatal(err)
	}

	bzzAddress2, err := bzz.ParseAddress(node1ma.Bytes(), overlay.Bytes(), bzzAddress.Signature, nonce, true, 3)
	if err != nil {
		t.Fatal(err)
	}

	if !bzzAddress.Equal(bzzAddress2) {
		t.Fatalf("got %s expected %s", bzzAddress2, bzzAddress)
	}

	bytes, err := bzzAddress.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	var newbzz bzz.Address
	if err := newbzz.UnmarshalJSON(bytes); err != nil {
		t.Fatal(err)
	}

	if !newbzz.Equal(bzzAddress) {
		t.Fatalf("got %s expected %s", newbzz, bzzAddress)
	}
}

func TestAreUnderlaysEqual(t *testing.T) {
	// --- Test Data Initialization ---
	addr1 := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/8001")
	addr2 := mustNewMultiaddr(t, "/ip4/192.168.1.1/tcp/8002")
	addr3 := mustNewMultiaddr(t, "/ip6/::1/udp/9000")
	addr4 := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/8001") // Identical to addr1

	// --- Test Cases Definition ---
	testCases := []struct {
		name string
		a    []multiaddr.Multiaddr
		b    []multiaddr.Multiaddr
		want bool
	}{
		{
			name: "two nil slices",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "one nil and one empty slice",
			a:    nil,
			b:    []multiaddr.Multiaddr{},
			want: true,
		},
		{
			name: "one empty and one nil slice",
			a:    []multiaddr.Multiaddr{},
			b:    nil,
			want: true,
		},
		{
			name: "two empty slices",
			a:    []multiaddr.Multiaddr{},
			b:    []multiaddr.Multiaddr{},
			want: true,
		},
		{
			name: "equal slices with same order",
			a:    []multiaddr.Multiaddr{addr1, addr2},
			b:    []multiaddr.Multiaddr{addr1, addr2},
			want: true,
		},
		{
			name: "equal slices with different order",
			a:    []multiaddr.Multiaddr{addr1, addr2, addr3},
			b:    []multiaddr.Multiaddr{addr3, addr1, addr2},
			want: true,
		},
		{
			name: "equal slices with identical (but not same instance) values",
			a:    []multiaddr.Multiaddr{addr1, addr2},
			b:    []multiaddr.Multiaddr{addr4, addr2},
			want: true,
		},
		{
			name: "slices with different lengths (a < b)",
			a:    []multiaddr.Multiaddr{addr1},
			b:    []multiaddr.Multiaddr{addr1, addr2},
			want: false,
		},
		{
			name: "slices with different lengths (b < a)",
			a:    []multiaddr.Multiaddr{addr1, addr2},
			b:    []multiaddr.Multiaddr{addr1},
			want: false,
		},
		{
			name: "slices with same length but different elements",
			a:    []multiaddr.Multiaddr{addr1, addr2},
			b:    []multiaddr.Multiaddr{addr1, addr3},
			want: false,
		},
		{
			name: "one slice is nil",
			a:    []multiaddr.Multiaddr{addr1},
			b:    nil,
			want: false,
		},
		{
			name: "slices with duplicates, equal",
			a:    []multiaddr.Multiaddr{addr1, addr2, addr1},
			b:    []multiaddr.Multiaddr{addr1, addr1, addr2},
			want: true,
		},
		{
			name: "slices with duplicates, not equal",
			a:    []multiaddr.Multiaddr{addr1, addr2, addr3},
			b:    []multiaddr.Multiaddr{addr1, addr1, addr2},
			want: false,
		},
		{
			name: "slices with different duplicates",
			a:    []multiaddr.Multiaddr{addr1, addr1, addr2},
			b:    []multiaddr.Multiaddr{addr1, addr2, addr2},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := bzz.AreUnderlaysEqual(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("AreUnderlaysEqual() = %v, want %v", got, tc.want)
			}
		})
	}
}
