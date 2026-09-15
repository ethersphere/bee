// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	soctesting "github.com/ethersphere/bee/v2/pkg/soc/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestRetrievalIndexItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       &chunkstore.RetrievalIndexItem{},
			Factory:    func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
			MarshalErr: chunkstore.ErrMarshalInvalidRetrievalIndexItemAddress,
		},
	}, {
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.RetrievalIndexItem{
				Address: swarm.ZeroAddress,
			},
			Factory:    func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
			MarshalErr: chunkstore.ErrMarshalInvalidRetrievalIndexItemAddress,
		},
	}, {
		name: "min values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.RetrievalIndexItem{
				Address: swarm.NewAddress(storagetest.MinAddressBytes[:]),
			},
			Factory: func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.RetrievalIndexItem{
				Address:   swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				Timestamp: math.MaxUint64,
				Location: sharky.Location{
					Shard:  math.MaxUint8,
					Slot:   math.MaxUint32,
					Length: math.MaxUint16,
				},
				RefCnt: math.MaxUint8,
			},
			Factory: func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
			UnmarshalErr: chunkstore.ErrUnmarshalInvalidRetrievalIndexItemSize,
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

type memFS struct {
	afero.Fs
}

func (m *memFS) Open(path string) (fs.File, error) {
	return m.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
}

