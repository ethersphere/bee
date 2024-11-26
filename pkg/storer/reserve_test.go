// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/postage"
	batchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	pullerMock "github.com/ethersphere/bee/v2/pkg/puller/mock"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestIndexCollision(t *testing.T) {
	t.Parallel()

	testF := func(t *testing.T, baseAddr swarm.Address, storer *storer.DB) {
		t.Helper()
		stamp := postagetesting.MustNewBatchStamp(postagetesting.MustNewBatch().ID)
		putter := storer.ReservePutter()

		ch1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(stamp)
		err := putter.Put(context.Background(), ch1)
		if err != nil {
			t.Fatal(err)
		}

		ch2 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(stamp)
		err = putter.Put(context.Background(), ch2)
		if err == nil {
			t.Fatal("expected index collision error")
		}

		ch1StampHash, err := ch1.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		_, err = storer.ReserveGet(context.Background(), ch2.Address(), ch2.Stamp().BatchID(), ch1StampHash)
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatal(err)
		}

		ch2StampHash, err := ch1.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		_, err = storer.ReserveGet(context.Background(), ch1.Address(), ch1.Stamp().BatchID(), ch2StampHash)
		if err != nil {
			t.Fatal(err)
		}

		t.Run("reserve size", reserveSizeTest(storer.Reserve(), 1))
	}

	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := diskStorer(t, dbTestOps(baseAddr, 10, nil, nil, time.Minute))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(0), networkRadiusFunc(0))
		testF(t, baseAddr, storer)
	})
	t.Run("mem", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := memStorer(t, dbTestOps(baseAddr, 10, nil, nil, time.Minute))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(0), networkRadiusFunc(0))
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

			putter := storer.ReservePutter()

			err := putter.Put(context.Background(), ch_1)
			if err != nil {
				t.Fatal(err)
			}

			err = putter.Put(context.Background(), ch_2)
			if err != nil {
				t.Fatal(err)
			}

			// Chunk 2 must be stored
			checkSaved(t, storer, ch_2, true, true)
			ch2StampHash, err := ch_2.Stamp().Hash()
			if err != nil {
				t.Fatal(err)
			}
			got, err := storer.ReserveGet(context.Background(), ch_2.Address(), ch_2.Stamp().BatchID(), ch2StampHash)
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
			item, err := stampindex.Load(storer.Storage().IndexStore(), "reserve", ch_1.Stamp())
			if err != nil {
				t.Fatal(err)
			}
			if !item.ChunkAddress.Equal(ch_2.Address()) {
				t.Fatalf("wanted addr %s, got %s", ch_1.Address(), item.ChunkAddress)
			}
			_, err = chunkstamp.Load(storer.Storage().IndexStore(), "reserve", ch_1.Address())
			if !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("wanted err %s, got err %s", storage.ErrNotFound, err)
			}

			ch1StampHash, err := ch_1.Stamp().Hash()
			if err != nil {
				t.Fatal(err)
			}
			_, err = storer.ReserveGet(context.Background(), ch_1.Address(), ch_1.Stamp().BatchID(), ch1StampHash)
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
		storer.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(0), networkRadiusFunc(0))
		testF(t, baseAddr, storer)
	})
	t.Run("mem", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := memStorer(t, dbTestOps(baseAddr, 10, nil, nil, time.Minute))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(0), networkRadiusFunc(0))
		testF(t, baseAddr, storer)
	})
}

