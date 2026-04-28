// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestStore(t *testing.T) {
	t.Parallel()

	store, _, err := leveldbstore.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	storagetest.TestStore(t, store)
}

func BenchmarkStore(b *testing.B) {
	st, _, err := leveldbstore.New("", &opt.Options{
		Compression: opt.SnappyCompression,
	})
	if err != nil {
		b.Fatalf("create store failed: %v", err)
	}
	b.Cleanup(func() { _ = st.Close() })
	storagetest.BenchmarkStore(b, st)
}

func TestBatchedStore(t *testing.T) {
	t.Parallel()

	st, _, err := leveldbstore.New("", nil)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	storagetest.TestBatchedStore(t, st)
}

func BenchmarkBatchedStore(b *testing.B) {
	st, _, err := leveldbstore.New("", &opt.Options{
		Compression: opt.SnappyCompression,
	})
	if err != nil {
		b.Fatalf("create store failed: %v", err)
	}
	b.Cleanup(func() { _ = st.Close() })
	storagetest.BenchmarkBatchedStore(b, st)
}

func TestDirtyMarker(t *testing.T) {
	t.Parallel()

	t.Run("clean on first open", func(t *testing.T) {
		t.Parallel()
		st, dirty, err := leveldbstore.New(t.TempDir(), nil)
		if err != nil {
			t.Fatalf("open failed: %v", err)
		}
		t.Cleanup(func() { _ = st.Close() })
		if dirty {
			t.Fatal("expected clean on first open, got dirty")
		}
	})

	t.Run("clean after clean close", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		st, _, err := leveldbstore.New(dir, nil)
		if err != nil {
			t.Fatalf("first open failed: %v", err)
		}
		if err := st.Close(); err != nil {
			t.Fatalf("close failed: %v", err)
		}

		st, dirty, err := leveldbstore.New(dir, nil)
		if err != nil {
			t.Fatalf("second open failed: %v", err)
		}
		t.Cleanup(func() { _ = st.Close() })
		if dirty {
			t.Fatal("expected clean after clean close, got dirty")
		}
	})

	t.Run("dirty after crash", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Simulate a previous session that crashed: open the raw leveldb
		// and write the dirty marker directly, then close without deleting it.
		db, err := leveldb.OpenFile(dir, nil)
		if err != nil {
			t.Fatalf("raw open failed: %v", err)
		}
		if err := db.Put([]byte(".store-dirty-shutdown"), []byte{}, nil); err != nil {
			t.Fatalf("write dirty key failed: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("raw close failed: %v", err)
		}

		// Now open via leveldbstore — should detect the marker as dirty.
		st, dirty, err := leveldbstore.New(dir, nil)
		if err != nil {
			t.Fatalf("open failed: %v", err)
		}
		t.Cleanup(func() { _ = st.Close() })
		if !dirty {
			t.Fatal("expected dirty after crash, got clean")
		}
	})
}