func TestChunkStore(t *testing.T) {
	t.Parallel()

	store := inmemstore.New()
	sharky, err := sharky.New(&memFS{Fs: afero.NewMemMapFs()}, 1, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}

	st := transaction.NewStorage(sharky, store)

	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("inmem store close failed: %v", err)
		}
	})

	testChunks := chunktest.GenerateTestRandomChunks(50)

	t.Run("put chunks", func(t *testing.T) {
		for _, ch := range testChunks {
			err := st.Run(context.Background(), func(s transaction.Store) error {
				return s.ChunkStore().Put(context.TODO(), ch)
			})
			if err != nil {
				t.Fatalf("failed putting new chunk: %v", err)
			}
		}
	})

	t.Run("put existing chunks", func(t *testing.T) {
		for idx, ch := range testChunks {
			// only put duplicates for odd numbered indexes
			if idx%2 != 0 {
				err := st.Run(context.Background(), func(s transaction.Store) error {
					return s.ChunkStore().Put(context.TODO(), ch)
				})
				if err != nil {
					t.Fatalf("failed putting new chunk: %v", err)
				}
			}
		}
	})

	t.Run("get chunks", func(t *testing.T) {
		for _, ch := range testChunks {
			readCh, err := st.ChunkStore().Get(context.TODO(), ch.Address())
			if err != nil {
				t.Fatalf("failed getting chunk: %v", err)
			}
			if !readCh.Equal(ch) {
				t.Fatal("read chunk doesn't match")
			}
		}
	})

	t.Run("has chunks", func(t *testing.T) {
		for _, ch := range testChunks {
			exists, err := st.ChunkStore().Has(context.TODO(), ch.Address())
			if err != nil {
				t.Fatalf("failed getting chunk: %v", err)
			}
			if !exists {
				t.Fatalf("chunk not found: %s", ch.Address())
			}
		}
	})

	t.Run("iterate chunks", func(t *testing.T) {
		count := 0
		err := chunkstore.Iterate(context.TODO(), store, sharky, func(_ swarm.Chunk) (bool, error) {
			count++
			return false, nil
		})
		if err != nil {
			t.Fatalf("unexpected error while iteration: %v", err)
		}
		if count != 50 {
			t.Fatalf("unexpected no of chunks, exp: %d, found: %d", 50, count)
		}
	})

	t.Run("iterate chunk entries", func(t *testing.T) {
		count, shared := 0, 0
		err := chunkstore.IterateChunkEntries(store, func(_ swarm.Address, cnt uint32) (bool, error) {
			count++
			if cnt > 1 {
				shared++
			}
			return false, nil
		})
		if err != nil {
			t.Fatalf("unexpected error while iteration: %v", err)
		}
		if count != 50 {
			t.Fatalf("unexpected no of chunks, exp: %d, found: %d", 50, count)
		}
		if shared != 25 {
			t.Fatalf("unexpected no of chunks, exp: %d, found: %d", 25, shared)
		}
	})

	t.Run("delete unique chunks", func(t *testing.T) {
		for idx, ch := range testChunks {
			// Delete all even numbered indexes along with 0
			if idx%2 == 0 {
				err := st.Run(context.Background(), func(s transaction.Store) error {
					return s.ChunkStore().Delete(context.TODO(), ch.Address())
				})
				if err != nil {
					t.Fatalf("failed deleting chunk: %v", err)
				}
			}
		}
	})

	t.Run("check deleted chunks", func(t *testing.T) {
		for idx, ch := range testChunks {
			if idx%2 == 0 {
				// Check even numbered indexes are deleted
				_, err := st.ChunkStore().Get(context.TODO(), ch.Address())
				if !errors.Is(err, storage.ErrNotFound) {
					t.Fatalf("expected storage not found error found: %v", err)
				}
				found, err := st.ChunkStore().Has(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("unexpected error in Has: %v", err)
				}
				if found {
					t.Fatal("expected chunk to not be found")
				}
			} else {
				// Check rest of the entries are intact
				readCh, err := st.ChunkStore().Get(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("failed getting chunk: %v", err)
				}
				if !readCh.Equal(ch) {
					t.Fatal("read chunk doesn't match")
				}
				exists, err := st.ChunkStore().Has(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("failed getting chunk: %v", err)
				}
				if !exists {
					t.Fatalf("chunk not found: %s", ch.Address())
				}
			}
		}
	})

	t.Run("iterate chunks after delete", func(t *testing.T) {
		count := 0
		err := chunkstore.Iterate(context.TODO(), store, sharky, func(_ swarm.Chunk) (bool, error) {
			count++
			return false, nil
		})
		if err != nil {
			t.Fatalf("unexpected error while iteration: %v", err)
		}
		if count != 25 {
			t.Fatalf("unexpected no of chunks, exp: %d, found: %d", 25, count)
		}
	})

	// due to refCnt 2, these chunks should be present in the store even after delete
	t.Run("delete duplicate chunks", func(t *testing.T) {
		for idx, ch := range testChunks {
			if idx%2 != 0 {
				err := st.Run(context.Background(), func(s transaction.Store) error {
					return s.ChunkStore().Delete(context.TODO(), ch.Address())
				})
				if err != nil {
					t.Fatalf("failed deleting chunk: %v", err)
				}
			}
		}
	})

	t.Run("check chunks still exists", func(t *testing.T) {
		for idx, ch := range testChunks {
			if idx%2 != 0 {
				readCh, err := st.ChunkStore().Get(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("failed getting chunk: %v", err)
				}
				if !readCh.Equal(ch) {
					t.Fatal("read chunk doesn't match")
				}
				exists, err := st.ChunkStore().Has(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("failed getting chunk: %v", err)
				}
				if !exists {
					t.Fatalf("chunk not found: %s", ch.Address())
				}
			}
		}
	})

	t.Run("delete duplicate chunks again", func(t *testing.T) {
		for idx, ch := range testChunks {
			if idx%2 != 0 {
				err := st.Run(context.Background(), func(s transaction.Store) error {
					return s.ChunkStore().Delete(context.TODO(), ch.Address())
				})
				if err != nil {
					t.Fatalf("failed deleting chunk: %v", err)
				}
			}
		}
	})

	t.Run("check all are deleted", func(t *testing.T) {
		count := 0
		err := chunkstore.Iterate(context.TODO(), store, sharky, func(_ swarm.Chunk) (bool, error) {
			count++
			return false, nil
		})
		if err != nil {
			t.Fatalf("unexpected error while iteration: %v", err)
		}
		if count != 0 {
			t.Fatalf("unexpected no of chunks, exp: %d, found: %d", 0, count)
		}
	})

	t.Run("replace chunk", func(t *testing.T) {
		privKey, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		signer := crypto.NewDefaultSigner(privKey)
		ctx := context.Background()

		ch1 := soctesting.GenerateMockSocWithSigner(t, []byte("data"), signer).Chunk()
		err = st.Run(context.Background(), func(s transaction.Store) error {
			return s.ChunkStore().Put(ctx, ch1)
		})
		if err != nil {
			t.Fatal(err)
		}

		tests := []struct {
			data         string
			emplace      bool
			wantRefCount uint32
		}{
			{
				data:         "data1",
				emplace:      true,
				wantRefCount: 2,
			},
			{
				data:         "data2",
				emplace:      false,
				wantRefCount: 2,
			},
			{
				data:         "data3",
				emplace:      true,
				wantRefCount: 3,
			},
		}

		for _, tt := range tests {
			ch2 := soctesting.GenerateMockSocWithSigner(t, []byte(tt.data), signer).Chunk()
			if !ch1.Address().Equal(ch2.Address()) {
				t.Fatal("chunk addresses don't match")
			}

			err = st.Run(ctx, func(s transaction.Store) error {
				return s.ChunkStore().Replace(ctx, ch2, tt.emplace)
			})
			if err != nil {
				t.Fatal(err)
			}

			ch, err := st.ChunkStore().Get(ctx, ch2.Address())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(ch.Data(), ch2.Data()) {
				t.Fatalf("expected data override")
			}

			rIdx := &chunkstore.RetrievalIndexItem{Address: ch2.Address()}
			err = st.IndexStore().Get(rIdx)
			if err != nil {
				t.Fatal(err)
			}

			if rIdx.RefCnt != tt.wantRefCount {
				t.Fatalf("expected ref count %d, got %d", tt.wantRefCount, rIdx.RefCnt)
			}
		}
	})

	t.Run("close store", func(t *testing.T) {
		err := st.Close()
		if err != nil {
			t.Fatalf("unexpected error during close: %v", err)
		}
	})
}

