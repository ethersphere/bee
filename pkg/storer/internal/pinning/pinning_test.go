// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pinstore_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"

	storagetest "github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type pinningCollection struct {
	root         swarm.Chunk
	uniqueChunks []swarm.Chunk
	dupChunks    []swarm.Chunk
}

func newTestStorage(t *testing.T) transaction.Storage {
	t.Helper()
	storg := internal.NewInmemStorage()
	return storg
}

func TestPinStore(t *testing.T) {
	tests := make([]pinningCollection, 0, 3)

	for _, tc := range []struct {
		dupChunks    int
		uniqueChunks int
	}{
		{
			dupChunks:    5,
			uniqueChunks: 10,
		},
		{
			dupChunks:    10,
			uniqueChunks: 20,
		},
		{
			dupChunks:    15,
			uniqueChunks: 130,
		},
	} {
		var c pinningCollection
		c.root = chunktest.GenerateTestRandomChunk()
		c.uniqueChunks = chunktest.GenerateTestRandomChunks(tc.uniqueChunks)
		dupChunk := chunktest.GenerateTestRandomChunk()
		for i := 0; i < tc.dupChunks; i++ {
			c.dupChunks = append(c.dupChunks, dupChunk)
		}
		tests = append(tests, c)
	}

	st := newTestStorage(t)

	t.Run("create new collections", func(t *testing.T) {
		for tCount, tc := range tests {
			t.Run(fmt.Sprintf("create collection %d", tCount), func(t *testing.T) {
				var putter internal.PutterCloserWithReference
				var err error
				err = st.Run(context.Background(), func(s transaction.Store) error {
					putter, err = pinstore.NewCollection(s.IndexStore())
					return err
				})
				if err != nil {
					t.Fatal(err)
				}

				for _, ch := range append(tc.uniqueChunks, tc.root) {
					if err := st.Run(context.Background(), func(s transaction.Store) error {
						return putter.Put(context.Background(), s, ch)
					}); err != nil {
						t.Fatal(err)
					}
				}
				for _, ch := range tc.dupChunks {
					if err := st.Run(context.Background(), func(s transaction.Store) error {
						return putter.Put(context.Background(), s, ch)
					}); err != nil {
						t.Fatal(err)
					}
				}

				if err := st.Run(context.Background(), func(s transaction.Store) error {
					return putter.Close(s.IndexStore(), tc.root.Address())
				}); err != nil {
					t.Fatal(err)
				}
			})
		}
	})

	t.Run("verify all collection data", func(t *testing.T) {
		for tCount, tc := range tests {
			t.Run(fmt.Sprintf("verify collection %d", tCount), func(t *testing.T) {
				allChunks := append(tc.uniqueChunks, tc.root)
				allChunks = append(allChunks, tc.dupChunks...)
				for _, ch := range allChunks {
					exists, err := st.ChunkStore().Has(context.TODO(), ch.Address())
					if err != nil {
						t.Fatal(err)
					}
					if !exists {
						t.Fatal("chunk should exist")
					}
					rch, err := st.ChunkStore().Get(context.TODO(), ch.Address())
					if err != nil {
						t.Fatal(err)
					}
					if !ch.Equal(rch) {
						t.Fatal("read chunk not equal")
					}
				}
			})
		}
	})

	t.Run("verify root pins", func(t *testing.T) {
		pins, err := pinstore.Pins(st.IndexStore())
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) != 3 {
			t.Fatalf("incorrect no of root pins, expected 3 found %d", len(pins))
		}
		for _, tc := range tests {
			found := false
			for _, f := range pins {
				if f.Equal(tc.root.Address()) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("pin %s not found", tc.root.Address())
			}
		}
	})

	t.Run("has pin", func(t *testing.T) {
		for _, tc := range tests {
			found, err := pinstore.HasPin(st.IndexStore(), tc.root.Address())
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatalf("expected the pin %s to be found", tc.root.Address())
			}
		}
	})

	t.Run("verify internal state", func(t *testing.T) {
		for _, tc := range tests {
			count := 0
			err := pinstore.IterateCollection(st.IndexStore(), tc.root.Address(), func(addr swarm.Address) (bool, error) {
				count++
				return false, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if count != len(tc.uniqueChunks)+2 {
				t.Fatalf("incorrect no of chunks in collection, expected %d found %d", len(tc.uniqueChunks)+2, count)
			}
			stat, err := pinstore.GetStat(st.IndexStore(), tc.root.Address())
			if err != nil {
				t.Fatal(err)
			}
			if stat.Total != uint64(len(tc.uniqueChunks)+len(tc.dupChunks)+1) {
				t.Fatalf("incorrect no of chunks, expected %d found %d", len(tc.uniqueChunks)+len(tc.dupChunks)+1, stat.Total)
			}
			if stat.DupInCollection != uint64(len(tc.dupChunks)-1) {
				t.Fatalf("incorrect no of duplicate chunks, expected %d found %d", len(tc.dupChunks)-1, stat.DupInCollection)
			}
		}
	})

	t.Run("iterate stats", func(t *testing.T) {
		count, total, dup := 0, 0, 0
		err := pinstore.IterateCollectionStats(st.IndexStore(), func(stat pinstore.CollectionStat) (bool, error) {
			count++
			total += int(stat.Total)
			dup += int(stat.DupInCollection)

			return false, nil
		})
		if err != nil {
			t.Fatalf("IterateCollectionStats: unexpected error: %v", err)
		}

		wantTotal, wantDup := 0, 0
		for _, tc := range tests {
			wantTotal += len(tc.uniqueChunks) + len(tc.dupChunks) + 1
			wantDup += len(tc.dupChunks) - 1
		}

		if count != len(tests) {
			t.Fatalf("unexpected collection count: want %d have: %d", len(tests), count)
		}
		if wantTotal != total {
			t.Fatalf("unexpected total count: want %d have: %d", wantTotal, total)
		}
		if wantDup != dup {
			t.Fatalf("unexpected dup count: want %d have: %d", wantDup, dup)
		}
	})

	t.Run("delete collection", func(t *testing.T) {
		err := pinstore.DeletePin(context.TODO(), st, tests[0].root.Address())
		if err != nil {
			t.Fatal(err)
		}

		found, err := pinstore.HasPin(st.IndexStore(), tests[0].root.Address())
		if err != nil {
			t.Fatal(err)
		}
		if found {
			t.Fatal("expected pin to not be found")
		}

		pins, err := pinstore.Pins(st.IndexStore())
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) != 2 {
			t.Fatalf("incorrect no of root pins, expected 2 found %d", len(pins))
		}

		allChunks := append(tests[0].uniqueChunks, tests[0].root)
		allChunks = append(allChunks, tests[0].dupChunks...)
		for _, ch := range allChunks {
			exists, err := st.ChunkStore().Has(context.TODO(), ch.Address())
			if err != nil {
				t.Fatal(err)
			}
			if exists {
				t.Fatal("chunk should not exist")
			}
			_, err = st.ChunkStore().Get(context.TODO(), ch.Address())
			if !errors.Is(err, storage.ErrNotFound) {
				t.Fatal(err)
			}
		}
	})

	t.Run("error after close", func(t *testing.T) {
		root := chunktest.GenerateTestRandomChunk()

		var (
			putter internal.PutterCloserWithReference
			err    error
		)
		err = st.Run(context.Background(), func(s transaction.Store) error {
			putter, err = pinstore.NewCollection(s.IndexStore())
			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		err = st.Run(context.Background(), func(s transaction.Store) error {
			return putter.Put(context.Background(), s, root)
		})
		if err != nil {
			t.Fatal(err)
		}

		err = st.Run(context.Background(), func(s transaction.Store) error {
			return putter.Close(s.IndexStore(), root.Address())
		})
		if err != nil {
			t.Fatal(err)
		}

		err = st.Run(context.Background(), func(s transaction.Store) error {
			return putter.Put(context.Background(), s, chunktest.GenerateTestRandomChunk())
		})
		if !errors.Is(err, pinstore.ErrPutterAlreadyClosed) {
			t.Fatalf("unexpected error during Put, want: %v, got: %v", pinstore.ErrPutterAlreadyClosed, err)
		}
	})

	t.Run("duplicate collection", func(t *testing.T) {
		root := chunktest.GenerateTestRandomChunk()

		var (
			putter internal.PutterCloserWithReference
			err    error
		)
		err = st.Run(context.Background(), func(s transaction.Store) error {
			putter, err = pinstore.NewCollection(s.IndexStore())
			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		err = st.Run(context.Background(), func(s transaction.Store) error {
			return putter.Put(context.Background(), s, root)
		})
		if err != nil {
			t.Fatal(err)
		}

		err = st.Run(context.Background(), func(s transaction.Store) error {
			return putter.Close(s.IndexStore(), root.Address())
		})
		if err != nil {
			t.Fatal(err)
		}

		err = st.Run(context.Background(), func(s transaction.Store) error {
			return putter.Close(s.IndexStore(), root.Address())
		})
		if err == nil || !errors.Is(err, pinstore.ErrDuplicatePinCollection) {
			t.Fatalf("unexpected error during CLose, want: %v, got: %v", pinstore.ErrDuplicatePinCollection, err)
		}
	})

	t.Run("zero address close", func(t *testing.T) {
		root := chunktest.GenerateTestRandomChunk()

		var (
			putter internal.PutterCloserWithReference
			err    error
		)
		err = st.Run(context.Background(), func(s transaction.Store) error {
			putter, err = pinstore.NewCollection(s.IndexStore())
			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		err = st.Run(context.Background(), func(s transaction.Store) error {
			return putter.Put(context.Background(), s, root)
		})
		if err != nil {
			t.Fatal(err)
		}

		err = st.Run(context.Background(), func(s transaction.Store) error {
			return putter.Close(s.IndexStore(), swarm.ZeroAddress)
		})
		if !errors.Is(err, pinstore.ErrCollectionRootAddressIsZero) {
			t.Fatalf("unexpected error on close, want: %v, got: %v", pinstore.ErrCollectionRootAddressIsZero, err)
		}
	})

	t.Run("have 0 pins", func(t *testing.T) {
		pins, err := pinstore.Pins(internal.NewInmemStorage().IndexStore())
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) != 0 {
			t.Fatalf("expected 0 pins, found %d", len(pins))
		}

		if pins == nil {
			t.Fatal("pins is nil")
		}
	})
}

func TestCleanup(t *testing.T) {
	t.Parallel()

	t.Run("cleanup putter", func(t *testing.T) {
		t.Parallel()

		st := newTestStorage(t)
		chunks := chunktest.GenerateTestRandomChunks(5)

		var (
			putter internal.PutterCloserWithReference
			err    error
		)
		err = st.Run(context.Background(), func(s transaction.Store) error {
			putter, err = pinstore.NewCollection(s.IndexStore())
			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		for _, ch := range chunks {
			err = st.Run(context.Background(), func(s transaction.Store) error {
				return putter.Put(context.Background(), s, ch)
			})
			if err != nil {
				t.Fatal(err)
			}
		}

		err = putter.Cleanup(st)
		if err != nil {
			t.Fatal(err)
		}

		for _, ch := range chunks {
			exists, err := st.ChunkStore().Has(context.Background(), ch.Address())
			if err != nil {
				t.Fatal(err)
			}
			if exists {
				t.Fatal("chunk should not exist")
			}
		}
	})

	t.Run("cleanup dirty", func(t *testing.T) {
		t.Parallel()

		st := newTestStorage(t)
		chunks := chunktest.GenerateTestRandomChunks(5)

		var (
			putter internal.PutterCloserWithReference
			err    error
		)
		err = st.Run(context.Background(), func(s transaction.Store) error {
			putter, err = pinstore.NewCollection(s.IndexStore())
			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		for _, ch := range chunks {
			err = st.Run(context.Background(), func(s transaction.Store) error {
				return putter.Put(context.Background(), s, ch)
			})
			if err != nil {
				t.Fatal(err)
			}
		}

		err = pinstore.CleanupDirty(st)
		if err != nil {
			t.Fatal(err)
		}

		for _, ch := range chunks {
			exists, err := st.ChunkStore().Has(context.Background(), ch.Address())
			if err != nil {
				t.Fatal(err)
			}
			if exists {
				t.Fatal("chunk should not exist")
			}
		}
	})
}

func TestPinCollectionItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       &pinstore.PinCollectionItem{},
			Factory:    func() storage.Item { return new(pinstore.PinCollectionItem) },
			MarshalErr: pinstore.ErrInvalidPinCollectionItemAddr,
		},
	}, {
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &pinstore.PinCollectionItem{
				Addr: swarm.ZeroAddress,
			},
			Factory:    func() storage.Item { return new(pinstore.PinCollectionItem) },
			MarshalErr: pinstore.ErrInvalidPinCollectionItemAddr,
		},
	}, {
		name: "zero UUID",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &pinstore.PinCollectionItem{
				Addr: swarm.NewAddress(storagetest.MinAddressBytes[:]),
			},
			Factory:    func() storage.Item { return new(pinstore.PinCollectionItem) },
			MarshalErr: pinstore.ErrInvalidPinCollectionItemUUID,
		},
	}, {
		name: "valid values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &pinstore.PinCollectionItem{
				Addr: swarm.NewAddress(storagetest.MinAddressBytes[:]),
				UUID: pinstore.NewUUID(),
			},
			Factory: func() storage.Item { return new(pinstore.PinCollectionItem) },
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &pinstore.PinCollectionItem{
				Addr: swarm.NewAddress(storagetest.MaxEncryptedRefBytes[:]),
				UUID: pinstore.NewUUID(),
				Stat: pinstore.CollectionStat{
					Total:           math.MaxUint64,
					DupInCollection: math.MaxUint64,
				},
			},
			Factory: func() storage.Item { return new(pinstore.PinCollectionItem) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(pinstore.PinCollectionItem) },
			UnmarshalErr: pinstore.ErrInvalidPinCollectionItemSize,
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

func TestPinChunkItem(t *testing.T) {
	t.Parallel()

	storagetest.TestItemClone(t, &storagetest.ItemCloneTest{
		Item: &pinstore.PinChunkItem{
			UUID: pinstore.NewUUID(),
			Addr: swarm.RandAddress(t),
		},
	})
}

func TestDirtyCollectionsItem(t *testing.T) {
	t.Parallel()

	storagetest.TestItemClone(t, &storagetest.ItemCloneTest{
		Item: &pinstore.DirtyCollection{
			UUID: pinstore.NewUUID(),
		},
	})
}
