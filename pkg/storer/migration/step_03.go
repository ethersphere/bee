// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"
	"errors"
	"os"

	"github.com/ethersphere/bee/v2/pkg/log"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// step_03 is a migration step that removes all BinItem entries and migrates
// ChunkBinItem and BatchRadiusItem entries to use a new BinID field.
func step_03(
	st transaction.Storage,
	chunkType func(swarm.Chunk) swarm.ChunkType,
) func() error {
	return func() error {
		/*
			STEP 1, remove all of the BinItem entires
			STEP 2, remove all of the ChunkBinItem entries
			STEP 3, iterate BatchRadiusItem, get new binID using IncBinID, create new ChunkBinItem and BatchRadiusItem
		*/

		rs := reserve.Reserve{}
		logger := log.NewLogger("migration-step-03", log.WithSink(os.Stdout))
		logger.Info("starting migration for reconstructing reserve bin IDs, do not interrupt or kill the process...")

		// STEP 1

		err := st.Run(context.Background(), func(s transaction.Store) error {
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
		err = st.ReadOnly().IndexStore().Iterate(
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

		for i := 0; i < len(chunkBinItems); i += 10000 {
			end := i + 10000
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
		logger.Info("removed all bin ids", "total_entries", len(chunkBinItems))
		chunkBinItems = nil

		// STEP 3
		var batchRadiusItems []*reserve.BatchRadiusItem
		err = st.ReadOnly().IndexStore().Iterate(
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

		batchSize := 10000
		missingChunks := 0

		for i := 0; i < len(batchRadiusItems); i += batchSize {
			end := i + batchSize
			if end > len(batchRadiusItems) {
				end = len(batchRadiusItems)
			}

			err := st.Run(context.Background(), func(s transaction.Store) error {
				for _, item := range batchRadiusItems[i:end] {
					chunk, err := s.ChunkStore().Get(context.Background(), item.Address)
					if err != nil && !errors.Is(err, storage.ErrNotFound) {
						return err
					}
					hasChunkEntry := err == nil

					if !hasChunkEntry {
						err = s.IndexStore().Delete(item)
						if err != nil {
							return err
						}
						missingChunks++
					} else {

						var newBinID uint64
						err = st.Run(context.Background(), func(s transaction.Store) error {
							newBinID, err = rs.IncBinID(s.IndexStore(), item.Bin)
							return err
						})
						if err != nil {
							return err
						}

						item.BinID = newBinID
						err = s.IndexStore().Put(item)
						if err != nil {
							return err
						}

						err = s.IndexStore().Put(&reserve.ChunkBinItem{
							BatchID: item.BatchID,
							Bin:     item.Bin,
							Address: item.Address,
							BinID:   newBinID,
							Type:    chunkType(chunk),
						})
						if err != nil {
							return err
						}
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}

		logger.Info("migrated all chunk entries", "new_size", len(batchRadiusItems)-missingChunks, "missing_chunks", missingChunks)
		return nil
	}
}
