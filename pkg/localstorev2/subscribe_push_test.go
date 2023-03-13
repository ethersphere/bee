// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	storer "github.com/ethersphere/bee/pkg/localstorev2"
	chunktesting "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
)

func TestPushSubscriber(t *testing.T) {
	t.Parallel()

	baseAddr := test.RandomAddress()
	t.Run("inmem", func(t *testing.T) {
		t.Skip()
		t.Parallel()
		memStorer := memStorer(t, dbTestOps(baseAddr, 10, nil, nil, nil, time.Second))
		testPushSubscriber(t, memStorer)
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		diskStorer := diskStorer(t, dbTestOps(baseAddr, 0, nil, nil, nil, time.Second))
		testPushSubscriber(t, diskStorer)
	})
}

func testPushSubscriber(t *testing.T, newLocalstore func() (*storer.DB, error)) {
	t.Helper()

	lstore, err := newLocalstore()
	if err != nil {
		t.Fatal(err)
	}

	chunks := make([]swarm.Chunk, 0)
	var chunksMu sync.Mutex

	chunkProcessedTimes := make([]int, 0)

	uploadRandomChunks := func(count int) {
		chunksMu.Lock()
		defer chunksMu.Unlock()

		id, err := lstore.NewSession()
		if err != nil {
			t.Fatal(err)
		}

		p, err := lstore.Upload(context.TODO(), false, id)
		if err != nil {
			t.Fatal(err)
		}

		ch := chunktesting.GenerateTestRandomChunks(count)

		var i = 0
		for ; i < count; i++ {
			if err := p.Put(context.TODO(), ch[i]); err != nil {
				t.Fatal(err)
			}

			// on windows the time granularity isn't enough to
			// generate distinct entries
			time.Sleep(100 * time.Millisecond)

			chunks = append(chunks, ch[i])
			chunkProcessedTimes = append(chunkProcessedTimes, 0)
		}

		_ = p.Done(ch[i-1].Address())
	}

	// prepopulate database with some chunks
	// before the subscription
	uploadRandomChunks(10)

	// set a timeout on subscription
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// collect all errors from validating addresses, even nil ones
	// to validate the number of addresses received by the subscription
	errChan := make(chan error)

	ch, _, stop := lstore.SubscribePush(ctx)
	defer stop()

	// receive and validate addresses from the subscription
	go func() {
		var (
			err, ierr           error
			gotStamp, wantStamp []byte
		)
		var i int // address index
		for {
			select {
			case got, ok := <-ch:
				if !ok {
					return
				}
				chunksMu.Lock()
				cIndex := i
				want := chunks[cIndex]
				chunkProcessedTimes[cIndex]++
				chunksMu.Unlock()
				if !bytes.Equal(got.Data(), want.Data()) {
					err = fmt.Errorf("got chunk %v data %x, want %x", i, got.Data(), want.Data())
				}
				if !got.Address().Equal(want.Address()) {
					err = fmt.Errorf("got chunk %v address %s, want %s", i, got.Address(), want.Address())
				}
				if gotStamp, ierr = got.Stamp().MarshalBinary(); ierr != nil {
					err = ierr
				}
				if wantStamp, ierr = want.Stamp().MarshalBinary(); ierr != nil {
					err = ierr
				}
				if !bytes.Equal(gotStamp, wantStamp) {
					err = fmt.Errorf("stamps don't match")
				}

				i++
				// send one and only one error per received address
				select {
				case errChan <- err:
				case <-ctx.Done():
					return
				}

				chunksMu.Lock()
				if i == len(chunks) {
					close(errChan)
				}
				chunksMu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	// upload some chunks just after subscribe
	uploadRandomChunks(5)

	time.Sleep(200 * time.Millisecond)

	// upload some chunks after some short time
	// to ensure that subscription will include them
	// in a dynamic environment
	uploadRandomChunks(3)

	checkErrChan(ctx, t, errChan, len(chunks))

	chunksMu.Lock()
	for i, pc := range chunkProcessedTimes {
		if pc != 1 {
			t.Fatalf("chunk on address %s processed %d times, should be only once", chunks[i].Address(), pc)
		}
	}
	chunksMu.Unlock()
}

// checkErrChan expects the number of wantedChunksCount errors from errChan
// and calls t.Error for the ones that are not nil.
func checkErrChan(ctx context.Context, t *testing.T, errChan chan error, wantedChunksCount int) {
	t.Helper()

	for err := range errChan {
		if err != nil {
			t.Error(err)
		}
	}
}
