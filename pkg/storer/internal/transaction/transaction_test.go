// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	test "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/cache"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
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

// TestErrAfterCommit checks that only an error reported after the index
// batch was written carries ErrAfterCommit: the writes of such a transaction
// are persisted.
func TestErrAfterCommit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sharkyStore, err := sharky.New(&dirFS{basedir: t.TempDir()}, 1, swarm.SocMaxChunkSize)
	assert.NoError(t, err)
	store, _, err := leveldbstore.New("", nil)
	assert.NoError(t, err)
	st := transaction.NewStorage(sharkyStore, store)
	t.Cleanup(func() {
		assert.NoError(t, st.Close())
	})

	t.Run("release after commit", func(t *testing.T) {
		t.Parallel()

		// A retrieval index entry pointing at a shard that doesn't exist:
		// deleting the chunk releases that location after the batch commit,
		// and the release fails.
		addr := swarm.RandAddress(t)
		err := store.Put(&chunkstore.RetrievalIndexItem{
			Address:  addr,
			Location: sharky.Location{Shard: 200, Slot: 1, Length: 10},
			RefCnt:   1,
		})
		assert.NoError(t, err)

		err = st.Run(ctx, func(s transaction.Store) error {
			return s.ChunkStore().Delete(ctx, addr)
		})
		if !errors.Is(err, transaction.ErrAfterCommit) {
			t.Fatalf("got %v, want an error carrying ErrAfterCommit", err)
		}
		if !errors.Is(err, sharky.ErrShardNotFound) {
			t.Fatalf("got %v, want the release error to be kept", err)
		}
		has, err := st.IndexStore().Has(&chunkstore.RetrievalIndexItem{Address: addr})
		assert.NoError(t, err)
		if has {
			t.Fatal("the transaction's delete was not committed")
		}
	})

	t.Run("callback error", func(t *testing.T) {
		t.Parallel()

		errTest := errors.New("test error")
		item := &cache.CacheEntryItem{Address: swarm.RandAddress(t), AccessTimestamp: 1}
		err := st.Run(ctx, func(s transaction.Store) error {
			assert.NoError(t, s.IndexStore().Put(item))
			return errTest
		})
		if !errors.Is(err, errTest) || errors.Is(err, transaction.ErrAfterCommit) {
			t.Fatalf("got %v, want the callback error without ErrAfterCommit", err)
		}
		has, err := st.IndexStore().Has(item)
		assert.NoError(t, err)
		if has {
			t.Fatal("the failed transaction was committed")
		}
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		err := st.Run(ctx, func(s transaction.Store) error {
			return s.ChunkStore().Put(ctx, test.GenerateTestRandomChunk())
		})
		assert.NoError(t, err)
	})
}
