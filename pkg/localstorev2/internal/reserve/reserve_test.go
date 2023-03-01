// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstamp"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/reserve"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	chunk "github.com/ethersphere/bee/pkg/storage/testing"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	kademlia "github.com/ethersphere/bee/pkg/topology/mock"
)

func TestReserve(t *testing.T) {
	t.Parallel()

	baseAddr := test.RandomAddress()

	ts, closer := internal.NewInmemStorage()
	t.Cleanup(func() {
		if err := closer(); err != nil {
			t.Errorf("failed closing the storage: %v", err)
		}
	})

	r, err := reserve.New(baseAddr, ts.IndexStore(), 0, 0, kademlia.NewTopologyDriver(), log.Noop)
	if err != nil {
		t.Fatal(err)
	}

	for b := 0; b < 2; b++ {
		for i := 0; i < 50; i++ {
			ch := chunk.GenerateTestRandomChunkAt(baseAddr, b)
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

func TestEvict(t *testing.T) {
	t.Parallel()

	baseAddr := test.RandomAddress()

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

	r, err := reserve.New(baseAddr, ts.IndexStore(), 0, 0, kademlia.NewTopologyDriver(), log.Noop)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < chunksPerBatch; i++ {
		for b := 0; b < 3; b++ {
			ch := chunk.GenerateTestRandomChunkAt(baseAddr, b).WithStamp(postagetesting.MustNewBatchStamp(batches[b].ID))
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
		evicted, err := r.EvictBatchBin(ts, uint8(i), evictBatch.ID)
		if err != nil {
			t.Fatal(err)
		}
		totalEvicted += evicted
	}

	if totalEvicted != chunksPerBatch {
		t.Fatalf("got %d, want %d", totalEvicted, chunksPerBatch)
	}

	for i, ch := range chunks {
		binID := i % chunksPerBatch
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

func TestOldRadius(t *testing.T) {
	t.Parallel()

	baseAddr := test.RandomAddress()

	ts, closer := internal.NewInmemStorage()
	t.Cleanup(func() {
		if err := closer(); err != nil {
			t.Errorf("failed closing the storage: %v", err)
		}
	})

	err := ts.IndexStore().Put(&reserve.RadiusItem{Radius: 3})
	if err != nil {
		t.Fatal(err)
	}

	r, err := reserve.New(baseAddr, ts.IndexStore(), 0, 0, kademlia.NewTopologyDriver(), log.Noop)
	if err != nil {
		t.Fatal(err)
	}

	if r.Radius() != 3 {
		t.Errorf("got %d, want %d", r.Radius(), 3)
	}
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
