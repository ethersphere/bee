// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func Test_ContainsAddress(t *testing.T) {
	t.Parallel()

	addrs := swarm.RandAddresses(t, 10)
	tt := []struct {
		addresses []swarm.Address
		search    swarm.Address
		contains  bool
	}{
		{addresses: nil, search: swarm.Address{}},
		{addresses: nil, search: swarm.RandAddress(t)},
		{addresses: make([]swarm.Address, 10), search: swarm.Address{}, contains: true},
		{addresses: swarm.RandAddresses(t, 0), search: swarm.RandAddress(t)},
		{addresses: swarm.RandAddresses(t, 10), search: swarm.RandAddress(t)},
		{addresses: addrs, search: addrs[0], contains: true},
		{addresses: addrs, search: addrs[1], contains: true},
		{addresses: addrs, search: addrs[3], contains: true},
		{addresses: addrs, search: addrs[9], contains: true},
	}

	for _, tc := range tt {
		contains := swarm.ContainsAddress(tc.addresses, tc.search)
		if contains != tc.contains {
			t.Fatalf("got %v, want %v", contains, tc.contains)
		}
	}
}

func Test_IndexOfAddress(t *testing.T) {
	t.Parallel()

	addrs := swarm.RandAddresses(t, 10)
	tt := []struct {
		addresses []swarm.Address
		search    swarm.Address
		result    int
	}{
		{addresses: nil, search: swarm.Address{}, result: -1},
		{addresses: nil, search: swarm.RandAddress(t), result: -1},
		{addresses: swarm.RandAddresses(t, 0), search: swarm.RandAddress(t), result: -1},
		{addresses: swarm.RandAddresses(t, 10), search: swarm.RandAddress(t), result: -1},
		{addresses: addrs, search: addrs[0], result: 0},
		{addresses: addrs, search: addrs[1], result: 1},
		{addresses: addrs, search: addrs[3], result: 3},
		{addresses: addrs, search: addrs[9], result: 9},
	}

	for _, tc := range tt {
		result := swarm.IndexOfAddress(tc.addresses, tc.search)
		if result != tc.result {
			t.Fatalf("got %v, want %v", result, tc.result)
		}
	}
}

func Test_RemoveAddress(t *testing.T) {
	t.Parallel()

	addrs := swarm.RandAddresses(t, 10)
	tt := []struct {
		addresses []swarm.Address
		remove    swarm.Address
	}{
		{addresses: nil, remove: swarm.Address{}},
		{addresses: nil, remove: swarm.RandAddress(t)},
		{addresses: swarm.RandAddresses(t, 0), remove: swarm.RandAddress(t)},
		{addresses: swarm.RandAddresses(t, 10), remove: swarm.RandAddress(t)},
		{addresses: addrs, remove: addrs[0]},
		{addresses: addrs, remove: addrs[1]},
		{addresses: addrs, remove: addrs[3]},
		{addresses: addrs, remove: addrs[9]},
		{addresses: addrs, remove: addrs[9]},
	}

	for i, tc := range tt {
		contains := swarm.ContainsAddress(tc.addresses, tc.remove)
		containsAfterRemove := swarm.ContainsAddress(
			swarm.RemoveAddress(cloneAddresses(tc.addresses), tc.remove),
			tc.remove,
		)

		if contains && containsAfterRemove {
			t.Fatalf("%d %d  address should be removed", len(tc.addresses), i)
		}
	}
}

