// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storagetest"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/storer/internal/cache"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/go-cmp/cmp"
)

func TestCacheStateItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:    &cache.CacheState{},
			Factory: func() storage.Item { return new(cache.CacheState) },
		},
	}, {
		name: "zero and non-zero 1",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &cache.CacheState{
				Tail: swarm.NewAddress(storagetest.MaxAddressBytes[:]),
			},
			Factory: func() storage.Item { return new(cache.CacheState) },
		},
	}, {
		name: "zero and non-zero 2",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &cache.CacheState{
				Head: swarm.NewAddress(storagetest.MaxAddressBytes[:]),
			},
			Factory: func() storage.Item { return new(cache.CacheState) },
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &cache.CacheState{
				Head:  swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				Tail:  swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				Count: math.MaxUint64,
			},
			Factory: func() storage.Item { return new(cache.CacheState) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(cache.CacheState) },
			UnmarshalErr: cache.ErrUnmarshalCacheStateInvalidSize,
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

func TestCacheEntryItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       &cache.CacheEntry{},
			Factory:    func() storage.Item { return new(cache.CacheEntry) },
			MarshalErr: cache.ErrMarshalCacheEntryInvalidAddress,
		},
	}, {
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &cache.CacheEntry{
				Address: swarm.NewAddress(storagetest.MaxAddressBytes[:]),
			},
			Factory: func() storage.Item { return new(cache.CacheEntry) },
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &cache.CacheEntry{
				Address: swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				Prev:    swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				Next:    swarm.NewAddress(storagetest.MaxAddressBytes[:]),
			},
			Factory: func() storage.Item { return new(cache.CacheEntry) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(cache.CacheEntry) },
			UnmarshalErr: cache.ErrUnmarshalCacheEntryInvalidSize,
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

type testStorage struct {
	internal.Storage
	putFn func(storage.Item) error
}

func (t *testStorage) IndexStore() storage.BatchedStore {
	return &wrappedStore{BatchedStore: t.Storage.IndexStore(), putFn: t.putFn}
}

type wrappedStore struct {
	storage.BatchedStore
	putFn func(storage.Item) error
}

func (w *wrappedStore) Put(i storage.Item) error {
	if w.putFn != nil {
		return w.putFn(i)
	}
	return w.BatchedStore.Put(i)
}

func (w *wrappedStore) Batch(ctx context.Context) (storage.Batch, error) {
	b, err := w.BatchedStore.Batch(ctx)
	if err != nil {
		return nil, err
	}
	return &wrappedBatch{Batch: b, putFn: w.putFn}, nil
}

type wrappedBatch struct {
	storage.Batch
	putFn func(storage.Item) error
}

func (w *wrappedBatch) Put(i storage.Item) error {
	if w.putFn != nil {
		return w.putFn(i)
	}
	return w.Batch.Put(i)
}

func newTestStorage(t *testing.T) *testStorage {
	t.Helper()

	storg, closer := internal.NewInmemStorage()
	t.Cleanup(func() {
		err := closer()
		if err != nil {
			t.Errorf("failed closing storage: %v", err)
		}
	})

	return &testStorage{Storage: storg}
}

func TestCache(t *testing.T) {
	t.Parallel()

	t.Run("fresh new cache", func(t *testing.T) {
		t.Parallel()

		st := newTestStorage(t)
		c, err := cache.New(context.TODO(), st, 10)
		if err != nil {
			t.Fatal(err)
		}
		verifyCacheState(t, st.IndexStore(), c, swarm.ZeroAddress, swarm.ZeroAddress, 0)
	})

	t.Run("putter", func(t *testing.T) {
		t.Parallel()

		st := newTestStorage(t)
		c, err := cache.New(context.TODO(), st, 10)
		if err != nil {
			t.Fatal(err)
		}

		chunks := chunktest.GenerateTestRandomChunks(10)

		t.Run("add till full", func(t *testing.T) {
			for idx, ch := range chunks {
				err := c.Putter(st).Put(context.TODO(), ch)
				if err != nil {
					t.Fatal(err)
				}
				verifyCacheState(t, st.IndexStore(), c, chunks[0].Address(), chunks[idx].Address(), uint64(idx+1))
				verifyCacheOrder(t, c, st.IndexStore(), chunks[:idx+1]...)
			}
		})

		t.Run("new cache retains state", func(t *testing.T) {
			c2, err := cache.New(context.TODO(), st, 10)
			if err != nil {
				t.Fatal(err)
			}
			verifyCacheState(t, st.IndexStore(), c2, chunks[0].Address(), chunks[len(chunks)-1].Address(), uint64(len(chunks)))
			verifyCacheOrder(t, c2, st.IndexStore(), chunks...)
		})

		chunks2 := chunktest.GenerateTestRandomChunks(10)

		t.Run("add over capacity", func(t *testing.T) {
			for idx, ch := range chunks2 {
				err := c.Putter(st).Put(context.TODO(), ch)
				if err != nil {
					t.Fatal(err)
				}
				if idx == len(chunks)-1 {
					verifyCacheState(t, st.IndexStore(), c, chunks2[0].Address(), chunks2[idx].Address(), 10)
					verifyCacheOrder(t, c, st.IndexStore(), chunks2...)
				} else {
					verifyCacheState(t, st.IndexStore(), c, chunks[idx+1].Address(), chunks2[idx].Address(), 10)
					verifyCacheOrder(t, c, st.IndexStore(), append(chunks[idx+1:], chunks2[:idx+1]...)...)
				}
			}
			verifyChunksDeleted(t, st.ChunkStore(), chunks...)
		})

		t.Run("new with lower capacity", func(t *testing.T) {
			c2, err := cache.New(context.TODO(), st, 5)
			if err != nil {
				t.Fatal(err)
			}
			verifyCacheState(t, st.IndexStore(), c2, chunks2[5].Address(), chunks2[len(chunks)-1].Address(), 5)
			verifyCacheOrder(t, c2, st.IndexStore(), chunks2[5:]...)
			verifyChunksDeleted(t, st.ChunkStore(), chunks[:5]...)
		})
	})

	t.Run("getter", func(t *testing.T) {
		t.Parallel()

		st := newTestStorage(t)
		c, err := cache.New(context.TODO(), st, 10)
		if err != nil {
			t.Fatal(err)
		}

		chunks := chunktest.GenerateTestRandomChunks(10)

		// this should have no effect on ordering
		t.Run("add and get last", func(t *testing.T) {
			for idx, ch := range chunks {
				err := c.Putter(st).Put(context.TODO(), ch)
				if err != nil {
					t.Fatal(err)
				}

				readChunk, err := c.Getter(st).Get(context.TODO(), ch.Address())
				if err != nil {
					t.Fatal(err)
				}
				if !readChunk.Equal(ch) {
					t.Fatalf("incorrect chunk: %s", ch.Address())
				}
				verifyCacheState(t, st.IndexStore(), c, chunks[0].Address(), chunks[idx].Address(), uint64(idx+1))
				verifyCacheOrder(t, c, st.IndexStore(), chunks[:idx+1]...)
			}
		})

		// getting the chunks in reverse order should reverse the ordering in the cache
		// at the end
		t.Run("get reverse order", func(t *testing.T) {
			var newOrder []swarm.Chunk
			for idx := len(chunks) - 1; idx >= 0; idx-- {
				readChunk, err := c.Getter(st).Get(context.TODO(), chunks[idx].Address())
				if err != nil {
					t.Fatal(err)
				}
				if !readChunk.Equal(chunks[idx]) {
					t.Fatalf("incorrect chunk: %s", chunks[idx].Address())
				}
				if idx == 0 {
					// once we access the first entry, the top will change
					verifyCacheState(t, st.IndexStore(), c, chunks[9].Address(), chunks[idx].Address(), 10)
				} else {
					verifyCacheState(t, st.IndexStore(), c, chunks[0].Address(), chunks[idx].Address(), 10)
				}
				newOrder = append(newOrder, chunks[idx])
			}
			verifyCacheOrder(t, c, st.IndexStore(), newOrder...)
		})

		t.Run("not in chunkstore returns error", func(t *testing.T) {
			for i := 0; i < 5; i++ {
				unknownChunk := chunktest.GenerateTestRandomChunk()
				_, err := c.Getter(st).Get(context.TODO(), unknownChunk.Address())
				if !errors.Is(err, storage.ErrNotFound) {
					t.Fatalf("expected error not found for chunk %s", unknownChunk.Address())
				}
			}
		})

		t.Run("not in cache doesnt affect state", func(t *testing.T) {
			state := c.State(st.IndexStore())

			for i := 0; i < 5; i++ {
				extraChunk := chunktest.GenerateTestRandomChunk()
				err := st.ChunkStore().Put(context.TODO(), extraChunk)
				if err != nil {
					t.Fatal(err)
				}
				readChunk, err := c.Getter(st).Get(context.TODO(), extraChunk.Address())
				if err != nil {
					t.Fatal(err)
				}
				if !readChunk.Equal(extraChunk) {
					t.Fatalf("incorrect chunk: %s", extraChunk.Address())
				}
				verifyCacheState(t, st.IndexStore(), c, state.Head, state.Tail, state.Count)
			}
		})
	})
	t.Run("handle error", func(t *testing.T) {
		t.Parallel()

		st := newTestStorage(t)
		c, err := cache.New(context.TODO(), st, 10)
		if err != nil {
			t.Fatal(err)
		}

		chunks := chunktest.GenerateTestRandomChunks(5)

		for _, ch := range chunks {
			err := c.Putter(st).Put(context.TODO(), ch)
			if err != nil {
				t.Fatal(err)
			}
		}
		// return error for state update, which occurs at the end of Get/Put operations
		retErr := errors.New("dummy error")
		st.putFn = func(i storage.Item) error {
			if i.Namespace() == "cacheState" {
				return retErr
			}
			return st.Storage.IndexStore().Put(i)
		}

		// on error the cache expects the overarching transactions to clean itself up
		// and undo any store updates. So here we only want to ensure the state is
		// reverted to correct one.
		t.Run("put error handling", func(t *testing.T) {
			newChunk := chunktest.GenerateTestRandomChunk()
			err := c.Putter(st).Put(context.TODO(), newChunk)
			if !errors.Is(err, retErr) {
				t.Fatalf("expected error %v during put, found %v", retErr, err)
			}

			// state should be preserved on failure
			verifyCacheState(t, st.IndexStore(), c, chunks[0].Address(), chunks[4].Address(), 5)
		})

		t.Run("get error handling", func(t *testing.T) {
			_, err := c.Getter(st).Get(context.TODO(), chunks[2].Address())
			if !errors.Is(err, retErr) {
				t.Fatalf("expected error %v during get, found %v", retErr, err)
			}

			// state should be preserved on failure
			verifyCacheState(t, st.IndexStore(), c, chunks[0].Address(), chunks[4].Address(), 5)
		})
	})
}

func TestMoveFromReserve(t *testing.T) {
	t.Parallel()

	st := newTestStorage(t)
	c, err := cache.New(context.Background(), st, 10)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("move from reserve", func(t *testing.T) {
		chunks := chunktest.GenerateTestRandomChunks(10)
		chunksToMove := make([]swarm.Address, 0, 10)

		// add the chunks to chunkstore. This simulates the reserve already populating
		// the chunkstore with chunks.
		for _, ch := range chunks {
			err := st.ChunkStore().Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
			chunksToMove = append(chunksToMove, ch.Address())
		}

		err = c.MoveFromReserve(context.Background(), st, chunksToMove...)
		if err != nil {
			t.Fatal(err)
		}

		verifyCacheState(t, st.IndexStore(), c, chunks[0].Address(), chunks[9].Address(), 10)
		verifyCacheOrder(t, c, st.IndexStore(), chunks...)

		// move again, should be no-op
		err = c.MoveFromReserve(context.Background(), st, chunksToMove...)
		if err != nil {
			t.Fatal(err)
		}

		verifyCacheState(t, st.IndexStore(), c, chunks[0].Address(), chunks[9].Address(), 10)
		verifyCacheOrder(t, c, st.IndexStore(), chunks...)
	})

	t.Run("move from reserve new chunks", func(t *testing.T) {
		chunks := chunktest.GenerateTestRandomChunks(10)
		chunksToMove := make([]swarm.Address, 0, 10)

		// add the chunks to chunkstore. This simulates the reserve already populating
		// the chunkstore with chunks.
		for _, ch := range chunks {
			err := st.ChunkStore().Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
			chunksToMove = append(chunksToMove, ch.Address())
		}

		// move new chunks
		err = c.MoveFromReserve(context.Background(), st, chunksToMove...)
		if err != nil {
			t.Fatal(err)
		}

		verifyCacheState(t, st.IndexStore(), c, chunks[0].Address(), chunks[9].Address(), 10)
		verifyCacheOrder(t, c, st.IndexStore(), chunks...)
	})

	t.Run("move from reserve over capacity", func(t *testing.T) {
		chunks := chunktest.GenerateTestRandomChunks(15)
		chunksToMove := make([]swarm.Address, 0, 15)

		// add the chunks to chunkstore. This simulates the reserve already populating
		// the chunkstore with chunks.
		for _, ch := range chunks {
			err := st.ChunkStore().Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
			chunksToMove = append(chunksToMove, ch.Address())
		}

		// move new chunks
		err = c.MoveFromReserve(context.Background(), st, chunksToMove...)
		if err != nil {
			t.Fatal(err)
		}

		verifyCacheState(t, st.IndexStore(), c, chunks[5].Address(), chunks[14].Address(), 10)
		verifyCacheOrder(t, c, st.IndexStore(), chunks[5:15]...)
	})
}

func verifyCacheState(
	t *testing.T,
	store storage.Store,
	c *cache.Cache,
	expStart, expEnd swarm.Address,
	expCount uint64,
) {
	t.Helper()

	state := c.State(store)
	expState := cache.CacheState{Head: expStart, Tail: expEnd, Count: expCount}

	if diff := cmp.Diff(expState, state); diff != "" {
		t.Fatalf("state mismatch (-want +have):\n%s", diff)
	}
}

func verifyCacheOrder(
	t *testing.T,
	c *cache.Cache,
	st storage.Store,
	chs ...swarm.Chunk,
) {
	t.Helper()

	state := c.State(st)

	if uint64(len(chs)) != state.Count {
		t.Fatalf("unexpected count, exp: %d found: %d", state.Count, len(chs))
	}

	idx := 0
	err := c.IterateOldToNew(st, state.Head, state.Tail, func(entry swarm.Address) (bool, error) {
		if !chs[idx].Address().Equal(entry) {
			return true, fmt.Errorf(
				"incorrect order of cache items, idx: %d exp: %s found: %s",
				idx, chs[idx].Address(), entry,
			)
		}
		idx++
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed at index %d err %s", idx, err)
	}
}

func verifyChunksDeleted(
	t *testing.T,
	chStore storage.ChunkStore,
	chs ...swarm.Chunk,
) {
	t.Helper()

	for _, ch := range chs {
		found, err := chStore.Has(context.TODO(), ch.Address())
		if err != nil {
			t.Fatal(err)
		}
		if found {
			t.Fatalf("chunk %s expected to not be found but exists", ch.Address())
		}
		_, err = chStore.Get(context.TODO(), ch.Address())
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("expected error %v but found %v", storage.ErrNotFound, err)
		}
	}
}
