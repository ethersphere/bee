// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/syndtr/goleveldb/leveldb"
)

// DBSchemaBatchIndex is the bee schema identifier for dead-push.
const DBSchemaDeadPush = "dead-push"

// migrateDeadPush cleans up dangling push index entries that make the pusher stop pushing entries
func migrateDeadPush(db *DB) error {
	start := time.Now()
	db.logger.Debug("removing dangling entries from push index")
	batch := new(leveldb.Batch)
	count := 0
	err := db.pushIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		has, err := db.retrievalDataIndex.Has(item)
		if err != nil {
			return true, err
		}
		if !has {
			if err = db.pushIndex.DeleteInBatch(batch, item); err != nil {
				return true, err
			}
			count++
		}
		return false, nil
	}, &shed.IterateOptions{
		StartFrom:         nil,
		SkipStartFromItem: true,
	},
	)
	if err != nil {
		return fmt.Errorf("iterate index: %w", err)
	}
	db.logger.Debugf("found %d entries to remove. trying to flush...", count)
	err = db.shed.WriteBatch(batch)
	if err != nil {
		return fmt.Errorf("write batch: %w", err)
	}
	db.logger.Debugf("done cleaning index. took %s", time.Since(start))
	return nil
}
