// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"bytes"
	"context"
	"errors"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	soctesting "github.com/ethersphere/bee/v2/pkg/soc/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/stretchr/testify/assert"
)

func TestReserve(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)

	ts := internal.NewInmemStorage()

	r, err := reserve.New(
		baseAddr,
		ts,
		0, kademlia.NewTopologyDriver(),
		log.Noop,
	)
	if err != nil {
		t.Fatal(err)
	}

	for b := 0; b < 2; b++ {
		for i := 1; i < 51; i++ {
			ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, b)
			err := r.Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
			stampHash, err := ch.Stamp().Hash()
			if err != nil {
				t.Fatal(err)
			}
			checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: uint8(b), BatchID: ch.Stamp().BatchID(), Address: ch.Address(), StampHash: stampHash}, false)
			checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: uint8(b), BinID: uint64(i), StampHash: stampHash}, false)
			checkChunk(t, ts, ch, false)
			h, err := r.Has(ch.Address(), ch.Stamp().BatchID(), stampHash)
			if err != nil {
				t.Fatal(err)
			}
			if !h {
				t.Fatalf("expected chunk addr %s binID %d", ch.Address(), i)
			}

			chGet, err := r.Get(context.Background(), ch.Address(), ch.Stamp().BatchID(), stampHash)
			if err != nil {
				t.Fatal(err)
			}
			if !chGet.Equal(ch) {
				t.Fatalf("expected addr %s, got %s", ch.Address(), chGet.Address())
			}
		}
	}
}

func TestReserveChunkType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)

	ts := internal.NewInmemStorage()

	r, err := reserve.New(
		baseAddr,
		ts,
		0, kademlia.NewTopologyDriver(),
		log.Noop,
	)
	if err != nil {
		t.Fatal(err)
	}

	storedChunksCA := 0
	storedChunksSO := 0
	for i := 0; i < 100; i++ {
		ch := chunk.GenerateTestRandomChunk()
		if rand.Intn(2) == 0 {
			storedChunksCA++
		} else {
			ch = chunk.GenerateTestRandomSoChunk(t, ch)
			storedChunksSO++
		}
		if err := r.Put(ctx, ch); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}

	err = ts.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &reserve.ChunkBinItem{} },
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*reserve.ChunkBinItem)
		if item.ChunkType == swarm.ChunkTypeContentAddressed {
			storedChunksCA--
		} else if item.ChunkType == swarm.ChunkTypeSingleOwner {
			storedChunksSO--
		} else {
			t.Fatalf("unexpected chunk type: %d", item.ChunkType)
		}
		return false, nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if storedChunksCA != 0 {
		t.Fatal("unexpected number of content addressed chunks")
	}
	if storedChunksSO != 0 {
		t.Fatal("unexpected number of single owner chunks")
	}
}

