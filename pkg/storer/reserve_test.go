// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	batchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	pullerMock "github.com/ethersphere/bee/pkg/puller/mock"
	"github.com/ethersphere/bee/pkg/spinlock"
	storage "github.com/ethersphere/bee/pkg/storage"
	chunk "github.com/ethersphere/bee/pkg/storage/testing"
	storer "github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/go-cmp/cmp"
)

func TestIndexCollision(t *testing.T) {
	t.Parallel()

	testF := func(t *testing.T, baseAddr swarm.Address, storer *storer.DB) {
		t.Helper()
		stamp := postagetesting.MustNewBatchStamp(postagetesting.MustNewBatch().ID)
		putter := storer.ReservePutter(context.Background())

		ch_1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(stamp)
		err := putter.Put(context.Background(), ch_1)
		if err != nil {
			t.Fatal(err)
		}

		ch_2 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(stamp)
		err = putter.Put(context.Background(), ch_2)
		if err == nil {
			t.Fatal("expected index collision error")
		}

		err = putter.Cleanup()
		if err != nil {
			t.Fatal(err)
		}

		_, err = storer.ReserveGet(context.Background(), ch_2.Address(), ch_2.Stamp().BatchID())
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatal(err)
		}

		_, err = storer.ReserveGet(context.Background(), ch_1.Address(), ch_1.Stamp().BatchID())
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatal(err)
		}

		t.Run("reserve size", reserveSizeTest(storer.Reserve(), 0))
	}

	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := diskStorer(t, dbTestOps(baseAddr, 10, nil, nil, time.Minute))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(pullerMock.NewMockRateReporter(0))
		testF(t, baseAddr, storer)
	})
	t.Run("mem", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := memStorer(t, dbTestOps(baseAddr, 10, nil, nil, time.Minute))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(pullerMock.NewMockRateReporter(0))
		testF(t, baseAddr, storer)
	})
}

func TestReplaceOldIndex(t *testing.T) {
	t.Parallel()

	testF := func(t *testing.T, baseAddr swarm.Address, storer *storer.DB) {
		t.Helper()

		t.Run("", func(t *testing.T) {
			batch := postagetesting.MustNewBatch()
			ch_1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 0))
			ch_2 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))

			putter := storer.ReservePutter(context.Background())

			err := putter.Put(context.Background(), ch_1)
			if err != nil {
				t.Fatal(err)
			}

			err = putter.Put(context.Background(), ch_2)
			if err != nil {
				t.Fatal(err)
			}

			err = putter.Done(swarm.ZeroAddress)
			if err != nil {
				t.Fatal(err)
			}

			// Chunk 2 must be stored
			checkSaved(t, storer, ch_2, true, true)
			got, err := storer.ReserveGet(context.Background(), ch_2.Address(), ch_2.Stamp().BatchID())
			if err != nil {
				t.Fatal(err)
			}
			if !got.Address().Equal(ch_2.Address()) {
				t.Fatalf("got addr %s, want %d", got.Address(), ch_2.Address())
			}
			if !bytes.Equal(got.Stamp().BatchID(), ch_2.Stamp().BatchID()) {
				t.Fatalf("got batchID %s, want %s", hex.EncodeToString(got.Stamp().BatchID()), hex.EncodeToString(ch_2.Stamp().BatchID()))
			}

			// Chunk 1 must be missing
			item, err := stampindex.Load(storer.Repo().IndexStore(), "reserve", ch_1)
			if err != nil {
				t.Fatal(err)
			}
			if !item.ChunkAddress.Equal(ch_2.Address()) {
				t.Fatalf("wanted addr %s, got %s", ch_1.Address(), item.ChunkAddress)
			}
			_, err = chunkstamp.Load(storer.Repo().IndexStore(), "reserve", ch_1.Address())
			if !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("wanted err %s, got err %s", storage.ErrNotFound, err)
			}
			_, err = storer.Repo().ChunkStore().Get(context.Background(), ch_1.Address())
			if !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("wanted err %s, got err %s", storage.ErrNotFound, err)
			}
			_, err = storer.ReserveGet(context.Background(), ch_1.Address(), ch_1.Stamp().BatchID())
			if !errors.Is(err, storage.ErrNotFound) {
				t.Fatal(err)
			}

			t.Run("reserve size", reserveSizeTest(storer.Reserve(), 1))
		})
	}

	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := diskStorer(t, dbTestOps(baseAddr, 10, nil, nil, time.Minute))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(pullerMock.NewMockRateReporter(0))
		testF(t, baseAddr, storer)
	})
	t.Run("mem", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := memStorer(t, dbTestOps(baseAddr, 10, nil, nil, time.Minute))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(pullerMock.NewMockRateReporter(0))
		testF(t, baseAddr, storer)
	})
}

