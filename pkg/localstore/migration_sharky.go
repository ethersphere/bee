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
		count      = 0
		batchSize  = 10000
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

	fmt.Printf("starting to move entries. batch size %d\n", batchSize)
	currentBatch := 0
	for {
		currentBatch = 0
		err = retrievalDataIndex.Iterate(func(item shed.Item) (stop bool, err error) {
			l, err := db.sharky.Write(context.Background(), item.Data)
			if err != nil {
				return false, err
			}
			dirtyLocations = append(dirtyLocations, l)
			item.Location, err = l.MarshalBinary()
			if err != nil {
				return false, err
			}
			if err = newRetrievalDataIndex.PutInBatch(batch, item); err != nil {
				return false, err
			}
			if err = retrievalDataIndex.DeleteInBatch(batch, item); err != nil {
				return false, err
			}
			count++
			currentBatch++
			if count%batchSize == 0 {
				// collected enough entries. close the iterator, try to flush, trigger compaction
				fmt.Printf("collected %d entries, trying to flush...\n", batchSize)
				return true, nil
			}
			return false, nil
		}, nil)
		if err != nil {
			return fmt.Errorf("iterate index: %w", err)
		}

		if currentBatch == 0 {
			break
		}

		err = db.shed.WriteBatch(batch)
		if err != nil {
			return fmt.Errorf("write batch: %w", err)
		} else {
			// release sharky locations, return error, potentially force deletion of the shards?
		}
		fmt.Printf("flush ok, progress so far: %d chunks\n", count)
		batch.Reset()
		fmt.Println("triggering leveldb compaction...")
		compactStart := time.Now()
		if err = db.shed.Compact(); err != nil {
			return fmt.Errorf("leveldb compaction failed: %w", err)
		}
		fmt.Printf("leveldb compaction done, took %s\n", time.Since(compactStart))
	}
	db.logger.Debugf("done migrating to sharky. it took me %s to move %d chunks.", time.Since(start), count)
	return nil
}
