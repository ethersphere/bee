// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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

func TestChunkBinItemAddressAndProximityFilter(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
	closeAddr := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000001")
	farAddr := swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000")

	itemClose := &reserve.ChunkBinItem{
		Bin:       0,
		BinID:     1,
		Address:   closeAddr,
		BatchID:   storagetest.MaxAddressBytes[:],
		StampHash: storagetest.MaxAddressBytes[:],
	}
	bufClose, err := itemClose.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	itemFar := &reserve.ChunkBinItem{
		Bin:       0,
		BinID:     2,
		Address:   farAddr,
		BatchID:   storagetest.MaxAddressBytes[:],
		StampHash: storagetest.MaxAddressBytes[:],
	}
	bufFar, err := itemFar.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	addr, ok := reserve.ChunkBinItemAddress(bufClose)
	if !ok || !swarm.NewAddress(addr).Equal(closeAddr) {
		t.Fatalf("expected address %s, got %s", closeAddr, swarm.NewAddress(addr))
	}

	filter := reserve.ProximityFilter(baseAddr.Bytes(), 1)
	// farAddr has proximity 0 to baseAddr (< 1) -> should be filtered out (true)
	if !filter("", bufFar) {
		t.Fatal("expected farAddr to be filtered out")
	}
	// closeAddr has proximity 255 to baseAddr (>= 1) -> should NOT be filtered out (false)
	if filter("", bufClose) {
		t.Fatal("expected closeAddr to NOT be filtered out")
	}

	// Corrupted buffer should not be filtered out so Unmarshal handles it
	if filter("", []byte{1, 2, 3}) {
		t.Fatal("expected invalid buffer to NOT be filtered out")
	}
}

// TestProximityFilterMatchesUnmarshal checks that ProximityFilter reaches the
// same verdict on the raw bytes as a proximity check on the unmarshaled item,
// for every proximity order and committed depth. It knows nothing about the
// layout, so a layout change leaves it untouched.
func TestProximityFilterMatchesUnmarshal(t *testing.T) {
	t.Parallel()

	anchor := swarm.RandAddress(t)

	for po := 0; po <= int(swarm.MaxPO); po++ {
		item := &reserve.ChunkBinItem{
			Bin:       uint8(po),
			BinID:     uint64(po) + 1,
			Address:   swarm.RandAddressAt(t, anchor, po),
			BatchID:   swarm.RandAddress(t).Bytes(),
			StampHash: swarm.RandAddress(t).Bytes(),
			ChunkType: swarm.ChunkTypeContentAddressed,
		}
		buf, err := item.Marshal()
		if err != nil {
			t.Fatal(err)
		}

		unmarshaled := &reserve.ChunkBinItem{}
		if err := unmarshaled.Unmarshal(buf); err != nil {
			t.Fatal(err)
		}

		for depth := uint8(0); depth <= swarm.MaxPO+1; depth++ {
			want := swarm.Proximity(unmarshaled.Address.Bytes(), anchor.Bytes()) < depth
			got := reserve.ProximityFilter(anchor.Bytes(), depth)("", buf)
			if got != want {
				t.Fatalf("po %d depth %d: filter excludes=%v, unmarshaled item excludes=%v", po, depth, got, want)
			}
		}
	}
}