// TestIterateLocations asserts that all stored chunks
// are retrievable by sharky using IterateLocations.
func TestIterateLocations(t *testing.T) {
	t.Parallel()

	const chunksCount = 50

	st := makeStorage(t)
	testChunks := chunktest.GenerateTestRandomChunks(chunksCount)
	ctx := context.Background()

	for _, ch := range testChunks {
		assert.NoError(t, st.Run(context.Background(), func(s transaction.Store) error { return s.ChunkStore().Put(ctx, ch) }))
	}

	readCount := 0
	respC := chunkstore.IterateLocations(ctx, st.IndexStore())

	for resp := range respC {
		assert.NoError(t, resp.Err)

		buf := make([]byte, resp.Location.Length)
		assert.NoError(t, st.sharky.Read(ctx, resp.Location, buf))

		assert.True(t, swarm.ContainsChunkWithData(testChunks, buf))
		readCount++
	}

	assert.Equal(t, chunksCount, readCount)
}

// TestIterateLocations_Stop asserts that IterateLocations will
// stop iteration when context is canceled.
func TestIterateLocations_Stop(t *testing.T) {
	t.Parallel()

	const chunksCount = 50
	const stopReadAt = 10

	st := makeStorage(t)
	testChunks := chunktest.GenerateTestRandomChunks(chunksCount)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, ch := range testChunks {
		assert.NoError(t, st.Run(context.Background(), func(s transaction.Store) error { return s.ChunkStore().Put(ctx, ch) }))
	}

	readCount := 0
	respC := chunkstore.IterateLocations(ctx, st.IndexStore())

	for resp := range respC {
		if resp.Err != nil {
			assert.ErrorIs(t, resp.Err, context.Canceled)
			break
		}

		buf := make([]byte, resp.Location.Length)
		if err := st.sharky.Read(ctx, resp.Location, buf); err != nil {
			assert.ErrorIs(t, err, context.Canceled)
			break
		}

		assert.True(t, swarm.ContainsChunkWithData(testChunks, buf))
		readCount++

		if readCount == stopReadAt {
			cancel()
		}
	}

	assert.InDelta(t, stopReadAt, readCount, 1)
}

type chunkStore struct {
	transaction.Storage
	sharky *sharky.Store
}

func makeStorage(tb testing.TB) *chunkStore {
	tb.Helper()

	store := inmemstore.New()
	sharky, err := sharky.New(&memFS{Fs: afero.NewMemMapFs()}, 1, swarm.SocMaxChunkSize)
	assert.NoError(tb, err)

	tb.Cleanup(func() {
		assert.NoError(tb, store.Close())
		assert.NoError(tb, sharky.Close())
	})

	return &chunkStore{transaction.NewStorage(sharky, store), sharky}
}

