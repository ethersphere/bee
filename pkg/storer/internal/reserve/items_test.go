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

func TestChunkBinItemDualFormat(t *testing.T) {
	t.Parallel()

	addr := swarm.NewAddress(storagetest.MaxAddressBytes[:])
	batchID := []byte("01234567890123456789012345678901")
	stampHash := []byte("abcdefghijklmnopqrstuvwxyz123456")
	loc := storage.ChunkLocation{1, 2, 3, 4, 5, 6, 7, 8}

	item := &reserve.ChunkBinItem{
		Bin:       5,
		BinID:     42,
		Address:   addr,
		BatchID:   batchID,
		StampHash: stampHash,
		ChunkType: swarm.ChunkTypeContentAddressed,
		Location:  loc,
	}

	// Marshal produces new format with location
	buf, err := item.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(buf) != 114 {
		t.Fatalf("expected 114 bytes, got %d", len(buf))
	}

	// Unmarshal new format preserves location
	recovered := new(reserve.ChunkBinItem)
	if err := recovered.Unmarshal(buf); err != nil {
		t.Fatalf("unmarshal new format: %v", err)
	}
	if recovered.Location != loc {
		t.Fatalf("expected location %v, got %v", loc, recovered.Location)
	}
	if !recovered.Address.Equal(addr) {
		t.Fatalf("expected address %v, got %v", addr, recovered.Address)
	}

	// Unmarshal legacy 106-byte format succeeds with zero location
	legacyBuf := buf[:106]
	legacyRecovered := new(reserve.ChunkBinItem)
	if err := legacyRecovered.Unmarshal(legacyBuf); err != nil {
		t.Fatalf("unmarshal legacy format: %v", err)
	}
	if !legacyRecovered.Location.IsZero() {
		t.Fatalf("expected zero location for legacy item, got %v", legacyRecovered.Location)
	}
	if !legacyRecovered.Address.Equal(addr) {
		t.Fatalf("expected address %v, got %v", addr, legacyRecovered.Address)
	}
}
