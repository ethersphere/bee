// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore_test

import (
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
)

func TestLocationGuard(t *testing.T) {
	t.Parallel()

	guard := chunkstore.NewLocationGuard()
	loc1 := storage.ChunkLocation{1, 0, 0, 0, 10, 0, 4}
	loc2 := storage.ChunkLocation{1, 0, 0, 0, 20, 0, 4}

	// Inactive guard: MarkFreed should be a no-op
	guard.MarkFreed(loc1)
	if guard.IsFreed(loc1) {
		t.Fatal("expected IsFreed to be false when guard is inactive")
	}

	// Start session
	done := guard.StartSession()

	// Marking freed during active session
	guard.MarkFreed(loc1)
	if !guard.IsFreed(loc1) {
		t.Fatal("expected IsFreed to be true for loc1")
	}
	if guard.IsFreed(loc2) {
		t.Fatal("expected IsFreed to be false for loc2")
	}

	// End session
	done()

	if guard.IsFreed(loc1) {
		t.Fatal("expected IsFreed to be false after session completed")
	}
}

func TestLocationGuardConcurrency(t *testing.T) {
	t.Parallel()

	guard := chunkstore.NewLocationGuard()
	done := guard.StartSession()
	defer done()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(2)
		loc := storage.ChunkLocation{1, byte(i), 0, 0, 0, 0, 0, 0}
		go func() {
			defer wg.Done()
			guard.MarkFreed(loc)
		}()
		go func() {
			defer wg.Done()
			_ = guard.IsFreed(loc)
		}()
	}
	wg.Wait()
}
