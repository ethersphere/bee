// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Failing gate for https://github.com/ethersphere/bee/issues/5335.
//
// Handwritten steps (read this first):
//  1. Open a disk reserve and one postage batch (one notebook).
//  2. Measure how long Reserve.Put takes alone (baseline).
//  3. Measure again while 32 writers Put into the SAME batch
//     (like pullsync and pushsync sharing one batch).
//  4. Fail if contended Put is much slower than baseline
//     (p99 over 10x, or any Put over 50ms).
//
// Expected today: FAIL. Shared-batch Put gets slow under concurrency.
// This shows millisecond-scale contention, not a 10-20s freeze by itself.
//
// Scope: Put vs Put on the same batch only (the reserve batch lock). It does
// not cover Puts while many Has calls run; that is
// TestIssue5335ReservePutMustStayFastUnderManyHas below.
func TestIssue5335ReservePutMustStayFastUnderSharedBatchWriters(t *testing.T) {
	const (
		writers           = 32
		duration          = 2 * time.Second
		maxP99Slowdown    = 10
		maxAbsolutePutMax = 50 * time.Millisecond
	)

	baseAddr := swarm.RandAddress(t)

	// measure runs Put for `duration` and returns every probe latency.
	// withWriters=false: quiet baseline.
	// withWriters=true: 32 background Puts on the same batch.
	measure := func(t *testing.T, withWriters bool) latencies {
		t.Helper()

		// Step 1: reserve + one batch.
		db, err := diskStorer(t, dbTestOps(baseAddr, 1<<22, nil, nil, time.Hour))()
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		putter := db.ReservePutter()
		batchID := postagetesting.MustNewBatch().ID

		stop := make(chan struct{})
		var wg sync.WaitGroup
		var written atomic.Int64

		// Step 3 helpers: background writers (same batch), if requested.
		if withWriters {
			for range writers {
				wg.Go(func() {
					for {
						select {
						case <-stop:
							return
						default:
						}
						ch := chunk.GenerateTestRandomChunk().WithStamp(postagetesting.MustNewBatchStamp(batchID))
						if err := putter.Put(ctx, ch); err != nil {
							t.Error(err)
							return
						}
						written.Add(1)
					}
				})
			}
		}

		// Step 2 / 3: timed probe Puts on that same batch.
		var putLat latencies
		deadline := time.Now().Add(duration)
		for time.Now().Before(deadline) {
			ch := chunk.GenerateTestRandomChunk().WithStamp(postagetesting.MustNewBatchStamp(batchID))
			start := time.Now()
			if err := putter.Put(ctx, ch); err != nil {
				t.Fatal(err)
			}
			putLat = append(putLat, time.Since(start))
			time.Sleep(5 * time.Millisecond)
		}
		close(stop)
		wg.Wait()

		t.Logf("with_writers=%v puts/s=%d put p50=%v p99=%v max=%v",
			withWriters, written.Load()/int64(duration/time.Second),
			putLat.pct(.5), putLat.pct(.99), putLat.pct(1))
		return putLat
	}

	// Steps 2 and 3.
	baseline := measure(t, false)
	contended := measure(t, true)

	// Step 4: compare.
	baseP99 := baseline.pct(.99)
	contP99 := contended.pct(.99)
	contMax := contended.pct(1)

	if baseP99 == 0 {
		t.Fatal("baseline Put p99 is zero; not enough samples")
	}
	if contP99 > time.Duration(maxP99Slowdown)*baseP99 {
		t.Errorf("reserve Put p99 under shared-batch writers too slow: baseline p99=%v contended p99=%v (limit %dx)",
			baseP99, contP99, maxP99Slowdown)
	}
	if contMax > maxAbsolutePutMax {
		t.Errorf("reserve Put max under shared-batch writers too slow: max=%v limit=%v",
			contMax, maxAbsolutePutMax)
	}
}

