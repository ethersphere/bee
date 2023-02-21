// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	storer "github.com/ethersphere/bee/pkg/localstorev2"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/reserve"
	"github.com/ethersphere/bee/pkg/postage"
	batchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/pullsync"
	"github.com/ethersphere/bee/pkg/spinlock"
	statestoreMock "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	chunk "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/ethersphere/bee/pkg/topology"
	kademlia "github.com/ethersphere/bee/pkg/topology/mock"
	"github.com/google/go-cmp/cmp"
)

func TestEvictBatch(t *testing.T) {

	baseAddr := test.RandomAddress()
	dbReserveOps(baseAddr, 100, nil, nil, nil, nil, time.Minute)
	storer, err := diskStorer(t, dbReserveOps(baseAddr, 100, nil, nil, nil, nil, time.Minute))()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var chunks []swarm.Chunk
	stamps := []*postage.Stamp{postagetesting.MustNewStamp(), postagetesting.MustNewStamp(), postagetesting.MustNewStamp()}
	evictBatchID := stamps[1].BatchID()

	putter := storer.ReservePutter(ctx)

	for i := 0; i < 10; i++ {
		for b := 0; b < 3; b++ {
			ch := chunk.GenerateTestRandomChunkAt(baseAddr, b)
			ch = ch.WithStamp(stamps[b])
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

	err = storer.EvictBatch(ctx, evictBatchID)
	if err != nil {
		t.Fatal(err)
	}

	chunkStore := storer.Repo().ChunkStore()
	reserve := storer.Reserve()

	for _, ch := range chunks {
		has, err := chunkStore.Has(ctx, ch.Address())
		if err != nil {
			t.Fatal(err)
		}

		if bytes.Equal(ch.Stamp().BatchID(), evictBatchID) {
			if has {
				t.Fatal("store should NOT have chunk")
			}
		} else if !has {
			t.Fatal("store should have chunk")
		}
	}

	if reserve.Size() != 20 {
		t.Fatalf("want size %d, got size %d", 20, reserve.Size())
	}

	if reserve.Radius() != 0 {
		t.Fatalf("want radius %d, got radius %d", 0, reserve.Radius())
	}
}

// 	err = reserve.Close()
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	reserve, err = createReserve(t, baseAddr, 100, ldbStore, nil, nil, chunkStore, nil, nil, nil, wakeup)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	// check size after Close
// 	if reserve.Size() != 20 {
// 		t.Fatalf("want size %d, got size %d", 20, reserve.Size())
// 	}
// }

func TestUnreserveCap(t *testing.T) {

	var (
		storageRadius = 2
		capacity      = 30
		bs            = batchstore.New()
		baseAddr      = test.RandomAddress()
	)

	storer, err := diskStorer(t, dbReserveOps(baseAddr, capacity, nil, bs, nil, nil, time.Minute))()
	if err != nil {
		t.Fatal(err)
	}

	chunkStore := storer.Repo().ChunkStore()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var chunksPO = make([][]swarm.Chunk, 5)

	batch := postagetesting.MustNewBatch()
	stamp := postagetesting.MustNewBatchStamp(batch.ID)
	err = bs.Save(batch)
	if err != nil {
		t.Fatal(err)
	}

	putter := storer.ReservePutter(ctx)

	for b := 0; b < 5; b++ {
		for i := 0; i < 10; i++ {
			ch := chunk.GenerateTestRandomChunkAt(baseAddr, b)
			ch = ch.WithStamp(stamp)
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

	for po, chunks := range chunksPO {
		for _, ch := range chunks {
			has, err := chunkStore.Has(ctx, ch.Address())
			if err != nil {
				t.Fatal(err)
			}
			if po < storageRadius {
				if has {
					t.Fatal("store should NOT have chunk at PO", po)
				}
			} else if !has {
				t.Fatal("store should have chunk at PO", po)
			}
		}
	}
}

func TestRadiusManager(t *testing.T) {

	baseAddr := test.RandomAddress()

	waitForRadius := func(t *testing.T, reserve *reserve.Reserve, expectedRadius uint8) {
		t.Helper()

		err := spinlock.Wait(time.Second*3, func() bool {
			return reserve.Radius() == expectedRadius
		})
		if err != nil {
			t.Fatalf("timed out waiting for depth, expected %d found %d", expectedRadius, reserve.Radius())
		}
	}

	t.Run("old nodes starts at previous radius", func(t *testing.T) {
		t.Parallel()
		ss := statestoreMock.NewStateStore()

		var radius uint8 = 0
		_ = ss.Put("reserve_storage_radius", &radius)

		var (
			capacity = 100
			bs       = batchstore.New(batchstore.WithReserveState(&postage.ReserveState{Radius: 3}))
		)

		storer, err := diskStorer(t, dbReserveOps(baseAddr, capacity, nil, bs, nil, nil, time.Millisecond))()
		if err != nil {
			t.Fatal(err)
		}

		if err != nil {
			t.Fatal(err)
		}
		waitForRadius(t, storer.Reserve(), 0)
	})

	t.Run("radius decrease due to under utilization", func(t *testing.T) {
		t.Parallel()
		ss := statestoreMock.NewStateStore()
		bs := batchstore.New(batchstore.WithReserveState(&postage.ReserveState{Radius: 3}))

		storer, err := diskStorer(t, dbReserveOps(baseAddr, 10, ss, bs, nil, nil, time.Second))()
		if err != nil {
			t.Fatal(err)
		}

		batch := postagetesting.MustNewBatch()
		stamp := postagetesting.MustNewBatchStamp(batch.ID)
		err = bs.Save(batch)
		if err != nil {
			t.Fatal(err)
		}

		putter := storer.ReservePutter(context.Background())

		for i := 0; i < 4; i++ {
			for j := 0; j < 10; j++ {
				ch := chunk.GenerateTestRandomChunkAt(baseAddr, i)
				ch = ch.WithStamp(stamp)
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

		err = storer.EvictBatch(context.Background(), batch.ID)
		if err != nil {
			t.Fatal(err)
		}

		waitForRadius(t, storer.Reserve(), 0)
	})

	t.Run("radius doesnt change due to non-zero pull rate", func(t *testing.T) {
		t.Parallel()

		ss := statestoreMock.NewStateStore()
		bs := batchstore.New(batchstore.WithReserveState(&postage.ReserveState{Radius: 3}))

		storer, err := diskStorer(t, dbReserveOps(baseAddr, 10, ss, bs, &mockSyncReporter{rate: 1}, nil, time.Millisecond))()
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Second)
		waitForRadius(t, storer.Reserve(), 3)

	})
}

func TestSubscribeBin(t *testing.T) {
	t.Parallel()

	baseAddr := test.RandomAddress()
	storer, err := diskStorer(t, dbReserveOps(baseAddr, 100, nil, nil, nil, nil, reserve.DefaultRadiusWakeUpTime))()
	if err != nil {
		t.Fatal(err)
	}

	var (
		chunks      []swarm.Chunk
		chunksPerPO uint64 = 5
	)

	putter := storer.ReservePutter(context.Background())

	for j := 0; j < 2; j++ {
		for i := uint64(0); i < chunksPerPO; i++ {
			ch := chunk.GenerateTestRandomChunkAt(baseAddr, j)
			chunks = append(chunks, ch)
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

	t.Run("subscribe full range", func(t *testing.T) {
		t.Parallel()

		binC, _ := storer.SubscribeBin(context.Background(), 0, 0, chunksPerPO-1)

		i := uint64(0)
		for c := range binC {
			fmt.Println("+", i, c.BinID, c.Address, chunks[i].Address())
			if !c.Address.Equal(chunks[i].Address()) {
				t.Fatal("mismatch of chunks at index", i)
			}
			i++
		}

		if i != chunksPerPO {
			t.Fatalf("mismatch of chunk count, got %d, want %d", i, chunksPerPO)
		}

	})

	t.Run("subscribe sub range", func(t *testing.T) {
		t.Parallel()

		binC, _ := storer.SubscribeBin(context.Background(), 0, 1, chunksPerPO-1)

		i := uint64(1)
		for c := range binC {
			fmt.Println("+", i, c.BinID, c.Address, chunks[i].Address())
			if !c.Address.Equal(chunks[i].Address()) {
				t.Fatal("mismatch of chunks at index", i)
			}
			i++
		}

		if i != chunksPerPO {
			t.Fatalf("mismatch of chunk count, got %d, want %d", i, chunksPerPO)
		}
	})

	t.Run("subscribe beyond range", func(t *testing.T) {
		t.Parallel()

		binC, _ := storer.SubscribeBin(context.Background(), 0, 1, chunksPerPO)
		i := uint64(1)
		timer := time.After(time.Millisecond * 500)

	loop:
		for {
			select {
			case c := <-binC:
				fmt.Println("+", i, c.BinID, c.Address, chunks[i].Address())
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

func TestSubscribeBinTrigger(t *testing.T) {
	t.Parallel()

	baseAddr := test.RandomAddress()
	storer, err := diskStorer(t, dbReserveOps(baseAddr, 100, nil, nil, nil, nil, reserve.DefaultRadiusWakeUpTime))()
	if err != nil {
		t.Fatal(err)
	}

	var (
		chunks      []swarm.Chunk
		chunksPerPO uint64 = 5
	)

	putter := storer.ReservePutter(context.Background())
	for j := 0; j < 2; j++ {
		for i := uint64(0); i < chunksPerPO; i++ {
			ch := chunk.GenerateTestRandomChunkAt(baseAddr, j)
			chunks = append(chunks, ch)
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

	binC, _ := storer.SubscribeBin(context.Background(), 0, 1, chunksPerPO)
	i := uint64(1)
	timer := time.After(time.Millisecond * 500)

loop:
	for {
		select {
		case c := <-binC:
			fmt.Println("+", i, c.BinID, c.Address, chunks[i].Address())
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

	newChunk := chunk.GenerateTestRandomChunkAt(baseAddr, 0)
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

const sampleSize = 8

func TestReserveSampler(t *testing.T) {
	const chunkCountPerPO = 10
	const maxPO = 10
	var chs []swarm.Chunk
	baseAddr := test.RandomAddress()

	storer, err := diskStorer(t, dbReserveOps(baseAddr, 1000, nil, nil, nil, nil, reserve.DefaultRadiusWakeUpTime))()
	if err != nil {
		t.Fatal(err)
	}

	timeVar := uint64(time.Now().UnixNano())

	for po := 0; po < maxPO; po++ {
		for i := 0; i < chunkCountPerPO; i++ {
			ch := chunk.GenerateTestRandomChunkAt(baseAddr, po).WithBatch(0, 3, 2, false)
			// override stamp timestamp to be before the consensus timestamp
			ch = ch.WithStamp(postagetesting.MustNewStampWithTimestamp(timeVar - 1))
			chs = append(chs, ch)
		}
	}

	putter := storer.ReservePutter(context.Background())
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

	t.Run("reserve size", reserveSizeTest(storer.Reserve(), chunkCountPerPO*maxPO))

	var sample1 reserve.Sample

	t.Run("reserve sample 1", func(t *testing.T) {
		sample, err := storer.ReserveSample(context.TODO(), []byte("anchor"), 5, timeVar)
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
			ch := chunk.GenerateTestRandomChunkAt(baseAddr, po).WithBatch(0, 3, 2, false)
			// override stamp timestamp to be after the consensus timestamp
			ch = ch.WithStamp(postagetesting.MustNewStampWithTimestamp(timeVar + 1))
			chs = append(chs, ch)
		}
	}

	putter = storer.ReservePutter(context.Background())
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

	t.Run("reserve size", reserveSizeTest(storer.Reserve(), 3*chunkCountPerPO*maxPO))

	// Now we generate another sample with the older timestamp. This should give us
	// the exact same sample, ensuring that none of the later chunks were considered.
	t.Run("reserve sample 2", func(t *testing.T) {
		sample, err := storer.ReserveSample(context.TODO(), []byte("anchor"), 5, timeVar)
		if err != nil {
			t.Fatal(err)
		}

		if !cmp.Equal(sample, sample1) {
			t.Fatalf("samples different (-want +have):\n%s", cmp.Diff(sample, sample1))
		}
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

func dbReserveOps(baseAddr swarm.Address, capacity int, stateStore storage.StateStorer, bs postage.Storer, syncer pullsync.SyncReporter, radiusSetter topology.SetStorageRadiuser, wakeUpTime time.Duration) *storer.Options {

	opts := storer.DefaultOptions()

	if stateStore == nil {
		stateStore = statestoreMock.NewStateStore()
	}

	if radiusSetter == nil {
		radiusSetter = kademlia.NewTopologyDriver()
	}

	if bs == nil {
		bs = batchstore.New()
	}

	if syncer == nil {
		syncer = &mockSyncReporter{}
	}

	opts.Address = baseAddr
	opts.RadiusSetter = radiusSetter
	opts.ReserveCapacity = capacity
	opts.Batchstore = bs
	opts.StateStore = stateStore
	opts.Syncer = syncer

	return opts

}

type mockSyncReporter struct {
	rate float64
}

func (m *mockSyncReporter) Rate() float64 {
	return m.rate
}
