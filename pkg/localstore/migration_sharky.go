// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/syndtr/goleveldb/leveldb"
)

// DBSchemaSharky is the bee schema identifier for sharky.
const DBSchemaSharky = "sharky"

// migrateDeadPush cleans up dangling push index entries that make the pusher stop pushing entries
func migrateSharky(db *DB) error {
	db.logger.Debug("starting sharky migration. have patience, this might take a while...")
	var (
		start      = time.Now()
		batch      = new(leveldb.Batch)
		batchSize  = 10000 // TODO: Tweak the compaction (less often then batch write).
		batchCount = 0
		headerSize = 16 + postage.StampSize
	)

	retrievalDataIndex, err := db.shed.NewIndex("Address->StoreTimestamp|BinID|BatchID|BatchIndex|Sig|Data", shed.IndexFuncs{
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
			value = append(b, fields.Data...)
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
			e.Data = value[headerSize:]
			return e, nil
		},
	})
	if err != nil {
		return err
	}

	newRetrievalDataIndex, err := db.shed.NewIndex("Address->StoreTimestamp|BinID|BatchID|BatchIndex|Sig|Location", shed.IndexFuncs{
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
		return err
	}

	var dirtyLocations []sharky.Location

	db.logger.Debugf("starting to move entries with batch size %d", batchSize)
	for {
		isBatchEmpty := true
		err = retrievalDataIndex.Iterate(func(item shed.Item) (stop bool, err error) {
			loc, err := db.sharky.Write(context.TODO(), item.Data)
			if err != nil {
				return false, err
			}
			dirtyLocations = append(dirtyLocations, loc)
			item.Location, err = loc.MarshalBinary()
			if err != nil {
				return false, err
			}
			if err = newRetrievalDataIndex.PutInBatch(batch, item); err != nil {
				return false, err
			}
			if err = retrievalDataIndex.DeleteInBatch(batch, item); err != nil {
				return false, err
			}
			batchCount++
			isBatchEmpty = false
			if batchCount%batchSize == 0 {
				db.logger.Debugf("collected %d entries; trying to flush...", batchSize)
				return true, nil
			}
			return false, nil
		}, nil)
		if err != nil {
			return fmt.Errorf("iterate index: %w", err)
		}

		if isBatchEmpty {
			break
		}

		if err := db.shed.WriteBatch(batch); err != nil {
			for _, loc := range dirtyLocations {
				db.sharky.Release(context.TODO(), loc)
			}
			return fmt.Errorf("write batch: %w", err)
		}

		db.logger.Debugf("flush ok; progress so far: %d chunks", batchCount)
		batch.Reset()
		db.logger.Debugf("triggering leveldb compaction...")
		compactStart := time.Now()
		if err = db.shed.Compact(); err != nil {
			return fmt.Errorf("leveldb compaction failed: %w", err)
		}
		db.logger.Debugf("leveldb compaction done, took %s\n", time.Since(compactStart))

		dirtyLocations = nil
	}
	db.logger.Debugf("done migrating to sharky. it took me %s to move %d chunks.", time.Since(start), batchCount)
	return nil
}