func TestSameChunkAddress(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)

	ts := internal.NewInmemStorage()

	r, err := reserve.New(
		baseAddr,
		ts,
		0, kademlia.NewTopologyDriver(),
		log.Noop,
	)
	if err != nil {
		t.Fatal(err)
	}

	binBinIDs := make(map[uint8]uint64)

	t.Run("same stamp index and older timestamp", func(t *testing.T) {
		size1 := r.Size()
		signer := getSigner(t)
		batch := postagetesting.MustNewBatch()
		s1 := soctesting.GenerateMockSocWithSigner(t, []byte("data"), signer)
		ch1 := s1.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))
		s2 := soctesting.GenerateMockSocWithSigner(t, []byte("update"), signer)
		ch2 := s2.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 0, 0))
		err = r.Put(ctx, ch1)
		if err != nil {
			t.Fatal(err)
		}
		bin := swarm.Proximity(baseAddr.Bytes(), ch1.Address().Bytes())
		binBinIDs[bin] += 1
		err = r.Put(ctx, ch2)
		if !errors.Is(err, storage.ErrOverwriteNewerChunk) {
			t.Fatal("expected error")
		}
		size2 := r.Size()
		if size2-size1 != 1 {
			t.Fatalf("expected reserve size to increase by 1, got %d", size2-size1)
		}
	})

	t.Run("different stamp index and older timestamp", func(t *testing.T) {
		size1 := r.Size()
		signer := getSigner(t)
		batch := postagetesting.MustNewBatch()
		s1 := soctesting.GenerateMockSocWithSigner(t, []byte("data"), signer)
		ch1 := s1.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 0, 2))
		s2 := soctesting.GenerateMockSocWithSigner(t, []byte("update"), signer)
		ch2 := s2.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 1, 0))
		err = r.Put(ctx, ch1)
		if err != nil {
			t.Fatal(err)
		}
		bin := swarm.Proximity(baseAddr.Bytes(), ch1.Address().Bytes())
		binBinIDs[bin] += 1
		err = r.Put(ctx, ch2)
		if err != nil {
			t.Fatal(err)
		}
		bin2 := swarm.Proximity(baseAddr.Bytes(), ch2.Address().Bytes())
		binBinIDs[bin2] += 1
		size2 := r.Size()
		if size2-size1 != 2 {
			t.Fatalf("expected reserve size to increase by 2, got %d", size2-size1)
		}
	})

	replace := func(t *testing.T, ch1 swarm.Chunk, ch2 swarm.Chunk, ch1BinID, ch2BinID uint64) {
		t.Helper()

		err := r.Put(ctx, ch1)
		if err != nil {
			t.Fatal(err)
		}

		err = r.Put(ctx, ch2)
		if err != nil {
			t.Fatal(err)
		}

		ch1StampHash, err := ch1.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}

		ch2StampHash, err := ch2.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}

		bin := swarm.Proximity(baseAddr.Bytes(), ch1.Address().Bytes())
		checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: bin, BatchID: ch1.Stamp().BatchID(), Address: ch1.Address(), StampHash: ch1StampHash}, true)
		checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: bin, BatchID: ch2.Stamp().BatchID(), Address: ch2.Address(), StampHash: ch2StampHash}, false)
		checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: bin, BinID: ch1BinID, StampHash: ch1StampHash}, true)
		checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: bin, BinID: ch2BinID, StampHash: ch2StampHash}, false)
		ch, err := ts.ChunkStore().Get(ctx, ch2.Address())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(ch.Data(), ch2.Data()) {
			t.Fatalf("expected chunk data to be updated")
		}
	}

	t.Run("same stamp index and newer timestamp", func(t *testing.T) {
		size1 := r.Size()
		signer := getSigner(t)
		batch := postagetesting.MustNewBatch()
		s1 := soctesting.GenerateMockSocWithSigner(t, []byte("data"), signer)
		ch1 := s1.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 0, 3))
		s2 := soctesting.GenerateMockSocWithSigner(t, []byte("update"), signer)
		ch2 := s2.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 0, 4))
		bin := swarm.Proximity(baseAddr.Bytes(), ch1.Address().Bytes())
		binBinIDs[bin] += 2
		replace(t, ch1, ch2, binBinIDs[bin]-1, binBinIDs[bin])
		size2 := r.Size()
		if size2-size1 != 1 {
			t.Fatalf("expected reserve size to increase by 1, got %d", size2-size1)
		}
	})

	t.Run("different stamp index and newer timestamp", func(t *testing.T) {
		size1 := r.Size()
		signer := getSigner(t)
		batch := postagetesting.MustNewBatch()
		s1 := soctesting.GenerateMockSocWithSigner(t, []byte("data"), signer)
		ch1 := s1.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 0, 5))
		s2 := soctesting.GenerateMockSocWithSigner(t, []byte("update"), signer)
		ch2 := s2.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 1, 6))
		bin := swarm.Proximity(baseAddr.Bytes(), ch1.Address().Bytes())
		err := r.Put(ctx, ch1)
		if err != nil {
			t.Fatal(err)
		}
		err = r.Put(ctx, ch2)
		if err != nil {
			t.Fatal(err)
		}
		binBinIDs[bin] += 2
		checkChunkInIndexStore(t, ts.IndexStore(), bin, binBinIDs[bin]-1, ch1)
		checkChunkInIndexStore(t, ts.IndexStore(), bin, binBinIDs[bin], ch2)
		size2 := r.Size()
		if size2-size1 != 2 {
			t.Fatalf("expected reserve size to increase by 2, got %d", size2-size1)
		}
	})

	t.Run("not a soc and newer timestamp", func(t *testing.T) {
		size1 := r.Size()
		batch := postagetesting.MustNewBatch()
		ch1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 7))
		ch2 := swarm.NewChunk(ch1.Address(), []byte("update")).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 8))
		err := r.Put(ctx, ch1)
		if err != nil {
			t.Fatal(err)
		}

		bin1 := swarm.Proximity(baseAddr.Bytes(), ch1.Address().Bytes())
		binBinIDs[bin1] += 1

		err = r.Put(ctx, ch2)
		if err != nil {
			t.Fatal(err)
		}

		bin2 := swarm.Proximity(baseAddr.Bytes(), ch2.Address().Bytes())
		binBinIDs[bin2] += 1

		ch, err := ts.ChunkStore().Get(ctx, ch2.Address())
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(ch.Data(), ch1.Data()) {
			t.Fatalf("expected chunk data to not be updated")
		}

		size2 := r.Size()
		if size2-size1 != 1 {
			t.Fatalf("expected reserve size to increase by 2, got %d", size2-size1)
		}
	})

	t.Run("chunk with different batchID remains untouched", func(t *testing.T) {
		checkReplace := func(ch1, ch2 swarm.Chunk, replace bool) {
			t.Helper()
			err = r.Put(ctx, ch1)
			if err != nil {
				t.Fatal(err)
			}

			err = r.Put(ctx, ch2)
			if err != nil {
				t.Fatal(err)
			}

			ch1StampHash, err := ch1.Stamp().Hash()
			if err != nil {
				t.Fatal(err)
			}
			ch2StampHash, err := ch2.Stamp().Hash()
			if err != nil {
				t.Fatal(err)
			}

			bin := swarm.Proximity(baseAddr.Bytes(), ch2.Address().Bytes())
			binBinIDs[bin] += 2

			// expect both entries in reserve
			checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: bin, BatchID: ch1.Stamp().BatchID(), Address: ch1.Address(), StampHash: ch1StampHash}, false)
			checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: bin, BatchID: ch2.Stamp().BatchID(), Address: ch2.Address(), StampHash: ch2StampHash}, false)
			checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: bin, BinID: binBinIDs[bin] - 1}, false)
			checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: bin, BinID: binBinIDs[bin]}, false)

			ch, err := ts.ChunkStore().Get(ctx, ch2.Address())
			if err != nil {
				t.Fatal(err)
			}
			if replace && bytes.Equal(ch.Data(), ch1.Data()) {
				t.Fatalf("expected chunk data to be updated")
			}
		}

		size1 := r.Size()

		// soc
		signer := getSigner(t)
		batch := postagetesting.MustNewBatch()
		s1 := soctesting.GenerateMockSocWithSigner(t, []byte("data"), signer)
		ch1 := s1.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 0, 3))
		batch = postagetesting.MustNewBatch()
		s2 := soctesting.GenerateMockSocWithSigner(t, []byte("update"), signer)
		ch2 := s2.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 0, 4))

		if !bytes.Equal(ch1.Address().Bytes(), ch2.Address().Bytes()) {
			t.Fatalf("expected chunk addresses to be the same")
		}
		checkReplace(ch1, ch2, true)

		// cac
		batch = postagetesting.MustNewBatch()
		ch1 = chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 5))
		batch = postagetesting.MustNewBatch()
		ch2 = swarm.NewChunk(ch1.Address(), []byte("update")).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 6))
		if !bytes.Equal(ch1.Address().Bytes(), ch2.Address().Bytes()) {
			t.Fatalf("expected chunk addresses to be the same")
		}
		checkReplace(ch1, ch2, false)
		size2 := r.Size()
		if size2-size1 != 4 {
			t.Fatalf("expected reserve size to increase by 4, got %d", size2-size1)
		}
	})

	t.Run("same address but index collision with different chunk", func(t *testing.T) {
		size1 := r.Size()
		batch := postagetesting.MustNewBatch()
		ch1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))
		err = r.Put(ctx, ch1)
		if err != nil {
			t.Fatal(err)
		}
		bin1 := swarm.Proximity(baseAddr.Bytes(), ch1.Address().Bytes())
		binBinIDs[bin1] += 1
		ch1BinID := binBinIDs[bin1]
		ch1StampHash, err := ch1.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}

		signer := getSigner(t)
		s1 := soctesting.GenerateMockSocWithSigner(t, []byte("data"), signer)
		ch2 := s1.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 1, 2))
		err = r.Put(ctx, ch2)
		if err != nil {
			t.Fatal(err)
		}
		bin2 := swarm.Proximity(baseAddr.Bytes(), ch2.Address().Bytes())
		binBinIDs[bin2] += 1
		ch2BinID := binBinIDs[bin2]
		ch2StampHash, err := ch2.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}

		checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: bin1, BatchID: ch1.Stamp().BatchID(), Address: ch1.Address(), StampHash: ch1StampHash}, false)
		checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: bin2, BatchID: ch2.Stamp().BatchID(), Address: ch2.Address(), StampHash: ch2StampHash}, false)
		checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: bin1, BinID: binBinIDs[bin1], StampHash: ch1StampHash}, false)
		checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: bin2, BinID: binBinIDs[bin2], StampHash: ch2StampHash}, false)

		// attempt to replace existing (unrelated) chunk that has timestamp
		s2 := soctesting.GenerateMockSocWithSigner(t, []byte("update"), signer)
		ch3 := s2.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 0, 0))
		err = r.Put(ctx, ch3)
		if !errors.Is(err, storage.ErrOverwriteNewerChunk) {
			t.Fatal("expected error")
		}

		s2 = soctesting.GenerateMockSocWithSigner(t, []byte("update"), signer)
		ch3 = s2.Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, 0, 3))
		err = r.Put(ctx, ch3)
		if err != nil {
			t.Fatal(err)
		}
		binBinIDs[bin2] += 1
		ch3StampHash, err := ch3.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		ch3BinID := binBinIDs[bin2]

		checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: bin1, BatchID: ch1.Stamp().BatchID(), Address: ch1.Address(), StampHash: ch1StampHash}, true)
		// different index, same batch
		checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: bin2, BatchID: ch2.Stamp().BatchID(), Address: ch2.Address(), StampHash: ch2StampHash}, false)
		checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: bin2, BatchID: ch3.Stamp().BatchID(), Address: ch3.Address(), StampHash: ch3StampHash}, false)
		checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: bin1, BinID: ch1BinID, StampHash: ch1StampHash}, true)
		checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: bin2, BinID: ch2BinID, StampHash: ch2StampHash}, false)
		checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: bin2, BinID: ch3BinID, StampHash: ch3StampHash}, false)

		size2 := r.Size()

		// (ch1 + ch2) == 2
		if size2-size1 != 2 {
			t.Fatalf("expected reserve size to increase by 2, got %d", size2-size1)
		}
	})

}

