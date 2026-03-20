// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

// TestCrashRecoveryDataIntegrity reproduces the corruption scenario described in
// https://github.com/ethersphere/bee/issues/4737.
//
// Scenario: the context is cancelled while concurrent writes are in flight,
// simulating a SIGKILL or power loss during an active upload. The store is
// then closed (best-effort) and reopened, which triggers the .DIRTY recovery
// path. Every chunk that was acknowledged as committed (Put returned nil) must
// still be readable and have a valid content hash.
//
// Before the group-commit fix, Sharky's WriteAt was not fsynced before
// LevelDB committed the chunk location. A power loss in that window left
// LevelDB pointing at slots containing stale or zeroed bytes, causing
// cac.Valid to return false for those chunks.
//
// Run with a high -count to stress the timing window:
//
//	go test -count=50 -run TestCrashRecoveryDataIntegrity ./pkg/storer/

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestCrashRecoveryDataIntegrity(t *testing.T) {
	const iterations = 10
	for i := range iterations {
		t.Run(fmt.Sprintf("iter%02d", i), func(t *testing.T) {
			t.Parallel()
			testCrashRecoveryOnce(t)
		})
	}
}

func testCrashRecoveryOnce(t *testing.T) {
	t.Helper()

	basePath := t.TempDir()
	baseAddr := swarm.RandAddress(t)
	opts := dbTestOps(baseAddr, 0, nil, nil, time.Second)

	st, err := storer.New(context.Background(), basePath, opts)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}

	const (
		nWorkers        = 8
		chunksPerWorker = 64
	)

	chunks := chunktesting.GenerateTestRandomChunks(nWorkers * chunksPerWorker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		committed []swarm.Chunk
		mu        sync.Mutex
		wg        sync.WaitGroup
	)

	// Each goroutine gets its own cache putter so concurrent transactions
	// are independent – this is the exact pattern that triggers the race
	// between Sharky WriteAt and LevelDB batch.Commit.
	for w := range nWorkers {
		wg.Add(1)
		go func(batch []swarm.Chunk) {
			defer wg.Done()
			putter := st.Cache()
			for _, ch := range batch {
				if err := putter.Put(ctx, ch); err == nil {
					mu.Lock()
					committed = append(committed, ch)
					mu.Unlock()
				}
			}
		}(chunks[w*chunksPerWorker : (w+1)*chunksPerWorker])
	}

	// Let writes start, then cancel to simulate an abrupt shutdown.
	time.AfterFunc(5*time.Millisecond, cancel)
	wg.Wait()

	// Best-effort close; may return context errors for in-flight ops.
	_ = st.Close()

	if len(committed) == 0 {
		t.Skip("no chunks committed before cancellation; increase nWorkers or chunksPerWorker")
	}

	// Reopen at the same path. The presence of .DIRTY triggers the recovery
	// path which (with the fix) also hash-validates every chunk, evicting any
	// corrupted entries from the index.
	st2, err := storer.New(context.Background(), basePath, opts)
	if err != nil {
		t.Fatalf("reopening store after simulated crash: %v", err)
	}
	t.Cleanup(func() { _ = st2.Close() })

	// Every chunk whose Put returned nil must be readable and have a valid
	// content hash. A corrupted chunk means Sharky held stale/zeroed bytes
	// that were never flushed before the LevelDB commit – the exact bug.
	var corrupted int
	for _, ch := range committed {
		got, err := st2.Storage().ChunkStore().Get(context.Background(), ch.Address())
		if err != nil {
			t.Errorf("committed chunk not found after recovery: %s", ch.Address())
			corrupted++
			continue
		}
		if !cac.Valid(got) {
			t.Errorf("committed chunk has corrupted data after recovery: %s", ch.Address())
			corrupted++
		}
	}
	if corrupted > 0 {
		t.Fatalf("%d/%d committed chunks corrupted after crash recovery", corrupted, len(committed))
	}
}
