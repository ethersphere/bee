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
	"encoding/binary"
	"errors"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

var (
	ErrOverwrite = errors.New("index already exists - double issuance on immutable batch")
)

// Put stores Chunks to database and depending
// on the Putter mode, it updates required indexes.
// Put is required to implement storage.Store
// interface.
func (db *DB) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {

	db.metrics.ModePut.Inc()
	defer totalTimeMetric(db.metrics.TotalTimePut, time.Now())

	exist, err = db.put(mode, chs...)
	if err != nil {
		db.metrics.ModePutFailure.Inc()
	}

	return exist, err
}

// put stores Chunks to database and updates other indexes. It acquires batchMu
// to protect two calls of this function for the same address in parallel. Item
// fields Address and Data must not be with their nil values. If chunks with the
// same address are passed in arguments, only the first chunk will be stored,
// and following ones will have exist set to true for their index in exist
// slice. This is the same behaviour as if the same chunks are passed one by one
// in multiple put method calls.
func (db *DB) put(mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	// this is an optimization that tries to optimize on already existing chunks
	// not needing to acquire batchMu. This is in order to reduce lock contention
	// when chunks are retried across the network for whatever reason.
	if len(chs) == 1 && mode != storage.ModePutRequestPin && mode != storage.ModePutUploadPin {
		has, err := db.retrievalDataIndex.Has(chunkToItem(chs[0]))
		if err != nil {
			return nil, err
		}
		if has {
			return []bool{true}, nil
		}
	}

	// protect parallel updates
	db.batchMu.Lock()
	defer db.batchMu.Unlock()
	if db.gcRunning {
		for _, ch := range chs {
			db.dirtyAddresses = append(db.dirtyAddresses, ch.Address())
		}
	}

	batch := new(leveldb.Batch)

	// variables that provide information for operations
	// to be done after write batch function successfully executes
	var gcSizeChange int64                      // number to add or subtract from gcSize
	var triggerPushFeed bool                    // signal push feed subscriptions to iterate
	triggerPullFeed := make(map[uint8]struct{}) // signal pull feed subscriptions to iterate

	exist = make([]bool, len(chs))

	// A lazy populated map of bin ids to properly set
	// BinID values for new chunks based on initial value from database
	// and incrementing them.
	// Values from this map are stored with the batch
	binIDs := make(map[uint8]uint64)

	switch mode {
	case storage.ModePutRequest, storage.ModePutRequestPin, storage.ModePutRequestCache:
		for i, ch := range chs {
			if containsChunk(ch.Address(), chs[:i]...) {
				exist[i] = true
				continue
			}
			item := chunkToItem(ch)
			pin := mode == storage.ModePutRequestPin     // force pin in this mode
			cache := mode == storage.ModePutRequestCache // force cache
			exists, c, err := db.putRequest(batch, binIDs, item, pin, cache)
			if err != nil {
				return nil, err
			}
			exist[i] = exists
			gcSizeChange += c
		}

	case storage.ModePutUpload, storage.ModePutUploadPin:
		for i, ch := range chs {
			if containsChunk(ch.Address(), chs[:i]...) {
				exist[i] = true
				continue
			}
			item := chunkToItem(ch)
			exists, c, err := db.putUpload(batch, binIDs, item)
			if err != nil {
				return nil, err
			}
			exist[i] = exists
			if !exists {
				// chunk is new so, trigger subscription feeds
				// after the batch is successfully written
				triggerPullFeed[db.po(ch.Address())] = struct{}{}
				triggerPushFeed = true
			}
			gcSizeChange += c
			if mode == storage.ModePutUploadPin {
				c, err = db.setPin(batch, item)
				if err != nil {
					return nil, err
				}
			}
			gcSizeChange += c
		}

	case storage.ModePutSync:
		for i, ch := range chs {
			if containsChunk(ch.Address(), chs[:i]...) {
				exist[i] = true
				continue
			}
			exists, c, err := db.putSync(batch, binIDs, chunkToItem(ch))
			if err != nil {
				return nil, err
			}
			exist[i] = exists
			if !exists {
				// chunk is new so, trigger pull subscription feed
				// after the batch is successfully written
				triggerPullFeed[db.po(ch.Address())] = struct{}{}
			}
			gcSizeChange += c
		}

	default:
		return nil, ErrInvalidMode
	}

	for po, id := range binIDs {
		db.binIDs.PutInBatch(batch, uint64(po), id)
	}

	err = db.incGCSizeInBatch(batch, gcSizeChange)
	if err != nil {
		return nil, err
	}

	err = db.shed.WriteBatch(batch)
	if err != nil {
		return nil, err
	}

	for po := range triggerPullFeed {
		db.triggerPullSubscriptions(po)
	}
	if triggerPushFeed {
		db.triggerPushSubscriptions()
	}
	return exist, nil
}