func TestReplaceOldIndex(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)

	ts := internal.NewInmemStorage()

	r, err := reserve.New(
		baseAddr,
		ts,
		0, kademlia.NewTopologyDriver(),
		log.Noop,
	)
	if err != nil {
		t.Fatal(err)
	}

	batch := postagetesting.MustNewBatch()
	ch1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 0))
	ch2 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))

	err = r.Put(context.Background(), ch1)
	if err != nil {
		t.Fatal(err)
	}

	err = r.Put(context.Background(), ch2)
	if err != nil {
		t.Fatal(err)
	}

	// Chunk 1 must be gone
	ch1StampHash, err := ch1.Stamp().Hash()
	if err != nil {
		t.Fatal(err)
	}
	checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: 0, BatchID: ch1.Stamp().BatchID(), Address: ch1.Address(), StampHash: ch1StampHash}, true)
	checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: 0, BinID: 1, StampHash: ch1StampHash}, true)
	checkChunk(t, ts, ch1, true)

	// Chunk 2 must be stored
	ch2StampHash, err := ch2.Stamp().Hash()
	if err != nil {
		t.Fatal(err)
	}
	checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: 0, BatchID: ch2.Stamp().BatchID(), Address: ch2.Address(), StampHash: ch2StampHash}, false)
	checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: 0, BinID: 2, StampHash: ch2StampHash}, false)
	checkChunk(t, ts, ch2, false)

	item, err := stampindex.Load(ts.IndexStore(), "reserve", ch2.Stamp())
	if err != nil {
		t.Fatal(err)
	}
	if !item.ChunkAddress.Equal(ch2.Address()) {
		t.Fatalf("wanted ch2 address")
	}
}

