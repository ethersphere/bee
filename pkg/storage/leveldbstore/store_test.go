// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestStore(t *testing.T) {
	t.Parallel()

	store, err := leveldbstore.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	storagetest.TestStore(t, store)
}

func BenchmarkStore(b *testing.B) {
	st, err := leveldbstore.New("", &opt.Options{
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

	st, err := leveldbstore.New("", nil)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	storagetest.TestBatchedStore(t, st)
}

func BenchmarkBatchedStore(b *testing.B) {
	st, err := leveldbstore.New("", &opt.Options{
		Compression: opt.SnappyCompression,
	})
	if err != nil {
		b.Fatalf("create store failed: %v", err)
	}
	b.Cleanup(func() { _ = st.Close() })
	storagetest.BenchmarkBatchedStore(b, st)
}