func TestEvictBatch(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)

	t.Cleanup(func() {})
	st, err := diskStorer(t, dbTestOps(baseAddr, 100, nil, nil, time.Minute))()
	if err != nil {
		t.Fatal(err)
	}
	st.StartReserveWorker(pullerMock.NewMockRateReporter(0))

	ctx := context.Background()

	var chunks []swarm.Chunk
	var chunksPerPO uint64 = 10
	batches := []*postage.Batch{postagetesting.MustNewBatch(), postagetesting.MustNewBatch(), postagetesting.MustNewBatch()}
	evictBatch := batches[1]

	putter := st.ReservePutter(ctx)

	for b := 0; b < 3; b++ {
		for i := uint64(0); i < chunksPerPO; i++ {
			ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, b)
			ch = ch.WithStamp(postagetesting.MustNewBatchStamp(batches[b].ID))
			chunks = append(chunks, ch)
			err := putter.Put(ctx, ch)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	err = putter.Done(swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	err = st.EvictBatch(ctx, evictBatch.ID)
	if err != nil {
		t.Fatal(err)
	}

	reserve := st.Reserve()

	for _, ch := range chunks {
		has, err := st.ReserveHas(ch.Address(), ch.Stamp().BatchID())
		if err != nil {
			t.Fatal(err)
		}

		if bytes.Equal(ch.Stamp().BatchID(), evictBatch.ID) {
			if has {
				t.Fatal("store should NOT have chunk")
			}
			checkSaved(t, st, ch, false, true)
		} else if !has {
			t.Fatal("store should have chunk")
			checkSaved(t, st, ch, true, true)
		}
	}

	t.Run("reserve size", reserveSizeTest(st.Reserve(), 20))

	if reserve.Radius() != 0 {
		t.Fatalf("want radius %d, got radius %d", 0, reserve.Radius())
	}

	ids, err := st.ReserveLastBinIDs()
	if err != nil {
		t.Fatal(err)
	}

	for bin, id := range ids {
		if bin < 3 && id != 9 {
			t.Fatalf("bin %d got binID %d, want %d", bin, id, 9)
		}
		if bin >= 3 && id != 0 {
			t.Fatalf("bin %d  got binID %d, want %d", bin, id, 0)
		}
	}
}

func TestUnreserveCap(t *testing.T) {
	t.Parallel()

	var (
		storageRadius = 2
		capacity      = 30
	)

	testF := func(t *testing.T, baseAddr swarm.Address, bs *batchstore.BatchStore, storer *storer.DB) {
		t.Helper()

		var chunksPO = make([][]swarm.Chunk, 5)
		var chunksPerPO uint64 = 10

		batch := postagetesting.MustNewBatch()
		err := bs.Save(batch)
		if err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()

		putter := storer.ReservePutter(ctx)

		for b := 0; b < 5; b++ {
			for i := uint64(0); i < chunksPerPO; i++ {
				ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, b)
				ch = ch.WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
				chunksPO[b] = append(chunksPO[b], ch)
				err := putter.Put(ctx, ch)
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		c, unsub := storer.Events().Subscribe("reserveUnreserved")
		t.Cleanup(func() { unsub() })

		err = putter.Done(swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}

		<-c

		t.Run("reserve size", reserveSizeTest(storer.Reserve(), capacity))

		for po, chunks := range chunksPO {
			for _, ch := range chunks {
				has, err := storer.ReserveHas(ch.Address(), ch.Stamp().BatchID())
				if err != nil {
					t.Fatal(err)
				}
				if po < storageRadius {
					if has {
						t.Fatal("store should NOT have chunk at PO", po)
					}
					checkSaved(t, storer, ch, false, true)
				} else if !has {
					t.Fatal("store should have chunk at PO", po)
				} else {
					checkSaved(t, storer, ch, true, true)
				}
			}
		}
	}

	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		bs := batchstore.New()
		baseAddr := swarm.RandAddress(t)
		storer, err := diskStorer(t, dbTestOps(baseAddr, capacity, bs, nil, time.Minute))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(pullerMock.NewMockRateReporter(0))
		testF(t, baseAddr, bs, storer)
	})
	t.Run("mem", func(t *testing.T) {
		t.Parallel()
		bs := batchstore.New()
		baseAddr := swarm.RandAddress(t)
		storer, err := memStorer(t, dbTestOps(baseAddr, capacity, bs, nil, time.Minute))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(pullerMock.NewMockRateReporter(0))
		testF(t, baseAddr, bs, storer)
	})
}

