// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"sync"
	"testing"

	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestRequeueAdd(t *testing.T) {
	t.Parallel()

	chunks := chunktesting.GenerateTestRandomChunks(3)
	q := storer.NewRequeue(2)

	if _, full := q.Add(chunks[0]); full {
		t.Fatal("add 1: queue reported full")
	}
	if _, full := q.Add(chunks[1]); full {
		t.Fatal("add 2: queue reported full")
	}
	oldest, full := q.Add(chunks[2])
	if !full {
		t.Fatal("add 3: queue not reported full")
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

func TestRequeueConcurrentAdd(t *testing.T) {
	t.Parallel()

	limit := storer.DirectUploadRequeueLimit
	q := storer.NewRequeue(limit)
	for _, ch := range chunktesting.GenerateTestRandomChunks(limit) {
		q.Add(ch)
	}

	added := chunktesting.GenerateTestRandomChunks(64)
	returned := make([]swarm.Chunk, len(added))
	fulls := make([]bool, len(added))

	var wg sync.WaitGroup
	for i, ch := range added {
		wg.Go(func() {
			returned[i], fulls[i] = q.Add(ch)
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
