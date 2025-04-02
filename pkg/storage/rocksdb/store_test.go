// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rocksdb_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storage/rocksdb"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	"github.com/linxGnu/grocksdb"
)

func TestStore(t *testing.T) {
	t.Parallel()

	store, err := rocksdb.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	storagetest.TestStore(t, store)
}

func BenchmarkStore(b *testing.B) {
	st, err := rocksdb.New("", nil)
	if err != nil {
		b.Fatalf("create store failed: %v", err)
	}
	b.Cleanup(func() { _ = st.Close() })
	storagetest.BenchmarkStore(b, st)
}

func TestBatchedStore(t *testing.T) {
	t.Parallel()

	st, err := rocksdb.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	storagetest.TestBatchedStore(t, st)
}

func BenchmarkBatchedStore(b *testing.B) {
	st, err := rocksdb.New(b.TempDir(), &grocksdb.Options{})
	if err != nil {
		b.Fatalf("create store failed: %v", err)
	}
	b.Cleanup(func() { _ = st.Close() })
	storagetest.BenchmarkBatchedStore(b, st)
}
