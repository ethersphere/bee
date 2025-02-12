// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstamp_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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

func TestStoreLoadDelete(t *testing.T) {
	t.Parallel()

	ts := internal.NewInmemStorage()

	for i, chunk := range chunktest.GenerateTestRandomChunks(10) {
		ns := fmt.Sprintf("namespace_%d", i)
		t.Run(ns, func(t *testing.T) {
			t.Run("store new chunk stamp", func(t *testing.T) {
				want := new(chunkstamp.Item).
					WithNamespace(ns).
					WithAddress(chunk.Address()).
					WithStamp(chunk.Stamp())

				have := want.Clone()

				if err := ts.IndexStore().Get(have); !errors.Is(err, storage.ErrNotFound) {
					t.Fatalf("Get(...): unexpected error: have: %v; want: %v", err, storage.ErrNotFound)
				}

				if err := ts.Run(context.Background(), func(s transaction.Store) error {
					return chunkstamp.Store(s.IndexStore(), ns, chunk)
				}); err != nil {
					t.Fatalf("Store(...): unexpected error: %v", err)
				}

				if err := ts.IndexStore().Get(have); err != nil {
					t.Fatalf("Get(...): unexpected error: %v", err)
				}

				opts := cmp.AllowUnexported(
					chunkstamp.Item{},
					postage.Stamp{},
				)
				if diff := cmp.Diff(want, have, opts); diff != "" {
					t.Fatalf("Get(...): mismatch (-want +have):\n%s", diff)
				}
			})

			t.Run("load stored chunk stamp", func(t *testing.T) {
				want := chunk.Stamp()

				have, err := chunkstamp.Load(ts.IndexStore(), ns, chunk.Address())
				if err != nil {
					t.Fatalf("Load(...): unexpected error: %v", err)
				}

				if diff := cmp.Diff(want, have, cmp.AllowUnexported(postage.Stamp{})); diff != "" {
					t.Fatalf("Load(...): mismatch (-want +have):\n%s", diff)
				}
			})

			t.Run("load stored chunk stamp with batch id and hash", func(t *testing.T) {
				want := chunk.Stamp()

				have, err := chunkstamp.LoadWithBatchID(ts.IndexStore(), ns, chunk.Address(), chunk.Stamp().BatchID())
				if err != nil {
					t.Fatalf("LoadWithBatchID(...): unexpected error: %v", err)
				}

				if diff := cmp.Diff(want, have, cmp.AllowUnexported(postage.Stamp{})); diff != "" {
					t.Fatalf("LoadWithBatchID(...): mismatch (-want +have):\n%s", diff)
				}

				h, err := want.Hash()
				if err != nil {
					t.Fatal(err)
				}

				have, err = chunkstamp.LoadWithStampHash(ts.IndexStore(), ns, chunk.Address(), h)
				if err != nil {
					t.Fatalf("LoadWithBatchID(...): unexpected error: %v", err)
				}

				if diff := cmp.Diff(want, have, cmp.AllowUnexported(postage.Stamp{})); diff != "" {
					t.Fatalf("LoadWithBatchID(...): mismatch (-want +have):\n%s", diff)
				}
			})

			t.Run("delete stored stamp", func(t *testing.T) {
				if i%2 == 0 {
					if err := ts.Run(context.Background(), func(s transaction.Store) error {
						return chunkstamp.Delete(s.IndexStore(), ns, chunk.Address(), chunk.Stamp().BatchID())
					}); err != nil {
						t.Fatalf("Delete(...): unexpected error: %v", err)
					}
				} else {
					if err := ts.Run(context.Background(), func(s transaction.Store) error {
						return chunkstamp.DeleteWithStamp(s.IndexStore(), ns, chunk.Address(), chunk.Stamp())
					}); err != nil {
						t.Fatalf("DeleteWithStamp(...): unexpected error: %v", err)
					}
				}

				have, err := chunkstamp.LoadWithBatchID(ts.IndexStore(), ns, chunk.Address(), chunk.Stamp().BatchID())
				if !errors.Is(err, storage.ErrNotFound) {
					t.Fatalf("Load(...): unexpected error: %v", err)
				}
				if have != nil {
					t.Fatalf("Load(...): loaded stamp should be nil")
				}
			})

			t.Run("delete all stored stamp index", func(t *testing.T) {
				if err := ts.Run(context.Background(), func(s transaction.Store) error {
					return chunkstamp.Store(s.IndexStore(), ns, chunk)
				}); err != nil {
					t.Fatalf("Store(...): unexpected error: %v", err)
				}

				if err := ts.Run(context.Background(), func(s transaction.Store) error {
					return chunkstamp.DeleteAll(s.IndexStore(), ns, chunk.Address())
				}); err != nil {
					t.Fatalf("DeleteAll(...): unexpected error: %v", err)
				}

				have, err := chunkstamp.Load(ts.IndexStore(), ns, chunk.Address())
				if !errors.Is(err, storage.ErrNotFound) {
					t.Fatalf("Load(...): unexpected error: %v", err)
				}
				if have != nil {
					t.Fatalf("Load(...): unexpected item %v", have)
				}

				cnt := 0
				err = ts.IndexStore().Iterate(
					storage.Query{
						Factory: func() storage.Item {
							return new(chunkstamp.Item)
						},
					},
					func(result storage.Result) (bool, error) {
						cnt++
						return false, nil
					},
				)
				if err != nil {
					t.Fatalf("IndexStore().Iterate(...): unexpected error: %v", err)
				}
				if want, have := 0, cnt; want != have {
					t.Fatalf("IndexStore().Iterate(...): chunk count mismatch:\nwant: %d\nhave: %d", want, have)
				}
			})
		})
	}
}
