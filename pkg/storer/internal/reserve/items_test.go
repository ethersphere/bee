// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestReserveItems(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{
		{
			name: "BatchRadiusItem",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &reserve.BatchRadiusItem{
					BatchID:   storagetest.MaxAddressBytes[:],
					Address:   swarm.NewAddress(storagetest.MaxAddressBytes[:]),
					Bin:       9,
					BinID:     100,
					StampHash: storagetest.MaxAddressBytes[:],
				},
				Factory: func() storage.Item { return new(reserve.BatchRadiusItem) },
			},
		},
		{
			name: "ChunkBinItem",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &reserve.ChunkBinItem{
					Address:   swarm.NewAddress(storagetest.MaxAddressBytes[:]),
					BatchID:   storagetest.MaxAddressBytes[:],
					Bin:       9,
					BinID:     100,
					StampHash: storagetest.MaxAddressBytes[:],
				},
				Factory: func() storage.Item { return new(reserve.ChunkBinItem) },
			},
		},
		{
			name: "BinItem",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &reserve.BinItem{
					BinID: 100,
				},
				Factory: func() storage.Item { return new(reserve.BinItem) },
			},
		},
		{
			name: "RadiusItem",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &reserve.RadiusItem{
					Radius: 9,
				},
				Factory: func() storage.Item { return new(reserve.RadiusItem) },
			},
		},
		{
			// Address and StampHash form the key, not the value, so the
			// value round trip restores BatchID and ChunkType only.
			name: "ChunkAddrItem",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &reserve.ChunkAddrItem{
					Address:   swarm.NewAddress(storagetest.MaxAddressBytes[:]),
					StampHash: storagetest.MaxAddressBytes[:],
					BatchID:   storagetest.MaxAddressBytes[:],
					ChunkType: swarm.ChunkTypeSingleOwner,
				},
				Factory: func() storage.Item { return new(reserve.ChunkAddrItem) },
				CmpOpts: []cmp.Option{cmpopts.IgnoreFields(reserve.ChunkAddrItem{}, "Address", "StampHash")},
			},
		},
		{
			name: "ChunkAddrValidItem",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item:    &reserve.ChunkAddrValidItem{Version: reserve.ChunkAddrIndexVersion},
				Factory: func() storage.Item { return new(reserve.ChunkAddrValidItem) },
			},
		},
		{
			name: "ChunkAddrValidItem with digest",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &reserve.ChunkAddrValidItem{
					Version: reserve.ChunkAddrIndexVersion,
					Digest: reserve.AddrIndexDigest{
						Epoch:  math.MaxUint64,
						BinIDs: [swarm.MaxBins]uint64{0: 1, 7: 1 << 40, swarm.MaxBins - 1: math.MaxUint64},
						Count:  4_194_304,
					},
				},
				Factory: func() storage.Item { return new(reserve.ChunkAddrValidItem) },
			},
		},
		{
			name: "ChunkAddrItem zero address",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item:       &reserve.ChunkAddrItem{BatchID: storagetest.MaxAddressBytes[:]},
				Factory:    func() storage.Item { return new(reserve.ChunkAddrItem) },
				MarshalErr: reserve.ErrMarshalInvalidAddress,
			},
		},
		{
			name: "ChunkAddrItem invalid size",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &storagetest.ItemStub{
					MarshalBuf:   []byte{0xFF},
					UnmarshalBuf: []byte{0xFF},
				},
				Factory:      func() storage.Item { return new(reserve.ChunkAddrItem) },
				UnmarshalErr: reserve.ErrUnmarshalInvalidSize,
			},
		},
		{
			name: "ChunkAddrValidItem invalid size",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &storagetest.ItemStub{
					MarshalBuf:   []byte{reserve.ChunkAddrIndexVersion, 0xFF},
					UnmarshalBuf: []byte{reserve.ChunkAddrIndexVersion, 0xFF},
				},
				Factory:      func() storage.Item { return new(reserve.ChunkAddrValidItem) },
				UnmarshalErr: reserve.ErrUnmarshalInvalidSize,
			},
		},
		{
			name: "BatchRadiusItem zero address",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &reserve.BatchRadiusItem{
					BatchID: storagetest.MaxAddressBytes[:],
				},
				Factory:    func() storage.Item { return new(reserve.BatchRadiusItem) },
				MarshalErr: reserve.ErrMarshalInvalidAddress,
			},
		},
		{
			name: "ChunkBinItem zero address",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item:       &reserve.ChunkBinItem{},
				Factory:    func() storage.Item { return new(reserve.ChunkBinItem) },
				MarshalErr: reserve.ErrMarshalInvalidAddress,
			},
		},
		{
			name: "BatchRadiusItem invalid size",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &storagetest.ItemStub{
					MarshalBuf:   []byte{0xFF},
					UnmarshalBuf: []byte{0xFF},
				},
				Factory:      func() storage.Item { return new(reserve.BatchRadiusItem) },
				UnmarshalErr: reserve.ErrUnmarshalInvalidSize,
			},
		},
		{
			name: "ChunkBinItem invalid size",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &storagetest.ItemStub{
					MarshalBuf:   []byte{0xFF},
					UnmarshalBuf: []byte{0xFF},
				},
				Factory:      func() storage.Item { return new(reserve.ChunkBinItem) },
				UnmarshalErr: reserve.ErrUnmarshalInvalidSize,
			},
		},
		{
			name: "BinItem invalid size",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &storagetest.ItemStub{
					MarshalBuf:   []byte{0xFF},
					UnmarshalBuf: []byte{0xFF},
				},
				Factory:      func() storage.Item { return new(reserve.BinItem) },
				UnmarshalErr: reserve.ErrUnmarshalInvalidSize,
			},
		},
		{
			name: "RadiusItem invalid size",
			test: &storagetest.ItemMarshalAndUnmarshalTest{
				Item: &storagetest.ItemStub{
					MarshalBuf:   []byte{0xFF, 0xFF},
					UnmarshalBuf: []byte{0xFF, 0xFF},
				},
				Factory:      func() storage.Item { return new(reserve.RadiusItem) },
				UnmarshalErr: reserve.ErrUnmarshalInvalidSize,
			},
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s marshal/unmarshal", tc.name), func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})

		t.Run(fmt.Sprintf("%s clone", tc.name), func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemClone(t, &storagetest.ItemCloneTest{
				Item:    tc.test.Item,
				CmpOpts: tc.test.CmpOpts,
			})
		})
	}
}

