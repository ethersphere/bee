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

	storer "github.com/ethersphere/bee/pkg/localstorev2"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/reserve"
	"github.com/ethersphere/bee/pkg/postage"
	batchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/spinlock"
	chunk "github.com/ethersphere/bee/pkg/storage/testing"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/google/go-cmp/cmp"
)

func TestIndexCollision(t *testing.T) {
	t.Parallel()

	baseAddr := test.RandomAddress()
	storer, err := diskStorer(t, dbTestOps(baseAddr, 10, nil, nil, nil, time.Minute))()
	if err != nil {
		t.Fatal(err)
	}

	stamp := postagetesting.MustNewBatchStamp(postagetesting.MustNewBatch().ID)
	putter := storer.ReservePutter(context.Background())

	ch_1 := chunk.GenerateTestRandomChunkAt(baseAddr, 0)
	err = putter.Put(context.Background(), ch_1.WithStamp(stamp))
	if err != nil {
		t.Fatal(err)
	}

	ch_2 := chunk.GenerateTestRandomChunkAt(baseAddr, 0)
	err = putter.Put(context.Background(), chunk.GenerateTestRandomChunkAt(baseAddr, 0).WithStamp(stamp))
	if err == nil {
		t.Fatal("expected index collision error")
	}

	err = putter.Done(swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	_, err = storer.ReserveGet(context.Background(), ch_2.Address())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(err)
	}

	got, err := storer.ReserveGet(context.Background(), ch_1.Address())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Address().Equal(ch_1.Address()) {
		t.Fatalf("got addr %s, want %d", got.Address(), ch_1.Address())
	}

	if !bytes.Equal(got.Stamp().BatchID(), ch_1.Stamp().BatchID()) {
		t.Fatalf("got batchID %s, want %s", hex.EncodeToString(got.Stamp().BatchID()), hex.EncodeToString(ch_1.Stamp().BatchID()))
	}
}

