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
			err := s.IndexStore().Put(&reserve.BatchRadiusItem{
				Bin:       uint8(i),
				BatchID:   ch.Stamp().BatchID(),
				BatchHash: nil, // exiting items don't have a batchHash
				Address:   ch.Address(),
			})
			if err != nil {
				return err
			}

			err = s.IndexStore().Put(&reserve.ChunkBinItem{
				Bin:       uint8(i),
				Address:   ch.Address(),
				BatchID:   ch.Stamp().BatchID(),
				BatchHash: nil, // existing items don't have a batchHash
			})
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

	var count int
	err = store.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &reserve.BatchRadiusItem{} },
	}, func(result storage.Result) (bool, error) {
		item := result.Entry.(*reserve.BatchRadiusItem)
		assert.True(t, bytes.Equal(item.BatchHash, swarm.EmptyAddress.Bytes()))
		count++
		return false, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 10, count)

	count = 0
	err = store.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &reserve.ChunkBinItem{} },
	}, func(result storage.Result) (bool, error) {
		item := result.Entry.(*reserve.ChunkBinItem)
		assert.True(t, bytes.Equal(item.BatchHash, swarm.EmptyAddress.Bytes()))
		count++
		return false, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 10, count)

	err = localmigration.Step_06(store)()
	require.NoError(t, err)

	count = 0
	err = store.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &reserve.BatchRadiusItem{} },
	}, func(result storage.Result) (bool, error) {
		item := result.Entry.(*reserve.BatchRadiusItem)
		assert.False(t, bytes.Equal(item.BatchHash, swarm.EmptyAddress.Bytes()))
		count++
		return false, nil
	})
	require.NoError(t, err)

	// previous items with old ID format should be deleted and replaced by new.
	assert.Equal(t, 10, count)
}
