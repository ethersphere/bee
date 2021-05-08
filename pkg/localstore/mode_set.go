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
	"time"

	"github.com/dgraph-io/badger/v3"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
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
// It acquires lockAddr to protect two calls
// of this function for the same address in parallel.
func (db *DB) set(mode storage.ModeSet, addrs ...swarm.Address) (err error) {
	// protect parallel updates
	db.batchMu.Lock()
	defer db.batchMu.Unlock()
	if db.gcRunning {
		db.dirtyAddresses = append(db.dirtyAddresses, addrs...)
	}

	batch := db.shed.GetBatch(true)

	// variables that provide information for operations
	// to be done after write batch function successfully executes
	var gcSizeChange int64                      // number to add or subtract from gcSize
	triggerPullFeed := make(map[uint8]struct{}) // signal pull feed subscriptions to iterate

	switch mode {
	case storage.ModeSetSync:
		for _, addr := range addrs {
			c, err := db.setSync(batch, addr, mode)
			if err != nil {
				return err
			}
			gcSizeChange += c
		}

	case storage.ModeSetRemove:
		for _, addr := range addrs {
			c, err := db.setRemove(batch, addr)
			if err != nil {
				return err
			}
			gcSizeChange += c
		}

	case storage.ModeSetPin:
		for _, addr := range addrs {
			has, err := db.retrievalDataIndex.Has(addressToItem(addr))
			if err != nil {
				return err
			}

			if !has {
				return storage.ErrNotFound
			}

			err = db.setPin(batch, addr)
			if err != nil {
				return err
			}
		}
	case storage.ModeSetUnpin:
		for _, addr := range addrs {
			err := db.setUnpin(batch, addr)
			if err != nil {
				return err
			}
		}
	default:
		return ErrInvalidMode
	}

	err = db.incGCSizeInBatch(batch, gcSizeChange)
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

// setSync adds the chunk to the garbage collection after syncing by updating indexes
// - ModeSetSync - the corresponding tag is incremented, then item is removed
//   from push sync index
// - update to gc index happens given item does not exist in pin index
// Provided batch is updated.
func (db *DB) setSync(batch *badger.Txn, addr swarm.Address, mode storage.ModeSet) (gcSizeChange int64, err error) {
	item := addressToItem(addr)

	// need to get access timestamp here as it is not
	// provided by the access function, and it is not
	// a property of a chunk provided to Accessor.Put.

	i, err := db.retrievalDataIndex.Get(item)
	if err != nil {
		if errors.Is(err, shed.ErrNotFound) {
			// chunk is not found,
			// no need to update gc index
			// just delete from the push index
			// if it is there
			err = db.pushIndex.DeleteInBatch(batch, item)
			if err != nil {
				return 0, err
			}
			return 0, nil
		}
		return 0, err
	}
	item.StoreTimestamp = i.StoreTimestamp
	item.BinID = i.BinID

	i, err = db.pushIndex.Get(item)
	if err != nil {
		if errors.Is(err, shed.ErrNotFound) {
			// we handle this error internally, since this is an internal inconsistency of the indices
			// this error can happen if the chunk is put with ModePutRequest or ModePutSync
			// but this function is called with ModeSetSync
			db.logger.Debugf("localstore: chunk with address %s not found in push index", addr)
		} else {
			return 0, err
		}
	}
	if err == nil && db.tags != nil && i.Tag != 0 {
		t, err := db.tags.Get(i.Tag)
		if err != nil {
			// we cannot break or return here since the function needs to
			// run to end from db.pushIndex.DeleteInBatch
			db.logger.Errorf("localstore: get tags on push sync set uid %d: %v", i.Tag, err)
		} else {
			err = t.Inc(tags.StateSynced)
			if err != nil {
				return 0, err
			}
		}
	}

	err = db.pushIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}

	i, err = db.retrievalAccessIndex.Get(item)
	switch {
	case err == nil:
		item.AccessTimestamp = i.AccessTimestamp
		err = db.gcIndex.DeleteInBatch(batch, item)
		if err != nil {
			return 0, err
		}
		gcSizeChange--
	case errors.Is(err, shed.ErrNotFound):
		// the chunk is not accessed before
	default:
		return 0, err
	}
	item.AccessTimestamp = now()
	err = db.retrievalAccessIndex.PutInBatch(batch, item)
	if err != nil {
		return 0, err
	}

	// Add in gcIndex only if this chunk is not pinned
	ok, err := db.pinIndex.Has(item)
	if err != nil {
		return 0, err
	}
	if !ok {
		err = db.gcIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
		gcSizeChange++
	}

	return gcSizeChange, nil
}

// setRemove removes the chunk by updating indexes:
//  - delete from retrieve, pull, gc
// Provided batch is updated.
func (db *DB) setRemove(batch *badger.Txn, addr swarm.Address) (gcSizeChange int64, err error) {
	item := addressToItem(addr)

	// need to get access timestamp here as it is not
	// provided by the access function, and it is not
	// a property of a chunk provided to Accessor.Put.
	i, err := db.retrievalAccessIndex.Get(item)
	switch {
	case err == nil:
		item.AccessTimestamp = i.AccessTimestamp
	case errors.Is(err, shed.ErrNotFound):
	default:
		return 0, err
	}
	i, err = db.retrievalDataIndex.Get(item)
	if err != nil {
		return 0, err
	}
	item.StoreTimestamp = i.StoreTimestamp
	item.BinID = i.BinID

	err = db.retrievalDataIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}
	err = db.retrievalAccessIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}
	err = db.pullIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}
	err = db.gcIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}
	// a check is needed for decrementing gcSize
	// as delete is not reporting if the key/value pair
	// is deleted or not
	if _, err := db.gcIndex.Get(item); err == nil {
		gcSizeChange = -1
	}

	return gcSizeChange, nil
}

// setPin increments pin counter for the chunk by updating
// pin index and sets the chunk to be excluded from garbage collection.
// Provided batch is updated.
func (db *DB) setPin(batch *badger.Txn, addr swarm.Address) (err error) {
	item := addressToItem(addr)

	// Get the existing pin counter of the chunk
	existingPinCounter := uint64(0)
	pinnedChunk, err := db.pinIndex.Get(item)
	if err != nil {
		if errors.Is(err, shed.ErrNotFound) {
			// If this Address is not present in DB, then its a new entry
			existingPinCounter = 0

			// Add in gcExcludeIndex of the chunk is not pinned already
			err = db.gcExcludeIndex.PutInBatch(batch, item)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		existingPinCounter = pinnedChunk.PinCounter
	}

	// Otherwise increase the existing counter by 1
	item.PinCounter = existingPinCounter + 1
	err = db.pinIndex.PutInBatch(batch, item)
	if err != nil {
		return err
	}

	return nil
}

// setUnpin decrements pin counter for the chunk by updating pin index.
// Provided batch is updated.
func (db *DB) setUnpin(batch *badger.Txn, addr swarm.Address) (err error) {
	item := addressToItem(addr)

	// Get the existing pin counter of the chunk
	pinnedChunk, err := db.pinIndex.Get(item)
	if err != nil {
		return err
	}

	// Decrement the pin counter or
	// delete it from pin index if the pin counter has reached 0
	if pinnedChunk.PinCounter > 1 {
		item.PinCounter = pinnedChunk.PinCounter - 1
		err = db.pinIndex.PutInBatch(batch, item)
		if err != nil {
			return err
		}
	} else {
		err = db.pinIndex.DeleteInBatch(batch, item)
		if err != nil {
			return err
		}
	}

	return nil
}