func TestEvict(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)

	ts := internal.NewInmemStorage()

	chunksPerBatch := 50
	var chunks []swarm.Chunk
	batches := []*postage.Batch{postagetesting.MustNewBatch(), postagetesting.MustNewBatch(), postagetesting.MustNewBatch()}
	evictBatch := batches[1]

	r, err := reserve.New(
		baseAddr,
		ts,
		0, kademlia.NewTopologyDriver(),
		log.Noop,
	)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < chunksPerBatch; i++ {
		for b := 0; b < 3; b++ {
			ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, b).WithStamp(postagetesting.MustNewBatchStamp(batches[b].ID))
			chunks = append(chunks, ch)
			err := r.Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	totalEvicted := 0
	for i := 0; i < 3; i++ {
		evicted, err := r.EvictBatchBin(context.Background(), evictBatch.ID, math.MaxInt, uint8(i))
		if err != nil {
			t.Fatal(err)
		}
		totalEvicted += evicted
	}

	if totalEvicted != chunksPerBatch {
		t.Fatalf("got %d, want %d", totalEvicted, chunksPerBatch)
	}

	time.Sleep(time.Second)

	for i, ch := range chunks {
		binID := i%chunksPerBatch + 1
		b := swarm.Proximity(baseAddr.Bytes(), ch.Address().Bytes())
		stampHash, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		_, err = r.Get(context.Background(), ch.Address(), ch.Stamp().BatchID(), stampHash)
		if bytes.Equal(ch.Stamp().BatchID(), evictBatch.ID) {
			if !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("got err %v, want %v", err, storage.ErrNotFound)
			}
			checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: b, BatchID: ch.Stamp().BatchID(), Address: ch.Address(), StampHash: stampHash}, true)
			checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: b, BinID: uint64(binID), StampHash: stampHash}, true)
			checkChunk(t, ts, ch, true)
		} else {
			if err != nil {
				t.Fatal(err)
			}
			checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: b, BatchID: ch.Stamp().BatchID(), Address: ch.Address(), StampHash: stampHash}, false)
			checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: b, BinID: uint64(binID), StampHash: stampHash}, false)
			checkChunk(t, ts, ch, false)
		}
	}
}