// putRequest adds an Item to the batch by updating required indexes:
//  - put to indexes: retrieve, gc
//  - it does not enter the syncpool
// The batch can be written to the database.
// Provided batch and binID map are updated.
func (db *DB) putRequest(batch *leveldb.Batch, binIDs map[uint8]uint64, item shed.Item, forcePin, forceCache bool) (exists bool, gcSizeChange int64, err error) {
	exists, err = db.retrievalDataIndex.Has(item)
	if err != nil {
		return false, 0, err
	}
	if exists {
		return true, 0, nil
	}

	previous, err := db.postageIndexIndex.Get(item)
	if err != nil {
		if !errors.Is(err, leveldb.ErrNotFound) {
			return false, 0, err
		}
	} else {
		if item.Immutable {
			return false, 0, ErrOverwrite
		}
		// if a chunk is found with the same postage stamp index,
		// replace it with the new one only if timestamp is later
		if !later(previous, item) {
			return false, 0, err
		}
		gcSizeChange, err = db.setRemove(batch, previous, true)
		if err != nil {
			return false, 0, err
		}
	}

	item.StoreTimestamp = now()
	item.BinID, err = db.incBinID(binIDs, db.po(swarm.NewAddress(item.Address)))
	if err != nil {
		return false, 0, err
	}
	err = db.retrievalDataIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}
	err = db.postageChunksIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}
	err = db.postageIndexIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}
	item.AccessTimestamp = now()
	err = db.retrievalAccessIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}

	gcSizeChangeNew, err := db.preserveOrCache(batch, item, forcePin, forceCache)
	if err != nil {
		return false, 0, err
	}

	if !forceCache {
		// if we are here it means the chunk has a valid stamp
		// therefore we'd like to be able to pullsync it
		err = db.pullIndex.PutInBatch(batch, item)
		if err != nil {
			return false, 0, err
		}
	}

	return false, gcSizeChange + gcSizeChangeNew, nil
}

// putUpload adds an Item to the batch by updating required indexes:
//  - put to indexes: retrieve, push, pull
// The batch can be written to the database.
// Provided batch and binID map are updated.
func (db *DB) putUpload(batch *leveldb.Batch, binIDs map[uint8]uint64, item shed.Item) (exists bool, gcSizeChange int64, err error) {
	exists, err = db.retrievalDataIndex.Has(item)
	if err != nil {
		return false, 0, err
	}
	if exists {
		return true, 0, nil
	}

	item.StoreTimestamp = now()
	item.BinID, err = db.incBinID(binIDs, db.po(swarm.NewAddress(item.Address)))
	if err != nil {
		return false, 0, err
	}
	err = db.retrievalDataIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}
	err = db.pullIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}
	err = db.pushIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}

	err = db.postageChunksIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}
	return false, 0, nil
}

// putSync adds an Item to the batch by updating required indexes:
//  - put to indexes: retrieve, pull, gc
// The batch can be written to the database.
// Provided batch and binID map are updated.
func (db *DB) putSync(batch *leveldb.Batch, binIDs map[uint8]uint64, item shed.Item) (exists bool, gcSizeChange int64, err error) {
	exists, err = db.retrievalDataIndex.Has(item)
	if err != nil {
		return false, 0, err
	}
	if exists {
		return true, 0, nil
	}
	previous, err := db.postageIndexIndex.Get(item)
	if err != nil {
		if !errors.Is(err, leveldb.ErrNotFound) {
			return false, 0, err
		}
	} else {
		// if a chunk is found with the same postage stamp, replace it with the new one
		gcSizeChange, err = db.setRemove(batch, previous, true)
		if err != nil {
			return false, 0, err
		}
	}

	item.StoreTimestamp = now()
	item.BinID, err = db.incBinID(binIDs, db.po(swarm.NewAddress(item.Address)))
	if err != nil {
		return false, 0, err
	}
	err = db.retrievalDataIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}
	err = db.pullIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}
	err = db.postageChunksIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}
	err = db.postageIndexIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}
	item.AccessTimestamp = now()
	err = db.retrievalAccessIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, err
	}

	gcSizeChangeNew, err := db.preserveOrCache(batch, item, false, false)
	if err != nil {
		return false, 0, err
	}

	return false, gcSizeChange + gcSizeChangeNew, nil
}

// preserveOrCache is a helper function used to add chunks to either a pinned reserve or gc cache
// (the retrieval access index and the gc index)
func (db *DB) preserveOrCache(batch *leveldb.Batch, item shed.Item, forcePin, forceCache bool) (gcSizeChange int64, err error) {
	// item needs to be populated with Radius
	item2, err := db.postageRadiusIndex.Get(item)
	if err != nil {
		// if there's an error, assume the chunk needs to be GCd
		forceCache = true
	} else {
		item.Radius = item2.Radius
	}
	if !forceCache && (withinRadiusFn(db, item) || forcePin) {
		return db.setPin(batch, item)
	}

	// add new entry to gc index ONLY if it is not present in pinIndex
	ok, err := db.pinIndex.Has(item)
	if err != nil {
		return 0, err
	}
	if ok {
		return gcSizeChange, nil
	}
	exists, err := db.gcIndex.Has(item)
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return 0, err
	}
	if exists {
		return 0, nil
	}
	err = db.gcIndex.PutInBatch(batch, item)
	if err != nil {
		return 0, err
	}
	gcSizeChange++

	return gcSizeChange, nil
}

// incBinID is a helper function for db.put* methods that increments bin id
// based on the current value in the database. This function must be called under
// a db.batchMu lock. Provided binID map is updated.
func (db *DB) incBinID(binIDs map[uint8]uint64, po uint8) (id uint64, err error) {
	if _, ok := binIDs[po]; !ok {
		binIDs[po], err = db.binIDs.Get(uint64(po))
		if err != nil {
			return 0, err
		}
	}
	binIDs[po]++
	return binIDs[po], nil
}

// containsChunk returns true if the chunk with a specific address
// is present in the provided chunk slice.
func containsChunk(addr swarm.Address, chs ...swarm.Chunk) bool {
	for _, c := range chs {
		if addr.Equal(c.Address()) {
			return true
		}
	}
	return false
}

func later(previous, current shed.Item) bool {
	pts := binary.BigEndian.Uint64(previous.Timestamp)
	cts := binary.BigEndian.Uint64(current.Timestamp)
	return cts > pts
}
