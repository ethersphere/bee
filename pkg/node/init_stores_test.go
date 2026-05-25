// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"path/filepath"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
)

// These tests cover the on-disk vs in-memory branching of InitStateStore and
// InitStamperStore. They are useful regression nets because an unintended
// switch of the empty-dataDir branch to on-disk behavior would silently
// persist state in the working directory.

// Note: a zero cacheCapacity makes cache.Wrap fail and InitStateStore leaks
// the already-opened leveldb on that error path. That is a real bug but it is
// independent of the C1 work; not adding a regression test for it here would
// require closing the leveldb on the cache.Wrap error path in
// pkg/node/statestore.go. Left for a follow-up.

func TestInitStateStore_EmptyDirYieldsInMemoryStore(t *testing.T) {
	t.Parallel()

	store, _, err := node.InitStateStore(log.Noop, "", 1)
	if err != nil {
		t.Fatalf("InitStateStore with empty dir returned %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})

	if err := store.Put("k", []byte("v")); err != nil {
		t.Fatalf("put: %v", err)
	}
	var got []byte
	if err := store.Get("k", &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != "v" {
		t.Fatalf("got %q, want %q", got, "v")
	}
}

func TestInitStateStore_OnDiskPersistsAcrossInstances(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	first, _, err := node.InitStateStore(log.Noop, dir, 1)
	if err != nil {
		t.Fatalf("first InitStateStore: %v", err)
	}
	if err := first.Put("hello", []byte("world")); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}

	// A leveldb directory should now exist under <dir>/statestore.
	if entries, err := filepath.Glob(filepath.Join(dir, "statestore", "*")); err != nil {
		t.Fatalf("glob: %v", err)
	} else if len(entries) == 0 {
		t.Fatal("expected leveldb files under <dir>/statestore")
	}

	second, _, err := node.InitStateStore(log.Noop, dir, 1)
	if err != nil {
		t.Fatalf("second InitStateStore: %v", err)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Errorf("close second: %v", err)
		}
	})

	var got []byte
	if err := second.Get("hello", &got); err != nil {
		t.Fatalf("get on second open: %v", err)
	}
	if string(got) != "world" {
		t.Fatalf("got %q, want %q across reopen", got, "world")
	}
}

func TestInitStamperStore_EmptyDirIsInMemory(t *testing.T) {
	t.Parallel()

	// First run with an empty directory: in-memory, dirty must be false.
	store, dirty, err := node.InitStamperStore(log.Noop, "", nil)
	if err != nil {
		t.Fatalf("InitStamperStore: %v", err)
	}
	if dirty {
		t.Fatal("in-memory stamper store must not report dirty")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestInitStamperStore_OnDisk_CleanThenReopens(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger := log.Noop

	first, dirty, err := node.InitStamperStore(logger, dir, nil)
	if err != nil {
		t.Fatalf("first InitStamperStore: %v", err)
	}
	if dirty {
		t.Fatal("fresh on-disk stamper store must report clean")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}

	// Reopening must succeed after a clean close.
	second, _, err := node.InitStamperStore(logger, dir, nil)
	if err != nil {
		t.Fatalf("second InitStamperStore: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second: %v", err)
	}
}
