// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"bytes"
	"context"
	"testing"

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

	chunks := chunktest.GenerateTestRandomChunks(10)
	ctx := context.Background()

	err = store.Run(ctx, func(s transaction.Store) error {
		for i, ch := range chunks {
			err := s.IndexStore().Put(&localmigration.OldBatchRadiusItem{
				BatchRadiusItem: &reserve.BatchRadiusItem{
					Bin:     uint8(i),
					BatchID: ch.Stamp().BatchID(),
					Address: ch.Address(),
				},
			})
			if err != nil {
				return err
			}

			err = s.IndexStore().Put(&localmigration.OldChunkBinItem{
				ChunkBinItem: &reserve.ChunkBinItem{
					Bin:     uint8(i),
					Address: ch.Address(),
					BatchID: ch.Stamp().BatchID(),
				},
			})
			if err != nil {
				return err
			}

			sIdxItem := &localmigration.OldStampIndexItem{
				Item: &stampindex.Item{
					BatchID:          ch.Stamp().BatchID(),
					StampIndex:       ch.Stamp().Index(),
					StampTimestamp:   ch.Stamp().Timestamp(),
					ChunkAddress:     ch.Address(),
					ChunkIsImmutable: false,
				},
			}
			sIdxItem.SetNamespace([]byte("reserve"))
			err = s.IndexStore().Put(sIdxItem)
			if err != nil {
				return err
			}

			err = chunkstamp.Store(s.IndexStore(), "reserve", ch)
			if err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)
	checkItems(t, store.IndexStore(), false, 10, &localmigration.OldBatchRadiusItem{BatchRadiusItem: &reserve.BatchRadiusItem{}})
	checkItems(t, store.IndexStore(), false, 10, &localmigration.OldChunkBinItem{ChunkBinItem: &reserve.ChunkBinItem{}})
	checkItems(t, store.IndexStore(), false, 10, &localmigration.OldStampIndexItem{Item: &stampindex.Item{}})

	err = localmigration.Step_06(store)()
	require.NoError(t, err)

	checkItems(t, store.IndexStore(), true, 10, &reserve.BatchRadiusItem{})
	checkItems(t, store.IndexStore(), true, 10, &reserve.ChunkBinItem{})
	checkItems(t, store.IndexStore(), true, 10, &stampindex.Item{})
}

func checkItems(t *testing.T, s storage.Reader, wantStampHash bool, wantCount int, fact storage.Item) {
	t.Helper()
	count := 0
	err := s.Iterate(storage.Query{
		Factory: func() storage.Item { return fact },
	}, func(result storage.Result) (bool, error) {
		var stampHash []byte
		switch result.Entry.(type) {
		case *localmigration.OldBatchRadiusItem:
			stampHash = result.Entry.(*localmigration.OldBatchRadiusItem).StampHash
		case *localmigration.OldChunkBinItem:
			stampHash = result.Entry.(*localmigration.OldChunkBinItem).StampHash
		case *localmigration.OldStampIndexItem:
			stampHash = result.Entry.(*localmigration.OldStampIndexItem).StampHash
		case *reserve.ChunkBinItem:
			stampHash = result.Entry.(*reserve.ChunkBinItem).StampHash
		case *reserve.BatchRadiusItem:
			stampHash = result.Entry.(*reserve.BatchRadiusItem).StampHash
		case *stampindex.Item:
			stampHash = result.Entry.(*stampindex.Item).StampHash
		}
		eq := bytes.Equal(stampHash, swarm.EmptyAddress.Bytes())
		assert.Equal(t, wantStampHash, !eq)
		if wantStampHash == eq {
			return false, nil
		}
		count++
		return false, nil
	})
	require.NoError(t, err)
	assert.Equal(t, wantCount, count)
}
