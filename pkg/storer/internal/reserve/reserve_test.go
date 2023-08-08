// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	storage "github.com/ethersphere/bee/pkg/storage"
	chunk "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/pkg/swarm"
	kademlia "github.com/ethersphere/bee/pkg/topology/mock"
)

func noopCacher(_ context.Context, _ internal.Storage, _ ...swarm.Address) error {
	return nil
}

func TestReserve(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)

	ts, closer := internal.NewInmemStorage()
	t.Cleanup(func() {
		if err := closer(); err != nil {
			t.Errorf("failed closing the storage: %v", err)
		}
	})

	r, err := reserve.New(
		baseAddr,
		ts.IndexStore(),
		0, kademlia.NewTopologyDriver(),
		log.Noop,
		noopCacher,
	)
	if err != nil {
		t.Fatal(err)
	}

	for b := 0; b < 2; b++ {
		for i := 1; i < 51; i++ {
			ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, b)
			c, err := r.Put(context.Background(), ts, ch)
			if err != nil {
				t.Fatal(err)
			}
			if !c {
				t.Fatal("entered unique chunk")
			}
			checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: uint8(b), BatchID: ch.Stamp().BatchID(), Address: ch.Address()}, false)
			checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: uint8(b), BinID: uint64(i)}, false)
			checkChunk(t, ts, ch, false)

			h, err := r.Has(ts.IndexStore(), ch.Address(), ch.Stamp().BatchID())
			if err != nil {
				t.Fatal(err)
			}
			if !h {
				t.Fatalf("expected chunk addr %s binID %d", ch.Address(), i)
			}

			chGet, err := r.Get(context.Background(), ts, ch.Address(), ch.Stamp().BatchID())
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

	ts, closer := internal.NewInmemStorage()
	t.Cleanup(func() {
		if err := closer(); err != nil {
			t.Errorf("failed closing the storage: %v", err)
		}
	})

	r, err := reserve.New(
		baseAddr,
		ts.IndexStore(),
		0, kademlia.NewTopologyDriver(),
		log.Noop,
		noopCacher,
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
		if _, err := r.Put(ctx, ts, ch); err != nil {
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

func TestReplaceOldIndex(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)

	ts, closer := internal.NewInmemStorage()
	t.Cleanup(func() {
		if err := closer(); err != nil {
			t.Errorf("failed closing the storage: %v", err)
		}
	})

	r, err := reserve.New(
		baseAddr,
		ts.IndexStore(),
		0, kademlia.NewTopologyDriver(),
		log.Noop,
		func(ctx context.Context, st internal.Storage, addrs ...swarm.Address) error {
			for _, addr := range addrs {
				err := st.ChunkStore().Delete(ctx, addr)
				if err != nil {
					return err
				}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	batch := postagetesting.MustNewBatch()
	ch1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 0))
	ch2 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))

	_, err = r.Put(context.Background(), ts, ch1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = r.Put(context.Background(), ts, ch2)
	if err != nil {
		t.Fatal(err)
	}

	// Chunk 1 must be gone
	checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: 0, BatchID: ch1.Stamp().BatchID(), Address: ch1.Address()}, true)
	checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: 0, BinID: 1}, true)
	checkChunk(t, ts, ch1, true)

	// Chunk 2 must be stored
	checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: 0, BatchID: ch2.Stamp().BatchID(), Address: ch2.Address()}, false)
	checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: 0, BinID: 2}, false)
	checkChunk(t, ts, ch2, false)

	item, err := stampindex.Load(ts.IndexStore(), "reserve", ch2)
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

	ts, closer := internal.NewInmemStorage()
	t.Cleanup(func() {
		if err := closer(); err != nil {
			t.Errorf("failed closing the storage: %v", err)
		}
	})

	chunksPerBatch := 50
	var chunks []swarm.Chunk
	batches := []*postage.Batch{postagetesting.MustNewBatch(), postagetesting.MustNewBatch(), postagetesting.MustNewBatch()}
	evictBatch := batches[1]

	r, err := reserve.New(
		baseAddr,
		ts.IndexStore(),
		0, kademlia.NewTopologyDriver(),
		log.Noop,
		func(ctx context.Context, st internal.Storage, addrs ...swarm.Address) error {
			for _, addr := range addrs {
				err := st.ChunkStore().Delete(ctx, addr)
				if err != nil {
					return err
				}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < chunksPerBatch; i++ {
		for b := 0; b < 3; b++ {
			ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, b).WithStamp(postagetesting.MustNewBatchStamp(batches[b].ID))
			chunks = append(chunks, ch)
			c, err := r.Put(context.Background(), ts, ch)
			if err != nil {
				t.Fatal(err)
			}
			if !c {
				t.Fatal("entered unique chunk")
			}
		}
	}

	totalEvicted := 0
	for i := 0; i < 3; i++ {
		evicted, err := r.EvictBatchBin(context.Background(), ts, uint8(i), evictBatch.ID)
		if err != nil {
			t.Fatal(err)
		}
		totalEvicted += evicted
	}

	if totalEvicted != chunksPerBatch {
		t.Fatalf("got %d, want %d", totalEvicted, chunksPerBatch)
	}

	for i, ch := range chunks {
		binID := i%chunksPerBatch + 1
		b := swarm.Proximity(baseAddr.Bytes(), ch.Address().Bytes())
		_, err := r.Get(context.Background(), ts, ch.Address(), ch.Stamp().BatchID())
		if bytes.Equal(ch.Stamp().BatchID(), evictBatch.ID) {
			if !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("got err %v, want %v", err, storage.ErrNotFound)
			}
			checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: b, BatchID: ch.Stamp().BatchID(), Address: ch.Address()}, true)
			checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: b, BinID: uint64(binID)}, true)
			checkChunk(t, ts, ch, true)
		} else {
			if err != nil {
				t.Fatal(err)
			}
			checkStore(t, ts.IndexStore(), &reserve.BatchRadiusItem{Bin: b, BatchID: ch.Stamp().BatchID(), Address: ch.Address()}, false)
			checkStore(t, ts.IndexStore(), &reserve.ChunkBinItem{Bin: b, BinID: uint64(binID)}, false)
			checkChunk(t, ts, ch, false)
		}
	}
}

