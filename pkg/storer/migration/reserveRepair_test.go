// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
)

func TestReserveRepair(t *testing.T) {
	t.Parallel()

	store := internal.NewInmemStorage()
	baseAddr := swarm.RandAddress(t)
	stepFn := localmigration.ReserveRepairer(store, func(_ swarm.Chunk) swarm.ChunkType {
		return swarm.ChunkTypeContentAddressed
	}, log.Noop)

	chunksPO := make([][]swarm.Chunk, 5)
	var chunksPerPO uint64 = 2

	for i := range swarm.MaxBins {
		err := store.Run(context.Background(), func(s transaction.Store) error {
			return s.IndexStore().Put(&reserve.BinItem{Bin: i, BinID: 10})
		})
		assert.NoError(t, err)
	}

	for b := range 5 {
		for range chunksPerPO {
			ch := chunktest.GenerateTestRandomChunkAt(t, baseAddr, b)
			stampHash, err := ch.Stamp().Hash()
			if err != nil {
				t.Fatal(err)
			}
			cb := &reserve.ChunkBinItem{
				Bin: uint8(b),
				// Assign 0 binID to all items to see if migration fixes this
				BinID:     0,
				Address:   ch.Address(),
				BatchID:   ch.Stamp().BatchID(),
				ChunkType: swarm.ChunkTypeContentAddressed,
				StampHash: stampHash,
			}
			err = store.Run(context.Background(), func(s transaction.Store) error {
				return s.IndexStore().Put(cb)
			})
			if err != nil {
				t.Fatal(err)
			}

			br := &reserve.BatchRadiusItem{
				BatchID:   ch.Stamp().BatchID(),
				Bin:       uint8(b),
				Address:   ch.Address(),
				BinID:     0,
				StampHash: stampHash,
			}
			err = store.Run(context.Background(), func(s transaction.Store) error {
				return s.IndexStore().Put(br)
			})
			if err != nil {
				t.Fatal(err)
			}

			if b < 2 {
				// simulate missing chunkstore entries
				continue
			}

			err = store.Run(context.Background(), func(s transaction.Store) error {
				err := chunkstamp.Store(s.IndexStore(), "reserve", ch)
				if err != nil {
					return err
				}
				return s.ChunkStore().Put(context.Background(), ch)
			})
			if err != nil {
				t.Fatal(err)
			}

			chunksPO[b] = append(chunksPO[b], ch)
		}
	}

	assert.NoError(t, stepFn())

	binIDs := make(map[uint8][]uint64)
	cbCount := 0
	err := store.IndexStore().Iterate(
		storage.Query{Factory: func() storage.Item { return &reserve.ChunkBinItem{} }},
		func(res storage.Result) (stop bool, err error) {
			cb := res.Entry.(*reserve.ChunkBinItem)
			if cb.ChunkType != swarm.ChunkTypeContentAddressed {
				return false, errors.New("chunk type should be content addressed")
			}
			binIDs[cb.Bin] = append(binIDs[cb.Bin], cb.BinID)
			cbCount++
			return false, nil
		},
	)
	assert.NoError(t, err)

	for b := range 5 {
		if b < 2 {
			if _, found := binIDs[uint8(b)]; found {
				t.Fatalf("bin %d should not have any binIDs", b)
			}
			continue
		}
		assert.Len(t, binIDs[uint8(b)], 2)
		for idx, binID := range binIDs[uint8(b)] {
			assert.Equal(t, uint64(idx+1), binID)

			item := &reserve.BinItem{Bin: uint8(b)}
			_ = store.IndexStore().Get(item)
			assert.Equal(t, item.BinID, uint64(2))
		}
	}

	brCount := 0
	err = store.IndexStore().Iterate(
		storage.Query{Factory: func() storage.Item { return &reserve.BatchRadiusItem{} }},
		func(res storage.Result) (stop bool, err error) {
			br := res.Entry.(*reserve.BatchRadiusItem)
			if br.Bin < 2 {
				return false, errors.New("bin should be >= 2")
			}
			brCount++
			return false, nil
		},
	)
	assert.NoError(t, err)

	assert.Equal(t, cbCount, brCount)

	has, err := store.IndexStore().Has(&reserve.EpochItem{})
	if has {
		t.Fatal("epoch item should be deleted")
	}
	assert.NoError(t, err)
}

