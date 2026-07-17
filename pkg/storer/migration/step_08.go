// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// step_08 backfills the pullsync divergence checksum (SWIP-101). The
// ChunkBinItem serialization grew by a trailing Sum field, so every existing
// ChunkBinItem is rewritten (reconstructed from the authoritative
// BatchRadiusItem, whose serialization is unchanged) with its Sum populated,
// and a companion ChunkSumItem existence row is created for the pullsync
// want-decision. Chunks missing from the chunkstore or with an invalid type are
// removed, consistent with the reserve repair procedure.
func step_08(
	st transaction.Storage,
	logger log.Logger,
) func() error {
	return func() error {
		logger.Info("starting pullsync chunk sum backfill migration; do not interrupt or kill the process...")

		var items []*reserve.BatchRadiusItem
		err := st.IndexStore().Iterate(storage.Query{
			Factory: func() storage.Item { return &reserve.BatchRadiusItem{} },
		}, func(res storage.Result) (bool, error) {
			items = append(items, res.Entry.(*reserve.BatchRadiusItem))
			return false, nil
		})
		if err != nil {
			return err
		}

		logger.Info("counted reserve entries to backfill", "total_entries", len(items))

		const batchSize = 1000
		removed := 0

		for i := 0; i < len(items); i += batchSize {
			end := min(i+batchSize, len(items))
			err := st.Run(context.Background(), func(s transaction.Store) error {
				for _, item := range items[i:end] {
					chunk, err := s.ChunkStore().Get(context.Background(), item.Address)
					if err != nil {
						if errors.Is(err, storage.ErrNotFound) {
							removed++
							if err := reserve.RemoveChunkWithItem(context.Background(), s, item); err != nil {
								return err
							}
							continue
						}
						return err
					}

					chunkType := storage.ChunkType(chunk)
					if chunkType == swarm.ChunkTypeUnspecified {
						removed++
						if err := reserve.RemoveChunkWithItem(context.Background(), s, item); err != nil {
							return err
						}
						continue
					}

					stamp, err := chunkstamp.LoadWithStampHash(s.IndexStore(), "reserve", item.Address, item.StampHash)
					if err != nil {
						return err
					}

					sum, err := storage.ChunkSum(chunk.WithStamp(stamp))
					if err != nil {
						return err
					}

					err = errors.Join(
						s.IndexStore().Put(&reserve.ChunkBinItem{
							Bin:       item.Bin,
							BinID:     item.BinID,
							Address:   item.Address,
							BatchID:   item.BatchID,
							StampHash: item.StampHash,
							ChunkType: chunkType,
							Sum:       sum,
						}),
						s.IndexStore().Put(&reserve.ChunkSumItem{Address: item.Address, Sum: sum}),
					)
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

		logger.Info("pullsync chunk sum backfill complete", "backfilled", len(items)-removed, "removed", removed)
		return nil
	}
}
