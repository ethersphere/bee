// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"golang.org/x/sync/errgroup"
)

// step_06 is a migration step that adds a stampHash to all BatchRadiusItems, ChunkBinItems and StampIndexItems.
func step_06(st transaction.Storage, logger log.Logger) func() error {
	return func() error {
		logger := logger.WithName("migration-step-06").Register()

		logger.Info("start adding stampHash to BatchRadiusItems, ChunkBinItems and StampIndexItems")

		seenCount, doneCount, err := addStampHash(logger, st)
		if err != nil {
			return fmt.Errorf("add stamp hash migration: %w", err)
		}
		logger.Info("finished migrating items", "seen", seenCount, "migrated", doneCount)
		return nil
	}
}

func addStampHash(logger log.Logger, st transaction.Storage) (int64, int64, error) {

	preBatchRadiusCnt, err := st.IndexStore().Count(&reserve.BatchRadiusItemV1{})
	if err != nil {
		return 0, 0, err
	}

	preChunkBinCnt, err := st.IndexStore().Count(&reserve.ChunkBinItemV1{})
	if err != nil {
		return 0, 0, err
	}

	if preBatchRadiusCnt != preChunkBinCnt {
		return 0, 0, fmt.Errorf("pre-migration check: index counts do not match, %d vs %d", preBatchRadiusCnt, preChunkBinCnt)
	}

	// Delete epoch timestamp
	err = st.Run(context.Background(), func(s transaction.Store) error {
		return s.IndexStore().Delete(&reserve.EpochItem{})
	})
	if err != nil {
		return 0, 0, err
	}

	itemC := make(chan *reserve.BatchRadiusItemV1)

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
				logger.Info("still migrating items...")
			case <-doneC:
				return
			}
		}
	}()

	go func() {
		_ = st.IndexStore().Iterate(storage.Query{
			Factory: func() storage.Item { return new(reserve.BatchRadiusItemV1) },
		}, func(result storage.Result) (bool, error) {
			seenCount++
			item := result.Entry.(*reserve.BatchRadiusItemV1)
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
		batchRadiusItemV1 := item
		eg.Go(func() error {
			err := st.Run(context.Background(), func(s transaction.Store) error {
				idxStore := s.IndexStore()
				stamp, err := chunkstamp.LoadWithBatchID(idxStore, "reserve", batchRadiusItemV1.Address, batchRadiusItemV1.BatchID)
				if err != nil {
					return err
				}
				stampHash, err := stamp.Hash()
				if err != nil {
					return err
				}

				// Since the ID format has changed, we should delete the old item and put a new one with the new ID format.
				err = idxStore.Delete(batchRadiusItemV1)
				if err != nil {
					return err
				}
				err = idxStore.Put(&reserve.BatchRadiusItem{
					StampHash: stampHash,
					Bin:       batchRadiusItemV1.Bin,
					BatchID:   batchRadiusItemV1.BatchID,
					Address:   batchRadiusItemV1.Address,
					BinID:     batchRadiusItemV1.BinID,
				})
				if err != nil {
					return err
				}

				chunkBinItemV1 := &reserve.ChunkBinItemV1{
					Bin:   batchRadiusItemV1.Bin,
					BinID: batchRadiusItemV1.BinID,
				}
				err = idxStore.Get(chunkBinItemV1)
				if err != nil {
					return err
				}

				// same id. Will replace.
				err = idxStore.Put(&reserve.ChunkBinItem{
					StampHash: stampHash,
					Bin:       chunkBinItemV1.Bin,
					BinID:     chunkBinItemV1.BinID,
					Address:   chunkBinItemV1.Address,
					BatchID:   chunkBinItemV1.BatchID,
					ChunkType: chunkBinItemV1.ChunkType,
				})
				if err != nil {
					return err
				}

				// same id. Will replace.
				stampIndexItem := &stampindex.Item{
					StampHash:      stampHash,
					BatchID:        chunkBinItemV1.BatchID,
					StampIndex:     stamp.Index(),
					StampTimestamp: stamp.Timestamp(),
					ChunkAddress:   chunkBinItemV1.Address,
				}
				stampIndexItem.SetScope([]byte("reserve"))
				err = idxStore.Put(stampIndexItem)
				if err != nil {
					return err
				}
				doneCount.Add(1)
				return nil
			})
			if err != nil {
				errC <- err
				return err
			}
			return nil
		})
	}

	err = eg.Wait()
	if err != nil {
		return 0, 0, err
	}

	postBatchRadiusCnt, err := st.IndexStore().Count(&reserve.BatchRadiusItem{})
	if err != nil {
		return 0, 0, err
	}

	postChunkBinCnt, err := st.IndexStore().Count(&reserve.ChunkBinItem{})
	if err != nil {
		return 0, 0, err
	}

	if postBatchRadiusCnt != postChunkBinCnt || preBatchRadiusCnt != postBatchRadiusCnt || preChunkBinCnt != postChunkBinCnt {
		return 0, 0, fmt.Errorf("post-migration check: index counts do not match, %d vs %d. It's recommended that the nuke cmd is run to reset the node", postBatchRadiusCnt, postChunkBinCnt)
	}

	err = st.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return new(reserve.ChunkBinItem) },
	}, func(result storage.Result) (bool, error) {
		item := result.Entry.(*reserve.ChunkBinItem)

		batchRadiusItem := &reserve.BatchRadiusItem{BatchID: item.BatchID, Bin: item.Bin, Address: item.Address, StampHash: item.StampHash}
		if err := st.IndexStore().Get(batchRadiusItem); err != nil {
			return false, fmt.Errorf("batch radius item get: %w", err)
		}

		stamp, err := chunkstamp.LoadWithBatchID(st.IndexStore(), "reserve", item.Address, item.BatchID)
		if err != nil {
			return false, fmt.Errorf("stamp item get: %w", err)
		}

		stampIndex, err := stampindex.Load(st.IndexStore(), "reserve", stamp)
		if err != nil {
			return false, fmt.Errorf("stamp index get: %w", err)
		}

		if !bytes.Equal(item.StampHash, batchRadiusItem.StampHash) {
			return false, fmt.Errorf("batch radius item stamp hash, %x vs %x", item.StampHash, batchRadiusItem.StampHash)
		}

		if !bytes.Equal(item.StampHash, stampIndex.StampHash) {
			return false, fmt.Errorf("stamp index item stamp hash, %x vs %x", item.StampHash, stampIndex.StampHash)
		}

		return false, nil
	})

	if err != nil {
		return 0, 0, errors.New("post-migration check: items fields not match. It's recommended that the nuke cmd is run to reset the node")
	}

	return seenCount, doneCount.Load(), nil
}