// TestReserveRepairDropsAddrIndex checks that the repair tool, which does not
// maintain the reserve address index, drops the index and its marker.
func TestReserveRepairDropsAddrIndex(t *testing.T) {
	t.Parallel()

	store := internal.NewInmemStorage()
	baseAddr := swarm.RandAddress(t)
	ctx := context.Background()

	items := make([]*reserve.ChunkAddrItem, 0, 3)
	for b := range 3 {
		ch := chunktest.GenerateTestRandomChunkAt(t, baseAddr, b)
		stampHash, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, &reserve.ChunkAddrItem{Address: ch.Address(), StampHash: stampHash, BatchID: ch.Stamp().BatchID()})
	}
	err := store.Run(ctx, func(s transaction.Store) error {
		for _, item := range items {
			if err := s.IndexStore().Put(item); err != nil {
				return err
			}
		}
		return s.IndexStore().Put(&reserve.ChunkAddrValidItem{Version: reserve.ChunkAddrIndexVersion})
	})
	if err != nil {
		t.Fatal(err)
	}

	err = localmigration.ReserveRepairer(store, func(swarm.Chunk) swarm.ChunkType { return swarm.ChunkTypeContentAddressed }, log.Noop)()
	if err != nil {
		t.Fatal(err)
	}

	if n, err := store.IndexStore().Count(&reserve.ChunkAddrItem{}); err != nil || n != 0 {
		t.Fatalf("%d address index entries left: %v", n, err)
	}
	if has, err := store.IndexStore().Has(&reserve.ChunkAddrValidItem{}); err != nil || has {
		t.Fatalf("marker left: %v, %v", has, err)
	}
}

// TestReserveRepairMissingChunk is a T6.2 finding (see the phase-6 plan
// file), about code that predates the address index: ReserveRepairer deletes
// every ChunkBinItem (its step 2), then renumbers the bins from 1 in parallel
// (step 3), where it removes a chunk whose data is missing with
// RemoveChunkWithItem, which deletes the ChunkBinItem of the chunk's old bin
// ID. If another chunk of the bin was given that bin ID first, that chunk's
// new ChunkBinItem is deleted instead. Steps 3 and 4 commit before the final
// check, so the reserve keeps a chunk without its ChunkBinItem, which no bin
// iteration finds, whether or not the check then fails the repair. Here four
// chunks are missing: they had bin IDs 1 to 4 and are the last ones the
// repairer handles (one batch, one bin, the highest addresses), so each can
// delete the new ChunkBinItem of a chunk renumbered before it; one is enough.
func TestReserveRepairMissingChunk(t *testing.T) {
	t.Parallel()
	t.Skip("T6.2 finding, see the phase-6 Follow-ups: ReserveRepairer deletes a renumbered chunk's ChunkBinItem when another chunk of its bin is missing")

	ctx := context.Background()
	store := internal.NewInmemStorage()
	baseAddr := swarm.RandAddress(t)
	batchID := swarm.RandAddress(t).Bytes()
	chunks := make([]swarm.Chunk, 4*runtime.NumCPU()+8)
	for i := range chunks {
		chunks[i] = chunktest.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewBatchStamp(batchID))
	}
	slices.SortFunc(chunks, func(a, b swarm.Chunk) int { return bytes.Compare(a.Address().Bytes(), b.Address().Bytes()) })
	const missing = 4 // the last ones, with bin IDs 1 to missing
	live := len(chunks) - missing

	err := store.Run(ctx, func(s transaction.Store) error {
		for i, ch := range chunks {
			binID := uint64(missing + i + 1)
			if i >= live {
				binID = uint64(i - live + 1)
			}
			stampHash, err := ch.Stamp().Hash()
			if err != nil {
				return err
			}
			err = errors.Join(
				s.IndexStore().Put(&reserve.ChunkBinItem{Bin: 0, BinID: binID, Address: ch.Address(), BatchID: batchID, ChunkType: swarm.ChunkTypeContentAddressed, StampHash: stampHash}),
				s.IndexStore().Put(&reserve.BatchRadiusItem{Bin: 0, BinID: binID, Address: ch.Address(), BatchID: batchID, StampHash: stampHash}),
			)
			if err != nil || i >= live {
				continue // the missing chunks have no data
			}
			err = errors.Join(err, chunkstamp.Store(s.IndexStore(), "reserve", ch), s.ChunkStore().Put(ctx, ch))
			if err != nil {
				return err
			}
		}
		return s.IndexStore().Put(&reserve.BinItem{Bin: 0, BinID: uint64(len(chunks))})
	})
	if err != nil {
		t.Fatal(err)
	}

	repairErr := localmigration.ReserveRepairer(store, func(swarm.Chunk) swarm.ChunkType { return swarm.ChunkTypeContentAddressed }, log.Noop)()

	orphans := 0
	err = store.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &reserve.BatchRadiusItem{} },
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*reserve.BatchRadiusItem)
		err := store.IndexStore().Get(&reserve.ChunkBinItem{Bin: item.Bin, BinID: item.BinID})
		if errors.Is(err, storage.ErrNotFound) {
			orphans++
			return false, nil
		}
		return false, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if repairErr != nil || orphans != 0 {
		t.Fatalf("repair left %d reserve chunks without a ChunkBinItem: %v", orphans, repairErr)
	}
}
