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
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethersphere/swarm/shed"
	"github.com/syndtr/goleveldb/leveldb"
)

var (
	// gcTargetRatio defines the target number of items
	// in garbage collection index that will not be removed
	// on garbage collection. The target number of items
	// is calculated by gcTarget function. This value must be
	// in range (0,1]. For example, with 0.9 value,
	// garbage collection will leave 90% of defined capacity
	// in database after its run. This prevents frequent
	// garbage collection runs.
	gcTargetRatio = 0.9
	// gcBatchSize limits the number of chunks in a single
	// leveldb batch on garbage collection.
	gcBatchSize uint64 = 200
)

// collectGarbageWorker is a long running function that waits for
// collectGarbageTrigger channel to signal a garbage collection
// run. GC run iterates on gcIndex and removes older items
// form retrieval and other indexes.
func (db *DB) collectGarbageWorker() {
	defer close(db.collectGarbageWorkerDone)

	for {
		select {
		case <-db.collectGarbageTrigger:
			// run a single collect garbage run and
			// if done is false, gcBatchSize is reached and
			// another collect garbage run is needed
			collectedCount, done, err := db.collectGarbage()
			if err != nil {
				log.Error("localstore collect garbage", "err", err)
			}
			// check if another gc run is needed
			if !done {
				db.triggerGarbageCollection()
			}

			if testHookCollectGarbage != nil {
				testHookCollectGarbage(collectedCount)
			}
		case <-db.close:
			return
		}
	}
}

// collectGarbage removes chunks from retrieval and other
// indexes if maximal number of chunks in database is reached.
// This function returns the number of removed chunks. If done
// is false, another call to this function is needed to collect
// the rest of the garbage as the batch size limit is reached.
// This function is called in collectGarbageWorker.
func (db *DB) collectGarbage() (collectedCount uint64, done bool, err error) {
	metricName := "localstore/gc"
	metrics.GetOrRegisterCounter(metricName, nil).Inc(1)
	defer totalTimeMetric(metricName, time.Now())
	defer func() {
		if err != nil {
			metrics.GetOrRegisterCounter(metricName+"/error", nil).Inc(1)
		}
	}()

	batch := new(leveldb.Batch)
	target := db.gcTarget()

	// protect database from changing idexes and gcSize
	db.batchMu.Lock()
	defer db.batchMu.Unlock()

	// run through the recently pinned chunks and
	// remove them from the gcIndex before iterating through gcIndex
	err = db.removeChunksInExcludeIndexFromGC()
	if err != nil {
		log.Error("localstore exclude pinned chunks", "err", err)
		return 0, true, err
	}

	gcSize, err := db.gcSize.Get()
	if err != nil {
		return 0, true, err
	}
	metrics.GetOrRegisterGauge(metricName+"/gcsize", nil).Update(int64(gcSize))

	done = true
	err = db.gcIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		if gcSize-collectedCount <= target {
			return true, nil
		}

		metrics.GetOrRegisterGauge(metricName+"/storets", nil).Update(item.StoreTimestamp)
		metrics.GetOrRegisterGauge(metricName+"/accessts", nil).Update(item.AccessTimestamp)

		// delete from retrieve, pull, gc
		db.retrievalDataIndex.DeleteInBatch(batch, item)
		db.retrievalAccessIndex.DeleteInBatch(batch, item)
		db.pullIndex.DeleteInBatch(batch, item)
		db.gcIndex.DeleteInBatch(batch, item)
		collectedCount++
		if collectedCount >= gcBatchSize {
			// bach size limit reached,
			// another gc run is needed
			done = false
			return true, nil
		}
		return false, nil
	}, nil)
	if err != nil {
		return 0, false, err
	}
	metrics.GetOrRegisterCounter(metricName+"/collected-count", nil).Inc(int64(collectedCount))

	db.gcSize.PutInBatch(batch, gcSize-collectedCount)

	err = db.shed.WriteBatch(batch)
	if err != nil {
		metrics.GetOrRegisterCounter(metricName+"/writebatch/err", nil).Inc(1)
		return 0, false, err
	}
	return collectedCount, done, nil
}

// removeChunksInExcludeIndexFromGC removed any recently chunks in the exclude Index, from the gcIndex.
func (db *DB) removeChunksInExcludeIndexFromGC() (err error) {
	metricName := "localstore/gc/exclude"
	metrics.GetOrRegisterCounter(metricName, nil).Inc(1)
	defer totalTimeMetric(metricName, time.Now())
	defer func() {
		if err != nil {
			metrics.GetOrRegisterCounter(metricName+"/error", nil).Inc(1)
		}
	}()

	batch := new(leveldb.Batch)
	excludedCount := 0
	var gcSizeChange int64
	err = db.gcExcludeIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		// Get access timestamp
		retrievalAccessIndexItem, err := db.retrievalAccessIndex.Get(item)
		if err != nil {
			return false, err
		}
		item.AccessTimestamp = retrievalAccessIndexItem.AccessTimestamp

		// Get the binId
		retrievalDataIndexItem, err := db.retrievalDataIndex.Get(item)
		if err != nil {
			return false, err
		}
		item.BinID = retrievalDataIndexItem.BinID

		// Check if this item is in gcIndex and remove it
		ok, err := db.gcIndex.Has(item)
		if ok {
			db.gcIndex.DeleteInBatch(batch, item)
			if _, err := db.gcIndex.Get(item); err == nil {
				gcSizeChange--
			}
			excludedCount++
			db.gcExcludeIndex.DeleteInBatch(batch, item)
		}

		return false, nil
	}, nil)
	if err != nil {
		return err
	}

	// update the gc size based on the no of entries deleted in gcIndex
	err = db.incGCSizeInBatch(batch, gcSizeChange)
	if err != nil {
		return err
	}

	metrics.GetOrRegisterCounter(metricName+"/excluded-count", nil).Inc(int64(excludedCount))
	err = db.shed.WriteBatch(batch)
	if err != nil {
		metrics.GetOrRegisterCounter(metricName+"/writebatch/err", nil).Inc(1)
		return err
	}

	return nil
}

// gcTrigger retruns the absolute value for garbage collection
// target value, calculated from db.capacity and gcTargetRatio.
func (db *DB) gcTarget() (target uint64) {
	return uint64(float64(db.capacity) * gcTargetRatio)
}

// triggerGarbageCollection signals collectGarbageWorker
// to call collectGarbage.
func (db *DB) triggerGarbageCollection() {
	select {
	case db.collectGarbageTrigger <- struct{}{}:
	case <-db.close:
	default:
	}
}

// incGCSizeInBatch changes gcSize field value
// by change which can be negative. This function
// must be called under batchMu lock.
func (db *DB) incGCSizeInBatch(batch *leveldb.Batch, change int64) (err error) {
	if change == 0 {
		return nil
	}
	gcSize, err := db.gcSize.Get()
	if err != nil {
		return err
	}

	var new uint64
	if change > 0 {
		new = gcSize + uint64(change)
	} else {
		// 'change' is an int64 and is negative
		// a conversion is needed with correct sign
		c := uint64(-change)
		if c > gcSize {
			// protect uint64 undeflow
			return nil
		}
		new = gcSize - c
	}
	db.gcSize.PutInBatch(batch, new)

	// trigger garbage collection if we reached the capacity
	if new >= db.capacity {
		db.triggerGarbageCollection()
	}
	return nil
}

// testHookCollectGarbage is a hook that can provide
// information when a garbage collection run is done
// and how many items it removed.
var testHookCollectGarbage func(collectedCount uint64)