func TestRadiusManager(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)

	waitForRadius := func(t *testing.T, reserve *reserve.Reserve, expectedRadius uint8) {
		t.Helper()
		err := spinlock.Wait(time.Second*30, func() bool {
			return reserve.Radius() == expectedRadius
		})
		if err != nil {
			t.Fatalf("timed out waiting for depth, expected %d found %d", expectedRadius, reserve.Radius())
		}
	}

	waitForSize := func(t *testing.T, reserve *reserve.Reserve, size int) {
		t.Helper()
		err := spinlock.Wait(time.Second*30, func() bool {
			return reserve.Size() == size
		})
		if err != nil {
			t.Fatalf("timed out waiting for reserve size, expected %d found %d", size, reserve.Size())
		}
	}

	t.Run("radius decrease due to under utilization", func(t *testing.T) {
		t.Parallel()
		bs := batchstore.New(batchstore.WithRadius(3))

		storer, err := memStorer(t, dbTestOps(baseAddr, 10, bs, nil, time.Millisecond*50))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(pullerMock.NewMockRateReporter(0))

		batch := postagetesting.MustNewBatch()
		err = bs.Save(batch)
		if err != nil {
			t.Fatal(err)
		}

		putter := storer.ReservePutter(context.Background())

		for i := 0; i < 4; i++ {
			for j := 0; j < 10; j++ {
				ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, i).WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
				err := putter.Put(context.Background(), ch)
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		err = putter.Done(swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}

		waitForRadius(t, storer.Reserve(), 3)

		waitForSize(t, storer.Reserve(), 10)

		err = storer.EvictBatch(context.Background(), batch.ID)
		if err != nil {
			t.Fatal(err)
		}

		waitForRadius(t, storer.Reserve(), 0)
	})

	t.Run("radius doesnt change due to non-zero pull rate", func(t *testing.T) {
		t.Parallel()
		bs := batchstore.New(batchstore.WithRadius(3))
		storer, err := diskStorer(t, dbTestOps(baseAddr, 10, bs, nil, time.Millisecond*10))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(pullerMock.NewMockRateReporter(1))
		waitForRadius(t, storer.Reserve(), 3)
	})
}

