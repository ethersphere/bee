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
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethersphere/swarm/chunk"
	"github.com/ethersphere/swarm/shed"
	"github.com/syndtr/goleveldb/leveldb"
)

// Put stores Chunks to database and depending
// on the Putter mode, it updates required indexes.
// Put is required to implement chunk.Store
// interface.
func (db *DB) Put(ctx context.Context, mode chunk.ModePut, chs ...chunk.Chunk) (exist []bool, err error) {
	metricName := fmt.Sprintf("localstore/Put/%s", mode)

	metrics.GetOrRegisterCounter(metricName, nil).Inc(1)
	defer totalTimeMetric(metricName, time.Now())

	exist, err = db.put(mode, chs...)
	if err != nil {
		metrics.GetOrRegisterCounter(metricName+"/error", nil).Inc(1)
	}

	return exist, err
}

// put stores Chunks to database and updates other indexes. It acquires lockAddr
// to protect two calls of this function for the same address in parallel. Item
// fields Address and Data must not be with their nil values. If chunks with the
// same address are passed in arguments, only the first chunk will be stored,
// and following ones will have exist set to true for their index in exist
// slice. This is the same behaviour as if the same chunks are passed one by one
// in multiple put method calls.
func (db *DB) put(mode chunk.ModePut, chs ...chunk.Chunk) (exist []bool, err error) {
	// protect parallel updates
	db.batchMu.Lock()
	defer db.batchMu.Unlock()

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
	case chunk.ModePutRequest:
		for i, ch := range chs {
			if containsChunk(ch.Address(), chs[:i]...) {
				exist[i] = true
				continue
			}
			exists, c, err := db.putRequest(batch, binIDs, chunkToItem(ch))
			if err != nil {
				return nil, err
			}
			exist[i] = exists
			gcSizeChange += c
		}

	case chunk.ModePutUpload:
		for i, ch := range chs {
			if containsChunk(ch.Address(), chs[:i]...) {
				exist[i] = true
				continue
			}
			exists, c, err := db.putUpload(batch, binIDs, chunkToItem(ch))
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
		}

	case chunk.ModePutSync:
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
func (db *DB) putRequest(batch *leveldb.Batch, binIDs map[uint8]uint64, item shed.Item) (exists bool, gcSizeChange int64, err error) {
	i, err := db.retrievalDataIndex.Get(item)
	switch err {
	case nil:
		exists = true
		item.StoreTimestamp = i.StoreTimestamp
		item.BinID = i.BinID
	case leveldb.ErrNotFound:
		// no chunk accesses
		exists = false
	default:
		return false, 0, err
	}
	if item.StoreTimestamp == 0 {
		item.StoreTimestamp = now()
	}
	if item.BinID == 0 {
		item.BinID, err = db.incBinID(binIDs, db.po(item.Address))
		if err != nil {
			return false, 0, err
		}
	}

	gcSizeChange, err = db.setGC(batch, item)
	if err != nil {
		return false, 0, err
	}

	db.retrievalDataIndex.PutInBatch(batch, item)

	return exists, gcSizeChange, nil
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
		if db.putToGCCheck(item.Address) {
			gcSizeChange, err = db.setGC(batch, item)
			if err != nil {
				return false, 0, err
			}
		}

		return true, 0, nil
	}
	anonymous := false
	if db.tags != nil && item.Tag != 0 {
		tag, err := db.tags.Get(item.Tag)
		if err != nil {
			return false, 0, err
		}
		anonymous = tag.Anonymous
	}

	item.StoreTimestamp = now()
	item.BinID, err = db.incBinID(binIDs, db.po(item.Address))
	if err != nil {
		return false, 0, err
	}
	db.retrievalDataIndex.PutInBatch(batch, item)
	db.pullIndex.PutInBatch(batch, item)
	if !anonymous {
		db.pushIndex.PutInBatch(batch, item)
	}

	if db.putToGCCheck(item.Address) {

		// TODO: this might result in an edge case where a node
		// that has very little storage and uploads using an anonymous
		// upload will have some of the content GCd before being able
		// to sync it
		gcSizeChange, err = db.setGC(batch, item)
		if err != nil {
			return false, 0, err
		}
	}

	return false, gcSizeChange, nil
}

// putSync adds an Item to the batch by updating required indexes:
//  - put to indexes: retrieve, pull
// The batch can be written to the database.
// Provided batch and binID map are updated.
func (db *DB) putSync(batch *leveldb.Batch, binIDs map[uint8]uint64, item shed.Item) (exists bool, gcSizeChange int64, err error) {
	exists, err = db.retrievalDataIndex.Has(item)
	if err != nil {
		return false, 0, err
	}
	if exists {
		if db.putToGCCheck(item.Address) {
			gcSizeChange, err = db.setGC(batch, item)
			if err != nil {
				return false, 0, err
			}
		}

		return true, gcSizeChange, nil
	}

	item.StoreTimestamp = now()
	item.BinID, err = db.incBinID(binIDs, db.po(item.Address))
	if err != nil {
		return false, 0, err
	}
	db.retrievalDataIndex.PutInBatch(batch, item)
	db.pullIndex.PutInBatch(batch, item)

	if db.putToGCCheck(item.Address) {
		// TODO: this might result in an edge case where a node
		// that has very little storage and uploads using an anonymous
		// upload will have some of the content GCd before being able
		// to sync it
		gcSizeChange, err = db.setGC(batch, item)
		if err != nil {
			return false, 0, err
		}
	}

	return false, gcSizeChange, nil
}

// setGC is a helper function used to add chunks to the retrieval access
// index and the gc index in the cases that the putToGCCheck condition
// warrants a gc set. this is to mitigate index leakage in edge cases where
// a chunk is added to a node's localstore and given that the chunk is
// already within that node's NN (thus, it can be added to the gc index
// safely)
func (db *DB) setGC(batch *leveldb.Batch, item shed.Item) (gcSizeChange int64, err error) {
	if item.BinID == 0 {
		i, err := db.retrievalDataIndex.Get(item)
		if err != nil {
			return 0, err
		}
		item.BinID = i.BinID
	}
	i, err := db.retrievalAccessIndex.Get(item)
	switch err {
	case nil:
		item.AccessTimestamp = i.AccessTimestamp
		db.gcIndex.DeleteInBatch(batch, item)
		gcSizeChange--
	case leveldb.ErrNotFound:
		// the chunk is not accessed before
	default:
		return 0, err
	}
	item.AccessTimestamp = now()
	db.retrievalAccessIndex.PutInBatch(batch, item)

	db.gcIndex.PutInBatch(batch, item)
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
func containsChunk(addr chunk.Address, chs ...chunk.Chunk) bool {
	for _, c := range chs {
		if bytes.Equal(addr, c.Address()) {
			return true
		}
	}
	return false
}
