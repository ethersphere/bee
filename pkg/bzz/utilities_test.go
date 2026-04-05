// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz_test

import (
	"testing"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

func Test_ContainsAddress(t *testing.T) {
	t.Parallel()

	addrs := makeAddreses(t, 10)
	multiAddr := makeMultiUnderlayAddress(t)

	tt := []struct {
		addresses []bzz.Address
		search    bzz.Address
		contains  bool
	}{
		{addresses: nil, search: bzz.Address{}},
		{addresses: nil, search: makeAddress(t)},
		{addresses: make([]bzz.Address, 10), search: bzz.Address{}, contains: true},
		{addresses: makeAddreses(t, 0), search: makeAddress(t)},
		{addresses: makeAddreses(t, 10), search: makeAddress(t)},
		{addresses: addrs, search: addrs[0], contains: true},
		{addresses: addrs, search: addrs[1], contains: true},
		{addresses: addrs, search: addrs[3], contains: true},
		{addresses: addrs, search: addrs[9], contains: true},
		{addresses: addrs, search: multiAddr, contains: false},
		{addresses: append(addrs, multiAddr), search: multiAddr, contains: true},
	}

	for _, tc := range tt {
		contains := bzz.ContainsAddress(tc.addresses, &tc.search)
		if contains != tc.contains {
			t.Fatalf("got %v, want %v", contains, tc.contains)
		}
	}
}
func makeAddreses(t *testing.T, count int) []bzz.Address {
	t.Helper()

	result := make([]bzz.Address, count)
	for i := range count {
		result[i] = makeAddress(t)
	}
	return result
}

func makeAddress(t *testing.T) bzz.Address {
	t.Helper()

	multiaddr, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}

	return bzz.Address{
		Underlays:       []ma.Multiaddr{multiaddr},
		Overlay:         swarm.RandAddress(t),
		Signature:       testutil.RandBytes(t, 12),
		Nonce:           testutil.RandBytes(t, 12),
		EthereumAddress: testutil.RandBytes(t, 32),
	}
}

func makeMultiUnderlayAddress(t *testing.T) bzz.Address {
	t.Helper()

	addrsStrings := []string{
		"/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA",
		"/ip4/10.34.35.60/tcp/35315/p2p/12D3KooWNZujn3N8EcGEFRJrQHw9Hb9v39bNyHC2uw2YNv9jBtjk",
		"/ip6/::1/tcp/46881/p2p/12D3KooWNZujn3N8EcGEFRJrQHw9Hb9v39bNyHC2uw2YNv9jBtjk",
	}

	underlays := make([]ma.Multiaddr, len(addrsStrings))
	for i, addr := range addrsStrings {
		multiaddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			t.Fatal(err)
		}
		underlays[i] = multiaddr
	}

	return bzz.Address{
		Underlays:       underlays,
		Overlay:         swarm.RandAddress(t),
		Signature:       testutil.RandBytes(t, 12),
		Nonce:           testutil.RandBytes(t, 12),
		EthereumAddress: testutil.RandBytes(t, 32),
	}
}
