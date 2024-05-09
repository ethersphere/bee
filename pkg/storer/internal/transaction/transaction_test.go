// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
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
)

type dirFS struct {
	basedir string
}

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0644)
}

func Test_TransactionStorage(t *testing.T) {
	t.Parallel()

	sharkyStore, err := sharky.New(&dirFS{basedir: t.TempDir()}, 32, swarm.SocMaxChunkSize)
	assert.NoError(t, err)

	store, err := leveldbstore.New("", nil)
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
