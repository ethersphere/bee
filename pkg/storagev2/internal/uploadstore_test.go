// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/internal"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
	"github.com/ethersphere/bee/pkg/swarm"
	swarmtesting "github.com/ethersphere/bee/pkg/swarm/test"
)

func TestTagIDAddressItem_MarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       &internal.TagIDAddressItem{},
			Factory:    func() storage.Item { return new(internal.TagIDAddressItem) },
			MarshalErr: internal.ErrTagIDAddressItemMarshalAddressIsZero,
		},
	}, {
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &internal.TagIDAddressItem{
				Address: swarm.ZeroAddress,
				TagID:   1,
			},
			Factory:    func() storage.Item { return new(internal.TagIDAddressItem) },
			MarshalErr: internal.ErrTagIDAddressItemMarshalAddressIsZero,
		},
	}, {
		name: "min values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &internal.TagIDAddressItem{
				Address: swarm.NewAddress(storagetest.MinAddressBytes[:]),
				TagID:   0,
			},
			Factory: func() storage.Item { return new(internal.TagIDAddressItem) },
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &internal.TagIDAddressItem{
				Address: swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				TagID:   math.MaxUint64,
			},
			Factory: func() storage.Item { return new(internal.TagIDAddressItem) },
		},
	}, {
		name: "random values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &internal.TagIDAddressItem{
				Address: swarmtesting.RandomAddress(),
				TagID:   rand.Uint64(),
			},
			Factory: func() storage.Item { return new(internal.TagIDAddressItem) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(internal.TagIDAddressItem) },
			UnmarshalErr: internal.ErrTagIDAddressItemUnmarshalInvalidSize,
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})
	}
}

func TestPushItem_MarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       &internal.PushItem{},
			Factory:    func() storage.Item { return new(internal.PushItem) },
			MarshalErr: internal.ErrPushItemMarshalAddressIsZero,
		},
	}, {
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &internal.PushItem{
				Timestamp: 1,
				Address:   swarm.ZeroAddress,
				TagID:     1,
			},
			Factory:    func() storage.Item { return new(internal.PushItem) },
			MarshalErr: internal.ErrPushItemMarshalAddressIsZero,
		},
	}, {
		name: "min values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &internal.PushItem{
				Timestamp: 0,
				Address:   swarm.NewAddress(storagetest.MinAddressBytes[:]),
				TagID:     0,
			},
			Factory: func() storage.Item { return new(internal.PushItem) },
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &internal.PushItem{
				Timestamp: math.MaxUint64,
				Address:   swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				TagID:     math.MaxUint64,
			},
			Factory: func() storage.Item { return new(internal.PushItem) },
		},
	}, {
		name: "random values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &internal.PushItem{
				Timestamp: rand.Uint64(),
				Address:   swarmtesting.RandomAddress(),
				TagID:     rand.Uint64(),
			},
			Factory: func() storage.Item { return new(internal.PushItem) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(internal.PushItem) },
			UnmarshalErr: internal.ErrPushItemUnmarshalInvalidSize,
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})
	}
}
