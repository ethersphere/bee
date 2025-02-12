// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type oldAndNewItem[K storage.Item, V storage.Item] struct {
	old K
	new V
}

func Test_Step_06(t *testing.T) {
	t.Parallel()

	sharkyDir := t.TempDir()
	sharkyStore, err := sharky.New(&dirFS{basedir: sharkyDir}, 1, swarm.SocMaxChunkSize)
	require.NoError(t, err)

	lstore, err := leveldbstore.New("", nil)
	require.NoError(t, err)

	store := transaction.NewStorage(sharkyStore, lstore)
	t.Cleanup(func() {
		err := store.Close()
		require.NoError(t, err)
	})

	chunks := chunktest.GenerateTestRandomChunks(100)
	ctx := context.Background()

	batchRadiusItems := make(map[string]oldAndNewItem[*reserve.BatchRadiusItemV1, *reserve.BatchRadiusItem])
	chunkBinItems := make(map[string]oldAndNewItem[*reserve.ChunkBinItemV1, *reserve.ChunkBinItem])
	stampIndexItems := make(map[string]oldAndNewItem[*stampindex.ItemV1, *stampindex.Item])

	for i, ch := range chunks {
		err = store.Run(ctx, func(s transaction.Store) error {
			b := &reserve.BatchRadiusItemV1{
				Bin:     uint8(i),
				BatchID: ch.Stamp().BatchID(),
				Address: ch.Address(),
				BinID:   uint64(i),
			}
			err := s.IndexStore().Put(b)
			if err != nil {
				return err
			}
			batchRadiusItems[string(b.BatchID)+string(b.Bin)+b.Address.ByteString()] = oldAndNewItem[*reserve.BatchRadiusItemV1, *reserve.BatchRadiusItem]{old: b, new: nil}

			c := &reserve.ChunkBinItemV1{
				Bin:       uint8(i),
				BinID:     uint64(i),
				Address:   ch.Address(),
				BatchID:   ch.Stamp().BatchID(),
				ChunkType: swarm.ChunkTypeSingleOwner,
			}
			err = s.IndexStore().Put(c)
			if err != nil {
				return err
			}
			chunkBinItems[c.ID()] = oldAndNewItem[*reserve.ChunkBinItemV1, *reserve.ChunkBinItem]{old: c, new: nil}

			sIdxItem := &stampindex.ItemV1{
				BatchID:          ch.Stamp().BatchID(),
				StampIndex:       ch.Stamp().Index(),
				StampTimestamp:   ch.Stamp().Timestamp(),
				ChunkAddress:     ch.Address(),
				ChunkIsImmutable: true,
			}
			sIdxItem.SetNamespace([]byte("reserve"))
			err = s.IndexStore().Put(sIdxItem)
			if err != nil {
				return err
			}

			stampIndexItems[sIdxItem.ID()] = oldAndNewItem[*stampindex.ItemV1, *stampindex.Item]{old: sIdxItem, new: nil}

			return chunkstamp.Store(s.IndexStore(), "reserve", ch)
		})
		require.NoError(t, err)
	}

	require.NoError(t, err)
	err = localmigration.Step_06(store, log.Noop)()
	require.NoError(t, err)

	has, err := store.IndexStore().Has(&reserve.EpochItem{})
	if has {
		t.Fatal("epoch item should be deleted")
	}
	require.NoError(t, err)

	checkBatchRadiusItems(t, store.IndexStore(), len(chunks), batchRadiusItems)
	checkChunkBinItems(t, store.IndexStore(), len(chunks), chunkBinItems)
	checkStampIndex(t, store.IndexStore(), len(chunks), stampIndexItems)
}

func checkBatchRadiusItems(t *testing.T, s storage.Reader, wantCount int, m map[string]oldAndNewItem[*reserve.BatchRadiusItemV1, *reserve.BatchRadiusItem]) {
	t.Helper()
	count := 0

	err := s.Iterate(storage.Query{
		Factory: func() storage.Item { return new(reserve.BatchRadiusItem) },
	}, func(result storage.Result) (bool, error) {
		count++
		b := result.Entry.(*reserve.BatchRadiusItem)
		id := string(b.BatchID) + string(b.Bin) + b.Address.ByteString()
		found, ok := m[id]
		require.True(t, ok)
		found.new = b
		m[id] = found
		return false, nil
	})
	require.NoError(t, err)
	assert.Equal(t, wantCount, count)

	for _, v := range m {
		assert.Equal(t, v.old.Bin, v.new.Bin)
		assert.Equal(t, v.old.BatchID, v.new.BatchID)
		assert.Equal(t, v.old.Address, v.new.Address)
		assert.Equal(t, v.old.BinID, v.new.BinID)
		assert.NotEqual(t, swarm.EmptyAddress.Bytes(), v.new.StampHash)
	}
}

func checkChunkBinItems(t *testing.T, s storage.Reader, wantCount int, m map[string]oldAndNewItem[*reserve.ChunkBinItemV1, *reserve.ChunkBinItem]) {
	t.Helper()
	count := 0
	err := s.Iterate(storage.Query{
		Factory: func() storage.Item { return new(reserve.ChunkBinItem) },
	}, func(result storage.Result) (bool, error) {
		count++
		b := result.Entry.(*reserve.ChunkBinItem)
		found, ok := m[b.ID()]
		require.True(t, ok)
		found.new = b
		m[b.ID()] = found
		return false, nil
	})
	require.NoError(t, err)
	assert.Equal(t, wantCount, count)
	for _, v := range m {
		assert.Equal(t, v.old.Bin, v.new.Bin)
		assert.Equal(t, v.old.BatchID, v.new.BatchID)
		assert.Equal(t, v.old.Address, v.new.Address)
		assert.Equal(t, v.old.BinID, v.new.BinID)
		assert.Equal(t, v.old.ChunkType, v.new.ChunkType)
		assert.NotEqual(t, swarm.EmptyAddress.Bytes(), v.new.StampHash)
	}
}

func checkStampIndex(t *testing.T, s storage.Reader, wantCount int, m map[string]oldAndNewItem[*stampindex.ItemV1, *stampindex.Item]) {
	t.Helper()
	count := 0
	err := s.Iterate(storage.Query{
		Factory: func() storage.Item { return new(stampindex.Item) },
	}, func(result storage.Result) (bool, error) {
		count++
		b := result.Entry.(*stampindex.Item)
		found, ok := m[b.ID()]
		require.True(t, ok)
		found.new = b
		m[b.ID()] = found
		return false, nil
	})
	require.NoError(t, err)
	assert.Equal(t, wantCount, count)
	for _, v := range m {
		assert.Equal(t, v.old.Namespace(), v.new.Namespace())
		assert.Equal(t, v.old.BatchID, v.new.BatchID)
		assert.Equal(t, v.old.StampIndex, v.new.StampIndex)
		assert.Equal(t, v.old.StampTimestamp, v.new.StampTimestamp)
		assert.Equal(t, v.old.ChunkAddress, v.new.ChunkAddress)
		assert.NotEqual(t, swarm.EmptyAddress.Bytes(), v.new.StampHash)
	}
}