func TestEvictSOC(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	ts := internal.NewInmemStorage()

	r, err := reserve.New(
		baseAddr,
		ts,
		0, kademlia.NewTopologyDriver(),
		log.Noop,
	)
	if err != nil {
		t.Fatal(err)
	}

	batch := postagetesting.MustNewBatch()
	signer := getSigner(t)

	var chunks []swarm.Chunk

	for i := 0; i < 10; i++ {
		ch := soctesting.GenerateMockSocWithSigner(t, []byte{byte(i)}, signer).Chunk().WithStamp(postagetesting.MustNewFields(batch.ID, uint64(i), uint64(i)))
		chunks = append(chunks, ch)
		err := r.Put(context.Background(), ch)
		if err != nil {
			t.Fatal(err)
		}
	}

	bin := swarm.Proximity(baseAddr.Bytes(), chunks[0].Address().Bytes())

	for i, ch := range chunks {
		stampHash, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: bin, BatchID: ch.Stamp().BatchID(), Address: ch.Address(), StampHash: stampHash}, false)
		checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: bin, BinID: uint64(i + 1), StampHash: stampHash}, false)
		checkChunk(t, ts, ch, false)
	}

	_, err = r.EvictBatchBin(context.Background(), batch.ID, 1, swarm.MaxBins)
	if err != nil {
		t.Fatal(err)
	}
	if has, _ := ts.ChunkStore().Has(context.Background(), chunks[0].Address()); !has {
		t.Fatal("same address chunk should still persist, eg refCnt > 0")
	}

	evicted, err := r.EvictBatchBin(context.Background(), batch.ID, 10, swarm.MaxBins)
	if err != nil {
		t.Fatal(err)
	}
	if evicted != 9 {
		t.Fatalf("wanted evicted count 10, got %d", evicted)
	}

	for i, ch := range chunks {
		stampHash, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: bin, BatchID: ch.Stamp().BatchID(), Address: ch.Address(), StampHash: stampHash}, true)
		checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: bin, BinID: uint64(i + 1), StampHash: stampHash}, true)
		checkChunk(t, ts, ch, true)
	}
}

