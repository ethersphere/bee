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

	"github.com/ethersphere/bee/pkg/localstorev2/internal/cache"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore"
	inmem "github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})
	}
}

type testStorage struct {
	store   storage.Store
	chStore storage.ChunkStore
	putFn   func(storage.Item) error
}

func (testStorage) Ctx() context.Context { return context.TODO() }

func (t *testStorage) Store() storage.Store {
	return &wrappedStore{Store: t.store, putFn: t.putFn}
}

func (t *testStorage) ChunkStore() storage.ChunkStore {
	return t.chStore
}

type wrappedStore struct {
	storage.Store
	putFn func(storage.Item) error
}

func (w *wrappedStore) Put(i storage.Item) error {
	if w.putFn != nil {
		return w.putFn(i)
	}
	return w.Store.Put(i)
}

func TestCache(t *testing.T) {
	t.Parallel()

	t.Run("fresh new cache", func(t *testing.T) {
		t.Parallel()

		st := &testStorage{
			store:   inmem.New(),
			chStore: inmemchunkstore.New(),
		}
		c, err := cache.New(st, 10)
		if err != nil {
			t.Fatal(err)
		}
		verifyCacheState(t, c, swarm.ZeroAddress, swarm.ZeroAddress, 0)
	})

	t.Run("putter", func(t *testing.T) {
		t.Parallel()

		st := &testStorage{
			store:   inmem.New(),
			chStore: inmemchunkstore.New(),
		}
		c, err := cache.New(st, 10)
		if err != nil {
			t.Fatal(err)
		}

		chunks := chunktest.GenerateTestRandomChunks(10)

		t.Run("add till full", func(t *testing.T) {
			for idx, ch := range chunks {
				found, err := c.Putter(st.Store(), st.ChunkStore()).Put(context.TODO(), ch)
				if err != nil {
					t.Fatal(err)
				}
				if found {
					t.Fatalf("expected chunk %s to not be found", ch.Address())
				}
				verifyCacheState(t, c, chunks[0].Address(), chunks[idx].Address(), uint64(idx+1))
				verifyCacheOrder(t, c, st.Store(), chunks[:idx+1]...)
			}
		})

		t.Run("new cache retains state", func(t *testing.T) {
			c2, err := cache.New(st, 10)
			if err != nil {
				t.Fatal(err)
			}
			verifyCacheState(t, c2, chunks[0].Address(), chunks[len(chunks)-1].Address(), uint64(len(chunks)))
			verifyCacheOrder(t, c2, st.Store(), chunks...)
		})

		chunks2 := chunktest.GenerateTestRandomChunks(10)

		t.Run("add over capacity", func(t *testing.T) {
			for idx, ch := range chunks2 {
				found, err := c.Putter(st.Store(), st.ChunkStore()).Put(context.TODO(), ch)
				if err != nil {
					t.Fatal(err)
				}
				if found {
					t.Fatalf("expected chunk %s to not be found", ch.Address())
				}
				if idx == len(chunks)-1 {
					verifyCacheState(t, c, chunks2[0].Address(), chunks2[idx].Address(), 10)
					verifyCacheOrder(t, c, st.Store(), chunks2...)
				} else {
					verifyCacheState(t, c, chunks[idx+1].Address(), chunks2[idx].Address(), 10)
					verifyCacheOrder(t, c, st.Store(), append(chunks[idx+1:], chunks2[:idx+1]...)...)
				}
			}
			verifyChunksDeleted(t, st.ChunkStore(), chunks...)
		})

		t.Run("new with lower capacity", func(t *testing.T) {
			c2, err := cache.New(st, 5)
			if err != nil {
				t.Fatal(err)
			}
			verifyCacheState(t, c2, chunks2[5].Address(), chunks2[len(chunks)-1].Address(), 5)
			verifyCacheOrder(t, c2, st.Store(), chunks2[5:]...)
			verifyChunksDeleted(t, st.ChunkStore(), chunks[:5]...)
		})
	})

	t.Run("getter", func(t *testing.T) {
		t.Parallel()

		st := &testStorage{
			store:   inmem.New(),
			chStore: inmemchunkstore.New(),
		}
		c, err := cache.New(st, 10)
		if err != nil {
			t.Fatal(err)
		}

		chunks := chunktest.GenerateTestRandomChunks(10)

		// this should have no effect on ordering
		t.Run("add and get last", func(t *testing.T) {
			for idx, ch := range chunks {
				found, err := c.Putter(st.Store(), st.ChunkStore()).Put(context.TODO(), ch)
				if err != nil {
					t.Fatal(err)
				}
				if found {
					t.Fatalf("expected chunk %s to not be found", ch.Address())
				}

				readChunk, err := c.Getter(st.Store(), st.ChunkStore()).Get(context.TODO(), ch.Address())
				if err != nil {
					t.Fatal(err)
				}
				if !readChunk.Equal(ch) {
					t.Fatalf("incorrect chunk: %s", ch.Address())
				}
				verifyCacheState(t, c, chunks[0].Address(), chunks[idx].Address(), uint64(idx+1))
				verifyCacheOrder(t, c, st.Store(), chunks[:idx+1]...)
			}
		})

		// getting the chunks in reverse order should reverse the ordering in the cache
		// at the end
		t.Run("get reverse order", func(t *testing.T) {
			var newOrder []swarm.Chunk
			for idx := len(chunks) - 1; idx >= 0; idx-- {
				readChunk, err := c.Getter(st.Store(), st.ChunkStore()).Get(context.TODO(), chunks[idx].Address())
				if err != nil {
					t.Fatal(err)
				}
				if !readChunk.Equal(chunks[idx]) {
					t.Fatalf("incorrect chunk: %s", chunks[idx].Address())
				}
				if idx == 0 {
					// once we access the first entry, the top will change
					verifyCacheState(t, c, chunks[9].Address(), chunks[idx].Address(), 10)
				} else {
					verifyCacheState(t, c, chunks[0].Address(), chunks[idx].Address(), 10)
				}
				newOrder = append(newOrder, chunks[idx])
			}
			verifyCacheOrder(t, c, st.Store(), newOrder...)
		})

		t.Run("not in chunkstore returns error", func(t *testing.T) {
			for i := 0; i < 5; i++ {
				unknownChunk := chunktest.GenerateTestRandomChunk()
				_, err := c.Getter(st.Store(), st.ChunkStore()).Get(context.TODO(), unknownChunk.Address())
				if !errors.Is(err, storage.ErrNotFound) {
					t.Fatalf("expected error not found for chunk %s", unknownChunk.Address())
				}
			}
		})

		t.Run("not in cache doesnt affect state", func(t *testing.T) {
			state := c.State()

			for i := 0; i < 5; i++ {
				extraChunk := chunktest.GenerateTestRandomChunk()
				_, err := st.ChunkStore().Put(context.TODO(), extraChunk)
				if err != nil {
					t.Fatal(err)
				}
				readChunk, err := c.Getter(st.Store(), st.ChunkStore()).Get(context.TODO(), extraChunk.Address())
				if err != nil {
					t.Fatal(err)
				}
				if !readChunk.Equal(extraChunk) {
					t.Fatalf("incorrect chunk: %s", extraChunk.Address())
				}
				verifyCacheState(t, c, state.Head, state.Tail, state.Count)
			}
		})
	})
	t.Run("handle error", func(t *testing.T) {
		t.Parallel()

		st := &testStorage{
			store:   inmem.New(),
			chStore: inmemchunkstore.New(),
		}
		c, err := cache.New(st, 10)
		if err != nil {
			t.Fatal(err)
		}

		chunks := chunktest.GenerateTestRandomChunks(5)

		for _, ch := range chunks {
			found, err := c.Putter(st.Store(), st.ChunkStore()).Put(context.TODO(), ch)
			if err != nil {
				t.Fatal(err)
			}
			if found {
				t.Fatalf("expected chunk %s to not be found", ch.Address())
			}
		}
		// return error for state update, which occurs at the end of Get/Put operations
		retErr := errors.New("dummy error")
		st.putFn = func(i storage.Item) error {
			if i.Namespace() == "cacheState" {
				return retErr
			}
			return st.store.Put(i)
		}

		// on error the cache expects the overarching transactions to clean itself up
		// and undo any store updates. So here we only want to ensure the state is
		// reverted to correct one.
		t.Run("put error handling", func(t *testing.T) {
			newChunk := chunktest.GenerateTestRandomChunk()
			_, err := c.Putter(st.Store(), st.ChunkStore()).Put(context.TODO(), newChunk)
			if !errors.Is(err, retErr) {
				t.Fatalf("expected error %v during put, found %v", retErr, err)
			}

			// state should be preserved on failure
			verifyCacheState(t, c, chunks[0].Address(), chunks[4].Address(), 5)
		})

		t.Run("get error handling", func(t *testing.T) {
			_, err := c.Getter(st.Store(), st.ChunkStore()).Get(context.TODO(), chunks[2].Address())
			if !errors.Is(err, retErr) {
				t.Fatalf("expected error %v during get, found %v", retErr, err)
			}

			// state should be preserved on failure
			verifyCacheState(t, c, chunks[0].Address(), chunks[4].Address(), 5)
		})
	})
}

func verifyCacheState(
	t *testing.T,
	c *cache.Cache,
	expStart, expEnd swarm.Address,
	expCount uint64,
) {
	t.Helper()

	state := c.State()
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

	state := c.State()

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
