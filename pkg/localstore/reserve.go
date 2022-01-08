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

// UnreserveBatch atomically gcs chunks of a batch in proximity order upto and including po.
func (db *DB) UnreserveBatch(id []byte, radius uint8) (evicted uint64, err error) {
	var (
		item = shed.Item{
			BatchID: id,
		}
		batch = new(leveldb.Batch)
	)

	var (
		gcSizeChange      int64 // number to add or subtract from gcSize and reserveSize
		reserveSizeChange uint64
	)

	gc := func(item shed.Item) (stop bool, err error) {
		addr := swarm.NewAddress(item.Address)

		_, err = db.setGc(batch, addr)
		if err != nil {
			if !errors.Is(err, leveldb.ErrNotFound) {
				return false, fmt.Errorf("gc: %w", err)
			}
			// this is possible when we are resyncing chain data after
			// a dirty shutdown
			db.logger.Tracef("unreserve set gc chunk %s: %v", addr.String(), err)
		} else {
			reserveSizeChange++
			gcSizeChange++
		}

		return false, nil
	}

	// iterate over chunk in bins
	for bin := uint8(0); bin < radius; bin++ {
		err := db.postageChunksIndex.Iterate(gc, &shed.IterateOptions{Prefix: append(id, bin)})
		if err != nil {
			return 0, err
		}
		// adjust gcSize
		if err := db.incGCSizeInBatch(batch, gcSizeChange); err != nil {
			return 0, err
		}
		item.Radius = bin
		if err := db.postageRadiusIndex.PutInBatch(batch, item); err != nil {
			return 0, err
		}
		if bin == swarm.MaxPO {
			if err := db.postageRadiusIndex.DeleteInBatch(batch, item); err != nil {
				return 0, err
			}
		}
		if err := db.shed.WriteBatch(batch); err != nil {
			return 0, err
		}
		batch = new(leveldb.Batch)
		gcSizeChange = 0
	}

	if radius != swarm.MaxPO+1 {
		item.Radius = radius
		if err := db.postageRadiusIndex.PutInBatch(batch, item); err != nil {
			return 0, err
		}
		if err := db.shed.WriteBatch(batch); err != nil {
			return 0, err
		}
	}

	gcSize, err := db.gcSize.Get()
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return 0, err
	}

	if reserveSizeChange > 0 {
		batch = new(leveldb.Batch)
		if err := db.incReserveSizeInBatch(batch, -int64(reserveSizeChange)); err != nil {
			return 0, err
		}
		if err := db.shed.WriteBatch(batch); err != nil {
			return 0, err
		}
	}

	// trigger garbage collection if we reached the capacity
	if gcSize >= db.cacheCapacity {
		db.triggerGarbageCollection()
	}

	return reserveSizeChange, nil
}

func withinRadius(db *DB, item shed.Item) bool {
	po := db.po(swarm.NewAddress(item.Address))
	return po >= item.Radius
}
