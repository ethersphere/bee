// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

// Throwaway reproduction for https://github.com/ethersphere/bee/issues/5335.
// Measures how long a pushsync-like reserve Put and a retrieval-like
// ReserveGet wait while pullsync-like writers and eviction hold the reserve
// locks (per batch, then per bin).

import (
	"context"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type latencies []time.Duration

func (l latencies) pct(p float64) time.Duration {
	if len(l) == 0 {
		return 0
	}
	s := slices.Clone(l)
	slices.Sort(s)
	return s[int(math.Ceil(p*float64(len(s))))-1]
}

func TestIssue5335ReserveLockContention(t *testing.T) {
	const (
		writers  = 32
		duration = 2 * time.Second
	)
	baseAddr := swarm.RandAddress(t)

	run := func(t *testing.T, name string, writerBatches int, evict bool) {
		t.Helper()
		db, err := diskStorer(t, dbTestOps(baseAddr, 1<<22, nil, nil, time.Hour))()
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		putter := db.ReservePutter()

		probeBatch := postagetesting.MustNewBatch().ID
		batches := make([][]byte, max(1, writerBatches))
		batches[0] = probeBatch // writers share the probe's batch, like one upload batch
		for i := 1; i < writerBatches; i++ {
			batches[i] = postagetesting.MustNewBatch().ID
		}

		// Chunk the retrieval probe reads back.
		getCh := chunk.GenerateTestRandomChunkAt(t, baseAddr, 5).WithStamp(postagetesting.MustNewBatchStamp(probeBatch))
		if err := putter.Put(ctx, getCh); err != nil {
			t.Fatal(err)
		}
		getStampHash, _ := getCh.Stamp().Hash()

		if evict {
			// Pre-fill bin 0 of the probe batch with chunks to evict.
			for range 20000 {
				ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewBatchStamp(probeBatch))
				if err := putter.Put(ctx, ch); err != nil {
					t.Fatal(err)
				}
			}
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

		var evictDur time.Duration
		if evict {
			wg.Go(func() {
				time.Sleep(200 * time.Millisecond)
				start := time.Now()
				n, err := db.Reserve().EvictBatchBin(ctx, probeBatch, math.MaxInt, 1) // evicts bins < 1
				evictDur = time.Since(start)
				if err != nil {
					t.Error(err)
				}
				t.Logf("  eviction: %d chunks of bin 0 in %v (batch lock held all along)", n, evictDur)
			})
		}

		var putLat, getLat latencies
		deadline := time.Now().Add(duration)
		for time.Now().Before(deadline) {
			ch := chunk.GenerateTestRandomChunk().WithStamp(postagetesting.MustNewBatchStamp(probeBatch))
			start := time.Now()
			if err := putter.Put(ctx, ch); err != nil {
				t.Fatal(err)
			}
			putLat = append(putLat, time.Since(start))

			start = time.Now()
			if _, err := db.ReserveGet(ctx, getCh.Address(), probeBatch, getStampHash); err != nil {
				t.Fatal(err)
			}
			getLat = append(getLat, time.Since(start))
			time.Sleep(5 * time.Millisecond)
		}
		close(stop)
		wg.Wait()

		t.Logf("%-34s pullsync-like puts/s=%6d | probe Put p50=%-10v p99=%-10v max=%-10v | ReserveGet p50=%-10v max=%v",
			name, written.Load()/int64(duration/time.Second),
			putLat.pct(.5), putLat.pct(.99), putLat.pct(1), getLat.pct(.5), getLat.pct(1))
	}

	t.Run("contention", func(t *testing.T) {
		run(t, "no load", 0, false)
		run(t, "32 writers, 1 batch", 1, false)
		run(t, "32 writers, 32 batches", 32, false)
		run(t, "32 writers, 1 batch + eviction", 1, true)
	})
}