func TestEvictBatch(t *testing.T) {
	t.Parallel()

	baseAddr := test.RandomAddress()

	dir := t.TempDir()
	opts := dbTestOps(baseAddr, 100, nil, nil, nil, time.Minute)
	st, err := storer.New(context.Background(), dir, opts)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var chunks []swarm.Chunk
	batches := []*postage.Batch{postagetesting.MustNewBatch(), postagetesting.MustNewBatch(), postagetesting.MustNewBatch()}
	evictBatch := batches[1]

	putter := st.ReservePutter(ctx)

	for i := 0; i < 10; i++ {
		for b := 0; b < 3; b++ {
			ch := chunk.GenerateTestRandomChunkAt(baseAddr, b)
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
		has, err := st.ReserveHas(ch.Address())
		if err != nil {
			t.Fatal(err)
		}

		if bytes.Equal(ch.Stamp().BatchID(), evictBatch.ID) {
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

	err = st.Close()
	if err != nil {
		t.Fatal(err)
	}

	st, err = storer.New(context.Background(), dir, opts)
	if err != nil {
		t.Fatal(err)
	}

	if got := st.Reserve().Size(); got != 20 {
		t.Fatalf("want size %d, got size %d", 20, got)
	}
}

func TestUnreserveCap(t *testing.T) {
	t.Parallel()

	var (
		storageRadius = 2
		capacity      = 30
		bs            = batchstore.New()
		baseAddr      = test.RandomAddress()
	)

	storer, err := diskStorer(t, dbTestOps(baseAddr, capacity, bs, nil, nil, time.Minute))()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	var chunksPO = make([][]swarm.Chunk, 5)

	batch := postagetesting.MustNewBatch()
	err = bs.Save(batch)
	if err != nil {
		t.Fatal(err)
	}

	putter := storer.ReservePutter(ctx)

	for b := 0; b < 5; b++ {
		for i := 0; i < 10; i++ {
			ch := chunk.GenerateTestRandomChunkAt(baseAddr, b)
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

	for po, chunks := range chunksPO {
		for _, ch := range chunks {
			has, err := storer.ReserveHas(ch.Address())
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
	t.Parallel()

	baseAddr := test.RandomAddress()

	waitForRadius := func(t *testing.T, reserve *reserve.Reserve, expectedRadius uint8) {
		t.Helper()
		err := spinlock.Wait(time.Second*5, func() bool {
			return reserve.Radius() == expectedRadius
		})
		if err != nil {
			t.Fatalf("timed out waiting for depth, expected %d found %d", expectedRadius, reserve.Radius())
		}
	}

	t.Run("old nodes starts at previous radius", func(t *testing.T) {
		t.Parallel()

		var (
			capacity = 100
			bs       = batchstore.New(batchstore.WithReserveState(&postage.ReserveState{Radius: 3}))
			dir      = t.TempDir()
		)

		opts := dbTestOps(baseAddr, capacity, bs, &mockSyncReporter{rate: 1}, nil, time.Millisecond*50)
		st, err := storer.New(context.Background(), dir, opts)
		if err != nil {
			t.Fatal(err)
		}
		waitForRadius(t, st.Reserve(), 3)
		err = st.Close()
		if err != nil {
			t.Fatal(err)
		}

		bs = batchstore.New(batchstore.WithReserveState(&postage.ReserveState{Radius: 4}))
		opts.Batchstore = bs
		st, err = storer.New(context.Background(), dir, opts)
		if err != nil {
			t.Fatal(err)
		}

		waitForRadius(t, st.Reserve(), 3)
	})

	t.Run("radius decrease due to under utilization", func(t *testing.T) {
		t.Parallel()
		bs := batchstore.New(batchstore.WithReserveState(&postage.ReserveState{Radius: 3}))

		storer, err := diskStorer(t, dbTestOps(baseAddr, 10, bs, nil, nil, time.Millisecond*50))()
		if err != nil {
			t.Fatal(err)
		}

		batch := postagetesting.MustNewBatch()
		err = bs.Save(batch)
		if err != nil {
			t.Fatal(err)
		}

		putter := storer.ReservePutter(context.Background())

		for i := 0; i < 4; i++ {
			for j := 0; j < 10; j++ {
				ch := chunk.GenerateTestRandomChunkAt(baseAddr, i)
				ch = ch.WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
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

		waitForRadius(t, storer.Reserve(), 4)

		err = storer.EvictBatch(context.Background(), batch.ID)
		if err != nil {
			t.Fatal(err)
		}

		waitForRadius(t, storer.Reserve(), 0)
	})

	t.Run("radius doesnt change due to non-zero pull rate", func(t *testing.T) {
		t.Parallel()
		bs := batchstore.New(batchstore.WithReserveState(&postage.ReserveState{Radius: 3}))
		storer, err := diskStorer(t, dbTestOps(baseAddr, 10, bs, &mockSyncReporter{rate: 1}, nil, time.Millisecond*50))()
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
	storer, err := diskStorer(t, dbTestOps(baseAddr, 100, nil, nil, nil, time.Second))()
	if err != nil {
		t.Fatal(err)
	}

	var (
		chunks      []swarm.Chunk
		chunksPerPO uint64 = 5
		putter             = storer.ReservePutter(context.Background())
	)

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

		binC, _, _ := storer.SubscribeBin(context.Background(), 0, 0, chunksPerPO-1)

		i := uint64(0)
		for c := range binC {
			if !c.Address.Equal(chunks[i].Address()) {
				t.Fatal("mismatch of chunks at index", i)
			}
			i++
		}

		if i != chunksPerPO {
			t.Fatalf("mismatch of chunk count, got %d, want %d", i, chunksPerPO)
		}

	})

	t.Run("subscribe unsub", func(t *testing.T) {
		t.Parallel()

		binC, unsub, _ := storer.SubscribeBin(context.Background(), 0, 0, chunksPerPO-1)

		<-binC
		unsub()

		select {
		case <-binC:
		case <-time.After(time.Second):
			t.Fatal("still waiting on result")
		}
	})

	t.Run("subscribe sub range", func(t *testing.T) {
		t.Parallel()

		binC, _, _ := storer.SubscribeBin(context.Background(), 0, 1, chunksPerPO-1)

		i := uint64(1)
		for c := range binC {
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

		binC, _, _ := storer.SubscribeBin(context.Background(), 0, 1, chunksPerPO)
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

func TestSubscribeBinTrigger(t *testing.T) {
	t.Parallel()

	baseAddr := test.RandomAddress()
	storer, err := diskStorer(t, dbTestOps(baseAddr, 100, nil, nil, nil, time.Second))()
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

	binC, _, _ := storer.SubscribeBin(context.Background(), 0, 1, chunksPerPO)
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

	st, err := diskStorer(t, dbTestOps(baseAddr, 1000, nil, nil, nil, time.Second))()
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

	putter := st.ReservePutter(context.Background())
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

	t.Run("reserve size", reserveSizeTest(st.Reserve(), 3*chunkCountPerPO*maxPO))

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

func reserveSizeTest(rs *reserve.Reserve, want int) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		got := rs.Size()
		if got != want {
			t.Errorf("got reserve size %v, want %v", got, want)
		}
	}
}

type mockSyncReporter struct {
	rate float64
}

func (m *mockSyncReporter) Rate() float64 {
	return m.rate
}
