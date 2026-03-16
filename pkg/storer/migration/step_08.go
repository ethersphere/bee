// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"golang.org/x/sync/errgroup"
)

// step_08 migrates ChunkBinItems to include the sharky.Location field.
// This denormalizes the location from RetrievalIndexItem into ChunkBinItem,
// enabling reserve sampling to read chunk data directly from sharky without
// an intermediate LevelDB lookup per chunk.
func step_08(st transaction.Storage, logger log.Logger) func() error {
	return func() error {
		logger := logger.WithName("migration-step-08").Register()

		logger.Info("start adding sharky location to ChunkBinItems")

		seenCount, doneCount, err := addChunkLocation(logger, st)
		if err != nil {
			return fmt.Errorf("add chunk location migration: %w", err)
		}
		logger.Info("finished migrating ChunkBinItems", "seen", seenCount, "migrated", doneCount)
		return nil
	}
}

func addChunkLocation(logger log.Logger, st transaction.Storage) (int64, int64, error) {

	preCnt, err := st.IndexStore().Count(&reserve.ChunkBinItemV2{})
	if err != nil {
		return 0, 0, err
	}

	itemC := make(chan *reserve.ChunkBinItemV2)

	errC := make(chan error, 1)
	doneC := make(chan any)
	defer close(doneC)
	defer close(errC)

	var eg errgroup.Group
	eg.SetLimit(runtime.NumCPU())

	var doneCount atomic.Int64
	var seenCount int64

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				logger.Info("still migrating ChunkBinItems...", "seen", seenCount, "migrated", doneCount.Load())
			case <-doneC:
				return
			}
		}
	}()

	go func() {
		_ = st.IndexStore().Iterate(storage.Query{
			Factory: func() storage.Item { return new(reserve.ChunkBinItemV2) },
		}, func(result storage.Result) (bool, error) {
			seenCount++
			item := result.Entry.(*reserve.ChunkBinItemV2)
			select {
			case itemC <- item:
			case err := <-errC:
				return true, err
			}
			return false, nil
		})
		close(itemC)
	}()

	for item := range itemC {
		oldItem := item
		eg.Go(func() error {
			err := st.Run(context.Background(), func(s transaction.Store) error {
				// Look up the RetrievalIndexItem to get the sharky location.
				rIdx := &chunkstore.RetrievalIndexItem{Address: oldItem.Address}
				err := s.IndexStore().Get(rIdx)
				if err != nil {
					return fmt.Errorf("retrieval index get for %s: %w", oldItem.Address, err)
				}

				// Same ID (bin/binID), so Put will overwrite the existing entry.
				return s.IndexStore().Put(&reserve.ChunkBinItem{
					Bin:       oldItem.Bin,
					BinID:     oldItem.BinID,
					Address:   oldItem.Address,
					BatchID:   oldItem.BatchID,
					ChunkType: oldItem.ChunkType,
					StampHash: oldItem.StampHash,
					Location:  rIdx.Location,
				})
			})
			if err != nil {
				errC <- err
				return err
			}
			doneCount.Add(1)
			return nil
		})
	}

	err = eg.Wait()
	if err != nil {
		return 0, 0, err
	}

	postCnt, err := st.IndexStore().Count(&reserve.ChunkBinItem{})
	if err != nil {
		return 0, 0, err
	}

	if preCnt != postCnt {
		return 0, 0, fmt.Errorf("post-migration check: count mismatch, pre=%d post=%d", preCnt, postCnt)
	}

	return seenCount, doneCount.Load(), nil
}