func TestIterate(t *testing.T) {
	t.Parallel()

	createReserve := func(t *testing.T) (*reserve.Reserve, internal.Storage) {
		t.Helper()

		baseAddr := swarm.RandAddress(t)

		ts, closer := internal.NewInmemStorage()
		t.Cleanup(func() {
			if err := closer(); err != nil {
				t.Errorf("failed closing the storage: %v", err)
			}
		})

		r, err := reserve.New(
			baseAddr,
			ts.IndexStore(),
			0, kademlia.NewTopologyDriver(),
			log.Noop,
			noopCacher,
		)
		if err != nil {
			t.Fatal(err)
		}

		for b := 0; b < 3; b++ {
			for i := 0; i < 10; i++ {
				ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, b)
				c, err := r.Put(context.Background(), ts, ch)
				if err != nil {
					t.Fatal(err)
				}
				if !c {
					t.Fatal("entered unique chunk")
				}
			}
		}

		return r, ts
	}

	t.Run("iterate bin", func(t *testing.T) {
		t.Parallel()

		r, ts := createReserve(t)

		var id uint64 = 1
		err := r.IterateBin(ts.IndexStore(), 1, 0, func(ch swarm.Address, binID uint64, _ []byte) (bool, error) {
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

		r, ts := createReserve(t)

		count := 0
		err := r.IterateChunks(ts, 2, func(_ swarm.Chunk) (bool, error) {
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

		r, ts := createReserve(t)

		count := 0
		err := r.IterateChunksItems(ts, 0, func(_ reserve.ChunkItem) (bool, error) {
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

		r, ts := createReserve(t)

		ids, err := r.LastBinIDs(ts.IndexStore())
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

func checkStore(t *testing.T, s storage.Store, k storage.Key, gone bool) {
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

func checkChunk(t *testing.T, s internal.Storage, ch swarm.Chunk, gone bool) {
	t.Helper()
	h, err := s.ChunkStore().Has(context.Background(), ch.Address())
	if err != nil {
		t.Fatal(err)
	}

	_, err = chunkstamp.LoadWithBatchID(s.IndexStore(), "reserve", ch.Address(), ch.Stamp().BatchID())
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
