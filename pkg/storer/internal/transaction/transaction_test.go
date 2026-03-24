// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"context"
	"io"
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

// mockCrashFile simulates a file that buffers writes in memory and only persists them when Sync() is called.
type mockCrashFile struct {
	fs.File // default unimplemented
	name    string
	data    []byte // "persisted" to disk
	dirty   []byte // "volatile" in OS page cache
}

func newMockCrashFile(name string, d []byte) *mockCrashFile {
	return &mockCrashFile{name: name, data: d, dirty: append([]byte(nil), d...)}
}

func (m *mockCrashFile) WriteAt(p []byte, off int64) (int, error) {
	end := int(off) + len(p)
	if end > len(m.dirty) {
		newDirty := make([]byte, end)
		copy(newDirty, m.dirty)
		m.dirty = newDirty
	}
	copy(m.dirty[off:], p)
	return len(p), nil
}

func (m *mockCrashFile) ReadAt(p []byte, off int64) (int, error) {
	if int(off) >= len(m.dirty) {
		return 0, os.ErrClosed
	}
	n := copy(p, m.dirty[off:])
	return n, nil
}

func (m *mockCrashFile) Sync() error {
	m.data = append([]byte(nil), m.dirty...)
	return nil
}

func (m *mockCrashFile) Close() error {
	return nil
}

// Read is called by io.ReadAll in slots.load() during sharky.New().
// Return EOF immediately to simulate an empty (newly created) slots file.
func (m *mockCrashFile) Read(p []byte) (n int, err error)             { return 0, io.EOF }
func (m *mockCrashFile) Write(p []byte) (n int, err error)            { panic("not impl") }
func (m *mockCrashFile) Seek(offset int64, whence int) (int64, error) { panic("not impl") }
func (m *mockCrashFile) Truncate(size int64) error                    { panic("not impl") }

// mockCrashFS simulates a filesystem that retains mocked files.
type mockCrashFS struct {
	files map[string]*mockCrashFile
}

func (fsys *mockCrashFS) Open(name string) (fs.File, error) {
	if fsys.files == nil {
		fsys.files = make(map[string]*mockCrashFile)
	}
	if f, ok := fsys.files[name]; ok {
		return f, nil
	}
	f := newMockCrashFile(name, nil)
	fsys.files[name] = f
	return f, nil
}

func Test_TransactionCrashCorruption(t *testing.T) {
	t.Parallel()

	// 1. Setup mock FS and LevelDB
	fsys := &mockCrashFS{files: make(map[string]*mockCrashFile)}
	sharkyStore, err := sharky.New(fsys, 1, swarm.SocMaxChunkSize)
	assert.NoError(t, err)

	dbStore, err := leveldbstore.New("", nil) // in-memory leveldb
	assert.NoError(t, err)

	st := transaction.NewStorage(sharkyStore, dbStore)

	// 2. Put a chunk using a transaction
	tx, done := st.NewTransaction(context.Background())
	defer done()

	ch1 := test.GenerateTestRandomChunk()
	assert.NoError(t, tx.ChunkStore().Put(context.Background(), ch1))

	// 3. Commit the transaction (this writes metadata to LevelDB)
	assert.NoError(t, tx.Commit())

	// 4. Simulate a crash!
	// The OS page cache drops unsynced dirty bytes.
	for _, f := range fsys.files {
		f.dirty = append([]byte(nil), f.data...) // Revert dirty to persisted
	}

	// 5. A committed chunk must survive a simulated crash.
	// On master (no SyncWait): Sharky never fsynced, dirty was dropped, Get fails → test FAILS.
	// On the fix branch (SyncWait calls Sync): dirty was persisted to data before commit,
	// so crash simulation leaves data intact and Get succeeds → test PASSES.
	got, err := st.ChunkStore().Get(context.Background(), ch1.Address())
	if err != nil {
		t.Fatalf("BUG: committed chunk unreadable after crash – Sharky was not synced before LevelDB commit: %v", err)
	}
	assert.Equal(t, ch1.Data(), got.Data(), "chunk data must survive a crash when Sharky is synced before LevelDB commit")
}