func TestEvictBatch(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)

	st, err := diskStorer(t, dbTestOps(baseAddr, 100, nil, nil, time.Minute))()
	if err != nil {
		t.Fatal(err)
	}
	st.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(0), networkRadiusFunc(0))

	ctx := context.Background()

	var chunks []swarm.Chunk
	var chunksPerPO uint64 = 10
	batches := []*postage.Batch{postagetesting.MustNewBatch(), postagetesting.MustNewBatch(), postagetesting.MustNewBatch()}
	evictBatch := batches[1]

	putter := st.ReservePutter()

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

	c, unsub := st.Events().Subscribe("batchExpiryDone")
	t.Cleanup(unsub)

	err = st.EvictBatch(ctx, evictBatch.ID)
	if err != nil {
		t.Fatal(err)
	}
	<-c

	reserve := st.Reserve()

	for _, ch := range chunks {
		stampHash, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		has, err := st.ReserveHas(ch.Address(), ch.Stamp().BatchID(), stampHash)
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
			checkSaved(t, st, ch, true, true)
		}
	}

	t.Run("reserve size", reserveSizeTest(st.Reserve(), 20))

	if reserve.Radius() != 0 {
		t.Fatalf("want radius %d, got radius %d", 0, reserve.Radius())
	}

	ids, _, err := st.ReserveLastBinIDs()
	if err != nil {
		t.Fatal(err)
	}

	for bin, id := range ids {
		if bin < 3 && id != 10 {
			t.Fatalf("bin %d got binID %d, want %d", bin, id, 10)
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

		putter := storer.ReservePutter()

		c, unsub := storer.Events().Subscribe("reserveUnreserved")
		defer unsub()

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

	done:
		for {
			select {
			case <-c:
				if storer.ReserveSize() == capacity {
					break done
				}
			case <-time.After(time.Second * 30):
				if storer.ReserveSize() != capacity {
					t.Fatal("timeout waiting for reserve to reach capacity")
				}
			}
		}

		for po, chunks := range chunksPO {
			for _, ch := range chunks {
				stampHash, err := ch.Stamp().Hash()
				if err != nil {
					t.Fatal(err)
				}
				has, err := storer.ReserveHas(ch.Address(), ch.Stamp().BatchID(), stampHash)
				if err != nil {
					t.Fatal(err)
				}
				if po < storageRadius {
					if has {
						t.Fatal("store should NOT have chunk at PO", po)
					}
					checkSaved(t, storer, ch, false, false)
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
		storer.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(0), networkRadiusFunc(0))
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
		storer.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(0), networkRadiusFunc(0))
		testF(t, baseAddr, bs, storer)
	})
}