func TestChunkAddrItem(t *testing.T) {
	t.Parallel()

	addr := swarm.RandAddress(t)
	stampHash := swarm.RandAddress(t).Bytes()
	batchID := swarm.RandAddress(t).Bytes()
	item := &reserve.ChunkAddrItem{
		Address:   addr,
		StampHash: stampHash,
		BatchID:   batchID,
		ChunkType: swarm.ChunkTypeContentAddressed,
	}

	t.Run("key", func(t *testing.T) {
		t.Parallel()

		if got, want := item.Namespace(), "chunkAddr"; got != want {
			t.Fatalf("namespace %q, want %q", got, want)
		}
		if got, want := item.ID(), string(addr.Bytes())+string(stampHash); got != want {
			t.Fatalf("id %x, want address ‖ stamp hash %x", got, want)
		}
		if got, want := item.String(), "chunkAddr/"+item.ID(); got != want {
			t.Fatalf("string %q, want %q", got, want)
		}
		// The key orders by address first, so a range of addresses is a
		// contiguous run of keys.
		other := &reserve.ChunkAddrItem{Address: addr, StampHash: bytes.Repeat([]byte{0xFF}, swarm.HashSize)}
		if !bytes.HasPrefix([]byte(other.ID()), addr.Bytes()) {
			t.Fatal("id does not start with the address")
		}
	})

	t.Run("unmarshal keeps the key fields", func(t *testing.T) {
		t.Parallel()

		buf, err := item.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if len(buf) != swarm.HashSize+1 {
			t.Fatalf("value size %d, want %d", len(buf), swarm.HashSize+1)
		}
		got := &reserve.ChunkAddrItem{Address: addr, StampHash: stampHash}
		if err := got.Unmarshal(buf); err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(item, got); diff != "" {
			t.Fatalf("round trip mismatch (-want +have):\n%s", diff)
		}
		buf[0] ^= 0xFF
		if bytes.Equal(got.BatchID[:1], buf[:1]) {
			t.Fatal("unmarshaled batch id aliases the value buffer")
		}
	})

	t.Run("clone is deep", func(t *testing.T) {
		t.Parallel()

		c := item.Clone().(*reserve.ChunkAddrItem)
		if diff := cmp.Diff(item, c); diff != "" {
			t.Fatalf("clone mismatch (-want +have):\n%s", diff)
		}
		c.StampHash[0] ^= 0xFF
		c.BatchID[0] ^= 0xFF
		if c.StampHash[0] == item.StampHash[0] || c.BatchID[0] == item.BatchID[0] {
			t.Fatal("clone shares byte slices with the original")
		}
		var nilItem *reserve.ChunkAddrItem
		if nilItem.Clone() != nil {
			t.Fatal("clone of nil item is not nil")
		}
	})

	t.Run("valid marker", func(t *testing.T) {
		t.Parallel()

		m := &reserve.ChunkAddrValidItem{}
		if m.Namespace() != "addrIndexValid" || m.ID() != "" || m.String() != "addrIndexValid" {
			t.Fatalf("marker key %q/%q/%q", m.Namespace(), m.ID(), m.String())
		}
		if err := m.Unmarshal(nil); !errors.Is(err, reserve.ErrUnmarshalInvalidSize) {
			t.Fatalf("unmarshal of empty value: %v", err)
		}
	})

	t.Run("other version", func(t *testing.T) {
		t.Parallel()

		// A marker of another version decodes to its version only, whatever
		// its length, so the caller sees the mismatch and refills.
		for _, buf := range [][]byte{
			{reserve.ChunkAddrIndexVersion + 1},
			{0},
			append([]byte{reserve.ChunkAddrIndexVersion + 1}, make([]byte, 300)...),
		} {
			m := &reserve.ChunkAddrValidItem{Digest: reserve.AddrIndexDigest{Count: 5}}
			if err := m.Unmarshal(buf); err != nil {
				t.Fatalf("unmarshal of version %d (%d bytes): %v", buf[0], len(buf), err)
			}
			if m.Version != buf[0] || m.Digest != (reserve.AddrIndexDigest{}) {
				t.Fatalf("decoded %+v", m)
			}
		}
	})
}
