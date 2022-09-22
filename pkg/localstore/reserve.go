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
		batch             = new(leveldb.Batch)
		oldRadius         uint8
		reserveSizeChange uint64
	)

	i, err := db.postageRadiusIndex.Get(item)
	if err != nil {
		if !errors.Is(err, leveldb.ErrNotFound) {
			return 0, err
		}
		oldRadius = 0
	} else {
		oldRadius = i.Radius
	}

	// iterate over chunk in bins
	for bin := oldRadius; bin < radius; bin++ {
		rSizeChange, err := db.unpinBatchChunks(id, bin)
		if err != nil {
			return 0, err
		}
		reserveSizeChange += rSizeChange
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
		if reserveSizeChange > 0 {
			if err := db.incReserveSizeInBatch(batch, -int64(reserveSizeChange)); err != nil {
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
			Address: generateAddressAt(db.baseKey, int(startPO)),
		},
	})

	return count, err
}

func generateAddressAt(baseBytes []byte, prox int) []byte {

	addr := make([]byte, 32)

	for po := 0; po < prox; po++ {
		index := po % 8
		if baseBytes[po/8]&(1<<(7-index)) > 0 { // if baseBytes bit is 1
			addr[po/8] |= 1 << (7 - index) // set addr bit to 1
		}
	}

	if baseBytes[prox/8]&(1<<(7-(prox%8))) == 0 { // if baseBytes PO bit is zero
		addr[prox/8] |= 1 << (7 - (prox % 8)) // set addr bit to 1
	}

	return addr
}
