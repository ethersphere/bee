// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

// Failing gate for https://github.com/ethersphere/bee/issues/5335.
// Asserts pushsync-like reserve Put stays close to the unloaded baseline
// while pullsync-like writers share one postage batch. Expected to fail
// until batch/bin lock contention is reduced.

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

func TestIssue5335ReservePutMustStayFastUnderSharedBatchWriters(t *testing.T) {
	const (
		writers           = 32
		duration          = 2 * time.Second
		maxP99Slowdown    = 10
		maxAbsolutePutMax = 50 * time.Millisecond
	)

	baseAddr := swarm.RandAddress(t)
	measure := func(t *testing.T, writerBatches int) latencies {
		t.Helper()
		db, err := diskStorer(t, dbTestOps(baseAddr, 1<<22, nil, nil, time.Hour))()
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		putter := db.ReservePutter()

		probeBatch := postagetesting.MustNewBatch().ID
		batches := make([][]byte, max(1, writerBatches))
		batches[0] = probeBatch
		for i := 1; i < writerBatches; i++ {
			batches[i] = postagetesting.MustNewBatch().ID
		}

		stop := make(chan struct{})
		var wg sync.WaitGroup
		var written atomic.Int64
		for w := range writers {
			if writerBatches == 0 {
				break
			}
			batch := batches[w%writerBatches]
			wg.Go(func() {
				for {
					select {
					case <-stop:
						return
					default:
					}
					ch := chunk.GenerateTestRandomChunk().WithStamp(postagetesting.MustNewBatchStamp(batch))
					if err := putter.Put(ctx, ch); err != nil {
						t.Error(err)
						return
					}
					written.Add(1)
				}
			})
		}

		var putLat latencies
		deadline := time.Now().Add(duration)
		for time.Now().Before(deadline) {
			ch := chunk.GenerateTestRandomChunk().WithStamp(postagetesting.MustNewBatchStamp(probeBatch))
			start := time.Now()
			if err := putter.Put(ctx, ch); err != nil {
				t.Fatal(err)
			}
			putLat = append(putLat, time.Since(start))
			time.Sleep(5 * time.Millisecond)
		}
		close(stop)
		wg.Wait()

		t.Logf("writers_batches=%d puts/s=%d put p50=%v p99=%v max=%v",
			writerBatches, written.Load()/int64(duration/time.Second),
			putLat.pct(.5), putLat.pct(.99), putLat.pct(1))
		return putLat
	}

	baseline := measure(t, 0)
	contended := measure(t, 1)

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
