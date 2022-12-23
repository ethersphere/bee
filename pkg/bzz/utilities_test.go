// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz_test

import (
	"crypto/rand"
	"testing"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/swarm"
)

func Test_AddressSliceContains(t *testing.T) {
	t.Parallel()

	addrs := makeAddreses(t, 10)
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
	}

	for _, tc := range tt {
		contains := bzz.AddressSliceContains(tc.addresses, &tc.search)
		if contains != tc.contains {
			t.Fatalf("got %v, want %v", contains, tc.contains)
		}
	}
}
func makeAddreses(t *testing.T, count int) []bzz.Address {
	t.Helper()

	result := make([]bzz.Address, count)
	for i := 0; i < count; i++ {
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
		Underlay:        multiaddr,
		Overlay:         swarm.NewAddress(randomBytes(t, 12)),
		Signature:       randomBytes(t, 12),
		Transaction:     randomBytes(t, 12),
		EthereumAddress: randomBytes(t, 32),
	}
}

func randomBytes(t *testing.T, size int) []byte {
	t.Helper()

	buf := make([]byte, size)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatal(err.Error())
	}
	return buf
}