func TestSubscribeBin(t *testing.T) {
	t.Parallel()

	testF := func(t *testing.T, baseAddr swarm.Address, storer *storer.DB) {
		t.Helper()
		var (
			chunks      []swarm.Chunk
			chunksPerPO uint64 = 50
			putter             = storer.ReservePutter(context.Background())
		)

		for j := 0; j < 2; j++ {
			for i := uint64(0); i < chunksPerPO; i++ {
				ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, j)
				chunks = append(chunks, ch)
				err := putter.Put(context.Background(), ch)
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		err := putter.Done(swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}

		t.Run("subscribe full range", func(t *testing.T) {
			t.Parallel()

			binC, _, _ := storer.SubscribeBin(context.Background(), 0, 0)

			i := uint64(0)
			for c := range binC {
				if !c.Address.Equal(chunks[i].Address()) {
					t.Fatal("mismatch of chunks at index", i)
				}
				i++
				if i == chunksPerPO {
					return
				}
			}
		})

		t.Run("subscribe unsub", func(t *testing.T) {
			t.Parallel()

			binC, unsub, _ := storer.SubscribeBin(context.Background(), 0, 0)

			<-binC
			unsub()

			select {
			case <-binC:
			case <-time.After(time.Second):
				t.Fatal("still waiting on result")
			}
		})

		t.Run("subscribe range higher bin", func(t *testing.T) {
			t.Parallel()

			binC, _, _ := storer.SubscribeBin(context.Background(), 0, 1)

			i := uint64(1)
			for c := range binC {
				if !c.Address.Equal(chunks[i].Address()) {
					t.Fatal("mismatch of chunks at index", i)
				}
				i++
				if i == chunksPerPO {
					return
				}
			}
		})

		t.Run("subscribe beyond range", func(t *testing.T) {
			t.Parallel()

			binC, _, _ := storer.SubscribeBin(context.Background(), 0, 1)
			i := uint64(1)
			timer := time.After(time.Millisecond * 500)

		loop:
			for {
				select {
				case c := <-binC:
					if !c.Address.Equal(chunks[i].Address()) {
						t.Fatal("mismatch of chunks at index", i)
					}
					i++
				case <-timer:
					break loop
				}
			}

			if i != chunksPerPO {
				t.Fatalf("mismatch of chunk count, got %d, want %d", i, chunksPerPO)
			}
		})
	}

	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := diskStorer(t, dbTestOps(baseAddr, 100, nil, nil, time.Second))()
		if err != nil {
			t.Fatal(err)
		}
		testF(t, baseAddr, storer)
	})
	t.Run("mem", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := memStorer(t, dbTestOps(baseAddr, 100, nil, nil, time.Second))()
		if err != nil {
			t.Fatal(err)
		}
		testF(t, baseAddr, storer)
	})
}

func TestSubscribeBinTrigger(t *testing.T) {
	t.Parallel()

	testF := func(t *testing.T, baseAddr swarm.Address, storer *storer.DB) {
		t.Helper()
		var (
			chunks      []swarm.Chunk
			chunksPerPO uint64 = 5
		)

		putter := storer.ReservePutter(context.Background())
		for j := 0; j < 2; j++ {
			for i := uint64(0); i < chunksPerPO; i++ {
				ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, j)
				chunks = append(chunks, ch)
				err := putter.Put(context.Background(), ch)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		err := putter.Done(swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}

		binC, _, _ := storer.SubscribeBin(context.Background(), 0, 1)
		i := uint64(1)
		timer := time.After(time.Millisecond * 500)

	loop:
		for {
			select {
			case c := <-binC:
				if !c.Address.Equal(chunks[i].Address()) {
					t.Fatal("mismatch of chunks at index", i)
				}
				i++
			case <-timer:
				break loop
			}
		}

		if i != chunksPerPO {
			t.Fatalf("mismatch of chunk count, got %d, want %d", i, chunksPerPO)
		}

		newChunk := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)
		putter = storer.ReservePutter(context.Background())
		err = putter.Put(context.Background(), newChunk)
		if err != nil {
			t.Fatal(err)
		}
		err = putter.Done(swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}

		select {
		case c := <-binC:
			if !c.Address.Equal(newChunk.Address()) {
				t.Fatal("mismatch of chunks")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for next chunk")
		}
	}

	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := diskStorer(t, dbTestOps(baseAddr, 100, nil, nil, time.Second))()
		if err != nil {
			t.Fatal(err)
		}
		testF(t, baseAddr, storer)
	})
	t.Run("mem", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := memStorer(t, dbTestOps(baseAddr, 100, nil, nil, time.Second))()
		if err != nil {
			t.Fatal(err)
		}
		testF(t, baseAddr, storer)
	})
}

const sampleSize = 8

