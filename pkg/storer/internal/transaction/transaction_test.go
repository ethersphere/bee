// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestGetIntoLoc_LocksAddress(t *testing.T) {
	t.Parallel()

	sharkyStore, err := sharky.New(&dirFS{basedir: t.TempDir()}, 32, swarm.SocMaxChunkSize)
	assert.NoError(t, err)

	store, _, err := leveldbstore.New("", nil)
	assert.NoError(t, err)

	st := transaction.NewStorage(sharkyStore, store)
	t.Cleanup(func() {
		assert.NoError(t, st.Close())
	})

	ctx := context.Background()
	ch := test.GenerateTestRandomChunk()

	var loc storage.ChunkLocation
	err = st.Run(ctx, func(s transaction.Store) error {
		lp, ok := s.ChunkStore().(storage.LocatingPutter)
		assert.True(t, ok)
		var putErr error
		loc, putErr = lp.PutLoc(ctx, ch)
		return putErr
	})
	assert.NoError(t, err)

	sessionDone := st.StartSamplingSession()
	defer sessionDone()

	// Start a transaction that locks ch.Address()
	tx, txDone := st.NewTransaction(ctx)
	defer txDone()

	err = tx.ChunkStore().Delete(ctx, ch.Address())
	assert.NoError(t, err)

	// Call GetIntoLoc in a goroutine; it must block on c.lock(addr) until the transaction finishes.
	started := make(chan struct{})
	completed := make(chan struct{})
	lg, ok := st.ChunkStore().(storage.LocatingGetterInto)
	assert.True(t, ok)

	buf := make([]byte, swarm.SocMaxChunkSize)
	var (
		readErr error
		n       int
	)

	go func() {
		close(started)
		n, readErr = lg.GetIntoLoc(ctx, ch.Address(), loc, buf)
		close(completed)
	}()

	<-started

	// Assert that GetIntoLoc is blocked while tx holds the address lock
	select {
	case <-completed:
		t.Fatal("GetIntoLoc must block while address lock is held by active transaction")
	case <-time.After(100 * time.Millisecond):
	}

	// Release the address lock by finishing the transaction
	txDone()

	// Assert that GetIntoLoc now unblocks and succeeds
	select {
	case <-completed:
		if readErr != nil {
			t.Fatalf("expected no error, got %v", readErr)
		}
		if !bytes.Equal(ch.Data(), buf[:n]) {
			t.Fatal("chunk data mismatch")
		}
	case <-time.After(time.Second):
		t.Fatal("GetIntoLoc failed to unblock after transaction finished")
	}
}

func TestGetIntoLoc_ConcurrentDelete_UnderLock(t *testing.T) {
	t.Parallel()

	sharkyStore, err := sharky.New(&dirFS{basedir: t.TempDir()}, 32, swarm.SocMaxChunkSize)
	assert.NoError(t, err)

	store, _, err := leveldbstore.New("", nil)
	assert.NoError(t, err)

	st := transaction.NewStorage(sharkyStore, store)
	t.Cleanup(func() {
		assert.NoError(t, st.Close())
	})

	ctx := context.Background()
	ch := test.GenerateTestRandomChunk()

	var loc storage.ChunkLocation
	err = st.Run(ctx, func(s transaction.Store) error {
		lp, ok := s.ChunkStore().(storage.LocatingPutter)
		assert.True(t, ok)
		var putErr error
		loc, putErr = lp.PutLoc(ctx, ch)
		return putErr
	})
	assert.NoError(t, err)

	sessionDone := st.StartSamplingSession()
	defer sessionDone()

	// Start a transaction that deletes ch.Address()
	tx, txDone := st.NewTransaction(ctx)
	defer txDone()

	// Delete calls guard.MarkFreed(loc) under the address lock
	err = tx.ChunkStore().Delete(ctx, ch.Address())
	assert.NoError(t, err)

	started := make(chan struct{})
	completed := make(chan struct{})
	lg, ok := st.ChunkStore().(storage.LocatingGetterInto)
	assert.True(t, ok)

	buf := make([]byte, swarm.SocMaxChunkSize)
	var readErr error

	go func() {
		close(started)
		_, readErr = lg.GetIntoLoc(ctx, ch.Address(), loc, buf)
		close(completed)
	}()

	<-started

	select {
	case <-completed:
		t.Fatal("GetIntoLoc must block while address lock is held by active transaction")
	case <-time.After(100 * time.Millisecond):
	}

	// Commit the delete: batch is written, sharky slot is released, address is unlocked
	assert.NoError(t, tx.Commit())

	// Once unblocked, GetIntoLoc sees guard.IsFreed(loc) == true and falls back to index GetInto
	select {
	case <-completed:
		if !errors.Is(readErr, storage.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", readErr)
		}
	case <-time.After(time.Second):
		t.Fatal("GetIntoLoc failed to unblock after commit")
	}
}
