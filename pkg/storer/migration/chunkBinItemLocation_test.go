// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
)

type dirFS struct {
	basedir string
}

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0o644)
}

// locatingStorage builds a storage backed by a real sharky store, so that
// ChunkStore().Put writes a retrieval index entry the backfill can read. The
// inmem storage used by the other migration tests keeps chunks in a plain map
// and writes no retrieval index at all.
func locatingStorage(t *testing.T) transaction.Storage {
	t.Helper()

	sharkyStore, err := sharky.New(&dirFS{basedir: t.TempDir()}, 1, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sharkyStore.Close(); err != nil {
			t.Errorf("close sharky: %v", err)
		}
	})

	return transaction.NewStorage(sharkyStore, inmemstore.New())
}

func TestBackfillChunkBinItemLocation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := locatingStorage(t)

	// chunks that exist in the chunkstore, with bin items carrying no location
	stored := make([]swarm.Chunk, 5)
	for i := range stored {
		ch := chunktest.GenerateTestRandomChunk()
		stored[i] = ch

		err := st.Run(ctx, func(s transaction.Store) error {
			if err := s.ChunkStore().Put(ctx, ch); err != nil {
				return err
			}
			return s.IndexStore().Put(&reserve.ChunkBinItem{
				Bin:       0,
				BinID:     uint64(i + 1),
				Address:   ch.Address(),
				ChunkType: swarm.ChunkTypeContentAddressed,
			})
		})
		assert.NoError(t, err)
	}

	// a bin item whose chunk is not in the chunkstore
	orphan := chunktest.GenerateTestRandomChunk()
	err := st.Run(ctx, func(s transaction.Store) error {
		return s.IndexStore().Put(&reserve.ChunkBinItem{
			Bin:       1,
			BinID:     1,
			Address:   orphan.Address(),
			ChunkType: swarm.ChunkTypeContentAddressed,
		})
	})
	assert.NoError(t, err)

	// a bin item that already carries a location must be left alone
	preset := storage.ChunkLocation{9, 9, 9, 9, 9, 9, 9, 9}
	presetCh := chunktest.GenerateTestRandomChunk()
	err = st.Run(ctx, func(s transaction.Store) error {
		if err := s.ChunkStore().Put(ctx, presetCh); err != nil {
			return err
		}
		return s.IndexStore().Put(&reserve.ChunkBinItem{
			Bin:       2,
			BinID:     1,
			Address:   presetCh.Address(),
			ChunkType: swarm.ChunkTypeContentAddressed,
			Location:  preset,
		})
	})
	assert.NoError(t, err)

	assert.NoError(t, localmigration.BackfillChunkBinItemLocation(st, log.Noop)())

	t.Run("stored chunks get the retrieval index location", func(t *testing.T) {
		for i, ch := range stored {
			rIdx := &chunkstore.RetrievalIndexItem{Address: ch.Address()}
			assert.NoError(t, st.IndexStore().Get(rIdx))

			item := &reserve.ChunkBinItem{Bin: 0, BinID: uint64(i + 1)}
			assert.NoError(t, st.IndexStore().Get(item))

			assert.False(t, item.Location.IsZero(), "location not filled for chunk %d", i)
			assert.Equal(t, chunkstore.LocationToChunkLocation(rIdx.Location), item.Location)
		}
	})

	t.Run("missing chunk keeps a zero location", func(t *testing.T) {
		item := &reserve.ChunkBinItem{Bin: 1, BinID: 1}
		assert.NoError(t, st.IndexStore().Get(item))
		assert.True(t, item.Location.IsZero())
	})

	t.Run("existing location is preserved", func(t *testing.T) {
		item := &reserve.ChunkBinItem{Bin: 2, BinID: 1}
		assert.NoError(t, st.IndexStore().Get(item))
		assert.Equal(t, preset, item.Location)
	})

	t.Run("idempotent", func(t *testing.T) {
		before := make([]storage.ChunkLocation, len(stored))
		for i := range stored {
			item := &reserve.ChunkBinItem{Bin: 0, BinID: uint64(i + 1)}
			assert.NoError(t, st.IndexStore().Get(item))
			before[i] = item.Location
		}

		assert.NoError(t, localmigration.BackfillChunkBinItemLocation(st, log.Noop)())

		for i := range stored {
			item := &reserve.ChunkBinItem{Bin: 0, BinID: uint64(i + 1)}
			assert.NoError(t, st.IndexStore().Get(item))
			assert.Equal(t, before[i], item.Location)
		}
	})
}

// TestBackfillChunkBinItemLocationSpansWindows drives more entries than the
// flush window so the windowed write-back path is exercised, not just a single
// trailing flush.
func TestBackfillChunkBinItemLocationSpansWindows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := locatingStorage(t)

	const count = 25

	for i := range count {
		ch := chunktest.GenerateTestRandomChunk()
		err := st.Run(ctx, func(s transaction.Store) error {
			if err := s.ChunkStore().Put(ctx, ch); err != nil {
				return err
			}
			return s.IndexStore().Put(&reserve.ChunkBinItem{
				Bin:       0,
				BinID:     uint64(i + 1),
				Address:   ch.Address(),
				ChunkType: swarm.ChunkTypeContentAddressed,
			})
		})
		assert.NoError(t, err)
	}

	assert.NoError(t, localmigration.BackfillChunkBinItemLocation(st, log.Noop)())

	filled := 0
	err := st.IndexStore().Iterate(
		storage.Query{Factory: func() storage.Item { return new(reserve.ChunkBinItem) }},
		func(res storage.Result) (bool, error) {
			if !res.Entry.(*reserve.ChunkBinItem).Location.IsZero() {
				filled++
			}
			return false, nil
		},
	)
	assert.NoError(t, err)
	assert.Equal(t, count, filled)
}
