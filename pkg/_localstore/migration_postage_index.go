// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/topology/kademlia/internal/shed"
	"github.com/syndtr/goleveldb/leveldb"
)

// DBSchemaDeadPostageIndex is the bee schema identifier for removing incorrect postageIndexIndex entries.
const DBSchemaDeadPostageIndex = "dead-postage-index"

// migrateDeadPostageIndex removes all the stale postageIndexIndex entries that have been GC'd and not
// cleaned up due to the bug found recently.
func migrateDeadPostageIndex(db *DB) error {
	db.logger.Info("starting dead-postage-index migration")
	start := time.Now()

	retrievalDataIndex, err := db.shed.NewIndex("Address->StoreTimestamp|BinID|BatchID|BatchIndex|Sig|Location", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			b := make([]byte, headerSize)
			binary.BigEndian.PutUint64(b[:8], fields.BinID)
			binary.BigEndian.PutUint64(b[8:16], uint64(fields.StoreTimestamp))
			stamp, err := postage.NewStamp(fields.BatchID, fields.Index, fields.Timestamp, fields.Sig).MarshalBinary()
			if err != nil {
				return nil, err
			}
			copy(b[16:], stamp)
			value = append(b, fields.Location...)
			return value, nil

		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.StoreTimestamp = int64(binary.BigEndian.Uint64(value[8:16]))
			e.BinID = binary.BigEndian.Uint64(value[:8])
			stamp := new(postage.Stamp)
			if err = stamp.UnmarshalBinary(value[16:headerSize]); err != nil {
				return e, err
			}
			e.BatchID = stamp.BatchID()
			e.Index = stamp.Index()
			e.Timestamp = stamp.Timestamp()
			e.Sig = stamp.Sig()
			e.Location = value[headerSize:]
			return e, nil
		},
	})
	if err != nil {
		return fmt.Errorf("failed creating retrievalDataIndex: %w", err)
	}

	postageIndexIndex, err := db.shed.NewIndex("BatchID|BatchIndex->Hash|Timestamp", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 40)
			copy(key[:32], fields.BatchID)
			copy(key[32:40], fields.Index)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.BatchID = key[:32]
			e.Index = key[32:40]
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			value = make([]byte, 40)
			copy(value, fields.Address)
			copy(value[32:], fields.Timestamp)
			return value, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.Address = value[:32]
			e.Timestamp = value[32:]
			return e, nil
		},
	})
	if err != nil {
		return fmt.Errorf("failed creating postageIndexIndex: %w", err)
	}

	const batchSize = 1_000_000
	var startItem *shed.Item
	totalCleanedUpEntries := 0
	for {
		batch := new(leveldb.Batch)
		currentBatchSize := 0
		more := false
		err := postageIndexIndex.Iterate(func(item shed.Item) (bool, error) {
			if currentBatchSize == batchSize {
				startItem = &item
				more = true
				return true, nil
			}
			has, err := retrievalDataIndex.Has(item)
			if err != nil {
				return true, err
			}
			if !has {
				err := postageIndexIndex.DeleteInBatch(batch, item)
				if err != nil {
					return true, err
				}
				currentBatchSize++
			}
			return false, nil
		}, &shed.IterateOptions{
			StartFrom: startItem,
		})
		if err != nil {
			return err
		}
		if err := db.shed.WriteBatch(batch); err != nil {
			return err
		}
		totalCleanedUpEntries += currentBatchSize
		if !more {
			break
		}
	}

	db.logger.Info("migration dead-postage-index done", "elapsed", time.Since(start), "no_of_cleaned_entries", totalCleanedUpEntries)
	return nil
}
