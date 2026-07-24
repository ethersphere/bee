// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"bytes"
	"context"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// step_08 backfills the pullsync divergence checksum (SWIP-101). The
// ChunkBinItem serialization grew by a trailing Sum field, so every existing
// ChunkBinItem is rewritten (reconstructed from the authoritative
// BatchRadiusItem, whose serialization is unchanged) with its Sum populated,
// and a companion ChunkSumItem existence row is created for the pullsync
// want-decision. The sum is derived from the batch ID and stamp hash already
// carried by the BatchRadiusItem, so stamps are never loaded. Chunks missing
// from the chunkstore, with an invalid type or with an unset stamp hash are
// removed, consistent with the reserve repair procedure. Finally, orphaned
// pre-migration ChunkBinItems (no matching BatchRadiusItem, hence never
// rewritten) are swept by raw key so no old-format record survives to break
// later iterations.
//
// The BatchRadiusItem namespace is paged through in fixed windows instead of
// being loaded whole: at reserve capacity the full index is millions of
// entries, and the underlying stores do not support writes during an
// iteration.
func step_08(
	st transaction.Storage,
	logger log.Logger,
) func() error {
	return func() error {
		logger.Info("starting pullsync chunk sum backfill migration; do not interrupt or kill the process...")

		const pageSize = 1000

		backfilled, removed := 0, 0
		lastID := ""

		for {
			var items []*reserve.BatchRadiusItem
			err := st.IndexStore().Iterate(storage.Query{
				Factory:       func() storage.Item { return &reserve.BatchRadiusItem{} },
				Prefix:        lastID,
				PrefixAtStart: true,
			}, func(res storage.Result) (bool, error) {
				// the resume key may have been deleted in the previous window,
				// in which case iteration lands on the next entry; matching on
				// the ID rather than skipping the first result unconditionally
				// avoids silently dropping that entry.
				if res.ID == lastID {
					return false, nil
				}
				items = append(items, res.Entry.(*reserve.BatchRadiusItem))
				return len(items) >= pageSize, nil
			})
			if err != nil {
				return err
			}
			if len(items) == 0 {
				break
			}
			lastID = items[len(items)-1].ID()

			err = st.Run(context.Background(), func(s transaction.Store) error {
				for _, item := range items {
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

					// a legacy entry with an unset stamp hash would get a sum no
					// peer can ever compute; remove it and let sync restore the
					// chunk with proper stamp data.
					if bytes.Equal(item.StampHash, swarm.EmptyAddress.Bytes()) {
						removed++
						if err := reserve.RemoveChunkWithItem(context.Background(), s, item); err != nil {
							return err
						}
						continue
					}

					// the sum only needs the batch ID and stamp hash, both already
					// on the item, so the stamp itself is never loaded.
					sum, err := storage.ChunkSumFromParts(item.BatchID, item.StampHash, chunk)
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
					backfilled++
				}
				return nil
			})
			if err != nil {
				return err
			}
		}

		swept, err := reserve.RemoveMalformedChunkBinItems(context.Background(), st)
		if err != nil {
			return err
		}

		logger.Info("pullsync chunk sum backfill complete", "backfilled", backfilled, "removed", removed, "swept_orphans", swept)
		return nil
	}
}
