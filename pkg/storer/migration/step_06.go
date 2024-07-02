// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"
	"fmt"
	"os"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// step_06 is a migration step that adds a stampHash to all BatchRadiusItems, ChunkBinItems and StampIndexItems.
func step_06(st transaction.Storage) func() error {
	return func() error {
		logger := log.NewLogger("migration-step-06", log.WithSink(os.Stdout))
		logger.Info("start adding stampHash to BatchRadiusItems, ChunkBinItems and StampIndexItems")
		err := st.Run(context.Background(), func(s transaction.Store) error {
			err := addStampHash(s.IndexStore(), &reserve.BatchRadiusItem{})
			if err != nil {
				return err
			}
			err = addStampHash(s.IndexStore(), &reserve.ChunkBinItem{})
			if err != nil {
				return err
			}
			return addStampHash(s.IndexStore(), &stampindex.Item{})
		})
		if err != nil {
			return err
		}
		logger.Info("finished migrating items")
		return nil
	}
}

// addStampHash adds a stampHash to a storage item.
// only BatchRadiusItem and ChunkBinItem are supported.
func addStampHash(st storage.IndexStore, fact storage.Item) error {
	return st.Iterate(storage.Query{
		Factory: func() storage.Item { return fact },
	}, func(res storage.Result) (bool, error) {
		var (
			addr    swarm.Address
			batchID []byte
		)

		switch t := res.Entry.(type) {
		case *reserve.ChunkBinItem:
			item := res.Entry.(*reserve.ChunkBinItem)
			addr = item.Address
			batchID = item.BatchID
		case *reserve.BatchRadiusItem:
			item := res.Entry.(*reserve.BatchRadiusItem)
			addr = item.Address
			batchID = item.BatchID
		case *stampindex.Item:
			item := res.Entry.(*stampindex.Item)
			addr = item.ChunkAddress
			batchID = item.BatchID
		default:
			return true, fmt.Errorf("unsupported item type: %T", t)
		}

		stamp, err := chunkstamp.LoadWithBatchID(st, "reserve", addr, batchID)
		if err != nil {
			return true, fmt.Errorf("load chunkstamp: %w", err)
		}
		hash, err := stamp.Hash()
		if err != nil {
			return true, fmt.Errorf("hash stamp: %w", err)
		}

		switch res.Entry.(type) {
		case *reserve.ChunkBinItem:
			item := res.Entry.(*reserve.ChunkBinItem)
			item.StampHash = hash
			err = st.Put(item)
		case *reserve.BatchRadiusItem:
			item := res.Entry.(*reserve.BatchRadiusItem)

			// Since the ID format has changed, we should delete the old item and put a new one with the new ID format.
			err = st.Delete(&oldBatchRadiusItem{item})
			if err != nil {
				return true, fmt.Errorf("delete old batch radius item: %w", err)
			}
			item.StampHash = hash
			err = st.Put(item)
		case *stampindex.Item:
			item := res.Entry.(*stampindex.Item)
			item.StampHash = hash
			err = st.Put(item)
		}
		if err != nil {
			return true, fmt.Errorf("put item: %w", err)
		}
		return false, nil
	})
}

type oldBatchRadiusItem struct {
	*reserve.BatchRadiusItem
}

// ID returns the old ID format for BatchRadiusItem ID. (batchId/bin/ChunkAddr).
func (b *oldBatchRadiusItem) ID() string {
	return string(b.BatchID) + string(b.Bin) + b.Address.ByteString()
}
