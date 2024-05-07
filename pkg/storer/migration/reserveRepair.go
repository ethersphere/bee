// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/log"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

// ReserveRepairer is a migration step that removes all BinItem entries and migrates
// ChunkBinItem and BatchRadiusItem entries to use a new BinID field.
func ReserveRepairer(
	st transaction.Storage,
	chunkTypeFunc func(swarm.Chunk) swarm.ChunkType,
	logger log.Logger,
) func() error {
	return func() error {
		/*
			STEP 0:	remove epoch item
			STEP 1:	remove all of the BinItem entires
			STEP 2:	remove all of the ChunkBinItem entries
			STEP 3:	iterate BatchRadiusItem, get new binID using IncBinID,
					create new ChunkBinItem and BatchRadiusItem if the chunk exists in the chunkstore
					if the chunk is invalid, it is removed from the chunkstore
		*/

		logger.Info("starting migration for reconstructing reserve bin IDs, do not interrupt or kill the process...")

		// STEP 0
		err := st.Run(context.Background(), func(s transaction.Store) error {
			return s.IndexStore().Delete(&reserve.EpochItem{})
		})
		if err != nil {
			return err
		}

		// STEP 1
		err = st.Run(context.Background(), func(s transaction.Store) error {
			for i := uint8(0); i < swarm.MaxBins; i++ {
				err := s.IndexStore().Delete(&reserve.BinItem{Bin: i})
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		logger.Info("removed all bin index entries")

		// STEP 2
		var chunkBinItems []*reserve.ChunkBinItem
		err = st.IndexStore().Iterate(
			storage.Query{
				Factory: func() storage.Item { return &reserve.ChunkBinItem{} },
			},
			func(res storage.Result) (bool, error) {
				item := res.Entry.(*reserve.ChunkBinItem)
				chunkBinItems = append(chunkBinItems, item)
				return false, nil
			},
		)
		if err != nil {
			return err
		}

		batchSize := 1000

		for i := 0; i < len(chunkBinItems); i += batchSize {
			end := i + batchSize
			if end > len(chunkBinItems) {
				end = len(chunkBinItems)
			}
			err := st.Run(context.Background(), func(s transaction.Store) error {
				for _, item := range chunkBinItems[i:end] {
					err := s.IndexStore().Delete(item)
					if err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		logger.Info("removed all chunk bin items", "total_entries", len(chunkBinItems))
		chunkBinItems = nil

		// STEP 3
		var batchRadiusItems []*reserve.BatchRadiusItem
		err = st.IndexStore().Iterate(
			storage.Query{
				Factory: func() storage.Item { return &reserve.BatchRadiusItem{} },
			},
			func(res storage.Result) (bool, error) {
				item := res.Entry.(*reserve.BatchRadiusItem)
				batchRadiusItems = append(batchRadiusItems, item)

				return false, nil
			},
		)
		if err != nil {
			return err
		}

		logger.Info("found reserve chunk entries, adding new entries", "total_entries", len(batchRadiusItems))

		var missingChunks atomic.Int64
		var invalidSharkyChunks atomic.Int64

		var bins [32]uint64
		var mtx sync.Mutex
		newID := func(bin int) uint64 {
			mtx.Lock()
			defer mtx.Unlock()

			bins[bin]++
			return bins[bin]
		}

		var eg errgroup.Group
		eg.SetLimit(8)

		for _, item := range batchRadiusItems {
			func(item *reserve.BatchRadiusItem) {
				eg.Go(func() error {
					return st.Run(context.Background(), func(s transaction.Store) error {

						chunk, err := s.ChunkStore().Get(context.Background(), item.Address)
						if err != nil {
							if errors.Is(err, storage.ErrNotFound) {
								missingChunks.Add(1)
								return reserve.RemoveChunkWithItem(context.Background(), s, item)
							}
							return err
						}

						chunkType := chunkTypeFunc(chunk)
						if chunkType == swarm.ChunkTypeUnspecified {
							invalidSharkyChunks.Add(1)
							return reserve.RemoveChunkWithItem(context.Background(), s, item)
						}

						item.BinID = newID(int(item.Bin))
						err = s.IndexStore().Put(item)
						if err != nil {
							return err
						}

						return s.IndexStore().Put(&reserve.ChunkBinItem{
							BatchID: item.BatchID,
							Bin:     item.Bin,
							Address: item.Address,
							BinID:   item.BinID,
							Type:    chunkType,
						})
					})
				})
			}(item)
		}

		err = eg.Wait()
		if err != nil {
			return err
		}

		// extra test that ensure that a unique binID has been issed to each item.
		binIds := make(map[uint8]map[uint64]int)
		err = st.IndexStore().Iterate(
			storage.Query{
				Factory: func() storage.Item { return &reserve.ChunkBinItem{} },
			},
			func(res storage.Result) (bool, error) {
				item := res.Entry.(*reserve.ChunkBinItem)
				if _, ok := binIds[item.Bin]; !ok {
					binIds[item.Bin] = make(map[uint64]int)
				}
				binIds[item.Bin][item.BinID]++
				if binIds[item.Bin][item.BinID] > 1 {
					return false, errors.New("already used binID")
				}

				return false, nil
			},
		)
		if err != nil {
			return err
		}

		logger.Info("migrated all chunk entries", "new_size", len(batchRadiusItems)-int(missingChunks.Load()), "missing_chunks", missingChunks.Load(), "invalid_sharky_chunks", invalidSharkyChunks.Load())
		return nil
	}
}
