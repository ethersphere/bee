// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// removeRogueChunks re-verifies every reserve entry not stored as a CAC: entries
// of unspecified type, which reserve.Put accepted before it validated the type,
// and SOCs, which were accepted before SOC validation checked the chunk address.
// Entries whose chunk fails chunkstore.Verify are removed; the others keep their
// place and only get their type corrected if needed.
func removeRogueChunks(st transaction.Storage, logger log.Logger) func() error {
	return func() error {
		ctx := context.Background()

		var rogue []*reserve.ChunkBinItem
		err := st.IndexStore().Iterate(
			storage.Query{
				Factory: func() storage.Item { return &reserve.ChunkBinItem{} },
			},
			func(res storage.Result) (bool, error) {
				item := res.Entry.(*reserve.ChunkBinItem)
				if item.ChunkType != swarm.ChunkTypeContentAddressed {
					rogue = append(rogue, item)
				}
				return false, nil
			},
		)
		if err != nil {
			return fmt.Errorf("iterate chunk bin items: %w", err)
		}

		if len(rogue) == 0 {
			return nil
		}

		logger.Info("verifying non-cac reserve chunks", "count", len(rogue))

		var removed, retyped int

		batchSize := 1000

		for i := 0; i < len(rogue); i += batchSize {
			end := min(i+batchSize, len(rogue))
			err := st.Run(ctx, func(s transaction.Store) error {
				for _, item := range rogue[i:end] {
					chunkType, err := chunkstore.Verify(s.ChunkStore().Get(ctx, item.Address))
					if err == nil {
						if item.ChunkType == chunkType {
							continue
						}
						item.ChunkType = chunkType
						if err := s.IndexStore().Put(item); err != nil {
							return fmt.Errorf("put chunk bin item %s: %w", item.Address, err)
						}
						retyped++
						continue
					}

					err = reserve.RemoveChunkWithItem(ctx, s, &reserve.BatchRadiusItem{
						Bin:       item.Bin,
						BatchID:   item.BatchID,
						Address:   item.Address,
						BinID:     item.BinID,
						StampHash: item.StampHash,
					})
					if err != nil {
						return fmt.Errorf("remove chunk %s: %w", item.Address, err)
					}
					removed++
				}
				return nil
			})
			if err != nil {
				return err
			}
		}

		logger.Info("removed rogue reserve chunks", "removed", removed, "retyped", retyped)

		return nil
	}
}