func Test_IndexOfChunkWithAddress(t *testing.T) {
	t.Parallel()

	chunks := []swarm.Chunk{
		swarm.NewChunk(swarm.RandAddress(t), nil),
		swarm.NewChunk(swarm.RandAddress(t), nil),
		swarm.NewChunk(swarm.RandAddress(t), nil),
	}
	tt := []struct {
		chunks  []swarm.Chunk
		address swarm.Address
		result  int
	}{
		{chunks: nil, address: swarm.Address{}, result: -1},
		{chunks: nil, address: swarm.RandAddress(t), result: -1},
		{chunks: make([]swarm.Chunk, 0), address: swarm.RandAddress(t), result: -1},
		{chunks: make([]swarm.Chunk, 10), address: swarm.RandAddress(t), result: -1},
		{chunks: make([]swarm.Chunk, 10), address: swarm.Address{}, result: -1},
		{chunks: chunks, address: swarm.RandAddress(t), result: -1},
		{chunks: chunks, address: chunks[0].Address(), result: 0},
		{chunks: chunks, address: chunks[1].Address(), result: 1},
		{chunks: chunks, address: chunks[2].Address(), result: 2},
	}

	for _, tc := range tt {
		result := swarm.IndexOfChunkWithAddress(tc.chunks, tc.address)
		if result != tc.result {
			t.Fatalf("got %v, want %v", result, tc.result)
		}
	}
}

func Test_ContainsChunkWithData(t *testing.T) {
	t.Parallel()

	chunks := []swarm.Chunk{
		swarm.NewChunk(swarm.RandAddress(t), nil),
		swarm.NewChunk(swarm.RandAddress(t), []byte{1, 1, 1}),
		swarm.NewChunk(swarm.RandAddress(t), []byte{2, 2, 2}),
	}
	tt := []struct {
		chunks   []swarm.Chunk
		data     []byte
		contains bool
	}{
		// contains
		{chunks: chunks, data: nil, contains: true},
		{chunks: chunks, data: []byte{1, 1, 1}, contains: true},
		{chunks: chunks, data: []byte{2, 2, 2}, contains: true},

		// do not contain
		{chunks: nil, data: nil},
		{chunks: chunks, data: []byte{3, 3, 3}},
		{chunks: chunks, data: []byte{1}},
		{chunks: chunks, data: []byte{2}},
		{chunks: make([]swarm.Chunk, 0), data: []byte{1, 1, 1}},
		{chunks: make([]swarm.Chunk, 10), data: nil},
	}

	for _, tc := range tt {
		contains := swarm.ContainsChunkWithData(tc.chunks, tc.data)
		if contains != tc.contains {
			t.Fatalf("got %v, want %v", contains, tc.contains)
		}
	}
}

func Test_FindStampWithBatchID(t *testing.T) {
	t.Parallel()

	stamps := []swarm.Stamp{
		makeStamp(t),
		makeStamp(t),
		makeStamp(t),
	}
	tt := []struct {
		stamps   []swarm.Stamp
		batchID  []byte
		contains bool
	}{
		// contains
		{stamps: stamps, batchID: stamps[0].BatchID(), contains: true},
		{stamps: stamps, batchID: stamps[1].BatchID(), contains: true},
		{stamps: stamps, batchID: stamps[2].BatchID(), contains: true},

		// do not contain
		{stamps: nil, batchID: nil},
		{stamps: nil, batchID: makeStamp(t).BatchID()},
		{stamps: make([]swarm.Stamp, 0), batchID: swarm.RandBatchID(t)},
		{stamps: make([]swarm.Stamp, 10), batchID: swarm.RandBatchID(t)},
		{stamps: make([]swarm.Stamp, 10), batchID: nil},
		{stamps: stamps, batchID: swarm.RandBatchID(t)},
	}

	for _, tc := range tt {
		st, found := swarm.FindStampWithBatchID(tc.stamps, tc.batchID)
		if found != tc.contains {
			t.Fatalf("got %v, want %v", found, tc.contains)
		}
		if found && st == nil {
			t.Fatal("stamp should not be nil")
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

func makeStamp(t *testing.T) swarm.Stamp {
	t.Helper()

	return stamp{
		batchID: swarm.RandBatchID(t),
	}
}

type stamp struct {
	batchID []byte
}

func (s stamp) BatchID() []byte { return s.batchID }

func (s stamp) Index() []byte { return nil }

func (s stamp) Sig() []byte { return nil }

func (s stamp) Timestamp() []byte { return nil }

func (s stamp) MarshalBinary() (data []byte, err error) { return nil, nil }

func (s stamp) UnmarshalBinary(data []byte) error { return nil }

func (s stamp) Clone() swarm.Stamp { return s }

func (s stamp) Hash() ([]byte, error) { return nil, nil }
