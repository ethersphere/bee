// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stampindex_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"

	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

// newTestStorage is a helper function that creates a new storage.
func newTestStorage(t *testing.T) transaction.Storage {
	t.Helper()
	inmemStorage := internal.NewInmemStorage()
	return inmemStorage
}

func TestStampIndexItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero namespace",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       stampindex.NewItemWithKeys("", nil, nil, nil),
			Factory:    func() storage.Item { return new(stampindex.Item) },
			MarshalErr: stampindex.ErrStampItemMarshalNamespaceInvalid,
			CmpOpts:    []cmp.Option{cmp.AllowUnexported(stampindex.Item{})},
		},
	}, {
		name: "zero batchID",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       stampindex.NewItemWithKeys("test_namespace", nil, nil, nil),
			Factory:    func() storage.Item { return new(stampindex.Item) },
			MarshalErr: stampindex.ErrStampItemMarshalBatchIDInvalid,
			CmpOpts:    []cmp.Option{cmp.AllowUnexported(stampindex.Item{})},
		},
	}, {
		name: "zero batchIndex",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       stampindex.NewItemWithKeys("test_namespace", []byte{swarm.HashSize - 1: 9}, nil, nil),
			Factory:    func() storage.Item { return new(stampindex.Item) },
			MarshalErr: stampindex.ErrStampItemMarshalBatchIndexInvalid,
			CmpOpts:    []cmp.Option{cmp.AllowUnexported(stampindex.Item{})},
		},
	}, {
		name: "valid values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:    stampindex.NewItemWithValues([]byte{swarm.StampTimestampSize - 1: 9}, swarm.RandAddress(t)),
			Factory: func() storage.Item { return new(stampindex.Item) },
			CmpOpts: []cmp.Option{cmp.AllowUnexported(stampindex.Item{})},
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:    stampindex.NewItemWithValues(storagetest.MaxBatchTimestampBytes[:], swarm.NewAddress(storagetest.MaxAddressBytes[:])),
			Factory: func() storage.Item { return new(stampindex.Item) },
			CmpOpts: []cmp.Option{cmp.AllowUnexported(stampindex.Item{})},
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(stampindex.Item) },
			UnmarshalErr: stampindex.ErrStampItemUnmarshalInvalidSize,
			CmpOpts:      []cmp.Option{cmp.AllowUnexported(stampindex.Item{})},
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

func TestStoreLoadDeleteWithStamp(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)
	chunks := chunktest.GenerateTestRandomChunks(10)

	for i, chunk := range chunks {
		ns := fmt.Sprintf("namespace_%d", i)
		t.Run(ns, func(t *testing.T) {
			t.Run("store new stamp index", func(t *testing.T) {
				err := ts.Run(context.Background(), func(s transaction.Store) error {
					return stampindex.Store(s.IndexStore(), ns, chunk)
				})
				if err != nil {
					t.Fatalf("Store(...): unexpected error: %v", err)
				}

				stampHash, err := chunk.Stamp().Hash()
				if err != nil {
					t.Fatal(err)
				}
				want := stampindex.NewItemWithKeys(ns, chunk.Stamp().BatchID(), chunk.Stamp().Index(), stampHash)
				want.StampTimestamp = chunk.Stamp().Timestamp()
				want.ChunkAddress = chunk.Address()

				have := stampindex.NewItemWithKeys(ns, chunk.Stamp().BatchID(), chunk.Stamp().Index(), stampHash)
				err = ts.IndexStore().Get(have)
				if err != nil {
					t.Fatalf("Get(...): unexpected error: %v", err)
				}

				if diff := cmp.Diff(want, have, cmp.AllowUnexported(stampindex.Item{})); diff != "" {
					t.Fatalf("Get(...): mismatch (-want +have):\n%s", diff)
				}
			})

			t.Run("load stored stamp index", func(t *testing.T) {
				stampHash, err := chunk.Stamp().Hash()
				if err != nil {
					t.Fatal(err)
				}
				want := stampindex.NewItemWithKeys(ns, chunk.Stamp().BatchID(), chunk.Stamp().Index(), stampHash)
				want.StampTimestamp = chunk.Stamp().Timestamp()
				want.ChunkAddress = chunk.Address()

				have, err := stampindex.Load(ts.IndexStore(), ns, chunk.Stamp())
				if err != nil {
					t.Fatalf("Load(...): unexpected error: %v", err)
				}

				if diff := cmp.Diff(want, have, cmp.AllowUnexported(stampindex.Item{})); diff != "" {
					t.Fatalf("Load(...): mismatch (-want +have):\n%s", diff)
				}
			})

			t.Run("delete stored stamp index", func(t *testing.T) {
				err := ts.Run(context.Background(), func(s transaction.Store) error {
					return stampindex.Delete(s.IndexStore(), ns, chunk.Stamp())
				})
				if err != nil {
					t.Fatalf("Delete(...): unexpected error: %v", err)
				}

				have, err := stampindex.Load(ts.IndexStore(), ns, chunk.Stamp())
				if have != nil {
					t.Fatalf("Load(...): unexpected item %v", have)
				}
				if !errors.Is(err, storage.ErrNotFound) {
					t.Fatalf("Load(...): unexpected error: %v", err)
				}

				cnt := 0
				err = ts.IndexStore().Iterate(
					storage.Query{
						Factory: func() storage.Item {
							return new(stampindex.Item)
						},
					},
					func(result storage.Result) (bool, error) {
						cnt++
						return false, nil
					},
				)
				if err != nil {
					t.Fatalf("Store().Iterate(...): unexpected error: %v", err)
				}
				if want, have := 0, cnt; want != have {
					t.Fatalf("Store().Iterate(...): chunk count mismatch:\nwant: %d\nhave: %d", want, have)
				}
			})
		})
	}
}

