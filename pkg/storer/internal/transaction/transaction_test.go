// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"bytes"
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

// mockCrashFile simulates a file that buffers writes in memory (page cache) and only
// persists them to "disk" (data) when Sync() is called. Crash() discards all unsynced
// dirty bytes, simulating an OS crash that drops the page cache.
type mockCrashFile struct {
	fs.File // default unimplemented
	name    string
	data    []byte // "persisted" to disk — only updated by Sync()
	dirty   []byte // "volatile" OS page cache — updated by writes
	pos     int64  // sequential read/write cursor (used by Read/Write/Seek)
}

func newMockCrashFile(name string, d []byte) *mockCrashFile {
	return &mockCrashFile{name: name, data: d, dirty: append([]byte(nil), d...)}
}

// Crash discards all unsynced dirty bytes and resets the read cursor, simulating
// an OS crash that drops the page cache without flushing it to disk.
func (m *mockCrashFile) Crash() {
	m.dirty = append([]byte(nil), m.data...)
	m.pos = 0
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

func (m *mockCrashFile) Write(p []byte) (int, error) {
	n, err := m.WriteAt(p, m.pos)
	m.pos += int64(n)
	return n, err
}

func (m *mockCrashFile) ReadAt(p []byte, off int64) (int, error) {
	if int(off) >= len(m.dirty) {
		return 0, io.EOF
	}
	n := copy(p, m.dirty[off:])
	if n < len(p) {
		return n, io.ErrUnexpectedEOF
	}
	return n, nil
}

// Read is called by io.ReadAll in slots.load() during sharky.New(). It reads
// sequentially from dirty (which reflects the on-disk state after a crash).
func (m *mockCrashFile) Read(p []byte) (int, error) {
	if m.pos >= int64(len(m.dirty)) {
		return 0, io.EOF
	}
	n := copy(p, m.dirty[m.pos:])
	m.pos += int64(n)
	return n, nil
}

func (m *mockCrashFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		m.pos = offset
	case io.SeekCurrent:
		m.pos += offset
	case io.SeekEnd:
		m.pos = int64(len(m.dirty)) + offset
	}
	return m.pos, nil
}

func (m *mockCrashFile) Truncate(size int64) error {
	if int(size) > len(m.dirty) {
		m.dirty = append(m.dirty, make([]byte, int(size)-len(m.dirty))...)
	} else {
		m.dirty = m.dirty[:size]
	}
	return nil
}

func (m *mockCrashFile) Sync() error {
	m.data = append([]byte(nil), m.dirty...)
	return nil
}

func (m *mockCrashFile) Close() error {
	return nil
}

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

	// 4. Simulate a crash: OS drops unsynced dirty bytes.
	for _, f := range fsys.files {
		f.Crash()
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

// Test_SharkyCrashAndRestart is a full node-lifecycle reproduction of issue #4737.
//
// It simulates:
//  1. An active node writing chunks (Phase 1)
//  2. A host crash that drops unsynced OS page cache (sharky shard files never fsynced)
//  3. A node restart that reopens the same storage (Phase 2)
//  4. Validation showing that committed chunks are unreadable — the exact corruption
//     reported in the issue ("read 0: EOF")
//
// On the fix branch (sharky Sync() called before LevelDB batch.Commit()), all chunks
// survive the crash and the test passes.
func Test_SharkyCrashAndRestart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Mock FS shared across both "node runs". Each file tracks its on-disk (data)
	// and page-cache (dirty) state independently so Crash() only loses unsynced bytes.
	fsys := &mockCrashFS{files: make(map[string]*mockCrashFile)}

	// LevelDB is on real disk so it survives the crash (it uses its own WAL+fsync).
	dbPath := t.TempDir()

	// ── Phase 1: node is running and writing chunks ──────────────────────────

	var written []swarm.Chunk
	{
		sharkyStore, err := sharky.New(fsys, 1, swarm.SocMaxChunkSize)
		assert.NoError(t, err)

		dbStore, err := leveldbstore.New(dbPath, nil)
		assert.NoError(t, err)

		st := transaction.NewStorage(sharkyStore, dbStore)

		for range 5 {
			ch := test.GenerateTestRandomChunk()
			written = append(written, ch)
			assert.NoError(t, st.Run(ctx, func(s transaction.Store) error {
				return s.ChunkStore().Put(ctx, ch)
			}))
		}
		t.Logf("wrote %d chunks to storage", len(written))

		// Close normally: sharky calls slots.save() which syncs the free-slot bitmap
		// (free_000) but does NOT sync the shard data file (shard_000).
		assert.NoError(t, st.Close())
	}

	// Log file states so it's clear what survived vs. what was lost.
	for name, f := range fsys.files {
		t.Logf("pre-crash  %q: on-disk=%d B  page-cache=%d B", name, len(f.data), len(f.dirty))
	}

	// ── Crash: OS drops all unsynced page cache ───────────────────────────────
	//
	// After Crash():
	//   free_000  → on-disk = page-cache = slot bitmap  (was synced by slots.save())
	//   shard_000 → on-disk = page-cache = 0 B          (was NEVER synced → data lost)
	for _, f := range fsys.files {
		f.Crash()
	}

	for name, f := range fsys.files {
		t.Logf("post-crash %q: on-disk=%d B", name, len(f.data))
	}

	// ── Phase 2: node restarts, reopens the same storage ─────────────────────

	t.Log("restarting node after crash...")

	sharkyStore, err := sharky.New(fsys, 1, swarm.SocMaxChunkSize)
	assert.NoError(t, err)

	dbStore, err := leveldbstore.New(dbPath, nil)
	assert.NoError(t, err)

	st := transaction.NewStorage(sharkyStore, dbStore)
	defer func() { assert.NoError(t, st.Close()) }()

	// ── Validate: try to read every chunk that was committed ──────────────────

	var corrupted int
	for _, ch := range written {
		got, err := st.ChunkStore().Get(ctx, ch.Address())
		if err != nil {
			t.Logf("CORRUPTION: chunk %s unreadable after restart: %v", ch.Address(), err)
			corrupted++
			continue
		}
		if !bytes.Equal(ch.Data(), got.Data()) {
			t.Logf("CORRUPTION: chunk %s data mismatch after restart", ch.Address())
			corrupted++
		}
	}

	if corrupted > 0 {
		t.Errorf("BUG: %d/%d chunks corrupted after simulated crash — "+
			"sharky shard data was not synced to disk before LevelDB committed the chunk metadata",
			corrupted, len(written))
	} else {
		t.Logf("all %d chunks intact after simulated crash", len(written))
	}
}
