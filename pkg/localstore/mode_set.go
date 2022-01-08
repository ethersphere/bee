// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package localstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

// Set updates database indexes for
// chunks represented by provided addresses.
// Set is required to implement chunk.Store
// interface.
func (db *DB) Set(ctx context.Context, mode storage.ModeSet, addrs ...swarm.Address) (err error) {
	db.metrics.ModeSet.Inc()
	defer totalTimeMetric(db.metrics.TotalTimeSet, time.Now())
	err = db.set(mode, addrs...)
	if err != nil {
		db.metrics.ModeSetFailure.Inc()
	}
	return err
}

// set updates database indexes for
// chunks represented by provided addresses.
func (db *DB) set(mode storage.ModeSet, addrs ...swarm.Address) (err error) {
	// protect parallel updates
	db.batchMu.Lock()
	defer db.batchMu.Unlock()
	if db.gcRunning {
		db.dirtyAddresses = append(db.dirtyAddresses, addrs...)
	}

	batch := new(leveldb.Batch)

	// variables that provide information for operations
	// to be done after write batch function successfully executes
	var (
		gcSizeChange      int64 // number to add or subtract from gcSize
		reserveSizeChange int64 // number of items to add or subtract from reserveSize
	)
	triggerPullFeed := make(map[uint8]struct{}) // signal pull feed subscriptions to iterate

	switch mode {

	case storage.ModeSetSync:
	case storage.ModeSetRemove:
		for _, addr := range addrs {
			item := addressToItem(addr)
			c, err := db.setRemove(batch, item, true)
			if err != nil {
				return err
			}
			gcSizeChange += c
		}

	case storage.ModeSetPin:
	case storage.ModeSetUnpin:
	default:
		return ErrInvalidMode
	}

	err = db.incGCSizeInBatch(batch, gcSizeChange)
	if err != nil {
		return err
	}

	err = db.incReserveSizeInBatch(batch, reserveSizeChange)
	if err != nil {
		return err
	}

	err = db.shed.WriteBatch(batch)
	if err != nil {
		return err
	}
	for po := range triggerPullFeed {
		db.triggerPullSubscriptions(po)
	}
	return nil
}

// setRemove removes the chunk by updating indexes:
//  - delete from retrieve, pull, gc
// Provided batch is updated.
func (db *DB) setRemove(batch *leveldb.Batch, item shed.Item, check bool) (gcSizeChange int64, err error) {
	if item.AccessTimestamp == 0 {
		i, err := db.retrievalAccessIndex.Get(item)
		switch {
		case err == nil:
			item.AccessTimestamp = i.AccessTimestamp
		case errors.Is(err, leveldb.ErrNotFound):
		default:
			return 0, err
		}
	}
	if item.StoreTimestamp == 0 {
		item, err = db.retrievalDataIndex.Get(item)
		if err != nil {
			return 0, fmt.Errorf("retrieval data index: %w", err)
		}
	}

	db.metrics.GCStoreTimeStamps.Set(float64(item.StoreTimestamp))
	db.metrics.GCStoreAccessTimeStamps.Set(float64(item.AccessTimestamp))

	err = db.retrievalDataIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}
	err = db.retrievalAccessIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}
	err = db.pushIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}
	err = db.pullIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}
	err = db.postageChunksIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}

	// unless called by GC which iterates through the gcIndex
	// a check is needed for decrementing gcSize
	// as delete is not reporting if the key/value pair is deleted or not
	if check {
		_, err := db.gcIndex.Get(item)
		if err != nil {
			if !errors.Is(err, leveldb.ErrNotFound) {
				return 0, fmt.Errorf("gc index get: %w", err)
			}
			return 0, db.pinIndex.DeleteInBatch(batch, item)
		}
	}
	err = db.gcIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}
	return -1, nil
}

func (db *DB) setGc(batch *leveldb.Batch, addr swarm.Address) (gcSizeChange int64, err error) {
	item := addressToItem(addr)

	i, err := db.retrievalDataIndex.Get(item)
	if err != nil {
		return 0, err
	}
	item.StoreTimestamp = i.StoreTimestamp
	item.BinID = i.BinID
	item.BatchID = i.BatchID

	i, err = db.retrievalAccessIndex.Get(item)
	if err != nil {
		if !errors.Is(err, leveldb.ErrNotFound) {
			return 0, err
		}
		item.AccessTimestamp = now()
		err = db.retrievalAccessIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
	} else {
		item.AccessTimestamp = i.AccessTimestamp
	}
	err = db.gcIndex.PutInBatch(batch, item)
	if err != nil {
		return 0, err
	}

	return 1, nil
}
