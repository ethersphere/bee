// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm_test

import (
	"crypto/rand"
	"testing"

	"github.com/ethersphere/bee/pkg/swarm"
)

func Test_AddressSliceContains(t *testing.T) {
	t.Parallel()

	addrs := makeAddreses(t, 10)
	tt := []struct {
		addresses []swarm.Address
		search    swarm.Address
		contains  bool
	}{
		{addresses: nil, search: swarm.Address{}},
		{addresses: nil, search: makeAddress(t)},
		{addresses: make([]swarm.Address, 10), search: swarm.Address{}, contains: true},
		{addresses: makeAddreses(t, 0), search: makeAddress(t)},
		{addresses: makeAddreses(t, 10), search: makeAddress(t)},
		{addresses: addrs, search: addrs[0], contains: true},
		{addresses: addrs, search: addrs[1], contains: true},
		{addresses: addrs, search: addrs[3], contains: true},
		{addresses: addrs, search: addrs[9], contains: true},
	}

	for _, tc := range tt {
		contains := swarm.AddressSliceContains(tc.addresses, tc.search)
		if contains != tc.contains {
			t.Fatalf("got %v, want %v", contains, tc.contains)
		}
	}
}

func Test_FindAddressIdx(t *testing.T) {
	t.Parallel()

	addrs := makeAddreses(t, 10)
	tt := []struct {
		addresses []swarm.Address
		search    swarm.Address
		result    int
	}{
		{addresses: nil, search: swarm.Address{}, result: -1},
		{addresses: nil, search: makeAddress(t), result: -1},
		{addresses: makeAddreses(t, 0), search: makeAddress(t), result: -1},
		{addresses: makeAddreses(t, 10), search: makeAddress(t), result: -1},
		{addresses: addrs, search: addrs[0], result: 0},
		{addresses: addrs, search: addrs[1], result: 1},
		{addresses: addrs, search: addrs[3], result: 3},
		{addresses: addrs, search: addrs[9], result: 9},
	}

	for _, tc := range tt {
		result := swarm.FindAddressIdx(tc.addresses, tc.search)
		if result != tc.result {
			t.Fatalf("got %v, want %v", result, tc.result)
		}
	}
}

func Test_AddressSliceRemove(t *testing.T) {
	t.Parallel()

	addrs := makeAddreses(t, 10)
	tt := []struct {
		addresses []swarm.Address
		remove    swarm.Address
	}{
		{addresses: nil, remove: swarm.Address{}},
		{addresses: nil, remove: makeAddress(t)},
		{addresses: makeAddreses(t, 0), remove: makeAddress(t)},
		{addresses: makeAddreses(t, 10), remove: makeAddress(t)},
		{addresses: addrs, remove: addrs[0]},
		{addresses: addrs, remove: addrs[1]},
		{addresses: addrs, remove: addrs[3]},
		{addresses: addrs, remove: addrs[9]},
		{addresses: addrs, remove: addrs[9]},
	}

	for i, tc := range tt {
		contains := swarm.AddressSliceContains(tc.addresses, tc.remove)
		containsAfterRemove := swarm.AddressSliceContains(
			swarm.AddressSliceRemove(cloneAddresses(tc.addresses), tc.remove),
			tc.remove,
		)

		if contains && containsAfterRemove {
			t.Fatalf("%d %d  address should be removed", len(tc.addresses), i)
		}
	}
}

func Test_FindChunkIdxWithAddress(t *testing.T) {
	t.Parallel()

	chunks := []swarm.Chunk{
		swarm.NewChunk(makeAddress(t), nil),
		swarm.NewChunk(makeAddress(t), nil),
		swarm.NewChunk(makeAddress(t), nil),
	}
	tt := []struct {
		chunks  []swarm.Chunk
		address swarm.Address
		result  int
	}{
		{chunks: nil, address: swarm.Address{}, result: -1},
		{chunks: nil, address: makeAddress(t), result: -1},
		{chunks: make([]swarm.Chunk, 0), address: makeAddress(t), result: -1},
		{chunks: make([]swarm.Chunk, 10), address: makeAddress(t), result: -1},
		{chunks: make([]swarm.Chunk, 10), address: swarm.Address{}, result: -1},
		{chunks: chunks, address: makeAddress(t), result: -1},
		{chunks: chunks, address: chunks[0].Address(), result: 0},
		{chunks: chunks, address: chunks[1].Address(), result: 1},
		{chunks: chunks, address: chunks[2].Address(), result: 2},
	}

	for _, tc := range tt {
		result := swarm.FindChunkIdxWithAddress(tc.chunks, tc.address)
		if result != tc.result {
			t.Fatalf("got %v, want %v", result, tc.result)
		}
	}
}

func cloneAddresses(addrs []swarm.Address) []swarm.Address {
	result := make([]swarm.Address, len(addrs))
	for i := 0; i < len(addrs); i++ {
		result[i] = addrs[i].Clone()
	}
	return result
}

func makeAddreses(t *testing.T, count int) []swarm.Address {
	t.Helper()

	result := make([]swarm.Address, count)
	for i := 0; i < count; i++ {
		result[i] = makeAddress(t)
	}
	return result
}

func makeAddress(t *testing.T) swarm.Address {
	t.Helper()

	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatal(err.Error())
	}

	return swarm.NewAddress(buf)
}
