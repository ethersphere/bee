// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

// ReserveRepair runs the manual reserve repair procedure used by the db repair command.
func ReserveRepair(
	st transaction.Storage,
	chunkTypeFunc func(swarm.Chunk) swarm.ChunkType,
	logger log.Logger,
) func() error {
	return func() error {
		checkBinIDs := func() error {
			binIds := make(map[uint8]map[uint64]int)
			return st.IndexStore().Iterate(
				storage.Query{
					Factory: func() storage.Item { return &reserve.BatchRadiusItem{} },
				},
				func(res storage.Result) (bool, error) {
					item := res.Entry.(*reserve.BatchRadiusItem)
					if _, ok := binIds[item.Bin]; !ok {
						binIds[item.Bin] = make(map[uint64]int)
					}
					binIds[item.Bin][item.BinID]++
					if binIds[item.Bin][item.BinID] > 1 {
						return false, fmt.Errorf("binID %d in bin %d already used", item.BinID, item.Bin)
					}

					err := st.IndexStore().Get(&reserve.ChunkBinItem{Bin: item.Bin, BinID: item.BinID})
					if err != nil {
						return false, fmt.Errorf("check failed: chunkBinItem, bin %d, binID %d: %w", item.Bin, item.BinID, err)
					}

					return false, nil
				},
			)
		}

		logger.Info("starting reserve repair tool, do not interrupt or kill the process...")

		err := checkBinIDs()
		if err != nil {
			logger.Info("pre-repair check failed", "error", err)
		}

		err = st.Run(context.Background(), func(s transaction.Store) error {
			return s.IndexStore().Delete(&reserve.EpochItem{})
		})
		if err != nil {
			return err
		}

		err = st.Run(context.Background(), func(s transaction.Store) error {
			for i := range swarm.MaxBins {
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

		const batchSize = 1000
		for i := 0; i < len(chunkBinItems); i += batchSize {
			end := min(i+batchSize, len(chunkBinItems))
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

		logger.Info("counted all batch radius entries", "total_entries", len(batchRadiusItems))

		var missingChunks atomic.Int64
		var invalidSharkyChunks atomic.Int64
		var bins [swarm.MaxBins]uint64
		var mtx sync.Mutex

		newID := func(bin int) uint64 {
			mtx.Lock()
			defer mtx.Unlock()

			bins[bin]++
			return bins[bin]
		}

		var eg errgroup.Group
		p := runtime.NumCPU()
		eg.SetLimit(p)

		logger.Info("parallel workers", "count", p)

		for _, item := range batchRadiusItems {
			item := item
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
					if bytes.Equal(item.StampHash, swarm.EmptyAddress.Bytes()) {
						stamp, err := chunkstamp.LoadWithStampHash(s.IndexStore(), "reserve", item.Address, item.StampHash)
						if err != nil {
							return err
						}
						stampHash, err := stamp.Hash()
						if err != nil {
							return err
						}
						item.StampHash = stampHash
					}

					err = s.IndexStore().Put(item)
					if err != nil {
						return err
					}

					return s.IndexStore().Put(&reserve.ChunkBinItem{
						BatchID:   item.BatchID,
						Bin:       item.Bin,
						Address:   item.Address,
						BinID:     item.BinID,
						StampHash: item.StampHash,
						ChunkType: chunkType,
					})
				})
			})
		}

		err = eg.Wait()
		if err != nil {
			return err
		}

		err = st.Run(context.Background(), func(s transaction.Store) error {
			for bin, id := range bins {
				err := s.IndexStore().Put(&reserve.BinItem{Bin: uint8(bin), BinID: id})
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		err = checkBinIDs()
		if err != nil {
			return err
		}

		batchRadiusCnt, err := st.IndexStore().Count(&reserve.BatchRadiusItem{})
		if err != nil {
			return err
		}

		chunkBinCnt, err := st.IndexStore().Count(&reserve.ChunkBinItem{})
		if err != nil {
			return err
		}

		logger.Info("migrated all chunk entries", "new_size", batchRadiusCnt, "missing_chunks", missingChunks.Load(), "invalid_sharky_chunks", invalidSharkyChunks.Load())

		if batchRadiusCnt != chunkBinCnt {
			return fmt.Errorf("index counts do not match, %d vs %d", batchRadiusCnt, chunkBinCnt)
		}

		return nil
	}
}
