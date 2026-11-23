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
 