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

	"github.com/ethersphere/bee/pkg/sharky"
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

	exist, err = db.put(ctx, mode, chs...)
	if err != nil {
		db.metrics.ModePutFailure.Inc()
	}

	return exist, err
}

type releaseLocations []sharky.Location

func (r *releaseLocations) add(loc sharky.Location) {
	*r = append(*r, loc)
}

// put stores Chunks to database and updates other indexes. It acquires batchMu
// to protect two calls of this function for the same address in parallel. Item
// fields Address and Data must not be with their nil values. If chunks with the
// same address are passed in arguments, only the first chunk will be stored,
// and following ones will have exist set to true for their index in exist
// slice. This is the same behaviour as if the same chunks are passed one by one
// in multiple put method calls.
func (db *DB) put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, retErr error) {
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

	var (
		releaseLocs        = new(releaseLocations)
		committedLocations []sharky.Location
	)

	putChunk := func(ch swarm.Chunk, index int, putOp func(shed.Item) (bool, int64, error)) (bool, int64, error) {
		if containsChunk(ch.Address(), chs[:index]...) {
			return true, 0, nil
		}
		item := chunkToItem(ch)
		loc, exists, err := db.putSharky(ctx, item)
		if err != nil {
			return false, 0, err
		}
		if exists {
			return true, 0, nil
		}
		committedLocations = append(committedLocations, loc)
		item.Location, err = loc.MarshalBinary()
		if err != nil {
			return false, 0, err
		}
		return putOp(item)
	}

	// If for whatever reason we fail to commit the batch, we should release all
	// the chunks that have been committed to sharky to free them up for the incoming
	// chunks
	defer func() {
		if retErr != nil {
			for _, l := range committedLocations {
				err := db.sharky.Release(ctx, l)
				if err != nil {
					db.logger.Warning("failed releasing sharky location", err)
				}
			}
		}
	}()

	switch mode {
	case storage.ModePutRequest, storage.ModePutRequestPin, storage.ModePutRequestCache:
		for i, ch := range chs {
			pin := mode == storage.ModePutRequestPin     // force pin in this mode
			cache := mode == storage.ModePutRequestCache // force cache
			exists, c, err := putChunk(ch, i, func(item shed.Item) (bool, int64, error) {
				return db.putRequest(ctx, releaseLocs, batch, binIDs, item, pin, cache)
			})
			if err != nil {
				return nil, err
			}
			exist[i] = exists
			gcSizeChange += c
		}

	case storage.ModePutUpload, storage.ModePutUploadPin:
		for i, ch := range chs {
			exists, c, err := putChunk(ch, i, func(item shed.Item) (bool, int64, error) {
				chExists, gcChange, err := db.putUpload(batch, releaseLocs, binIDs, item)
				if err == nil && mode == storage.ModePutUploadPin {
					c2, err := db.setPin(batch, item)
					if err == nil {
						gcChange += c2
					}
				}
				return chExists, gcChange, err
			})
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

	case storage.ModePutSync:
		for i, ch := range chs {
			exists, c, err := putChunk(ch, i, func(item shed.Item) (bool, int64, error) {
				return db.putSync(batch, releaseLocs, binIDs, item)
			})
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

	err := db.incGCSizeInBatch(batch, gcSizeChange)
	if err != nil {
		return nil, err
	}

	err = db.shed.WriteBatch(batch)
	if err != nil {
		return nil, err
	}

	for _, v := range *releaseLocs {
		err = db.sharky.Release(ctx, v)
		if err != nil {
			db.logger.Warning("failed releasing sharky location", v)
		}
	}

	for po := range triggerPullFeed {
		db.triggerPullSubscriptions(po)
	}
	if triggerPushFeed {
		db.triggerPushSubscriptions()
	}
	return exist, nil
}

// putSharky will add the item to sharky storage if it doesnt exist
func (db *DB) putSharky(ctx context.Context, item shed.Item) (loc sharky.Location, exists bool, err error) {
	exists, err = db.retrievalDataIndex.Has(item)
	if err != nil {
		return loc, false, err
	}
	if exists {
		return loc, true, nil
	}
	l, err := db.sharky.Write(ctx, item.Data)
	if err != nil {
		return loc, false, err
	}
	return l, false, nil
}

// putRequest adds an Item to the batch by updating required indexes:
//  - put to indexes: retrieve, gc
//  - it does not enter the syncpool
// The batch can be written to the database.
// Provided batch and binID map are updated.
func (db *DB) putRequest(ctx context.Context, loc *releaseLocations, batch *leveldb.Batch, binIDs map[uint8]uint64, item shed.Item, forcePin, forceCache bool) (exists bool, gcSizeChange int64, err error) {
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
			return false, 0, nil
		}
		gcSizeChange, err = db.setRemove(batch, previous, true)
		if err != nil {
			return false, 0, err
		}

		l, err := sharky.LocationFromBinary(previous.Location)
		if err != nil {
			return false, 0, err
		}
		loc.add(l)

		radius, err := db.postageRadiusIndex.Get(item)
		if err != nil {
			if !errors.Is(err, leveldb.ErrNotFound) {
				return false, 0, err
			}
		} else {
			if db.po(swarm.NewAddress(item.Address)) >= radius.Radius {
				if err := db.incReserveSizeInBatch(batch, -1); err != nil {
					return false, 0, err
				}
			}
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
func (db *DB) putUpload(batch *leveldb.Batch, loc *releaseLocations, binIDs map[uint8]uint64, item shed.Item) (exists bool, gcSizeChange int64, err error) {
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
			return false, 0, nil
		}
		_, err = db.setRemove(batch, previous, true)
		if err != nil {
			return false, 0, err
		}

		l, err := sharky.LocationFromBinary(previous.Location)
		if err != nil {
			return false, 0, err
		}

		loc.add(l)
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
	err = db.postageIndexIndex.PutInBatch(batch, item)
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
func (db *DB) putSync(batch *leveldb.Batch, loc *releaseLocations, binIDs map[uint8]uint64, item shed.Item) (exists bool, gcSizeChange int64, err error) {
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
			return false, 0, nil
		}
		_, err = db.setRemove(batch, previous, true)
		if err != nil {
			return false, 0, err
		}

		l, err := sharky.LocationFromBinary(previous.Location)
		if err != nil {
			return false, 0, err
		}

		loc.add(l)

		radius, err := db.postageRadiusIndex.Get(item)
		if err != nil {
			if !errors.Is(err, leveldb.ErrNotFound) {
				return false, 0, err
			}
		} else {
			if db.po(swarm.NewAddress(item.Address)) >= radius.Radius {
				if err := db.incReserveSizeInBatch(batch, -1); err != nil {
					return false, 0, err
				}
			}
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
	if !forceCache && (withinRadiusFn(db, item) || forcePin) {
		if !forcePin {
			if err := db.incReserveSizeInBatch(batch, 1); err != nil {
				return 0, err
			}
		}
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
