// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"fmt"
	"path"
	"sort"
	"time"

	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
)

func Compact(ctx context.Context, basePath string, opts *Options) error {

	logger := opts.Logger

	n := time.Now()
	defer func() {
		logger.Info("compaction finished", "duration", time.Since(n))
	}()

	logger.Info("starting compaction")

	store, err := initStore(basePath, opts)
	if err != nil {
		return fmt.Errorf("failed creating levelDB index store: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error(err, "failed closing store")
		}
	}()

	sharkyRecover, err := sharky.NewRecovery(path.Join(basePath, sharkyPath), sharkyNoOfShards, swarm.SocMaxChunkSize)
	if err != nil {
		return err
	}
	defer func() {
		if err := sharkyRecover.Close(); err != nil {
			logger.Error(err, "failed closing sharky recovery")
		}
	}()

	iteratateItemsC := make(chan chunkstore.IterateResult)
	chunkstore.Iterate(ctx, store, iteratateItemsC)

	var shards [][]*chunkstore.RetrievalIndexItem
	for i := 0; i < sharkyNoOfShards; i++ {
		shards = append(shards, []*chunkstore.RetrievalIndexItem{})
	}

	count := 0
	for c := range iteratateItemsC {
		if c.Err != nil {
			return fmt.Errorf("location read: %w", err)
		}
		count++
		shards[c.Item.Location.Shard] = append(shards[c.Item.Location.Shard], c.Item)
	}

	logger.Info("total items", "count", count)

	for shard := 0; shard < sharkyNoOfShards; shard++ {

		locs := shards[shard]

		logger.Info("starting shard", "shard", shard)

		sort.Slice(locs, func(i, j int) bool {
			return locs[i].Location.Slot < locs[j].Location.Slot
		})

		lastUsedSlot := locs[len(locs)-1].Location.Slot
		slots := make([]*chunkstore.RetrievalIndexItem, lastUsedSlot+1) // marks free and used slots
		for _, l := range locs {
			slots[l.Location.Slot] = l
		}

		start := uint32(0)
		end := lastUsedSlot

		for start < end {
			if slots[start] == nil { // free
				if slots[end] != nil { // used
					from := slots[end]
					to := sharky.Location{Slot: start, Length: from.Location.Length, Shard: from.Location.Shard}
					if err := sharkyRecover.Move(ctx, from.Location, to); err != nil {
						return fmt.Errorf("sharky move: %w", err)
					}
					if err := sharkyRecover.Add(to); err != nil {
						return fmt.Errorf("sharky add: %w", err)
					}

					from.Location = to
					if err := store.Put(from); err != nil {
						return fmt.Errorf("sharky add: %w", err)
					}

					start++
					end--
				} else {
					end-- // keep moving to the left until a used slot is found
				}
			} else {
				start++ // keep moving to the right until a free slot is found
			}
		}

		logger.Info("shard truncated", "shard", shard, "slot", end)

		fmt.Println("truncate", shard, end)
		if err := sharkyRecover.TruncateAt(ctx, uint8(shard), end+1); err != nil {
			return fmt.Errorf("sharky truncate: %w", err)
		}
	}

	return sharkyRecover.Save()
}
