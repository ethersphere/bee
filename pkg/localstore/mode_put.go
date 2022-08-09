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
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

var (
	ErrOverwriteImmutable = errors.New("index already exists - double issuance on immutable batch")
	ErrOverwrite          = errors.New("index already exists with newer timestamp - double issuance on batch")
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
	// protect parallel updates
	db.lock.Lock(lockKeyGC)
	if db.gcRunning {
		for _, ch := range chs {
			db.dirtyAddresses = append(db.dirtyAddresses, ch.Address())
		}
	}
	db.lock.Unlock(lockKeyGC)

	batch := new(leveldb.Batch)

	// variables that provide information for operations
	// to be done after write batch function successfully executes
	var (
		gcSizeChange int64 // number to add or subtract from gcSize
	)
	var triggerPushFeed bool                    // signal push feed subscriptions to iterate
	triggerPullFeed := make(map[uint8]struct{}) // signal pull feed subscriptions to iterate

	exist = make([]bool, len(chs))

	// A lazy populated map of bin ids to properly set
	// BinID values for new chunks based on initial value from database
	// and incrementing them.
	// Values from this map are stored with the batch
	binIDs := make(map[uint8]uint64)

	var (
		// this is the list of locations that need to be released if the batch is
		// successfully committed due to postageIndex collisions
		releaseLocs = new(releaseLocations)
		// this is the list of locations that need to be released if the batch is NOT
		// successfully committed as they have already been committed to sharky
		committedLocations []sharky.Location
	)

	putChunk := func(ch swarm.Chunk, index int, putOp func(shed.Item, bool) (int64, error)) (bool, int64, error) {
		if swarm.ContainsChunkWithAddress(chs[:index], ch.Address()) {
			return true, 0, nil
		}
		item := chunkToItem(ch)

		storedItem, err := db.retrievalDataIndex.Get(item)
		if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
			return false, 0, fmt.Errorf("failed reading retrievalIndex: %w", err)
		}
		if errors.Is(err, leveldb.ErrNotFound) {
			// This is a new chunk so add to sharky. Also check for double issuance.
			gcChange, err := db.checkAndRemoveStampIndex(item, batch, releaseLocs)
			if err != nil {
				if errors.Is(err, ErrOverwrite) && mode == storage.ModePutSync {
					// if the chunk is overwriting a newer valid chunk for the
					// same postage index, ignore it and dont return error so that
					// syncing can continue
					return false, 0, nil
				}
				return false, 0, err
			}
			l, err := db.sharky.Write(ctx, item.Data)
			if err != nil {
				return false, 0, fmt.Errorf("failed writing to sharky: %w", err)
			}
			committedLocations = append(committedLocations, l)
			item.Location, err = l.MarshalBinary()
			if err != nil {
				return false, 0, fmt.Errorf("failed serializing sharky location: %w", err)
			}

			gcChangeNew, err := putOp(item, false)
			return false, gcChangeNew + gcChange, err
		}

		// if access index is present, fill it as it is required for GC operations
		accessIdx, err := db.retrievalAccessIndex.Get(storedItem)
		if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
			return false, 0, err
		}
		storedItem.AccessTimestamp = accessIdx.AccessTimestamp

		gcChange, err := putOp(storedItem, true)
		if err != nil {
			return false, 0, err
		}
		return true, gcChange, nil
	}

	// If for whatever reason we fail to commit the batch, we should release all
	// the chunks that have been committed to sharky
	defer func() {
		if retErr != nil {
			for _, l := range committedLocations {
				db.sharky.Release(l)
			}
		}
	}()

	switch mode {
	case storage.ModePutRequest, storage.ModePutRequestPin, storage.ModePutRequestCache:
		db.lock.Lock(lockKeyGC)
		defer db.lock.Unlock(lockKeyGC)

		for i, ch := range chs {
			pin := mode == storage.ModePutRequestPin     // force pin in this mode
			cache := mode == storage.ModePutRequestCache // force cache
			exists, c, err := putChunk(ch, i, func(item shed.Item, exists bool) (int64, error) {
				return db.putRequest(ctx, batch, binIDs, item, pin, cache, exists)
			})
			if err != nil {
				return nil, fmt.Errorf("put request: %w", err)
			}
			exist[i] = exists
			gcSizeChange += c
		}

	case storage.ModePutUpload, storage.ModePutUploadPin:
		db.lock.Lock(lockKeyUpload)
		defer db.lock.Unlock(lockKeyUpload)

		for i, ch := range chs {
			pin := mode == storage.ModePutUploadPin
			exists, c, err := putChunk(ch, i, func(item shed.Item, exists bool) (int64, error) {
				return db.putUpload(batch, binIDs, item, pin, exists)
			})
			if err != nil {
				return nil, fmt.Errorf("put upload: %w", err)
			}
			exist[i] = exists
			if !exists {
				// chunk is new so, trigger subscription feeds
				// after the batch is successfully written
				triggerPushFeed = true
			}
			gcSizeChange += c
		}

	case storage.ModePutSync:
		db.lock.Lock(lockKeyGC)
		defer db.lock.Unlock(lockKeyGC)

		for i, ch := range chs {
			exists, c, err := putChunk(ch, i, func(item shed.Item, exists bool) (int64, error) {
				return db.putSync(batch, binIDs, item, exists)
			})
			if err != nil {
				return nil, fmt.Errorf("put sync: %w", err)
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
		return nil, fmt.Errorf("inc gc: %w", err)
	}

	err = db.shed.WriteBatch(batch)
	if err != nil {
		return nil, fmt.Errorf("write batch: %w", err)
	}

	for _, v := range *releaseLocs {
		db.sharky.Release(v)
	}

	for po := range triggerPullFeed {
		db.triggerPullSubscriptions(po)
	}
	if triggerPushFeed {
		db.triggerPushSubscriptions()
	}
	return exist, nil
}

// checkAndRemoveStampIndex will check if we have the postageIndexIndex already taken
// for a particular {BatchID, BatchIndex}. If yes and the batch is immutable, we
// return error, if the batch is not immutable we replace the index to point to the
// new chunk if the timestamp of the new chunk is later.
// If the index is not taken, we do nothing. This is done to guard against
// overissuance of batches.
func (db *DB) checkAndRemoveStampIndex(
	item shed.Item,
	batch *leveldb.Batch,
	loc *releaseLocations,
) (int64, error) {
	previous, err := db.postageIndexIndex.Get(item)
	if errors.Is(err, leveldb.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed reading postageIndexIndex: %w", err)
	}
	if item.Immutable {
		return 0, ErrOverwriteImmutable
	}
	// if a chunk is found with the same postage stamp index,
	// replace it with the new one only if timestamp is later
	if prev, cur := timestamps(previous, item); prev >= cur {
		db.logger.Warning("postage stamp index exists", "prev", prev, "cur", cur, "chunk_address", hex.EncodeToString(item.Address))
		return 0, ErrOverwrite
	}

	// remove older chunk
	previousIdx, err := db.retrievalDataIndex.Get(previous)
	if err != nil {
		if errors.Is(err, leveldb.ErrNotFound) {
			// currently there are stale postageIndexIndex entries in the localstore
			// due to a bug found recently. This error is mainly ignored as the
			// chunk is already gone and the index is overwritten.
			db.logger.Debug("old postage stamp index missing", "Address", swarm.NewAddress(previous.Address))
			return 0, nil
		}
		return 0, fmt.Errorf("could not fetch previous item: %w", err)
	}

	gcSizeChange, err := db.setRemove(batch, previousIdx, true)
	if err != nil {
		return 0, fmt.Errorf("setRemove on double issuance: %w", err)
	}

	l, err := sharky.LocationFromBinary(previousIdx.Location)
	if err != nil {
		return 0, fmt.Errorf("failed getting location: %w", err)
	}
	loc.add(l)

	return gcSizeChange, nil
}

// putRequest adds an Item to the batch by updating required indexes:
//   - put to indexes: retrieve, gc
//   - it does not enter the syncpool
//
// The batch can be written to the database.
// Provided batch and binID map are updated.
func (db *DB) putRequest(
	ctx context.Context,
	batch *leveldb.Batch,
	binIDs map[uint8]uint64,
	item shed.Item,
	forcePin, forceCache, exists bool,
) (int64, error) {

	var err error
	if !exists {
		item.StoreTimestamp = now()
		item.BinID, err = db.incBinID(binIDs, db.po(swarm.NewAddress(item.Address)))
		if err != nil {
			return 0, err
		}
		err = db.retrievalDataIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
		err = db.postageChunksIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
		err = db.postageIndexIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
		item.AccessTimestamp = now()
		err = db.retrievalAccessIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
	}

	// If forceCache is set, the stamp is invalid and we are storing it just for
	// bandwidth incentives so we add to cache.
	// If the request doesnt explicitly want to pin the chunk and it is not within
	// our radius, we add it to cache. The 'within radius' part is a little debatable,
	// but this is mainly done to opportunistically make the chunk available for pullSyncing.
	if forceCache || (!forcePin && !withinRadiusFn(db, item)) {
		return db.addToCache(batch, item)
	}

	if withinRadiusFn(db, item) {
		found, err := db.pullIndex.Has(item)
		if err != nil {
			return 0, err
		}
		if found {
			// this means it could be a duplicate put request. Dont update the
			// pin counter.
			return 0, nil
		}
		err = db.pullIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
	}

	return db.setPin(batch, item)
}

// putUpload adds an Item to the batch by updating required indexes:
//   - put to indexes: retrieve, push
//
// The batch can be written to the database.
// Provided batch and binID map are updated.
func (db *DB) putUpload(
	batch *leveldb.Batch,
	binIDs map[uint8]uint64,
	item shed.Item,
	pin bool,
	exists bool,
) (int64, error) {

	var err error
	if !exists {
		item.StoreTimestamp = now()
		item.BinID, err = db.incBinID(binIDs, db.po(swarm.NewAddress(item.Address)))
		if err != nil {
			return 0, fmt.Errorf("inc bin id: %w", err)
		}
		err = db.retrievalDataIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
		err = db.postageIndexIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
		err = db.postageChunksIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
	}

	err = db.pushIndex.PutInBatch(batch, item)
	if err != nil {
		return 0, err
	}

	if pin {
		return db.setPin(batch, item)
	}
	return 0, nil
}

// putSync adds an Item to the batch by updating required indexes:
//   - put to indexes: retrieve, pull, gc
//
// The batch can be written to the database.
// Provided batch and binID map are updated.
func (db *DB) putSync(
	batch *leveldb.Batch,
	binIDs map[uint8]uint64,
	item shed.Item,
	exists bool,
) (gcSizeChange int64, err error) {

	if !exists {
		item.StoreTimestamp = now()
		item.BinID, err = db.incBinID(binIDs, db.po(swarm.NewAddress(item.Address)))
		if err != nil {
			return 0, err
		}
		err = db.retrievalDataIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
		err = db.postageChunksIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
		err = db.postageIndexIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
		item.AccessTimestamp = now()
		err = db.retrievalAccessIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
	}

	// if we try to add a new item at a lesser radius than the last known eviction
	// radius of the batch, we should not add the chunk to reserve, but to cache
	if !withinRadiusFn(db, item) {
		return db.addToCache(batch, item)
	}

	found, err := db.pullIndex.Has(item)
	if err != nil {
		return 0, err
	}
	if found {
		// this means it could be a duplicate put request. Dont update the
		// pin counter.
		return 0, nil
	}
	// if this is an existing chunk being Put with ModeSync, we just need to add
	// the pullIndex and pin it
	err = db.pullIndex.PutInBatch(batch, item)
	if err != nil {
		return 0, err
	}

	return db.setPin(batch, item)
}

func (db *DB) addToCache(
	batch *leveldb.Batch,
	item shed.Item,
) (int64, error) {
	// add new entry to gc index ONLY if it is not present in pinIndex
	ok, err := db.pinIndex.Has(item)
	if err != nil {
		return 0, fmt.Errorf("failed checking pinIndex: %w", err)
	}
	if ok {
		// if the chunk is pinned we dont add it to cache
		return 0, nil
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
	err = db.pullIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}

	return 1, nil
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

func timestamps(previous, current shed.Item) (uint64, uint64) {
	return binary.BigEndian.Uint64(previous.Timestamp), binary.BigEndian.Uint64(current.Timestamp)
}
