// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stampindex_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/stampindex"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/google/go-cmp/cmp"
)

// MaxBatchTimestampBytes represents bytes that can be used to represent a max. BatchTimestamp.
var MaxBatchTimestampBytes = [swarm.StampTimestampSize]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}

// newTestStorage is a helper function that creates a new storage.
func newTestStorage(t *testing.T) internal.Storage {
	t.Helper()

	inmemStorage, closer := internal.NewInmemStorage()
	t.Cleanup(func() {
		if err := closer(); err != nil {
			t.Errorf("failed closing the storage: %v", err)
		}
	})
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
			Item:       stampindex.NewItemWithKeys("", nil, nil),
			Factory:    func() storage.Item { return new(stampindex.Item) },
			MarshalErr: stampindex.ErrStampItemMarshalNamespaceInvalid,
			CmpOpts:    []cmp.Option{cmp.AllowUnexported(stampindex.Item{})},
		},
	}, {
		name: "zero batchID",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       stampindex.NewItemWithKeys("test_namespace", nil, nil),
			Factory:    func() storage.Item { return new(stampindex.Item) },
			MarshalErr: stampindex.ErrStampItemMarshalBatchIDInvalid,
			CmpOpts:    []cmp.Option{cmp.AllowUnexported(stampindex.Item{})},
		},
	}, {
		name: "zero batchIndex",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       stampindex.NewItemWithKeys("test_namespace", []byte{swarm.HashSize - 1: 9}, nil),
			Factory:    func() storage.Item { return new(stampindex.Item) },
			MarshalErr: stampindex.ErrStampItemMarshalBatchIndexInvalid,
			CmpOpts:    []cmp.Option{cmp.AllowUnexported(stampindex.Item{})},
		},
	}, {
		name: "valid values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: stampindex.NewItemWithValues(
				[]byte{swarm.StampTimestampSize - 1: 9},
				test.RandomAddress(),
				false,
			),
			Factory: func() storage.Item { return new(stampindex.Item) },
			CmpOpts: []cmp.Option{cmp.AllowUnexported(stampindex.Item{})},
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: stampindex.NewItemWithValues(
				MaxBatchTimestampBytes[:],
				swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				true,
			),
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

func TestStoreLoadDelete(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)
	chunks := chunktest.GenerateTestRandomChunks(10)

	for i, chunk := range chunks {
		ns := fmt.Sprintf("namespace_%d", i)
		t.Run(ns, func(t *testing.T) {
			t.Run("store new stamp index", func(t *testing.T) {
				err := stampindex.Store(ts, ns, chunk)
				if err != nil {
					t.Fatalf("Store(...): unexpected error: %v", err)
				}

				want := stampindex.NewItemWithKeys(
					ns,
					chunk.Stamp().BatchID(),
					chunk.Stamp().Index(),
				)
				want.BatchTimestamp = chunk.Stamp().Timestamp()
				want.ChunkAddress = chunk.Address()
				want.ChunkIsImmutable = chunk.Immutable()

				have := stampindex.NewItemWithKeys(
					ns,
					chunk.Stamp().BatchID(),
					chunk.Stamp().Index(),
				)
				err = ts.IndexStore().Get(have)
				if err != nil {
					t.Fatalf("Get(...): unexpected error: %v", err)
				}

				if diff := cmp.Diff(want, have, cmp.AllowUnexported(stampindex.Item{})); diff != "" {
					t.Fatalf("Get(...): mismatch (-want +have):\n%s", diff)
				}
			})

			t.Run("load stored stamp index", func(t *testing.T) {
				want := stampindex.NewItemWithKeys(
					ns,
					chunk.Stamp().BatchID(),
					chunk.Stamp().Index(),
				)
				want.BatchTimestamp = chunk.Stamp().Timestamp()
				want.ChunkAddress = chunk.Address()
				want.ChunkIsImmutable = chunk.Immutable()

				have, err := stampindex.Load(ts, ns, chunk)
				if err != nil {
					t.Fatalf("Load(...): unexpected error: %v", err)
				}

				if diff := cmp.Diff(want, have, cmp.AllowUnexported(stampindex.Item{})); diff != "" {
					t.Fatalf("Load(...): mismatch (-want +have):\n%s", diff)
				}
			})

			t.Run("delete stored stamp index", func(t *testing.T) {
				err := stampindex.Delete(ts, ns, chunk)
				if err != nil {
					t.Fatalf("Delete(...): unexpected error: %v", err)
				}

				have, err := stampindex.Load(ts, ns, chunk)
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
			want := stampindex.NewItemWithKeys(
				ns,
				chunk.Stamp().BatchID(),
				chunk.Stamp().Index(),
			)
			want.BatchTimestamp = chunk.Stamp().Timestamp()
			want.ChunkAddress = chunk.Address()
			want.ChunkIsImmutable = chunk.Immutable()

			have, loaded, err := stampindex.LoadOrStore(ts, ns, chunk)
			if err != nil {
				t.Fatalf("LoadOrStore(...): unexpected error: %v", err)
			}
			if loaded {
				t.Fatalf("LoadOrStore(...): unexpected loaded flag")
			}

			if diff := cmp.Diff(want, have, cmp.AllowUnexported(stampindex.Item{})); diff != "" {
				t.Fatalf("Get(...): mismatch (-want +have):\n%s", diff)
			}

			have, loaded, err = stampindex.LoadOrStore(ts, ns, chunk)
			if err != nil {
				t.Fatalf("LoadOrStore(...): unexpected error: %v", err)
			}
			if !loaded {
				t.Fatalf("LoadOrStore(...): unexpected loaded flag")
			}

			if diff := cmp.Diff(want, have, cmp.AllowUnexported(stampindex.Item{})); diff != "" {
				t.Fatalf("Get(...): mismatch (-want +have):\n%s", diff)
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
			if want, have := i+1, cnt; want != have {
				t.Fatalf("Store().Iterate(...): chunk count mismatch:\nwant: %d\nhave: %d", want, have)
			}
		})
	}
}
