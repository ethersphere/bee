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
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
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
	// badger transaction on garbage collection.
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
				db.logger.Errorf("localstore: collect garbage: %v", err)
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
	fmt.Println("trig")
	db.metrics.GCCounter.Inc()
	defer totalTimeMetric(db.metrics.TotalTimeCollectGarbage, time.Now())
	defer func() {
		if err != nil {
			db.metrics.GCErrorCounter.Inc()
		}
	}()

	batch := new(leveldb.Batch)
	target := db.gcTarget()

	// protect database from changing idexes and gcSize
	db.batchMu.Lock()
	defer db.batchMu.Unlock()

	gcSize, err := db.gcSize.Get()
	if err != nil {
		return 0, true, err
	}
	db.metrics.GCSize.Inc()

	done = true
	err = db.gcIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		if gcSize-collectedCount <= target {
			return true, nil
		}

		db.metrics.GCStoreTimeStamps.Set(float64(item.StoreTimestamp))
		db.metrics.GCStoreAccessTimeStamps.Set(float64(item.AccessTimestamp))

		// delete from retrieve, pull, gc
		err = db.retrievalDataIndex.DeleteInBatch(batch, item)
		if err != nil {
			return true, nil
		}
		err = db.retrievalAccessIndex.DeleteInBatch(batch, item)
		if err != nil {
			return true, nil
		}
		err = db.pullIndex.DeleteInBatch(batch, item)
		if err != nil {
			return true, nil
		}
		err = db.gcIndex.DeleteInBatch(batch, item)
		if err != nil {
			return true, nil
		}
		err = db.postage.deleteInBatch(batch, item)
		if err != nil {
			return true, nil
		}

		collectedCount++
		if collectedCount >= gcBatchSize {
			// batch size limit reached, another gc run is needed
			done = false
			return true, nil
		}
		return false, nil
	}, nil)
	if err != nil {
		return 0, false, err
	}
	db.metrics.GCCollectedCounter.Inc()

	db.gcSize.PutInBatch(batch, gcSize-collectedCount)
	err = db.shed.WriteBatch(batch)
	if err != nil {
		db.metrics.GCWriteBatchError.Inc()
		return 0, false, err
	}
	return collectedCount, done, nil
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
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return err
	}

	var newSize uint64
	if change > 0 {
		newSize = gcSize + uint64(change)
	} else {
		// 'change' is an int64 and is negative
		// a conversion is needed with correct sign
		c := uint64(-change)
		if c > gcSize {
			// protect uint64 undeflow
			return nil
		}
		newSize = gcSize - c
	}
	db.gcSize.PutInBatch(batch, newSize)

	// trigger garbage collection if we reached the capacity
	if newSize >= db.capacity {
		db.triggerGarbageCollection()
	}
	return nil
}

// testHookCollectGarbage is a hook that can provide
// information when a garbage collection run is done
// and how many items it removed.
var testHookCollectGarbage func(collectedCount uint64)
