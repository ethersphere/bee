// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/sharky"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/inmemstore"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	localmigration "github.com/ethersphere/bee/pkg/storer/migration"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/stretchr/testify/assert"
)

func Test_Step_03(t *testing.T) {
	t.Parallel()

	stepFn := localmigration.Step_03
	store := inmemstore.New()
	baseAddr := swarm.RandAddress(t)

	var chunksPO = make([][]swarm.Chunk, 5)
	var chunksPerPO uint64 = 2

	for i := uint8(0); i < swarm.MaxBins; i++ {
		err := store.Put(&reserve.BinItem{Bin: i, BinID: 10})
		assert.NoError(t, err)
	}

	for b := 0; b < 5; b++ {
		for i := uint64(0); i < chunksPerPO; i++ {
			ch := chunktest.GenerateTestRandomChunkAt(t, baseAddr, b)
			cb := &reserve.ChunkBinItem{
				Bin: uint8(b),
				// Assign 0 binID to all items to see if migration fixes this
				BinID:     0,
				Address:   ch.Address(),
				BatchID:   ch.Stamp().BatchID(),
				ChunkType: swarm.ChunkTypeContentAddressed,
			}
			err := store.Put(cb)
			if err != nil {
				t.Fatal(err)
			}

			br := &reserve.BatchRadiusItem{
				BatchID: ch.Stamp().BatchID(),
				Bin:     uint8(b),
				Address: ch.Address(),
				BinID:   0,
			}
			err = store.Put(br)
			if err != nil {
				t.Fatal(err)
			}

			if b < 2 {
				// simulate missing chunkstore entries
				continue
			}

			rIdx := &chunkstore.RetrievalIndexItem{
				Address:   ch.Address(),
				Timestamp: uint64(time.Now().Unix()),
				RefCnt:    1,
				Location: sharky.Location{
					Shard:  0,
					Slot:   1,
					Length: swarm.ChunkSize,
				},
			}
			err = store.Put(rIdx)
			if err != nil {
				t.Fatal(err)
			}
			chunksPO[b] = append(chunksPO[b], ch)
		}
	}

	assert.NoError(t, stepFn(store))

	binIDs := make(map[uint8][]uint64)
	cbCount := 0
	err := store.Iterate(
		storage.Query{Factory: func() storage.Item { return &reserve.ChunkBinItem{} }},
		func(res storage.Result) (stop bool, err error) {
			cb := res.Entry.(*reserve.ChunkBinItem)
			binIDs[cb.Bin] = append(binIDs[cb.Bin], cb.BinID)
			cbCount++
			return false, nil
		},
	)
	assert.NoError(t, err)

	for b := 0; b < 5; b++ {
		if b < 2 {
			if _, found := binIDs[uint8(b)]; found {
				t.Fatalf("bin %d should not have any binIDs", b)
			}
			continue
		}
		assert.Len(t, binIDs[uint8(b)], 2)
		for idx, binID := range binIDs[uint8(b)] {
			assert.Equal(t, uint64(idx+1), binID)
		}
	}

	brCount := 0
	err = store.Iterate(
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
}