// Failing gate for https://github.com/ethersphere/bee/issues/5335, Has variant.
//
// Steps:
//  1. Open a disk reserve and seed it with chunks, so Has reads leveldb.
//  2. Measure how long Reserve.Put takes alone (baseline).
//  3. Measure again while 256 goroutines call ReserveHas in a loop
//     (like pullsync checking every offered chunk, pullsync.go ReserveHas),
//     98% of them on stored chunks.
//  4. Fail if Put is much slower than baseline
//     (p99 over 10x, or any Put over 50ms).
//
// Reserve.Has takes no reserve lock, so any slowdown here comes from leveldb
// (Has readers vs the Put commit), not from the batch/bin locks.
func TestIssue5335ReservePutMustStayFastUnderManyHas(t *testing.T) {
	const (
		seedChunks        = 20_000
		seeders           = 16
		hasWorkers        = 256
		hitPercent        = 98
		duration          = 2 * time.Second
		maxP99Slowdown    = 10
		maxAbsolutePutMax = 50 * time.Millisecond
	)

	baseAddr := swarm.RandAddress(t)

	type hasKey struct {
		addr             swarm.Address
		batchID, stampHs []byte
	}

	// oneBatch=false: every chunk has its own random batch (pullsync mix).
	// oneBatch=true: seeded chunks, Has keys and probe Puts all share one batch.
	measure := func(t *testing.T, withHas, oneBatch bool) latencies {
		t.Helper()

		// Step 1: reserve seeded with chunks.
		db, err := diskStorer(t, dbTestOps(baseAddr, 1<<22, nil, nil, time.Hour))()
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		putter := db.ReservePutter()
		batchID := postagetesting.MustNewBatch().ID
		newChunk := func() swarm.Chunk {
			ch := chunk.GenerateTestRandomChunk()
			if oneBatch {
				return ch.WithStamp(postagetesting.MustNewBatchStamp(batchID))
			}
			return ch
		}

		keys := make([][]hasKey, seeders)
		var seedWG sync.WaitGroup
		for s := range seeders {
			seedWG.Go(func() {
				for range seedChunks / seeders {
					ch := newChunk()
					if err := putter.Put(ctx, ch); err != nil {
						t.Error(err)
						return
					}
					sh, err := ch.Stamp().Hash()
					if err != nil {
						t.Error(err)
						return
					}
					keys[s] = append(keys[s], hasKey{ch.Address(), ch.Stamp().BatchID(), sh})
				}
			})
		}
		seedWG.Wait()
		var hits []hasKey
		for _, k := range keys {
			hits = append(hits, k...)
		}
		// Precompute misses so Has workers do no chunk/stamp work in the loop.
		misses := make([]hasKey, 1024)
		for i := range misses {
			miss := newChunk()
			misses[i] = hasKey{miss.Address(), miss.Stamp().BatchID(), miss.Stamp().BatchID()}
		}

		stop := make(chan struct{})
		var wg sync.WaitGroup
		var hasCalls atomic.Int64

		// Step 3 helpers: Has callers, if requested.
		if withHas {
			for w := range hasWorkers {
				wg.Go(func() {
					for i := w; ; i++ {
						select {
						case <-stop:
							return
						default:
						}
						var k hasKey
						if i%100 < hitPercent {
							k = hits[(i*7919)%len(hits)]
						} else {
							k = misses[i%len(misses)]
						}
						if _, err := db.ReserveHas(k.addr, k.batchID, k.stampHs); err != nil {
							t.Error(err)
							return
						}
						hasCalls.Add(1)
					}
				})
			}
		}

		// Step 2 / 3: timed probe Puts (new chunks, as pullsync delivers).
		var putLat latencies
		deadline := time.Now().Add(duration)
		for time.Now().Before(deadline) {
			ch := newChunk()
			start := time.Now()
			if err := putter.Put(ctx, ch); err != nil {
				t.Fatal(err)
			}
			putLat = append(putLat, time.Since(start))
			time.Sleep(5 * time.Millisecond)
		}
		close(stop)
		wg.Wait()

		t.Logf("one_batch=%v with_has=%v has/s=%d put p50=%v p99=%v max=%v", oneBatch,
			withHas, hasCalls.Load()/int64(duration/time.Second),
			putLat.pct(.5), putLat.pct(.99), putLat.pct(1))
		return putLat
	}

	for _, tc := range []struct {
		name     string
		oneBatch bool
	}{
		{"many_batches", false},
		{"one_batch", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Steps 2 and 3.
			baseline := measure(t, false, tc.oneBatch)
			contended := measure(t, true, tc.oneBatch)

			// Step 4: compare.
			baseP99 := baseline.pct(.99)
			contP99 := contended.pct(.99)
			contMax := contended.pct(1)

			if baseP99 == 0 {
				t.Fatal("baseline Put p99 is zero; not enough samples")
			}
			if contP99 > time.Duration(maxP99Slowdown)*baseP99 {
				t.Errorf("reserve Put p99 under many Has callers too slow: baseline p99=%v contended p99=%v (limit %dx)",
					baseP99, contP99, maxP99Slowdown)
			}
			if contMax > maxAbsolutePutMax {
				t.Errorf("reserve Put max under many Has callers too slow: max=%v limit=%v",
					contMax, maxAbsolutePutMax)
			}
		})
	}
}
