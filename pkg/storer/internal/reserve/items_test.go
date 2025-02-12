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
