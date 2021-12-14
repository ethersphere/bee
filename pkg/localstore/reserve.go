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

	unpin := func(item shed.Item) (stop bool, err error) {
		addr := swarm.NewAddress(item.Address)
		c, err := db.setUnpin(batch, addr)
		if err != nil {
			if !errors.Is(err, leveldb.ErrNotFound) {
				return false, fmt.Errorf("unpin: %w", err)
			} else {
				// this is possible when we are resyncing chain data after
				// a dirty shutdown
				db.logger.Tracef("unreserve set unpin chunk %s: %v", addr.String(), err)
			}
		} else {
			// we need to do this because a user might pin a chunk on top of
			// the reserve pinning. when we unpin due to an unreserve call, then
			// we should logically deduct the chunk anyway from the reserve size
			// otherwise the reserve size leaks, since c returned from setUnpin
			// will be zero.
			reserveSizeChange++
		}

		gcSizeChange += c
		return false, nil
	}

	// iterate over chunk in bins
	for bin := uint8(0); bin < radius; bin++ {
		err := db.postageChunksIndex.Iterate(unpin, &shed.IterateOptions{Prefix: append(id, bin)})
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
