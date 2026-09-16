// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	test "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/cache"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type dirFS struct {
	basedir string
}

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0o644)
}

func Test_TransactionStorage(t *testing.T) {
	t.Parallel()

	sharkyStore, err := sharky.New(&dirFS{basedir: t.TempDir()}, 32, swarm.SocMaxChunkSize)
	assert.NoError(t, err)

	store, _, err := leveldbstore.New("", nil)
	assert.NoError(t, err)

	st := transaction.NewStorage(sharkyStore, store)
	t.Cleanup(func() {
		assert.NoError(t, st.Close())
	})

	t.Run("put", func(t *testing.T) {
		t.Parallel()

		tx, done := st.NewTransaction(context.Background())
		defer done()

		ch1 := test.GenerateTestRandomChunk()
		ch2 := test.GenerateTestRandomChunk()

		assert.NoError(t, tx.IndexStore().Put(&cache.CacheEntryItem{Address: ch1.Address(), AccessTimestamp: 1}))
		assert.NoError(t, tx.ChunkStore().Put(context.Background(), ch1))
		assert.NoError(t, tx.IndexStore().Put(&cache.CacheEntryItem{Address: ch2.Address(), AccessTimestamp: 1}))
		assert.NoError(t, tx.ChunkStore().Put(context.Background(), ch2))
		assert.NoError(t, tx.Commit())

		item := cache.CacheEntryItem{Address: ch1.Address()}
		assert.NoError(t, st.IndexStore().Get(&item))
		assert.Equal(t, item, cache.CacheEntryItem{Address: ch1.Address(), AccessTimestamp: 1})

		ch1_get, err := st.ChunkStore().Get(context.Background(), ch1.Address())
		assert.NoError(t, err)
		assert.Equal(t, ch1.Data(), ch1_get.Data())
		assert.Equal(t, ch1.Address(), ch1_get.Address())

		item = cache.CacheEntryItem{Address: ch2.Address()}
		assert.NoError(t, st.IndexStore().Get(&item))
		assert.Equal(t, item, cache.CacheEntryItem{Address: ch2.Address(), AccessTimestamp: 1})

		ch2_get, err := st.ChunkStore().Get(context.Background(), ch1.Address())
		assert.NoError(t, err)
		assert.Equal(t, ch1.Data(), ch2_get.Data())
		assert.Equal(t, ch1.Address(), ch2_get.Address())
	})

	t.Run("put-forget commit", func(t *testing.T) {
		t.Parallel()

		tx, done := st.NewTransaction(context.Background())

		ch1 := test.GenerateTestRandomChunk()
		ch2 := test.GenerateTestRandomChunk()

		assert.NoError(t, tx.IndexStore().Put(&cache.CacheEntryItem{Address: ch1.Address(), AccessTimestamp: 1}))
		assert.NoError(t, tx.ChunkStore().Put(context.Background(), ch1))
		assert.NoError(t, tx.IndexStore().Put(&cache.CacheEntryItem{Address: ch2.Address(), AccessTimestamp: 1}))
		assert.NoError(t, tx.ChunkStore().Put(context.Background(), ch2))

		done()

		assert.ErrorIs(t, st.IndexStore().Get(&cache.CacheEntryItem{Address: ch1.Address()}), storage.ErrNotFound)
		assert.ErrorIs(t, st.IndexStore().Get(&cache.CacheEntryItem{Address: ch2.Address()}), storage.ErrNotFound)
		_, err := st.ChunkStore().Get(context.Background(), ch1.Address())
		assert.ErrorIs(t, err, storage.ErrNotFound)
		_, err = st.ChunkStore().Get(context.Background(), ch2.Address())
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("put-delete", func(t *testing.T) {
		t.Parallel()

		ch1 := test.GenerateTestRandomChunk()
		ch2 := test.GenerateTestRandomChunk()

		_ = st.Run(context.Background(), func(s transaction.Store) error {
			assert.NoError(t, s.IndexStore().Put(&cache.CacheEntryItem{Address: ch1.Address(), AccessTimestamp: 1}))
			assert.NoError(t, s.ChunkStore().Put(context.Background(), ch1))
			assert.NoError(t, s.IndexStore().Put(&cache.CacheEntryItem{Address: ch2.Address(), AccessTimestamp: 1}))
			assert.NoError(t, s.ChunkStore().Put(context.Background(), ch2))
			return nil
		})

		item := cache.CacheEntryItem{Address: ch1.Address()}
		assert.NoError(t, st.IndexStore().Get(&item))
		assert.Equal(t, item, cache.CacheEntryItem{Address: ch1.Address(), AccessTimestamp: 1})

		ch1_get, err := st.ChunkStore().Get(context.Background(), ch1.Address())
		assert.NoError(t, err)
		assert.Equal(t, ch1.Data(), ch1_get.Data())
		assert.Equal(t, ch1.Address(), ch1_get.Address())

		item = cache.CacheEntryItem{Address: ch2.Address()}
		assert.NoError(t, st.IndexStore().Get(&item))
		assert.Equal(t, item, cache.CacheEntryItem{Address: ch2.Address(), AccessTimestamp: 1})

		ch2_get, err := st.ChunkStore().Get(context.Background(), ch1.Address())
		assert.NoError(t, err)
		assert.Equal(t, ch1.Data(), ch2_get.Data())
		assert.Equal(t, ch1.Address(), ch2_get.Address())

		_ = st.Run(context.Background(), func(s transaction.Store) error {
			assert.NoError(t, s.IndexStore().Delete(&cache.CacheEntryItem{Address: ch1.Address(), AccessTimestamp: 1}))
			assert.NoError(t, s.ChunkStore().Delete(context.Background(), ch1.Address()))
			assert.NoError(t, s.IndexStore().Delete(&cache.CacheEntryItem{Address: ch2.Address(), AccessTimestamp: 1}))
			assert.NoError(t, s.ChunkStore().Delete(context.Background(), ch2.Address()))
			return nil
		})

		assert.ErrorIs(t, st.IndexStore().Get(&cache.CacheEntryItem{Address: ch1.Address()}), storage.ErrNotFound)
		assert.ErrorIs(t, st.IndexStore().Get(&cache.CacheEntryItem{Address: ch2.Address()}), storage.ErrNotFound)
		_, err = st.ChunkStore().Get(context.Background(), ch1.Address())
		assert.ErrorIs(t, err, storage.ErrNotFound)
		_, err = st.ChunkStore().Get(context.Background(), ch2.Address())
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("put-delete-chunk", func(t *testing.T) {
		t.Parallel()

		ch1 := test.GenerateTestRandomChunk()

		_ = st.Run(context.Background(), func(s transaction.Store) error {
			assert.NoError(t, s.ChunkStore().Put(context.Background(), ch1))
			assert.NoError(t, s.ChunkStore().Put(context.Background(), ch1))
			assert.NoError(t, s.ChunkStore().Delete(context.Background(), ch1.Address()))
			return nil
		})

		has, err := st.ChunkStore().Has(context.Background(), ch1.Address())
		assert.NoError(t, err)
		if !has {
			t.Fatal("should have chunk")
		}
	})

	t.Run("put-delete-chunk-twice", func(t *testing.T) {
		t.Parallel()

		ch1 := test.GenerateTestRandomChunk()

		_ = st.Run(context.Background(), func(s transaction.Store) error {
			assert.NoError(t, s.ChunkStore().Put(context.Background(), ch1))
			assert.NoError(t, s.ChunkStore().Put(context.Background(), ch1))
			assert.NoError(t, s.ChunkStore().Delete(context.Background(), ch1.Address()))
			assert.NoError(t, s.ChunkStore().Delete(context.Background(), ch1.Address()))
			return nil
		})

		has, err := st.ChunkStore().Has(context.Background(), ch1.Address())
		assert.NoError(t, err)
		if !has {
			t.Fatal("should NOT have chunk")
		}
	})
}

// TestChunkStoreDontFillCache proves that chunk reads made through a chunk
// store derived with storage.WithDontFillCache leave the block cache of the
// index store as it is, while the plain chunk store populates it.
func TestChunkStoreDontFillCache(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	sharkyStore, err := sharky.New(&dirFS{basedir: t.TempDir()}, 32, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatalf("create sharky: %v", err)
	}

	store, _, err := leveldbstore.New(t.TempDir(), &opt.Options{
		BlockCacheCapacity:  1024 * 1024,
		BlockSize:           1024,
		CompactionTableSize: 64 * 1024,
	})
	if err != nil {
		t.Fatalf("create index store: %v", err)
	}

	st := transaction.NewStorage(sharkyStore, store)
	t.Cleanup(func() {
		assert.NoError(t, st.Close())
	})

	const numChunks = 500
	chunks := make([]swarm.Chunk, numChunks)
	err = st.Run(ctx, func(s transaction.Store) error {
		for i := range chunks {
			chunks[i] = test.GenerateTestRandomChunk()
			if err := s.ChunkStore().Put(ctx, chunks[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("put chunks: %v", err)
	}
	// Flush the memtable into tables so that reads go through the block cache.
	if err := store.DB().CompactRange(util.Range{}); err != nil {
		t.Fatalf("compact: %v", err)
	}

	blockCacheSize := func() int {
		t.Helper()
		var stats leveldb.DBStats
		if err := store.DB().Stats(&stats); err != nil {
			t.Fatalf("stats: %v", err)
		}
		return stats.BlockCacheSize
	}

	readAll := func(cs storage.ReadOnlyChunkStore) {
		t.Helper()
		for _, ch := range chunks {
			got, err := cs.Get(ctx, ch.Address())
			if err != nil {
				t.Fatalf("get %s: %v", ch.Address(), err)
			}
			if !bytes.Equal(got.Data(), ch.Data()) {
				t.Fatalf("get %s: data mismatch", ch.Address())
			}
		}
	}

	noFill := st.ChunkStore(storage.WithDontFillCache())

	// The first pass still caches the index blocks of every table,
	// the second pass must not add anything on top of that.
	readAll(noFill)
	afterFirstPass := blockCacheSize()
	readAll(noFill)
	afterSecondPass := blockCacheSize()
	if afterSecondPass != afterFirstPass {
		t.Fatalf("no-fill reads changed the block cache size: %d -> %d", afterFirstPass, afterSecondPass)
	}

	readAll(st.ChunkStore())
	afterFillPass := blockCacheSize()
	if afterFillPass <= afterSecondPass {
		t.Fatalf("plain reads did not add data blocks to the block cache: %d -> %d", afterSecondPass, afterFillPass)
	}
}
