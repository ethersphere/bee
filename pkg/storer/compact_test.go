// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	pullerMock "github.com/ethersphere/bee/pkg/puller/mock"
	chunk "github.com/ethersphere/bee/pkg/storage/testing"
	storer "github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestCompact creates two batches and puts chunks belonging to both batches.
// The first batch is then expired, causing free slots to accumulate in sharky.
// Next, sharky is compacted, after which, it is tested that valid chunks can still be retrieved.
func TestCompact(t *testing.T) {

	baseAddr := swarm.RandAddress(t)
	ctx := context.Background()
	basePath := t.TempDir()

	opts := dbTestOps(baseAddr, 10_000, nil, nil, time.Second)
	opts.CacheCapacity = 0

	st, err := storer.New(ctx, basePath, opts)
	if err != nil {
		t.Fatal(err)
	}
	st.StartReserveWorker(ctx, pullerMock.NewMockRateReporter(0), networkRadiusFunc(0))

	var chunks []swarm.Chunk
	batches := []*postage.Batch{postagetesting.MustNewBatch(), postagetesting.MustNewBatch(), postagetesting.MustNewBatch()}
	evictBatch := batches[1]

	putter := st.ReservePutter()

	for b := 0; b < len(batches); b++ {
		for i := uint64(0); i < 100; i++ {
			ch := chunk.GenerateTestRandomChunk()
			ch = ch.WithStamp(postagetesting.MustNewBatchStamp(batches[b].ID))
			chunks = append(chunks, ch)
			err := putter.Put(ctx, ch)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	err = st.EvictBatch(ctx, evictBatch.ID)
	if err != nil {
		t.Fatal(err)
	}

	c, unsub := st.Events().Subscribe("batchExpiryDone")
	t.Cleanup(unsub)
	<-c

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	err = storer.Compact(ctx, basePath, opts, true)
	if err != nil {
		t.Fatal(err)
	}

	st, err = storer.New(ctx, basePath, opts)
	if err != nil {
		t.Fatal(err)
	}

	putter = st.ReservePutter()
	for i := uint64(0); i < 100; i++ {
		ch := chunk.GenerateTestRandomChunk()
		ch = ch.WithStamp(postagetesting.MustNewBatchStamp(batches[0].ID))
		chunks = append(chunks, ch)
		err := putter.Put(ctx, ch)
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, ch := range chunks {
		has, err := st.ReserveHas(ch.Address(), ch.Stamp().BatchID())
		if err != nil {
			t.Fatal(err)
		}

		if bytes.Equal(ch.Stamp().BatchID(), evictBatch.ID) {
			if has {
				t.Fatal("store should NOT have chunk")
			}
			checkSaved(t, st, ch, false, false)
		} else if !has {
			t.Fatal("store should have chunk")
		} else {
			checkSaved(t, st, ch, true, true)
		}
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestCompactNoEvictions compacts a store that has no free slots to ensure that no chunks get lost.
func TestCompactNoEvictions(t *testing.T) {

	baseAddr := swarm.RandAddress(t)
	ctx := context.Background()
	basePath := t.TempDir()

	opts := dbTestOps(baseAddr, 10_000, nil, nil, time.Second)
	opts.CacheCapacity = 0

	st, err := storer.New(ctx, basePath, opts)
	if err != nil {
		t.Fatal(err)
	}
	st.StartReserveWorker(ctx, pullerMock.NewMockRateReporter(0), networkRadiusFunc(0))

	var chunks []swarm.Chunk
	batches := []*postage.Batch{postagetesting.MustNewBatch(), postagetesting.MustNewBatch(), postagetesting.MustNewBatch()}

	putter := st.ReservePutter()

	for b := 0; b < len(batches); b++ {
		for i := uint64(0); i < 100; i++ {
			ch := chunk.GenerateTestRandomChunk()
			ch = ch.WithStamp(postagetesting.MustNewBatchStamp(batches[b].ID))
			chunks = append(chunks, ch)
			err := putter.Put(ctx, ch)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	err = storer.Compact(ctx, basePath, opts, false)
	if err != nil {
		t.Fatal(err)
	}

	st, err = storer.New(ctx, basePath, opts)
	if err != nil {
		t.Fatal(err)
	}

	putter = st.ReservePutter()
	for i := uint64(0); i < 100; i++ {
		ch := chunk.GenerateTestRandomChunk()
		ch = ch.WithStamp(postagetesting.MustNewBatchStamp(batches[0].ID))
		chunks = append(chunks, ch)
		err := putter.Put(ctx, ch)
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, ch := range chunks {
		has, err := st.ReserveHas(ch.Address(), ch.Stamp().BatchID())
		if err != nil {
			t.Fatal(err)
		}

		if !has {
			t.Fatal("store should have chunk")
		}

		checkSaved(t, st, ch, true, true)
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
}
