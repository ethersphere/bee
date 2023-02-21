// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstamp_test

import (
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstamp"
	"github.com/ethersphere/bee/pkg/postage"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/go-cmp/cmp"
)

func TestChunkStampItem(t *testing.T) {
	t.Parallel()

	minAddress := swarm.NewAddress(storagetest.MinAddressBytes[:])
	minStamp := postage.NewStamp(make([]byte, 32), make([]byte, 8), make([]byte, 8), make([]byte, 65))
	rndChunk := chunktest.GenerateTestRandomChunk()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero namespace",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       new(chunkstamp.Item),
			Factory:    func() storage.Item { return new(chunkstamp.Item) },
			MarshalErr: chunkstamp.ErrMarshalInvalidChunkStampItemNamespace,
			CmpOpts:    []cmp.Option{cmp.AllowUnexported(chunkstamp.Item{})},
		},
	}, {
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: new(chunkstamp.Item).
				WithNamespace("test_namespace").
				WithAddress(swarm.ZeroAddress),
			Factory:    func() storage.Item { return new(chunkstamp.Item) },
			MarshalErr: chunkstamp.ErrMarshalInvalidChunkStampItemAddress,
			CmpOpts:    []cmp.Option{cmp.AllowUnexported(chunkstamp.Item{})},
		},
	}, {
		name: "nil stamp",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: new(chunkstamp.Item).
				WithNamespace("test_namespace").
				WithAddress(minAddress),
			Factory:    func() storage.Item { return new(chunkstamp.Item) },
			MarshalErr: chunkstamp.ErrMarshalInvalidChunkStampItemStamp,
			CmpOpts:    []cmp.Option{cmp.AllowUnexported(chunkstamp.Item{})},
		},
	}, {
		name: "zero stamp",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: new(chunkstamp.Item).
				WithNamespace("test_namespace").
				WithAddress(minAddress).
				WithStamp(new(postage.Stamp)),
			Factory:    func() storage.Item { return new(chunkstamp.Item) },
			MarshalErr: postage.ErrInvalidBatchID,
			CmpOpts:    []cmp.Option{cmp.AllowUnexported(postage.Stamp{}, chunkstamp.Item{})},
		},
	}, {
		name: "min values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: new(chunkstamp.Item).
				WithNamespace("test_namespace").
				WithAddress(minAddress).
				WithStamp(minStamp),
			Factory: func() storage.Item { return new(chunkstamp.Item).WithAddress(minAddress) },
			CmpOpts: []cmp.Option{cmp.AllowUnexported(postage.Stamp{}, chunkstamp.Item{})},
		},
	}, {
		name: "valid values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: new(chunkstamp.Item).
				WithNamespace("test_namespace").
				WithAddress(rndChunk.Address()).
				WithStamp(rndChunk.Stamp()),
			Factory: func() storage.Item { return new(chunkstamp.Item).WithAddress(rndChunk.Address()) },
			CmpOpts: []cmp.Option{cmp.AllowUnexported(postage.Stamp{}, chunkstamp.Item{})},
		},
	}, {
		name: "nil address on unmarshal",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: new(chunkstamp.Item).
				WithNamespace("test_namespace").
				WithAddress(minAddress).
				WithStamp(rndChunk.Stamp()),
			Factory:      func() storage.Item { return new(chunkstamp.Item) },
			UnmarshalErr: chunkstamp.ErrUnmarshalInvalidChunkStampItemAddress,
			CmpOpts:      []cmp.Option{cmp.AllowUnexported(postage.Stamp{}, chunkstamp.Item{})},
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(chunkstamp.Item).WithAddress(rndChunk.Address()) },
			UnmarshalErr: chunkstamp.ErrUnmarshalInvalidChunkStampItemSize,
			CmpOpts:      []cmp.Option{cmp.AllowUnexported(chunkstamp.Item{})},
		},
	}}

	for _, tc := range tests {
		tc := tc

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