func TestLoadOrStore(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)
	chunks := chunktest.GenerateTestRandomChunks(10)

	for i, chunk := range chunks {
		ns := fmt.Sprintf("namespace_%d", i)
		t.Run(ns, func(t *testing.T) {
			stampHash, err := chunk.Stamp().Hash()
			if err != nil {
				t.Fatal(err)
			}
			want := stampindex.NewItemWithKeys(ns, chunk.Stamp().BatchID(), chunk.Stamp().Index(), stampHash)
			want.StampTimestamp = chunk.Stamp().Timestamp()
			want.ChunkAddress = chunk.Address()

			trx, done := ts.NewTransaction(context.Background())

			have, loaded, err := stampindex.LoadOrStore(trx.IndexStore(), ns, chunk)
			if err != nil {
				t.Fatalf("LoadOrStore(...): unexpected error: %v", err)
			}
			if loaded {
				t.Fatalf("LoadOrStore(...): unexpected loaded flag")
			}
			if diff := cmp.Diff(want, have, cmp.AllowUnexported(stampindex.Item{})); diff != "" {
				t.Fatalf("Get(...): mismatch (-want +have):\n%s", diff)
			}
			assert.NoError(t, trx.Commit())
			done()

			trx, done = ts.NewTransaction(context.Background())
			defer done()

			have, loaded, err = stampindex.LoadOrStore(trx.IndexStore(), ns, chunk)
			if err != nil {
				t.Fatalf("LoadOrStore(...): unexpected error: %v", err)
			}
			if !loaded {
				t.Fatalf("LoadOrStore(...): unexpected loaded flag")
			}

			if diff := cmp.Diff(want, have, cmp.AllowUnexported(stampindex.Item{})); diff != "" {
				t.Fatalf("Get(...): mismatch (-want +have):\n%s", diff)
			}
			assert.NoError(t, trx.Commit())

			cnt := 0
			err = ts.IndexStore().Iterate(
				storage.Query{
					Factory: func() storage.Item {
						return new(stampindex.Item)
					},
				},
				func(result storage.Result) (bool, error) {
					cnt++
					return false, nil
				},
			)
			if err != nil {
				t.Fatalf("Store().Iterate(...): unexpected error: %v", err)
			}
			if want, have := i+1, cnt; want != have {
				t.Fatalf("Store().Iterate(...): chunk count mismatch:\nwant: %d\nhave: %d", want, have)
			}
		})
	}
}