func TestEvictMaxCount(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)

	ts := internal.NewInmemStorage()

	r, err := reserve.New(
		baseAddr,
		ts,
		0, kademlia.NewTopologyDriver(),
		log.Noop,
	)
	if err != nil {
		t.Fatal(err)
	}

	var chunks []swarm.Chunk

	batch := postagetesting.MustNewBatch()

	for b := 0; b < 2; b++ {
		for i := 0; i < 10; i++ {
			ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, b).WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
			chunks = append(chunks, ch)
			err := r.Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	evicted, err := r.EvictBatchBin(context.Background(), batch.ID, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if evicted != 10 {
		t.Fatalf("wanted evicted count 10, got %d", evicted)
	}

	for i, ch := range chunks {
		stampHash, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		if i < 10 {
			checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: 0, BatchID: ch.Stamp().BatchID(), Address: ch.Address(), StampHash: stampHash}, true)
			checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: 0, BinID: uint64(i + 1), StampHash: stampHash}, true)
			checkChunk(t, ts, ch, true)
		} else {
			checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: 1, BatchID: ch.Stamp().BatchID(), Address: ch.Address(), StampHash: stampHash}, false)
			checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: 1, BinID: uint64(i - 10 + 1), StampHash: stampHash}, false)
			checkChunk(t, ts, ch, false)
		}
	}
}

func TestIterate(t *testing.T) {
	t.Parallel()

	createReserve := func(t *testing.T) *reserve.Reserve {
		t.Helper()

		baseAddr := swarm.RandAddress(t)

		ts := internal.NewInmemStorage()

		r, err := reserve.New(
			baseAddr,
			ts,
			0, kademlia.NewTopologyDriver(),
			log.Noop,
		)
		if err != nil {
			t.Fatal(err)
		}

		for b := 0; b < 3; b++ {
			for i := 0; i < 10; i++ {
				ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, b)
				err := r.Put(context.Background(), ch)
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		return r
	}

	t.Run("iterate bin", func(t *testing.T) {
		t.Parallel()

		r := createReserve(t)

		var id uint64 = 1
		err := r.IterateBin(1, 0, func(ch swarm.Address, binID uint64, _, _ []byte) (bool, error) {
			if binID != id {
				t.Fatalf("got %d, want %d", binID, id)
			}
			id++
			return false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if id != 11 {
			t.Fatalf("got %d, want %d", id, 11)
		}
	})

	t.Run("iterate chunks", func(t *testing.T) {
		t.Parallel()

		r := createReserve(t)

		count := 0
		err := r.IterateChunks(2, func(_ swarm.Chunk) (bool, error) {
			count++
			return false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if count != 10 {
			t.Fatalf("got %d, want %d", count, 10)
		}
	})

	t.Run("iterate chunk items", func(t *testing.T) {
		t.Parallel()

		r := createReserve(t)

		count := 0
		err := r.IterateChunksItems(0, func(_ *reserve.ChunkBinItem) (bool, error) {
			count++
			return false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if count != 30 {
			t.Fatalf("got %d, want %d", count, 30)
		}
	})

	t.Run("last bin id", func(t *testing.T) {
		t.Parallel()

		r := createReserve(t)

		ids, _, err := r.LastBinIDs()
		if err != nil {
			t.Fatal(err)
		}
		for i, id := range ids {
			if i < 3 {
				if id != 10 {
					t.Fatalf("got %d, want %d", id, 10)
				}
			} else {
				if id != 0 {
					t.Fatalf("got %d, want %d", id, 0)
				}
			}
		}
	})
}

func TestReset(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)

	ts := internal.NewInmemStorage()

	r, err := reserve.New(
		baseAddr,
		ts,
		0, kademlia.NewTopologyDriver(),
		log.Noop,
	)
	if err != nil {
		t.Fatal(err)
	}

	var chs []swarm.Chunk

	var (
		bins         = 5
		chunksPerBin = 100
		total        = bins * chunksPerBin
	)

	for b := 0; b < bins; b++ {
		for i := 1; i <= chunksPerBin; i++ {
			ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, b)
			err := r.Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
			stampHash, err := ch.Stamp().Hash()
			if err != nil {
				t.Fatal(err)
			}
			checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: uint8(b), BatchID: ch.Stamp().BatchID(), Address: ch.Address(), StampHash: stampHash}, false)
			checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: uint8(b), BinID: uint64(i), StampHash: stampHash}, false)
			checkChunk(t, ts, ch, false)
			_, err = r.Get(context.Background(), ch.Address(), ch.Stamp().BatchID(), stampHash)
			if err != nil {
				t.Fatal(err)
			}
			chs = append(chs, ch)
		}
	}

	c, err := ts.IndexStore().Count(&reserve.BatchRadiusItem{})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, c, total)
	c, err = ts.IndexStore().Count(&reserve.ChunkBinItem{})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, c, total)
	c, err = ts.IndexStore().Count(&stampindex.Item{})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, c, total)

	cItem := &chunkstamp.Item{}
	cItem.SetScope([]byte("reserve"))
	c, err = ts.IndexStore().Count(cItem)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, c, total)

	checkStore(t, ts.IndexStore(), &reserve.EpochItem{}, false)

	ids, _, err := r.LastBinIDs()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, ids[0], uint64(chunksPerBin))

	err = r.Reset(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	c, err = ts.IndexStore().Count(&reserve.BatchRadiusItem{})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, c, 0)
	c, err = ts.IndexStore().Count(&reserve.ChunkBinItem{})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, c, 0)
	c, err = ts.IndexStore().Count(&stampindex.Item{})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, c, 0)

	c, err = ts.IndexStore().Count(cItem)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, c, 0)

	checkStore(t, ts.IndexStore(), &reserve.EpochItem{}, true)

	_, _, err = r.LastBinIDs()
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("wanted %v, got %v", storage.ErrNotFound, err)
	}

	for _, c := range chs {
		h, err := c.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		_, err = r.Get(context.Background(), c.Address(), c.Stamp().BatchID(), h)
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("expected error %v, got %v", storage.ErrNotFound, err)
		}
	}
}

