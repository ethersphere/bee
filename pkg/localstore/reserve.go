// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

// UnreserveBatch atomically unpins chunks of a batch in proximity order upto and including po.
// Unpinning will result in all chunks with pincounter 0 to be put in the gc index
// so if a chunk was only pinned by the reserve, unreserving it  will make it gc-able.
func (db *DB) UnreserveBatch(id []byte, radius uint8) error {
	db.batchMu.Lock()
	defer db.batchMu.Unlock()
	var (
		item = shed.Item{
			BatchID: id,
		}
		batch = new(leveldb.Batch)
	)

	i, err := db.postageRadiusIndex.Get(item)
	if err != nil {
		if !errors.Is(err, leveldb.ErrNotFound) {
			return err
		}
		item.Radius = radius
		if err := db.postageRadiusIndex.PutInBatch(batch, item); err != nil {
			return err
		}
		return db.shed.WriteBatch(batch)
	}
	oldRadius := i.Radius
	var gcSizeChange int64 // number to add or subtract from gcSize
	unpin := func(item shed.Item) (stop bool, err error) {
		c, err := db.setUnpin(batch, swarm.NewAddress(item.Address))
		if err != nil {
			return false, fmt.Errorf("unpin: %w", err)
		}

		gcSizeChange += c
		return false, err
	}

	// iterate over chunk in bins
	for bin := oldRadius; bin < radius; bin++ {
		err := db.postageChunksIndex.Iterate(unpin, &shed.IterateOptions{Prefix: append(id, bin)})
		if err != nil {
			return err
		}
		// adjust gcSize
		if err := db.incGCSizeInBatch(batch, gcSizeChange); err != nil {
			return err
		}
		item.Radius = bin
		if err := db.postageRadiusIndex.PutInBatch(batch, item); err != nil {
			return err
		}
		if bin == swarm.MaxPO {
			if err := db.postageRadiusIndex.DeleteInBatch(batch, item); err != nil {
				return err
			}
		}
		if err := db.shed.WriteBatch(batch); err != nil {
			return err
		}
		batch = new(leveldb.Batch)
		gcSizeChange = 0
	}

	gcSize, err := db.gcSize.Get()
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return err
	}
	// trigger garbage collection if we reached the capacity
	if gcSize >= db.cacheCapacity {
		db.triggerGarbageCollection()
	}

	return nil
}

func withinRadius(db *DB, item shed.Item) bool {
	po := db.po(swarm.NewAddress(item.Address))
	return po >= item.Radius
}
