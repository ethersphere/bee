// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"sync"
	"testing"
	"time"

	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestRequeueAdd(t *testing.T) {
	t.Parallel()

	chunks := chunktesting.GenerateTestRandomChunks(3)
	q := storer.NewRequeue(2, 0)
	now := time.Now()

	if _, _, full := q.Add(chunks[0], now); full {
		t.Fatal("add 1: queue reported full")
	}
	if _, _, full := q.Add(chunks[1], now); full {
		t.Fatal("add 2: queue reported full")
	}
	oldest, wait, full := q.Add(chunks[2], now)
	if !full {
		t.Fatal("add 3: queue not reported full")
	}
	if wait != 0 {
		t.Fatalf("add 3: got wait %v, want 0", wait)
	}
	if !oldest.Address().Equal(chunks[0].Address()) {
		t.Fatalf("add 3: got oldest %s, want %s", oldest.Address(), chunks[0].Address())
	}

	got := q.Drain()
	if len(got) != 2 || !got[0].Address().Equal(chunks[1].Address()) || !got[1].Address().Equal(chunks[2].Address()) {
		t.Fatalf("drain: got %v, want [%s %s]", addresses(got), chunks[1].Address(), chunks[2].Address())
	}
	if got := q.Drain(); len(got) != 0 {
		t.Fatalf("second drain: got %d entries, want 0", len(got))
	}
}

// TestRequeueMinAge verifies the wait returned for the oldest entry of a
// full queue.
func TestRequeueMinAge(t *testing.T) {
	t.Parallel()

	const minAge = 30 * time.Second
	chunks := chunktesting.GenerateTestRandomChunks(3)
	q := storer.NewRequeue(1, minAge)
	start := time.Now()

	q.Add(chunks[0], start)

	// 10 s old: 20 s to go
	if _, wait, full := q.Add(chunks[1], start.Add(10*time.Second)); !full || wait != 20*time.Second {
		t.Fatalf("add 2: got full=%v wait=%v, want true 20s", full, wait)
	}
	// old enough: no wait
	if _, wait, full := q.Add(chunks[2], start.Add(time.Minute)); !full || wait != 0 {
		t.Fatalf("add 3: got full=%v wait=%v, want true 0", full, wait)
	}
}

func TestRequeueConcurrentAdd(t *testing.T) {
	t.Parallel()

	limit := storer.DirectUploadRequeueLimit
	q := storer.NewRequeue(limit, 0)
	now := time.Now()
	for _, ch := range chunktesting.GenerateTestRandomChunks(limit) {
		q.Add(ch, now)
	}

	added := chunktesting.GenerateTestRandomChunks(64)
	returned := make([]swarm.Chunk, len(added))
	fulls := make([]bool, len(added))

	var wg sync.WaitGroup
	for i, ch := range added {
		wg.Go(func() {
			returned[i], _, fulls[i] = q.Add(ch, now)
		})
	}
	wg.Wait()

	seen := make(map[string]bool)
	for i := range added {
		if !fulls[i] {
			t.Fatalf("add %d: queue not reported full", i)
		}
		key := returned[i].Address().ByteString()
		if seen[key] {
			t.Fatalf("add %d: chunk %s returned twice", i, returned[i].Address())
		}
		seen[key] = true
	}
	if got := len(q.Drain()); got != limit {
		t.Fatalf("drain: got %d entries, want %d", got, limit)
	}
}

func addresses(chunks []swarm.Chunk) []swarm.Address {
	addrs := make([]swarm.Address, len(chunks))
	for i, ch := range chunks {
		addrs[i] = ch.Address()
	}
	return addrs
}
