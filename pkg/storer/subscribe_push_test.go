// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestPushSubscriber(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	t.Run("inmem", func(t *testing.T) {
		t.Parallel()
		memStorer := memStorer(t, dbTestOps(baseAddr, 10, nil, nil, time.Second))
		testPushSubscriber(t, memStorer)
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		diskStorer := diskStorer(t, dbTestOps(baseAddr, 0, nil, nil, time.Second))
		testPushSubscriber(t, diskStorer)
	})
}

func testPushSubscriber(t *testing.T, newLocalstore func() (*storer.DB, error)) {
	t.Helper()

	lstore, err := newLocalstore()
	if err != nil {
		t.Fatal(err)
	}

	chunks := chunktesting.GenerateTestRandomChunks(18)
	chunksProcessedMu := sync.Mutex{}
	chunksProcessed := make(map[string]struct{})

	uploadChunks := func(chunksToUpload ...swarm.Chunk) {
		id, err := lstore.NewSession()
		if err != nil {
			t.Fatal(err)
		}

		p, err := lstore.Upload(context.Background(), false, id.TagID)
		if err != nil {
			t.Fatal(err)
		}

		for _, ch := range chunksToUpload {
			if err := p.Put(context.Background(), ch); err != nil {
				t.Fatal(err)
			}
		}

		err = p.Done(chunksToUpload[len(chunksToUpload)-1].Address())
		if err != nil {
			t.Fatalf("session.Done(...): unexpected error: %v", err)
		}
	}

	verify := func(got swarm.Chunk) {
		idx := swarm.IndexOfChunkWithAddress(chunks, got.Address())
		if idx < 0 {
			t.Fatalf("chunk not found %s", got.Address())
		}
		want := chunks[idx]
		if !got.Equal(want) {
			t.Fatalf("chunk not equal %s", got.Address())
		}
		gotStamp, err := got.Stamp().MarshalBinary()
		if err != nil {
			t.Fatalf("stamp.MarshalBinary: unexpected error: %v", err)
		}
		wantStamp, err := want.Stamp().MarshalBinary()
		if err != nil {
			t.Fatalf("stamp.MarshalBinary: unexpected error: %v", err)
		}
		if !bytes.Equal(gotStamp, wantStamp) {
			t.Fatalf("stamps dont match")
		}

		chunksProcessedMu.Lock()
		chunksProcessed[got.Address().ByteString()] = struct{}{}
		chunksProcessedMu.Unlock()
	}

	// prepopulate database with some chunks
	// before the subscription
	uploadChunks(chunks[:10]...)

	// set a timeout on subscription
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, stop := lstore.SubscribePush(ctx)
	defer stop()

	// receive and validate addresses from the subscription
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case got, ok := <-ch:
				if !ok {
					return
				}
				verify(got)

				chunksProcessedMu.Lock()
				count := len(chunksProcessed)
				chunksProcessedMu.Unlock()

				if count == len(chunks) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// upload some chunks just after subscribe
	uploadChunks(chunks[10:15]...)

	time.Sleep(200 * time.Millisecond)

	// upload some chunks after some short time
	// to ensure that subscription will include them
	// in a dynamic environment
	uploadChunks(chunks[15:]...)

	<-done
}
