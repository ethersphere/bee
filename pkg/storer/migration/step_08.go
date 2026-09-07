// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/stampindex"
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
				if res.Entry.ID() == lastID {
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

			// Classify outside a transaction and remove each entry in its own
			// transaction: a transaction reads the reference count from the
			// committed store, so two removals at one address batched together
			// would both see the same count and leak the payload.
			var backfill []*reserve.ChunkBinItem
			for _, item := range items {
				chunk, err := st.ChunkStore().Get(context.Background(), item.Address)
				remove := false
				switch {
				case errors.Is(err, storage.ErrNotFound):
					remove = true
				case err != nil:
					return err
				}

				var chunkType swarm.ChunkType
				if !remove {
					chunkType = storage.ChunkType(chunk)
					// a legacy entry with an unset stamp hash would get a sum no
					// peer can ever compute; remove it and let sync restore the
					// chunk with proper stamp data.
					remove = chunkType == swarm.ChunkTypeUnspecified ||
						bytes.Equal(item.StampHash, swarm.EmptyAddress.Bytes())
				}

				if remove {
					removed++
					err := st.Run(context.Background(), func(s transaction.Store) error {
						return removeEntry(context.Background(), s, item)
					})
					if err != nil {
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
				backfill = append(backfill, &reserve.ChunkBinItem{
					Bin:       item.Bin,
					BinID:     item.BinID,
					Address:   item.Address,
					BatchID:   item.BatchID,
					StampHash: item.StampHash,
					ChunkType: chunkType,
					Sum:       sum,
				})
			}

			err = st.Run(context.Background(), func(s transaction.Store) error {
				for _, cbi := range backfill {
					err := errors.Join(
						s.IndexStore().Put(cbi),
						s.IndexStore().Put(&reserve.ChunkSumItem{Address: cbi.Address, Sum: cbi.Sum}),
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
			backfilled += len(backfill)
		}

		swept, err := reserve.RemoveMalformedChunkBinItems(context.Background(), st)
		if err != nil {
			return err
		}

		logger.Info("pullsync chunk sum backfill complete", "backfilled", backfilled, "removed", removed, "swept_orphans", swept)
		return nil
	}
}

// removeEntry drops a reserve entry together with its stamp rows. Stamp index
// and chunk stamp rows are keyed by batch and index, and the generic removal
// resolves them through the entry's stamp hash. A legacy entry recorded with
// an unset hash cannot resolve them that way, and leaving them behind poisons
// the stamp slot: the same chunk can never be restored and no newer chunk can
// take the slot. Stamps at the address from the entry's batch that no live
// entry references are therefore removed explicitly first.
func removeEntry(ctx context.Context, s transaction.Store, item *reserve.BatchRadiusItem) error {
	var orphans []swarm.Stamp
	err := chunkstamp.IterateAll(s.IndexStore(), "reserve", item.Address, func(stamp swarm.Stamp) (bool, error) {
		if !bytes.Equal(stamp.BatchID(), item.BatchID) {
			return false, nil
		}
		hash, err := stamp.Hash()
		if err != nil {
			return true, err
		}
		if bytes.Equal(hash, item.StampHash) {
			return false, nil // resolved and removed by RemoveChunkWithItem
		}
		has, err := s.IndexStore().Has(&reserve.BatchRadiusItem{Bin: item.Bin, BatchID: item.BatchID, Address: item.Address, StampHash: hash})
		if err != nil {
			return true, err
		}
		if !has {
			orphans = append(orphans, stamp)
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("iterate stamps of %s: %w", item.Address, err)
	}
	for _, stamp := range orphans {
		err := errors.Join(
			stampindex.Delete(s.IndexStore(), "reserve", stamp),
			chunkstamp.DeleteWithStamp(s.IndexStore(), "reserve", item.Address, stamp),
		)
		if err != nil {
			return err
		}
	}
	return reserve.RemoveChunkWithItem(ctx, s, item)
}
