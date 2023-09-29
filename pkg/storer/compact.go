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
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
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
		validationWork(logger, store, sharkyRecover)
	}

	logger.Info("starting compaction")

	n := time.Now()

	for shard := 0; shard < sharkyNoOfShards; shard++ {

		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), sharkyRecover.Save())
		default:
		}

		items := make([]*chunkstore.RetrievalIndexItem, 0, 1_000_000)
		// we deliberately choose to iterate the whole store again for each shard
		// so that we do not store all the items in memory (for operators with huge localstores)
		_ = chunkstore.Iterate(store, func(item *chunkstore.RetrievalIndexItem) error {
			if item.Location.Shard == uint8(shard) {
				items = append(items, item)
			}
			return nil
		})

		sort.Slice(items, func(i, j int) bool {
			return items[i].Location.Slot < items[j].Location.Slot
		})

		lastUsedSlot := items[len(items)-1].Location.Slot
		slots := make([]*chunkstore.RetrievalIndexItem, lastUsedSlot+1) // marks free and used slots
		for _, l := range items {
			slots[l.Location.Slot] = l
		}

		// start begins at the zero slot. The loop below will increment the position of start until a free slot is found.
		// end points to the last slot, and the loop will decrement the position of end until a used slot is found.
		// Once start and end point to free and used slots, respectively, the swap of the chunk location will occur.
		start := uint32(0)
		end := lastUsedSlot

		batch, err := store.Batch(ctx)
		if err != nil {
			return err
		}

		for start < end {

			if slots[start] != nil {
				start++ // walk to the right until a free slot is found
				continue
			}

			if slots[end] == nil {
				end-- // walk to the left until a used slot found
				continue
			}

			from := slots[end]
			to := sharky.Location{Slot: start, Length: from.Location.Length, Shard: from.Location.Shard}
			if err := sharkyRecover.Move(context.Background(), from.Location, to); err != nil {
				return fmt.Errorf("sharky move: %w", err)
			}
			if err := sharkyRecover.Add(to); err != nil {
				return fmt.Errorf("sharky add: %w", err)
			}

			from.Location = to
			if err := batch.Put(from); err != nil {
				return fmt.Errorf("store put: %w", err)
			}

			start++
			end--
		}

		if err := batch.Commit(); err != nil {
			return err
		}

		logger.Info("shard truncated", "shard", fmt.Sprintf("%d/%d", shard, sharkyNoOfShards-1), "slot", end)

		if err := sharkyRecover.TruncateAt(context.Background(), uint8(shard), end+1); err != nil {
			return fmt.Errorf("sharky truncate: %w", err)
		}
	}

	if err := sharkyRecover.Save(); err != nil {
		return fmt.Errorf("sharky save: %w", err)
	}

	logger.Info("compaction finished", "duration", time.Since(n))

	if validate {
		logger.Info("performing chunk validation after compaction")
		validationWork(logger, store, sharkyRecover)
	}

	return nil
}

func validationWork(logger log.Logger, store storage.Store, sharky *sharky.Recovery) {

	n := time.Now()
	defer func() {
		logger.Info("validation finished", "duration", time.Since(n))
	}()

	iteratateItemsC := make(chan *chunkstore.RetrievalIndexItem)

	validChunk := func(item *chunkstore.RetrievalIndexItem, buf []byte) {
		err := sharky.Read(context.Background(), item.Location, buf)
		if err != nil {
			logger.Warning("invalid chunk", "address", item.Address, "timestamp", time.Unix(int64(item.Timestamp), 0), "location", item.Location, "error", err)
			return
		}

		ch := swarm.NewChunk(item.Address, buf)
		if !cac.Valid(ch) && !soc.Valid(ch) {

			logger.Info("invalid cac/soc chunk ", "address", item.Address, "timestamp", time.Unix(int64(item.Timestamp), 0))

			h, err := cac.DoHash(buf[swarm.SpanSize:], buf[:swarm.SpanSize])
			if err != nil {
				logger.Error(err, "cac hash")
				return
			}

			computedAddr := swarm.NewAddress(h)

			if !cac.Valid(swarm.NewChunk(computedAddr, buf)) {
				logger.Info("computed chunk is also an invalid cac")
				return
			}

			shardedEntry := chunkstore.RetrievalIndexItem{Address: computedAddr}
			err = store.Get(&shardedEntry)
			if err != nil {
				logger.Info("no shared entry found")
				return
			}

			logger.Info("retrieved chunk with shared slot", "shared_address", shardedEntry.Address, "shared_timestamp", time.Unix(int64(shardedEntry.Timestamp), 0))
		}
	}

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, swarm.SocMaxChunkSize)
			for item := range iteratateItemsC {
				validChunk(item, buf[:item.Location.Length])
			}
		}()
	}

	count := 0
	_ = chunkstore.Iterate(store, func(item *chunkstore.RetrievalIndexItem) error {
		iteratateItemsC <- item
		count++
		if count%100_000 == 0 {
			logger.Info("..still validating chunks", "count", count)
		}
		return nil
	})

	close(iteratateItemsC)

	wg.Wait()
}