func TestNetworkRadius(t *testing.T) {
	t.Parallel()

	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := diskStorer(t, dbTestOps(baseAddr, 10, nil, nil, time.Minute))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(0), networkRadiusFunc(1))
		time.Sleep(time.Second)
		if want, got := uint8(1), storer.StorageRadius(); want != got {
			t.Fatalf("want radius %d, got radius %d", want, got)
		}
	})
	t.Run("mem", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		storer, err := memStorer(t, dbTestOps(baseAddr, 10, nil, nil, time.Minute))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(0), networkRadiusFunc(1))
		time.Sleep(time.Second)
		if want, got := uint8(1), storer.StorageRadius(); want != got {
			t.Fatalf("want radius %d, got radius %d", want, got)
		}
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
		bs := batchstore.New()

		storer, err := memStorer(t, dbTestOps(baseAddr, 10, bs, nil, time.Millisecond*500))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(0), networkRadiusFunc(3))

		batch := postagetesting.MustNewBatch()
		err = bs.Save(batch)
		if err != nil {
			t.Fatal(err)
		}

		putter := storer.ReservePutter()

		for i := 0; i < 4; i++ {
			for j := 0; j < 10; j++ {
				ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, i).WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
				err := putter.Put(context.Background(), ch)
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		waitForSize(t, storer.Reserve(), 10)
		waitForRadius(t, storer.Reserve(), 3)

		err = storer.EvictBatch(context.Background(), batch.ID)
		if err != nil {
			t.Fatal(err)
		}
		waitForRadius(t, storer.Reserve(), 0)
	})

	t.Run("radius doesn't change due to non-zero pull rate", func(t *testing.T) {
		t.Parallel()
		storer, err := diskStorer(t, dbTestOps(baseAddr, 10, nil, nil, time.Millisecond*500))()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(1), networkRadiusFunc(3))
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
			putter             = storer.ReservePutter()
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

			binC, _, _ := storer.SubscribeBin(context.Background(), 0, 2)

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

			binC, _, _ := storer.SubscribeBin(context.Background(), 0, 2)
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

		putter := storer.ReservePutter()
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

		binC, _, _ := storer.SubscribeBin(context.Background(), 0, 2)
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
		putter = storer.ReservePutter()
		err := putter.Put(context.Background(), newChunk)
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

func TestNeighborhoodStats(t *testing.T) {
	t.Parallel()

	const (
		chunkCountPerPO          = 16
		maxPO                    = 5
		committedDepth     uint8 = 4
		doublingFactor     uint8 = 2
		responsibiliyDepth uint8 = committedDepth - doublingFactor
	)

	mustParse := func(s string) swarm.Address {
		addr, err := swarm.ParseBitStrAddress(s)
		if err != nil {
			t.Fatal(err)
		}
		return addr
	}

	var (
		baseAddr = mustParse("10000")
		sister1  = mustParse("10010")
		sister2  = mustParse("10100")
		sister3  = mustParse("10110")
	)

	putChunks := func(addr swarm.Address, startingRadius int, st *storer.DB) {
		putter := st.ReservePutter()
		for i := 0; i < chunkCountPerPO; i++ {
			ch := chunk.GenerateValidRandomChunkAt(t, addr, startingRadius)
			err := putter.Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	testF := func(t *testing.T, st *storer.DB) {
		t.Helper()

		putChunks(baseAddr, int(committedDepth), st)
		putChunks(sister1, int(committedDepth), st)
		putChunks(sister2, int(committedDepth), st)
		putChunks(sister3, int(committedDepth), st)

		neighs, err := st.NeighborhoodsStat(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		if len(neighs) != (1 << doublingFactor) {
			t.Fatalf("number of neighborhoods does not matche. wanted %d, got %d", 1<<doublingFactor, len(neighs))
		}

		for _, n := range neighs {
			if n.ReserveSizeWithinRadius != chunkCountPerPO {
				t.Fatalf("chunk count does not match. wanted %d, got %d, prox %d", chunkCountPerPO, n.ReserveSizeWithinRadius, swarm.Proximity(baseAddr.Bytes(), n.Neighborhood.Bytes()))
			}
		}

		if !neighs[0].Neighborhood.Equal(swarm.NewNeighborhood(baseAddr, committedDepth)) ||
			!neighs[1].Neighborhood.Equal(swarm.NewNeighborhood(sister1, committedDepth)) ||
			!neighs[2].Neighborhood.Equal(swarm.NewNeighborhood(sister2, committedDepth)) ||
			!neighs[3].Neighborhood.Equal(swarm.NewNeighborhood(sister3, committedDepth)) {
			t.Fatal("chunk addresses do not match")
		}

		if neighs[0].Proximity != committedDepth ||
			neighs[1].Proximity != 3 ||
			neighs[2].Proximity != 2 ||
			neighs[3].Proximity != 2 {
			t.Fatalf("wrong proximity")
		}
	}

	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		opts := dbTestOps(baseAddr, 1000, nil, nil, time.Minute)
		opts.ReserveCapacityDoubling = int(doublingFactor)
		storer, err := diskStorer(t, opts)()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(0), networkRadiusFunc(responsibiliyDepth))
		err = spinlock.Wait(time.Minute, func() bool { return storer.StorageRadius() == responsibiliyDepth })
		if err != nil {
			t.Fatal(err)
		}
		testF(t, storer)
	})
	t.Run("mem", func(t *testing.T) {
		t.Parallel()
		opts := dbTestOps(baseAddr, 1000, nil, nil, time.Minute)
		opts.ReserveCapacityDoubling = int(doublingFactor)
		storer, err := memStorer(t, opts)()
		if err != nil {
			t.Fatal(err)
		}
		storer.StartReserveWorker(context.Background(), pullerMock.NewMockRateReporter(0), networkRadiusFunc(responsibiliyDepth))
		err = spinlock.Wait(time.Minute, func() bool { return storer.StorageRadius() == responsibiliyDepth })
		if err != nil {
			t.Fatal(err)
		}
		testF(t, storer)
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
	_, err := stampindex.Load(st.Storage().IndexStore(), "reserve", ch.Stamp())
	if !errors.Is(err, stampWantedErr) {
		t.Fatalf("wanted err %s, got err %s", stampWantedErr, err)
	}
	_, err = chunkstamp.Load(st.Storage().IndexStore(), "reserve", ch.Address())
	if !errors.Is(err, stampWantedErr) {
		t.Fatalf("wanted err %s, got err %s", stampWantedErr, err)
	}

	var chunkStoreWantedErr error
	if !chunkStoreSaved {
		chunkStoreWantedErr = storage.ErrNotFound
	}
	gotCh, err := st.Storage().ChunkStore().Get(context.Background(), ch.Address())
	if !errors.Is(err, chunkStoreWantedErr) {
		t.Fatalf("wanted err %s, got err %s", chunkStoreWantedErr, err)
	}
	if chunkStoreSaved {
		if !bytes.Equal(ch.Data(), gotCh.Data()) {
			t.Fatalf("chunks are not equal: %s", ch.Address())
		}
	}
}

func BenchmarkReservePutter(b *testing.B) {
	baseAddr := swarm.RandAddress(b)
	storer, err := diskStorer(b, dbTestOps(baseAddr, 10000, nil, nil, time.Second))()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	storagetest.BenchmarkChunkStoreWriteSequential(b, storer.ReservePutter())
}

func networkRadiusFunc(r uint8) func() (uint8, error) {
	return func() (uint8, error) { return r, nil }
}
