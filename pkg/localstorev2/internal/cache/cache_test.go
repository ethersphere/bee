package cache_test

import (
	"context"
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
				End: swarm.NewAddress(storagetest.MaxAddressBytes[:]),
			},
			Factory: func() storage.Item { return new(cache.CacheState) },
		},
	}, {
		name: "zero and non-zero 2",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &cache.CacheState{
				Start: swarm.NewAddress(storagetest.MaxAddressBytes[:]),
			},
			Factory: func() storage.Item { return new(cache.CacheState) },
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &cache.CacheState{
				Start: swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				End:   swarm.NewAddress(storagetest.MaxAddressBytes[:]),
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
}

func (testStorage) Ctx() context.Context { return context.TODO() }

func (t *testStorage) Store() storage.Store {
	return t.store
}

func (t *testStorage) ChunkStore() storage.ChunkStore {
	return t.chStore
}

func TestCache(t *testing.T) {
	t.Parallel()

	t.Run("fresh new cache", func(t *testing.T) {
		st := &testStorage{
			store:   inmem.New(),
			chStore: inmemchunkstore.New(),
		}
		_, err := cache.New(st, 10)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("putter", func(t *testing.T) {
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
			}
		})

		t.Run("new cache retains state", func(t *testing.T) {
			c2, err := cache.New(st, 10)
			if err != nil {
				t.Fatal(err)
			}
			verifyCacheState(t, c2, chunks[0].Address(), chunks[len(chunks)-1].Address(), uint64(len(chunks)))
		})

		t.Run("add over capacity", func(t *testing.T) {
			chunks2 := chunktest.GenerateTestRandomChunks(10)
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
				} else {
					verifyCacheState(t, c, chunks[idx+1].Address(), chunks2[idx].Address(), 10)
				}
			}
		})
	})

	t.Run("getter", func(t *testing.T) {
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

	})
}

func verifyCacheState(
	t *testing.T,
	c *cache.Cache,
	expStart, expEnd swarm.Address,
	expCount uint64,
) {
	t.Helper()

	start, end, count := c.State()

	if !start.Equal(expStart) {
		t.Fatalf("unexpected start, exp: %s found: %s", expStart, start)
	}

	if !end.Equal(expEnd) {
		t.Fatalf("unexpected end, exp: %s found: %s", expEnd, end)
	}

	if expCount != count {
		t.Fatalf("unexpected count, exp: %d found: %d", expCount, count)
	}
}

func verifyCacheOrder(
	t *testing.T,
	c *cache.Cache,
	st storage.Store,
	chs ...swarm.Chunk,
) {
	t.Helper()

	start, end, count := c.State()

	if uint64(len(chs)) != count {
		t.Fatalf("unexpected count, exp: %d found: %d", count, len(chs))
	}

	idx := 0
	err := c.IterateOldToNew(st, start, end, func(entry swarm.Address) (bool, error) {
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
