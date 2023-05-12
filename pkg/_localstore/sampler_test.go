// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/kademlia/internal/shed"
	"github.com/google/go-cmp/cmp"
)

// nolint:paralleltest
func TestReserveSampler(t *testing.T) {
	const chunkCountPerPO = 10
	const maxPO = 10
	var chs []swarm.Chunk

	t.Cleanup(setValidChunkFunc(func(swarm.Chunk) bool { return true }))

	db := newTestDB(t, &Options{
		Capacity:        1000,
		ReserveCapacity: 1000,
	})

	timeVar := uint64(time.Now().UnixNano())

	for po := 0; po < maxPO; po++ {
		for i := 0; i < chunkCountPerPO; i++ {
			ch := generateTestRandomChunkAt(t, swarm.NewAddress(db.baseKey), po).WithBatch(0, 3, 2, false)
			// override stamp timestamp to be before the consensus timestamp
			ch = ch.WithStamp(postagetesting.MustNewStampWithTimestamp(timeVar - 1))
			chs = append(chs, ch)
		}
	}

	_, err := db.Put(context.Background(), storage.ModePutSync, chs...)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("reserve size", reserveSizeTest(db, chunkCountPerPO*maxPO, 0))

	var sample1 storage.Sample

	t.Run("reserve sample 1", func(t *testing.T) {
		sample, err := db.ReserveSample(context.TODO(), []byte("anchor"), 5, timeVar)
		if err != nil {
			t.Fatal(err)
		}
		if len(sample.Items) != sampleSize {
			t.Fatalf("incorrect no of sample items exp %d found %d", sampleSize, len(sample.Items))
		}
		for i := 0; i < len(sample.Items)-2; i++ {
			if bytes.Compare(sample.Items[i].Bytes(), sample.Items[i+1].Bytes()) != -1 {
				t.Fatalf("incorrect order of samples %+q", sample.Items)
			}
		}

		sample1 = sample
	})

	// We generate another 100 chunks. With these new chunks in the reserve, statistically
	// some of them should definitely make it to the sample based on lex ordering.
	for po := 0; po < maxPO; po++ {
		for i := 0; i < chunkCountPerPO; i++ {
			ch := generateTestRandomChunkAt(t, swarm.NewAddress(db.baseKey), po).WithBatch(0, 3, 2, false)
			// override stamp timestamp to be after the consensus timestamp
			ch = ch.WithStamp(postagetesting.MustNewStampWithTimestamp(timeVar + 1))
			chs = append(chs, ch)
		}
	}

	_, err = db.Put(context.Background(), storage.ModePutSync, chs...)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("reserve size", reserveSizeTest(db, 2*chunkCountPerPO*maxPO, 0))

	// Now we generate another sample with the older timestamp. This should give us
	// the exact same sample, ensuring that none of the later chunks were considered.
	t.Run("reserve sample 2", func(t *testing.T) {
		sample, err := db.ReserveSample(context.TODO(), []byte("anchor"), 5, timeVar)
		if err != nil {
			t.Fatal(err)
		}

		if !cmp.Equal(sample, sample1) {
			t.Fatalf("samples different (-want +have):\n%s", cmp.Diff(sample, sample1))
		}
	})
}

func TestReserveSamplerStop_FLAKY(t *testing.T) {
	const chunkCountPerPO = 10
	const maxPO = 10
	var (
		chs      []swarm.Chunk
		batchIDs [][]byte
		closed   chan struct{}
		doneMtx  sync.Mutex
		mtx      sync.Mutex
	)
	startWait, waitChan := make(chan struct{}), make(chan struct{})
	doneWaiting := false

	t.Cleanup(setWithinRadiusFunc(func(*DB, shed.Item) bool { return true }))

	testHookEvictionChan := make(chan uint64)
	t.Cleanup(setTestHookEviction(func(count uint64) {
		if count == 0 {
			return
		}
		select {
		case testHookEvictionChan <- count:
		case <-closed:
		}
	}))

	db := newTestDB(t, &Options{
		ReserveCapacity: 90,
		UnreserveFunc: func(f postage.UnreserveIteratorFn) error {
			mtx.Lock()
			defer mtx.Unlock()
			for i := 0; i < len(batchIDs); i++ {
				// pop an element from batchIDs, call the Unreserve
				item := batchIDs[i]
				// here we mock the behavior of the batchstore
				// that would call the localstore back with the
				// batch IDs and the radiuses from the FIFO queue
				stop, err := f(item, 2)
				if err != nil {
					return err
				}
				if stop {
					return nil
				}
			}
			batchIDs = nil
			return nil
		},
		ValidStamp: func(_ swarm.Chunk, stampBytes []byte) (chunk swarm.Chunk, err error) {
			doneMtx.Lock()
			defer doneMtx.Unlock()

			if !doneWaiting {
				// signal that we have started sampling
				close(startWait)
				// this makes sampling wait till we trigger eviction for the test
				<-waitChan
			}
			doneWaiting = true
			return nil, nil
		},
	})
	closed = db.close

	for po := 0; po < maxPO; po++ {
		for i := 0; i < chunkCountPerPO; i++ {
			ch := generateTestRandomChunkAt(t, swarm.NewAddress(db.baseKey), po).WithBatch(2, 3, 2, false)
			mtx.Lock()
			chs = append(chs, ch)
			batchIDs = append(batchIDs, ch.Stamp().BatchID())
			mtx.Unlock()
		}
	}

	_, err := db.Put(context.Background(), storage.ModePutSync, chs...)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		<-startWait
		// this will trigger the eviction
		_, _ = db.ComputeReserveSize(0)
		<-testHookEvictionChan
		close(waitChan)
	}()

	_, err = db.ReserveSample(context.TODO(), []byte("anchor"), 5, uint64(time.Now().UnixNano()))
	if !errors.Is(err, errSamplerStopped) {
		t.Fatalf("expected sampler stopped error, found: %v", err)
	}
}
