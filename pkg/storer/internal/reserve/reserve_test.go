// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"testing/synctest"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	soctesting "github.com/ethersphere/bee/v2/pkg/soc/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
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

	for b := range 2 {
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
	for range 100 {
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
		switch item.ChunkType {
		case swarm.ChunkTypeContentAddressed:
			storedChunksCA--
		case swarm.ChunkTypeSingleOwner:
			storedChunksSO--
		default:
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
	synctest.Test(t, func(t *testing.T) {
		baseAddr := swarm.RandAddress(t)

		ts := internal.NewInmemStorage()

		chunksPerBatch := 50
		chunks := make([]swarm.Chunk, 0, 3*chunksPerBatch)
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

		for range chunksPerBatch {
			for b := range 3 {
				ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, b).WithStamp(postagetesting.MustNewBatchStamp(batches[b].ID))
				chunks = append(chunks, ch)
				err := r.Put(context.Background(), ch)
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		totalEvicted := 0
		for i := range 3 {
			evicted, err := r.EvictBatchBin(context.Background(), evictBatch.ID, math.MaxInt, uint8(i))
			if err != nil {
				t.Fatal(err)
			}
			totalEvicted += evicted
		}

		if totalEvicted != chunksPerBatch {
			t.Fatalf("got %d, want %d", totalEvicted, chunksPerBatch)
		}

		synctest.Wait()

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
	})
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

	chunks := make([]swarm.Chunk, 0, 10)

	for i := range 10 {
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

	chunks := make([]swarm.Chunk, 0, 20)

	batch := postagetesting.MustNewBatch()

	for b := range 2 {
		for range 10 {
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

		for b := range 3 {
			for range 10 {
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
		err := r.IterateBin(1, 0, func(ch swarm.Address, binID uint64, _, _, _ []byte) (bool, error) {
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

	for b := range bins {
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

// TestEvictRemovesPinnedContent checks that pinned chunks are protected from eviction.
func TestEvictRemovesPinnedContent(t *testing.T) {
	t.Parallel()

	const (
		numChunks       = 5
		numPinnedChunks = 3
	)

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	ts := internal.NewInmemStorage()

	r, err := reserve.New(
		baseAddr,
		ts,
		0,
		kademlia.NewTopologyDriver(),
		log.Noop,
	)
	if err != nil {
		t.Fatal(err)
	}

	batch := postagetesting.MustNewBatch()

	chunks := make([]swarm.Chunk, numChunks)
	for i := range numChunks {
		ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
		chunks[i] = ch

		err := r.Put(ctx, ch)
		if err != nil {
			t.Fatal(err)
		}
	}

	var pinningPutter internal.PutterCloserWithReference
	err = ts.Run(ctx, func(store transaction.Store) error {
		pinningPutter, err = pinstore.NewCollection(store.IndexStore())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add chunks to pin collection
	pinnedChunks := chunks[:numPinnedChunks]
	for _, ch := range pinnedChunks {
		err = ts.Run(ctx, func(s transaction.Store) error {
			return pinningPutter.Put(ctx, s, ch)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ts.Run(ctx, func(s transaction.Store) error {
		return pinningPutter.Close(s.IndexStore(), pinnedChunks[0].Address())
	})
	if err != nil {
		t.Fatal(err)
	}

	// evict all chunks from this batch - this should NOT remove pinned chunks
	evicted, err := r.EvictBatchBin(ctx, batch.ID, numChunks, swarm.MaxBins)
	if err != nil {
		t.Fatal(err)
	}
	if evicted != numChunks {
		t.Fatalf("expected %d evicted chunks, got %d", numChunks, evicted)
	}

	uuids, err := pinstore.GetCollectionUUIDs(ts.IndexStore())
	if err != nil {
		t.Fatal(err)
	}
	if len(uuids) != 1 {
		t.Fatalf("expected exactly one pin collection, but found %d", len(uuids))
	}

	for i, ch := range chunks {
		stampHash, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}

		// Try to get the chunk from reserve, error is checked later
		_, err = r.Get(ctx, ch.Address(), ch.Stamp().BatchID(), stampHash)

		// Also try to get chunk directly from chunkstore (like bzz/bytes endpoints do)
		_, chunkStoreErr := ts.ChunkStore().Get(ctx, ch.Address())

		pinned := false
		for _, uuid := range uuids {
			has, err := pinstore.IsChunkPinnedInCollection(ts.IndexStore(), ch.Address(), uuid)
			if err != nil {
				t.Fatal(err)
			}
			if has {
				pinned = true
			}
		}

		if i < len(pinnedChunks) {
			if pinned {
				// This chunk is pinned, so it should NOT be accessible from reserve but SHOULD be accessible from chunkstore
				if !errors.Is(err, storage.ErrNotFound) {
					t.Errorf("Pinned chunk %s should have been evicted from reserve", ch.Address())
				}
				if errors.Is(chunkStoreErr, storage.ErrNotFound) {
					t.Errorf("Pinned chunk %s was deleted from chunkstore - should remain retrievable!", ch.Address())
				} else if chunkStoreErr != nil {
					t.Fatal(chunkStoreErr)
				}
			} else {
				t.Errorf("Chunk %s should be pinned", ch.Address())
			}
		} else { // unpinned chunks
			if !pinned {
				// Unpinned chunks should be completely evicted (both reserve and chunkstore)
				if !errors.Is(err, storage.ErrNotFound) {
					t.Errorf("Unpinned chunk %s should have been evicted from reserve", ch.Address())
				}
				if !errors.Is(chunkStoreErr, storage.ErrNotFound) {
					t.Errorf("Unpinned chunk %s should have been evicted from chunkstore", ch.Address())
				}
			} else {
				t.Errorf("Chunk %s should not be pinned", ch.Address())
			}
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

// TestChunkSumIndexLockstep asserts the invariant the pullsync want-decision
// depends on: a ChunkSumItem exists exactly as long as its chunk is in the
// reserve. A stale entry would make the node silently refuse to sync a chunk
// it no longer holds.
func TestChunkSumIndexLockstep(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	ts := internal.NewInmemStorage()
	r, err := reserve.New(baseAddr, ts, 0, kademlia.NewTopologyDriver(), log.Noop)
	if err != nil {
		t.Fatal(err)
	}

	batch := postagetesting.MustNewBatch()
	ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
	if err := r.Put(ctx, ch); err != nil {
		t.Fatal(err)
	}

	sum, err := storage.ChunkSum(ch)
	if err != nil {
		t.Fatal(err)
	}

	has, err := r.HasSum(ch.Address(), sum)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected chunk sum to be indexed after put")
	}

	evicted, err := r.EvictBatchBin(ctx, batch.ID, math.MaxInt, swarm.MaxBins)
	if err != nil {
		t.Fatal(err)
	}
	if evicted != 1 {
		t.Fatalf("expected 1 chunk evicted, got %d", evicted)
	}

	has, err = r.HasSum(ch.Address(), sum)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected chunk sum index entry to be removed by eviction")
	}
}

// TestSOCSiblingSumRefresh covers a single owner chunk address stored under
// multiple batches. The payload is stored once per address, so replacing it
// through one batch's entry must refresh the divergence checksums of the other
// batches' entries; a stale sum would keep advertising content the node no
// longer holds and keep matching offers for content it cannot store.
func TestSOCSiblingSumRefresh(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	ts := internal.NewInmemStorage()
	r, err := reserve.New(baseAddr, ts, 0, kademlia.NewTopologyDriver(), log.Noop)
	if err != nil {
		t.Fatal(err)
	}

	signer := getSigner(t)
	batchA := postagetesting.MustNewBatch()
	batchB := postagetesting.MustNewBatch()

	// same owner and id: all three chunks share one SOC address
	s1 := soctesting.GenerateMockSocWithSigner(t, []byte("v1"), signer)
	s2 := soctesting.GenerateMockSocWithSigner(t, []byte("v2"), signer)
	s3 := soctesting.GenerateMockSocWithSigner(t, []byte("v3"), signer)

	stampA := postagetesting.MustNewFields(batchA.ID, 0, 1)
	stampB := postagetesting.MustNewFields(batchB.ID, 0, 1)
	chA := s1.Chunk().WithStamp(stampA)
	chB := s2.Chunk().WithStamp(stampB)

	addr := chA.Address()
	bin := swarm.Proximity(baseAddr.Bytes(), addr.Bytes())

	stampHashA, err := stampA.Hash()
	if err != nil {
		t.Fatal(err)
	}
	stampHashB, err := stampB.Hash()
	if err != nil {
		t.Fatal(err)
	}

	// sumOf reads the sum stored on the ChunkBinItem of the entry identified
	// by the given batch and stamp hash.
	sumOf := func(batchID, stampHash []byte) []byte {
		t.Helper()
		item := &reserve.BatchRadiusItem{Bin: bin, BatchID: batchID, Address: addr, StampHash: stampHash}
		if err := ts.IndexStore().Get(item); err != nil {
			t.Fatal(err)
		}
		cbi := &reserve.ChunkBinItem{Bin: bin, BinID: item.BinID}
		if err := ts.IndexStore().Get(cbi); err != nil {
			t.Fatal(err)
		}
		return cbi.Sum
	}

	if err := r.Put(ctx, chA); err != nil {
		t.Fatal(err)
	}
	staleSumA := sumOf(batchA.ID, stampHashA)

	// storing the same address under batch B replaces the shared payload with
	// v2; batch A's entry must be re-summed against the new payload.
	if err := r.Put(ctx, chB); err != nil {
		t.Fatal(err)
	}

	freshSumA, err := storage.ChunkSumFromParts(batchA.ID, stampHashA, chB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sumOf(batchA.ID, stampHashA), freshSumA) {
		t.Fatal("expected batch A entry to be re-summed against the replacing payload")
	}
	has, err := r.HasSum(addr, staleSumA)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected the stale sum of batch A to be dropped")
	}
	has, err = r.HasSum(addr, freshSumA)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected the refreshed sum of batch A to be indexed")
	}

	// the same-batch replacement path (higher stamp timestamp) must refresh
	// batch B's entry the same way.
	staleSumB := sumOf(batchB.ID, stampHashB)
	chA2 := s3.Chunk().WithStamp(postagetesting.MustNewFields(batchA.ID, 0, 2))
	if err := r.Put(ctx, chA2); err != nil {
		t.Fatal(err)
	}

	freshSumB, err := storage.ChunkSumFromParts(batchB.ID, stampHashB, chA2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sumOf(batchB.ID, stampHashB), freshSumB) {
		t.Fatal("expected batch B entry to be re-summed against the replacing payload")
	}
	has, err = r.HasSum(addr, staleSumB)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected the stale sum of batch B to be dropped")
	}
}

// TestChunkSumIndexRandomOps drives the reserve with a randomized sequence of
// overlapping SOC puts (shared addresses across batches, timestamp
// replacements), CAC puts and batch evictions, and repeatedly asserts the
// invariant the pullsync want-decision depends on: the ChunkSumItem index is
// exactly the set of (address, sum) pairs of the live ChunkBinItems, and every
// stored sum matches the payload actually held in the chunkstore.
func TestChunkSumIndexRandomOps(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	st := internal.NewInmemStorage()
	r, err := reserve.New(baseAddr, st, 0, kademlia.NewTopologyDriver(), log.Noop)
	if err != nil {
		t.Fatal(err)
	}

	rng := rand.New(rand.NewSource(42))

	// small pools force address sharing, stamp collisions and replacements
	signers := []crypto.Signer{getSigner(t), getSigner(t)}
	batches := [][]byte{
		postagetesting.MustNewBatch().ID,
		postagetesting.MustNewBatch().ID,
		postagetesting.MustNewBatch().ID,
	}

	checkInvariant := func(op int) {
		t.Helper()

		// live (address, sum) pairs according to the chunk bin index; sums
		// must match the payload currently in the chunkstore.
		live := make(map[string]int)
		err := st.IndexStore().Iterate(
			storage.Query{Factory: func() storage.Item { return &reserve.ChunkBinItem{} }},
			func(res storage.Result) (bool, error) {
				cbi := res.Entry.(*reserve.ChunkBinItem)
				ch, err := st.ChunkStore().Get(ctx, cbi.Address)
				if err != nil {
					return false, fmt.Errorf("op %d: chunk missing for live index entry %s: %w", op, cbi.Address, err)
				}
				want, err := storage.ChunkSumFromParts(cbi.BatchID, cbi.StampHash, ch)
				if err != nil {
					return false, err
				}
				if !bytes.Equal(cbi.Sum, want) {
					return false, fmt.Errorf("op %d: stale sum on entry %s", op, cbi.Address)
				}
				live[cbi.Address.ByteString()+string(cbi.Sum)]++
				return false, nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		// the sum index must be exactly the live set, in both directions
		indexed := make(map[string]int)
		err = st.IndexStore().Iterate(
			storage.Query{
				Factory:      func() storage.Item { return &reserve.ChunkSumItem{} },
				ItemProperty: storage.QueryItemID,
			},
			func(res storage.Result) (bool, error) {
				if len(res.ID) != swarm.HashSize+storage.ChunkSumSize {
					return false, fmt.Errorf("op %d: malformed chunk sum key length %d", op, len(res.ID))
				}
				indexed[res.ID]++
				return false, nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		for k := range indexed {
			if live[k] == 0 {
				t.Fatalf("op %d: orphaned chunk sum entry (no live chunk bin item)", op)
			}
		}
		for k, n := range live {
			if n > 1 {
				t.Fatalf("op %d: %d chunk bin items share one (address, sum) pair", op, n)
			}
			if indexed[k] == 0 {
				t.Fatalf("op %d: live chunk bin item without chunk sum entry", op)
			}
		}
	}

	ts := uint64(0)
	for op := range 200 {
		switch v := rng.Intn(10); {
		case v < 6: // SOC put: shared addresses, random payload, random batch
			ts++
			s := soctesting.GenerateMockSocWithSigner(t, fmt.Appendf(nil, "payload-%d", rng.Intn(4)), signers[rng.Intn(len(signers))])
			stamp := postagetesting.MustNewFields(batches[rng.Intn(len(batches))], 0, ts)
			err = r.Put(ctx, s.Chunk().WithStamp(stamp))
		case v < 8: // CAC put
			ch, cerr := cac.New(fmt.Appendf(nil, "cac-%d", op))
			if cerr != nil {
				t.Fatal(cerr)
			}
			err = r.Put(ctx, ch.WithStamp(postagetesting.MustNewBatchStamp(batches[rng.Intn(len(batches))])))
		default: // evict a whole batch
			_, err = r.EvictBatchBin(ctx, batches[rng.Intn(len(batches))], math.MaxInt, swarm.MaxBins)
		}
		if err != nil && !errors.Is(err, storage.ErrOverwriteNewerChunk) {
			t.Fatalf("op %d: %v", op, err)
		}

		if op%20 == 19 {
			checkInvariant(op)
		}
	}
	checkInvariant(200)
}
