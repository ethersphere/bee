// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"time"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

// Compact minimizes sharky disk usage by, using the current sharky locations from the storer,
// relocating chunks starting from the end of the used slots to the first available slots.
func Compact(ctx context.Context, basePath string, opts *Options, validate bool) error {

	logger := opts.Logger

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

	if validate {
		logger.Info("performing chunk validation before compaction")
		validationWork(ctx, logger, store, sharkyRecover)
	}

	logger.Info("starting compaction")

	n := time.Now()

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

		shards[c.Item.Location.Shard] = append(shards[c.Item.Location.Shard], c.Item)
	}

	logger.Info("total items", "count", count)

	for shard := 0; shard < sharkyNoOfShards; shard++ {

		locs := shards[shard]

		sort.Slice(locs, func(i, j int) bool {
			return locs[i].Location.Slot < locs[j].Location.Slot
		})

		lastUsedSlot := locs[len(locs)-1].Location.Slot
		slots := make([]*chunkstore.RetrievalIndexItem, lastUsedSlot+1) // marks free and used slots
		for _, l := range locs {
			slots[l.Location.Slot] = l
		}

		// start begins at the zero slot. The loop below will increment the position of start until a free slot is found.
		// end points to the last slot, and the loop will decrement the position of end until a used slot is found.
		// Once start and end point to free and used slots, respectively, the swap of the chunk location will occur.
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
						return fmt.Errorf("store put: %w", err)
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

		if err := sharkyRecover.TruncateAt(ctx, uint8(shard), end+1); err != nil {
			return fmt.Errorf("sharky truncate: %w", err)
		}
	}

	if err := sharkyRecover.Save(); err != nil {
		return fmt.Errorf("sharky save: %w", err)
	}

	logger.Info("compaction finished", "duration", time.Since(n))

	if validate {
		logger.Info("performing chunk validation after compaction")
		validationWork(ctx, logger, store, sharkyRecover)
	}

	return nil
}

func validationWork(ctx context.Context, logger log.Logger, store storage.Store, sharky *sharky.Recovery) {

	n := time.Now()
	defer func() {
		logger.Info("validation finished", "duration", time.Since(n))
	}()

	iteratateItemsC := make(chan chunkstore.IterateResult)
	chunkstore.Iterate(ctx, store, iteratateItemsC)

	validChunk := func(item *chunkstore.RetrievalIndexItem, buf []byte) error {
		err := sharky.Read(ctx, item.Location, buf)
		if err != nil {
			return err
		}

		ch := swarm.NewChunk(item.Address, buf[:item.Location.Length])
		if !cac.Valid(ch) && !soc.Valid(ch) {
			return errors.New("invalid chunk")
		}

		return nil
	}

	eg, ctx := errgroup.WithContext(ctx)

	for i := 0; i < 8; i++ {
		eg.Go(func() error {
			buf := make([]byte, swarm.SocMaxChunkSize)
			for item := range iteratateItemsC {
				if err := validChunk(item.Item, buf); err != nil {
					logger.Info("invalid chunk", "address", item.Item.Address, "error", err)
				}
			}
			return nil
		})
	}

	_ = eg.Wait()
}
