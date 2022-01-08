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
	"fmt"
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
			return nil, fmt.Errorf("initial has check: %w", err)
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
	var (
		gcSizeChange      int64 // number to add or subtract from gcSize
		reserveSizeChange int64 // number to add or subtract from reserveSize
	)
	var triggerPushFeed bool                    // signal push feed subscriptions to iterate
	triggerPullFeed := make(map[uint8]struct{}) // signal pull feed subscriptions to iterate

	exist = make([]bool, len(chs))

	// A lazy populated map of bin ids to properly set
	// BinID values for new chunks based on initial value from database
	// and incrementing them.
	// Values from this map are stored with the batch
	binIDs := make(map[uint8]uint64)

	switch mode {

	case storage.ModePutSync:
		for i, ch := range chs {
			if containsChunk(ch.Address(), chs[:i]...) {
				exist[i] = true
				continue
			}
			exists, c, r, err := db.putSync(batch, binIDs, chunkToItem(ch))
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
			reserveSizeChange += r
		}

	default:
		return nil, ErrInvalidMode
	}

	for po, id := range binIDs {
		db.binIDs.PutInBatch(batch, uint64(po), id)
	}

	err = db.incGCSizeInBatch(batch, gcSizeChange)
	if err != nil {
		return nil, fmt.Errorf("inc gc: %w", err)
	}

	err = db.incReserveSizeInBatch(batch, reserveSizeChange)
	if err != nil {
		return nil, fmt.Errorf("inc reserve: %w", err)
	}

	err = db.shed.WriteBatch(batch)
	if err != nil {
		return nil, fmt.Errorf("write batch: %w", err)
	}

	for po := range triggerPullFeed {
		db.triggerPullSubscriptions(po)
	}
	if triggerPushFeed {
		db.triggerPushSubscriptions()
	}
	return exist, nil
}

func (db *DB) putSync(batch *leveldb.Batch, binIDs map[uint8]uint64, item shed.Item) (exists bool, gcSizeChange, reserveSizeChange int64, err error) {
	exists, err = db.retrievalDataIndex.Has(item)
	if err != nil {
		return false, 0, 0, err
	}
	if exists {
		return true, 0, 0, nil
	}

	previous, err := db.postageIndexIndex.Get(item)
	if err != nil {
		if !errors.Is(err, leveldb.ErrNotFound) {
			return false, 0, 0, err
		}
	} else {
		if item.Immutable {
			return false, 0, 0, ErrOverwrite
		}
		// if a chunk is found with the same postage stamp index,
		// replace it with the new one only if timestamp is later
		if !later(previous, item) {
			return false, 0, 0, nil
		}
		_, err = db.setRemove(batch, previous, true)
		if err != nil {
			return false, 0, 0, err
		}
		radius, err := db.postageRadiusIndex.Get(item)
		if err != nil {
			if !errors.Is(err, leveldb.ErrNotFound) {
				return false, 0, 0, err
			}
		} else {
			if db.po(swarm.NewAddress(item.Address)) >= radius.Radius {
				reserveSizeChange--
			}
		}
	}

	item.StoreTimestamp = now()
	item.BinID, err = db.incBinID(binIDs, db.po(swarm.NewAddress(item.Address)))
	if err != nil {
		return false, 0, 0, err
	}
	err = db.retrievalDataIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, 0, err
	}
	err = db.pullIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, 0, err
	}
	err = db.postageChunksIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, 0, err
	}
	err = db.postageIndexIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, 0, err
	}
	item.AccessTimestamp = now()
	err = db.retrievalAccessIndex.PutInBatch(batch, item)
	if err != nil {
		return false, 0, 0, err
	}

	if withinRadiusFn(db, item) {
		reserveSizeChange++
	} else {
		exists, err := db.gcIndex.Has(item)
		if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
			return false, 0, 0, err
		}
		if !exists {
			err = db.gcIndex.PutInBatch(batch, item)
			if err != nil {
				return false, 0, 0, err
			}
		}
		gcSizeChange++
	}

	return false, gcSizeChange, reserveSizeChange, nil
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