// BenchmarkChunkStoreGet measures a single chunk read: one retrieval-index
// lookup followed by one sharky read. It is the micro-benchmark for the read
// path that the reserve sampler drives once per chunk.
//
// The two variants differ only in where the ChunkStore handle comes from.
// "per_call" mirrors what the sampler does today, building a fresh handle for
// every chunk; "hoisted" builds it once. The gap between them is the cost of
// that handle alone.
func BenchmarkChunkStoreGet(b *testing.B) {
	ctx := context.Background()

	setup := func(b *testing.B) (*chunkStore, swarm.Address) {
		b.Helper()

		st := makeStorage(b)
		ch := chunktest.GenerateTestRandomChunk()

		err := st.Run(ctx, func(s transaction.Store) error {
			return s.ChunkStore().Put(ctx, ch)
		})
		if err != nil {
			b.Fatal(err)
		}

		return st, ch.Address()
	}

	b.Run("per_call", func(b *testing.B) {
		st, addr := setup(b)

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			if _, err := st.ChunkStore().Get(ctx, addr); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("hoisted", func(b *testing.B) {
		st, addr := setup(b)
		cs := st.ChunkStore()

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			if _, err := cs.Get(ctx, addr); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("get_into", func(b *testing.B) {
		st, addr := setup(b)
		cs := st.ChunkStore()
		buf := make([]byte, swarm.SocMaxChunkSize)

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			if _, err := cs.GetInto(ctx, addr, buf); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestLocatingChunkStore(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	sharky, err := sharky.New(&memFS{Fs: fs}, 1, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}

	store := inmemstore.New()
	st := transaction.NewStorage(sharky, store)
	defer st.Close()

	ch := chunktest.GenerateTestRandomChunk()
	ctx := context.Background()

	var loc storage.ChunkLocation
	err = st.Run(ctx, func(s transaction.Store) error {
		lp, ok := s.ChunkStore().(storage.LocatingPutter)
		if !ok {
			return errors.New("chunkStore does not implement LocatingPutter")
		}
		var err error
		loc, err = lp.PutLoc(ctx, ch)
		return err
	})
	if err != nil {
		t.Fatalf("putLoc: %v", err)
	}
	if loc.IsZero() {
		t.Fatal("expected non-zero ChunkLocation from PutLoc")
	}

	cs := st.ChunkStore()
	lg, ok := cs.(storage.LocatingGetterInto)
	if !ok {
		t.Fatal("chunkStore does not implement LocatingGetterInto")
	}

	buf := make([]byte, swarm.SocMaxChunkSize)

	// 1. Without active session, GetIntoLoc falls back to standard GetInto safely
	n, err := lg.GetIntoLoc(ctx, ch.Address(), loc, buf)
	if err != nil {
		t.Fatalf("getIntoLoc without session: %v", err)
	}
	if !bytes.Equal(buf[:n], ch.Data()) {
		t.Fatal("chunk data does not match")
	}

	// 2. Fallback read via zero ChunkLocation
	n, err = lg.GetIntoLoc(ctx, ch.Address(), storage.ChunkLocation{}, buf)
	if err != nil {
		t.Fatalf("getIntoLoc zero: %v", err)
	}
	if !bytes.Equal(buf[:n], ch.Data()) {
		t.Fatal("chunk data does not match on zero location fallback")
	}

	// 3. With active session, direct read succeeds
	done := st.StartSamplingSession()

	n, err = lg.GetIntoLoc(ctx, ch.Address(), loc, buf)
	if err != nil {
		t.Fatalf("getIntoLoc with active session: %v", err)
	}
	if !bytes.Equal(buf[:n], ch.Data()) {
		t.Fatal("chunk data does not match with active session")
	}

	// 4. Guard active and chunk deleted: GetIntoLoc falls back and returns ErrNotFound
	err = st.Run(ctx, func(s transaction.Store) error {
		return s.ChunkStore().Delete(ctx, ch.Address())
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = lg.GetIntoLoc(ctx, ch.Address(), loc, buf)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for deleted chunk, got %v", err)
	}

	done()
}
