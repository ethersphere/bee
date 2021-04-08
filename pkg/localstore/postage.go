// Copyright 2020 The Swarm Authors. All rights reserved.
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

var (
	// ErrBatchOverissued is returned if number of chunks found in neighbourhood extrapolates to overissued stamp
	// count(batch, po) > 1<< (depth(batch) - po)
	ErrBatchOverissued = errors.New("postage batch overissued")
)

// UnreserveBatch atomically unpins chunks of a batch in proximity order upto and including po
// and marks the batch pinned within radius po
// if batch is marked as pinned within radius r>po, then do nothing
// unpinning will result in all chunks  with pincounter 0 to be put in the gc index
// so if a chunk was only pinned by the reserve, unreserving it  will make it gc-able
func (db *DB) UnreserveBatch(id []byte, oldRadius, newRadius uint8) error {
	db.batchMu.Lock()
	defer db.batchMu.Unlock()
	// todo add metrics

	batch := new(leveldb.Batch)
	var gcSizeChange int64 // number to add or subtract from gcSize
	unpin := func(item shed.Item) (stop bool, err error) {
		c, err := db.setUnpin(batch, swarm.NewAddress(item.Address))
		if err != nil {
			return false, fmt.Errorf("unpin: %w", err)
		}

		// if the batch is unreserved we should remove the chunk
		// from the pull index
		item2, err := db.retrievalDataIndex.Get(item)
		if err != nil {
			return false, err
		}
		err = db.pullIndex.DeleteInBatch(batch, item2)
		if err != nil {
			return false, err
		}

		gcSizeChange += c
		return false, err
	}

	// iterate over chunk in bins
	// TODO the initial value needs to change to the previous
	// batch radius value.
	for bin := oldRadius; bin < newRadius; bin++ {
		err := db.postageChunksIndex.Iterate(unpin, &shed.IterateOptions{Prefix: append(id, bin)})
		if err != nil {
			return err
		}
		// adjust gcSize
		if err := db.incGCSizeInBatch(batch, gcSizeChange); err != nil {
			return err
		}
		if err := db.shed.WriteBatch(batch); err != nil {
			return err
		}
		batch = new(leveldb.Batch)
		gcSizeChange = 0
	}

	gcSize, err := db.gcSize.Get()
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return err
	}
	// trigger garbage collection if we reached the capacity
	if gcSize >= db.capacity {
		db.triggerGarbageCollection()
	}

	return nil
}

func withinRadius(db *DB, item shed.Item) bool {
	po := db.po(swarm.NewAddress(item.Address))
	return po >= item.Radius
}