func checkStore(t *testing.T, s storage.Reader, k storage.Key, gone bool) {
	t.Helper()
	h, err := s.Has(k)
	if err != nil {
		t.Fatal(err)
	}
	if gone && h {
		t.Fatalf("unexpected entry in %s-%s ", k.Namespace(), k.ID())
	}
	if !gone && !h {
		t.Fatalf("expected entry in %s-%s ", k.Namespace(), k.ID())
	}
}

func checkChunk(t *testing.T, s transaction.ReadOnlyStore, ch swarm.Chunk, gone bool) {
	t.Helper()
	h, err := s.ChunkStore().Has(context.Background(), ch.Address())
	if err != nil {
		t.Fatal(err)
	}

	hash, err := ch.Stamp().Hash()
	if err != nil {
		t.Fatal(err)
	}

	_, err = chunkstamp.LoadWithStampHash(s.IndexStore(), "reserve", ch.Address(), hash)
	if !gone && err != nil {
		t.Fatal(err)
	}
	if gone && !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("got err %v, want %v", err, storage.ErrNotFound)
	}

	if gone && h {
		t.Fatalf("unexpected entry %s", ch.Address())
	}
	if !gone && !h {
		t.Fatalf("expected entry %s", ch.Address())
	}
}

func getSigner(t *testing.T) crypto.Signer {
	t.Helper()
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	return crypto.NewDefaultSigner(privKey)
}

func checkChunkInIndexStore(t *testing.T, s storage.Reader, bin uint8, binId uint64, ch swarm.Chunk) {
	t.Helper()
	stampHash, err := ch.Stamp().Hash()
	if err != nil {
		t.Fatal(err)
	}

	checkStore(t, s, &reserve.BatchRadiusItem{Bin: bin, BatchID: ch.Stamp().BatchID(), Address: ch.Address(), StampHash: stampHash}, false)
	checkStore(t, s, &reserve.ChunkBinItem{Bin: bin, BinID: binId, StampHash: stampHash}, false)
}
