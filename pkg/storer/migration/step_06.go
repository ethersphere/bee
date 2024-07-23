// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"
	"fmt"
	"os"
	"runtime"
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
func step_06(st transaction.Storage) func() error {
	return func() error {
		logger := log.NewLogger("migration-step-06", log.WithSink(os.Stdout))
		logger.Info("start adding stampHash to BatchRadiusItems, ChunkBinItems and StampIndexItems")

		err := addStampHash(logger, st)
		if err != nil {
			return fmt.Errorf("add stamp hash migration: %w", err)
		}
		logger.Info("finished migrating items")
		return nil
	}
}

func addStampHash(logger log.Logger, st transaction.Storage) error {
	itemC := make(chan *reserve.BatchRadiusItemV1)
	errC := make(chan error)

	var eg errgroup.Group
	p := runtime.NumCPU()
	eg.SetLimit(p)

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				logger.Info("still migrating items...")
			case <-errC:
				return
			}
		}
	}()

	go func() {
		for oldBatchRadiusItem := range itemC {
			eg.Go(func() error {
				err := st.Run(context.Background(), func(s transaction.Store) error {
					idxStore := s.IndexStore()
					stamp, err := chunkstamp.LoadWithBatchID(idxStore, "reserve", oldBatchRadiusItem.Address, oldBatchRadiusItem.BatchID)
					if err != nil {
						return err
					}
					stampHash, err := stamp.Hash()
					if err != nil {
						return err
					}

					// Since the ID format has changed, we should delete the old item and put a new one with the new ID format.
					err = idxStore.Delete(oldBatchRadiusItem)
					if err != nil {
						return err
					}
					err = idxStore.Put(&reserve.BatchRadiusItem{
						Bin:       oldBatchRadiusItem.Bin,
						BatchID:   oldBatchRadiusItem.BatchID,
						StampHash: stampHash,
						Address:   oldBatchRadiusItem.Address,
						BinID:     oldBatchRadiusItem.BinID,
					})
					if err != nil {
						return err
					}

					oldChunkBinItem := &reserve.ChunkBinItemV1{
						Bin:   oldBatchRadiusItem.Bin,
						BinID: oldBatchRadiusItem.BinID,
					}
					err = idxStore.Get(oldChunkBinItem)
					if err != nil {
						return err
					}

					// same id. Will replace.
					err = idxStore.Put(&reserve.ChunkBinItem{
						Bin:       oldChunkBinItem.Bin,
						BinID:     oldChunkBinItem.BinID,
						Address:   oldChunkBinItem.Address,
						BatchID:   oldChunkBinItem.BatchID,
						StampHash: stampHash,
						ChunkType: oldChunkBinItem.ChunkType,
					})
					if err != nil {
						return err
					}

					// same id. Will replace.
					stampIndexItem := &stampindex.Item{
						BatchID:        oldChunkBinItem.BatchID,
						StampIndex:     stamp.Index(),
						StampHash:      stampHash,
						StampTimestamp: stamp.Timestamp(),
						ChunkAddress:   oldChunkBinItem.Address,
					}
					stampIndexItem.SetNamespace([]byte("reserve"))
					return idxStore.Put(stampIndexItem)
				})
				if err != nil {
					errC <- err
				}
				return err
			})
		}
		close(errC)
	}()

	err := st.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return new(reserve.BatchRadiusItemV1) },
	}, func(result storage.Result) (bool, error) {
		item := result.Entry.(*reserve.BatchRadiusItemV1)
		select {
		case itemC <- item:
		case err := <-errC:
			return true, err
		}
		return false, nil
	})
	close(itemC)
	if err != nil {
		return err
	}

	return eg.Wait()
}
