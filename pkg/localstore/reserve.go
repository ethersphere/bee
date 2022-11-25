// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"encoding/hex"
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
		reserveSizeChange uint64
	)

	evictBatch := radius == swarm.MaxBins
	if evictBatch {
		if err := db.postageRadiusIndex.Delete(item); err != nil {
			return 0, err
		}
	}

	// iterate over chunk in bins
	for bin := uint8(0); bin < radius; bin++ {
		rSizeChange, err := db.unpinBatchChunks(id, bin)
		if err != nil {
			db.logger.Debug("unreserve batch", "batch", hex.EncodeToString(id), "bin", bin, "error", err)
			return 0, err
		}
		reserveSizeChange += rSizeChange
		item.Radius = bin
		if !evictBatch {
			if err := db.postageRadiusIndex.Put(item); err != nil {
				return 0, err
			}
		}
	}

	if !evictBatch {
		item.Radius = radius
		if err := db.postageRadiusIndex.Put(item); err != nil {
			return 0, err
		}
	}

	gcSize, err := db.gcSize.Get()
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return 0, err
	}

	// trigger garbage collection if we reached the capacity
	if gcSize >= db.cacheCapacity {
		db.triggerGarbageCollection()
	}

	return reserveSizeChange, nil
}

var unpinBatchSize = 10000

func (db *DB) unpinBatchChunks(id []byte, bin uint8) (uint64, error) {
	loggerV1 := db.logger.V(1).Register()
	var (
		batch                  = new(leveldb.Batch)
		gcSizeChange           int64 // number to add or subtract from gcSize and reserveSize
		reserveSizeChange      uint64
		totalReserveSizeChange uint64
	)
	unpin := func(item shed.Item) (stop bool, err error) {
		addr := swarm.NewAddress(item.Address)
		c, err := db.setUnpin(batch, addr)
		if err != nil {
			if !errors.Is(err, leveldb.ErrNotFound) {
				return false, fmt.Errorf("unpin: %w", err)
			}
			// this is possible when we are resyncing chain data after
			// a dirty shutdown
			loggerV1.Debug("unreserve set unpin chunk failed", "chunk", addr, "error", err)
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

	var startItem *shed.Item
	for {
		currentBatchSize := 0
		more := false
		err := db.postageChunksIndex.Iterate(func(item shed.Item) (bool, error) {
			if currentBatchSize > unpinBatchSize {
				startItem = &item
				more = true
				return true, nil
			}
			currentBatchSize++
			return unpin(item)
		}, &shed.IterateOptions{
			Prefix:    append(id, bin),
			StartFrom: startItem,
		})
		if err != nil {
			return 0, err
		}
		// adjust gcSize
		if gcSizeChange > 0 {
			if err := db.incGCSizeInBatch(batch, gcSizeChange); err != nil {
				return 0, err
			}
		}
		if err := db.shed.WriteBatch(batch); err != nil {
			return 0, err
		}
		batch = new(leveldb.Batch)
		gcSizeChange = 0
		totalReserveSizeChange += reserveSizeChange
		reserveSizeChange = 0

		if !more {
			break
		}
	}

	return totalReserveSizeChange, nil
}

func withinRadius(db *DB, item shed.Item) bool {
	po := db.po(swarm.NewAddress(item.Address))
	return po >= item.Radius
}

// ReserveCapacity returns the configured capacity
func (db *DB) ReserveCapacity() uint64 {
	return db.reserveCapacity
}

// ComputeReserveSize iterates on the pull index to count all chunks
// starting at some proximity order with an generated address whose PO
// is used as a starting prefix by the index.
func (db *DB) ComputeReserveSize(startPO uint8) (uint64, error) {

	var count uint64

	err := db.pullIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		count++
		return false, nil
	}, &shed.IterateOptions{
		StartFrom: &shed.Item{
			Address: db.addressInBin(startPO).Bytes(),
		},
	})
	if err == nil {
		err = db.setReserveSize(count)
		if err != nil {
			return 0, fmt.Errorf("failed setting reserve size: %w", err)
		}
		db.metrics.ReserveSize.Set(float64(count))
	}

	return count, err
}

// SetReserveSize will update the localstore reserve size as calculated by the
// depthmonitor using the updated storage depth
func (db *DB) setReserveSize(size uint64) error {
	err := db.reserveSize.Put(size)
	if err != nil {
		return fmt.Errorf("failed updating reserve size: %w", err)
	}
	if size > db.reserveCapacity {
		db.triggerReserveEviction()
	}
	return nil
}
