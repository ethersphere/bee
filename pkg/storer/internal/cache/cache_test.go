// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storagetest"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/storer/internal/cache"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/go-cmp/cmp"
)

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
			Factory:    func() storage.Item { return new(cache.CacheEntry) },
			MarshalErr: cache.ErrMarshalCacheEntryInvalidTimestamp,
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &cache.CacheEntry{
				Address:         swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				AccessTimestamp: math.MaxInt64,
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

type timeProvider struct {
	t   int64
	mtx sync.Mutex
}

func (t *timeProvider) Now() func() time.Time {
	return func() time.Time {
		t.mtx.Lock()
		defer t.mtx.Unlock()
		t.t++
		return time.Unix(0, t.t)
	}
}

func TestMain(m *testing.M) {
	p := &timeProvider{t: time.Now().UnixNano()}
	done := cache.ReplaceTimeNow(p.Now())
	defer func() {
		done()
	}()
	code := m.Run()
	os.Exit(code)
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
				verifyCacheState(t, st.IndexStore(), c, state.Head, state.Tail, state.Size)
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
			if i.Namespace() == "cacheOrderIndex" {
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

func TestShallowCopy(t *testing.T) {
	t.Parallel()

	st := newTestStorage(t)
	c, err := cache.New(context.Background(), st, 10)
	if err != nil {
		t.Fatal(err)
	}

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

	err = c.ShallowCopy(context.Background(), st, chunksToMove...)
	if err != nil {
		t.Fatal(err)
	}

	verifyCacheState(t, st.IndexStore(), c, chunks[0].Address(), chunks[9].Address(), 10)
	verifyCacheOrder(t, c, st.IndexStore(), chunks...)

	// move again, should be no-op
	err = c.ShallowCopy(context.Background(), st, chunksToMove...)
	if err != nil {
		t.Fatal(err)
	}

	verifyCacheState(t, st.IndexStore(), c, chunks[0].Address(), chunks[9].Address(), 10)
	verifyCacheOrder(t, c, st.IndexStore(), chunks...)

	chunks1 := chunktest.GenerateTestRandomChunks(10)
	chunksToMove1 := make([]swarm.Address, 0, 10)

	// add the chunks to chunkstore. This simulates the reserve already populating
	// the chunkstore with chunks.
	for _, ch := range chunks1 {
		err := st.ChunkStore().Put(context.Background(), ch)
		if err != nil {
			t.Fatal(err)
		}
		chunksToMove1 = append(chunksToMove1, ch.Address())
	}

	// move new chunks
	err = c.ShallowCopy(context.Background(), st, chunksToMove1...)
	if err != nil {
		t.Fatal(err)
	}

	verifyCacheState(t, st.IndexStore(), c, chunks[0].Address(), chunks1[9].Address(), 20)
	verifyCacheOrder(t, c, st.IndexStore(), append(chunks, chunks1...)...)

	err = c.RemoveOldest(context.Background(), st, st.ChunkStore(), 10)
	if err != nil {
		t.Fatal(err)
	}

	verifyChunksDeleted(t, st.ChunkStore(), chunks...)
}

func TestShallowCopyOverCap(t *testing.T) {
	t.Parallel()

	st := newTestStorage(t)
	c, err := cache.New(context.Background(), st, 10)
	if err != nil {
		t.Fatal(err)
	}

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
	err = c.ShallowCopy(context.Background(), st, chunksToMove...)
	if err != nil {
		t.Fatal(err)
	}

	verifyCacheState(t, st.IndexStore(), c, chunks[5].Address(), chunks[14].Address(), 10)
	verifyCacheOrder(t, c, st.IndexStore(), chunks[5:15]...)

	err = c.RemoveOldest(context.Background(), st, st.ChunkStore(), 5)
	if err != nil {
		t.Fatal(err)
	}

	verifyChunksDeleted(t, st.ChunkStore(), chunks[5:10]...)
}

func TestShallowCopyAlreadyCached(t *testing.T) {
	t.Parallel()

	st := newTestStorage(t)
	c, err := cache.New(context.Background(), st, 1000)
	if err != nil {
		t.Fatal(err)
	}

	chunks := chunktest.GenerateTestRandomChunks(10)
	chunksToMove := make([]swarm.Address, 0, 10)

	for _, ch := range chunks {
		// add the chunks to chunkstore. This simulates the reserve already populating the chunkstore with chunks.
		err := st.ChunkStore().Put(context.Background(), ch)
		if err != nil {
			t.Fatal(err)
		}
		// already cached
		err = c.Putter(st).Put(context.Background(), ch)
		if err != nil {
			t.Fatal(err)
		}
		chunksToMove = append(chunksToMove, ch.Address())
	}

	// move new chunks
	err = c.ShallowCopy(context.Background(), st, chunksToMove...)
	if err != nil {
		t.Fatal(err)
	}

	verifyChunksExist(t, st.ChunkStore(), chunks...)

	err = c.RemoveOldest(context.Background(), st, st.ChunkStore(), 10)
	if err != nil {
		t.Fatal(err)
	}

	verifyChunksDeleted(t, st.ChunkStore(), chunks...)
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
	expState := cache.CacheState{Head: expStart, Tail: expEnd, Size: expCount}

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

	if uint64(len(chs)) != state.Size {
		t.Fatalf("unexpected count, exp: %d found: %d", state.Size, len(chs))
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

func verifyChunksExist(
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
		if !found {
			t.Fatalf("chunk %s expected to be found but not exists", ch.Address())
		}
	}
}
