// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
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
				// zeroed sum: the repair must recompute it
				Sum: make([]byte, storage.ChunkSumSize),
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

	// seed a stale chunk sum entry with no backing chunk; the repair sweep
	// must remove it, otherwise the node would refuse to sync that chunk.
	staleSum := &reserve.ChunkSumItem{
		Address: swarm.RandAddress(t),
		Sum:     make([]byte, storage.ChunkSumSize),
	}
	err := store.Run(context.Background(), func(s transaction.Store) error {
		return s.IndexStore().Put(staleSum)
	})
	assert.NoError(t, err)

	assert.NoError(t, stepFn())

	has, err := store.IndexStore().Has(staleSum)
	assert.NoError(t, err)
	if has {
		t.Fatal("expected stale chunk sum item to be removed by repair")
	}

	// every surviving chunk must have its sum rebuilt in both indexes.
	for b := 2; b < 5; b++ {
		for _, ch := range chunksPO[b] {
			sum, err := storage.ChunkSum(ch)
			assert.NoError(t, err)
			has, err := store.IndexStore().Has(&reserve.ChunkSumItem{Address: ch.Address(), Sum: sum})
			assert.NoError(t, err)
			if !has {
				t.Fatalf("expected chunk sum item for chunk %s after repair", ch.Address())
			}
		}
	}

	chunkByAddr := make(map[string]swarm.Chunk)
	for b := 2; b < 5; b++ {
		for _, ch := range chunksPO[b] {
			chunkByAddr[ch.Address().ByteString()] = ch
		}
	}

	binIDs := make(map[uint8][]uint64)
	cbCount := 0
	err = store.IndexStore().Iterate(
		storage.Query{Factory: func() storage.Item { return &reserve.ChunkBinItem{} }},
		func(res storage.Result) (stop bool, err error) {
			cb := res.Entry.(*reserve.ChunkBinItem)
			if cb.ChunkType != swarm.ChunkTypeContentAddressed {
				return false, errors.New("chunk type should be content addressed")
			}
			ch, ok := chunkByAddr[cb.Address.ByteString()]
			if !ok {
				return false, errors.New("chunk bin item for unknown chunk")
			}
			sum, err := storage.ChunkSum(ch)
			if err != nil {
				return false, err
			}
			if !bytes.Equal(cb.Sum, sum) {
				return false, errors.New("chunk bin item sum not recomputed by repair")
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

	has, err = store.IndexStore().Has(&reserve.EpochItem{})
	if has {
		t.Fatal("epoch item should be deleted")
	}
	assert.NoError(t, err)
}
