// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pebblestore_test

import (
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/ethersphere/bee/v2/pkg/storage/pebblestore"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
)

func TestStore(t *testing.T) {
	t.Parallel()

	// Use a temporary directory for on‑disk storage.
	store, err := pebblestore.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	storagetest.TestStore(t, store)
}

func BenchmarkStore(b *testing.B) {
	// Passing an empty path ("") makes the store run in memory.
	st, err := pebblestore.New("", &pebble.Options{})
	if err != nil {
		b.Fatalf("create store failed: %v", err)
	}
	b.Cleanup(func() { _ = st.Close() })

	storagetest.BenchmarkStore(b, st)
}

func TestBatchedStore(t *testing.T) {
	t.Parallel()

	st, err := pebblestore.New("", nil)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	storagetest.TestBatchedStore(t, st)
}

func BenchmarkBatchedStore(b *testing.B) {
	st, err := pebblestore.New("", &pebble.Options{})
	if err != nil {
		b.Fatalf("create store failed: %v", err)
	}
	b.Cleanup(func() { _ = st.Close() })

	storagetest.BenchmarkBatchedStore(b, st)
}