func TestReserveSampler(t *testing.T) {
	const chunkCountPerPO = 10
	const maxPO = 10

	testF := func(t *testing.T, baseAddr swarm.Address, st *storer.DB) {
		t.Helper()
		var chs []swarm.Chunk

		timeVar := uint64(time.Now().UnixNano())

		for po := 0; po < maxPO; po++ {
			for i := 0; i < chunkCountPerPO; i++ {
				ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, po).WithBatch(0, 3, 2, false)
				if rand.Intn(2) == 0 { // 50% chance to wrap CAC into SOC
					ch = chunk.GenerateTestRandomSoChunk(t, ch)
				}

				// override stamp timestamp to be before the consensus timestamp
				ch = ch.WithStamp(postagetesting.MustNewStampWithTimestamp(timeVar - 1))
				chs = append(chs, ch)
			}
		}

		putter := st.ReservePutter(context.Background())
		for _, ch := range chs {
			err := putter.Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
		}
		err := putter.Done(swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}

		t.Run("reserve size", reserveSizeTest(st.Reserve(), chunkCountPerPO*maxPO))

		var sample1 storer.Sample

		t.Run("reserve sample 1", func(t *testing.T) {
			sample, err := st.ReserveSample(context.TODO(), []byte("anchor"), 5, timeVar)
			if err != nil {
				t.Fatal(err)
			}
			if len(sample.Items) != sampleSize {
				t.Fatalf("incorrect no of sample items exp %d found %d", sampleSize, len(sample.Items))
			}
			for i := 0; i < len(sample.Items)-2; i++ {
				if bytes.Compare(sample.Items[i].TransformedAddress.Bytes(), sample.Items[i+1].TransformedAddress.Bytes()) != -1 {
					t.Fatalf("incorrect order of samples %+q", sample.Items)
				}
			}

			sample1 = sample
		})

		// We generate another 100 chunks. With these new chunks in the reserve, statistically
		// some of them should definitely make it to the sample based on lex ordering.
		for po := 0; po < maxPO; po++ {
			for i := 0; i < chunkCountPerPO; i++ {
				ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, po).WithBatch(0, 3, 2, false)
				// override stamp timestamp to be after the consensus timestamp
				ch = ch.WithStamp(postagetesting.MustNewStampWithTimestamp(timeVar + 1))
				chs = append(chs, ch)
			}
		}

		putter = st.ReservePutter(context.Background())
		for _, ch := range chs {
			err := putter.Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
		}
		err = putter.Done(swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Second)

		t.Run("reserve size", reserveSizeTest(st.Reserve(), 2*chunkCountPerPO*maxPO))

		// Now we generate another sample with the older timestamp. This should give us
		// the exact same sample, ensuring that none of the later chunks were considered.
		t.Run("reserve sample 2", func(t *testing.T) {
			sample, err := st.ReserveSample(context.TODO(), []byte("anchor"), 5, timeVar)
			if err != nil {
				t.Fatal(err)
			}

			if !cmp.Equal(sample, sample1) {
				t.Fatalf("samples different (-want +have):\n%s", cmp.Diff(sample, sample1))
			}
		})

	}

	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := diskStorer(t, dbTestOps(baseAddr, 1000, nil, nil, time.Second))()
		if err != nil {
			t.Fatal(err)
		}
		testF(t, baseAddr, storer)
	})
	t.Run("mem", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := memStorer(t, dbTestOps(baseAddr, 1000, nil, nil, time.Second))()
		if err != nil {
			t.Fatal(err)
		}
		testF(t, baseAddr, storer)
	})
}

func reserveSizeTest(rs *reserve.Reserve, want int) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()
		got := rs.Size()
		if got != want {
			t.Errorf("got reserve size %v, want %v", got, want)
		}
	}
}

func checkSaved(t *testing.T, st *storer.DB, ch swarm.Chunk, stampSaved, chunkStoreSaved bool) {
	t.Helper()

	var stampWantedErr error
	if !stampSaved {
		stampWantedErr = storage.ErrNotFound
	}
	_, err := stampindex.Load(st.Repo().IndexStore(), "reserve", ch)
	if !errors.Is(err, stampWantedErr) {
		t.Fatalf("wanted err %s, got err %s", stampWantedErr, err)
	}
	_, err = chunkstamp.Load(st.Repo().IndexStore(), "reserve", ch.Address())
	if !errors.Is(err, stampWantedErr) {
		t.Fatalf("wanted err %s, got err %s", stampWantedErr, err)
	}

	var chunkStoreWantedErr error
	if !chunkStoreSaved {
		chunkStoreWantedErr = storage.ErrNotFound
	}
	_, err = st.Repo().ChunkStore().Get(context.Background(), ch.Address())
	if !errors.Is(err, chunkStoreWantedErr) {
		t.Fatalf("wanted err %s, got err %s", chunkStoreWantedErr, err)
	}
}
