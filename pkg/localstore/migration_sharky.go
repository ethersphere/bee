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
	"github.com/hashicorp/go-multierror"
	"github.com/syndtr/goleveldb/leveldb"
)

// DBSchemaSharky is the bee schema identifier for sharky.
const DBSchemaSharky = "sharky"

// migrateSharky writes the new retrievalDataIndex format by storing chunk data in sharky
func migrateSharky(db *DB) error {
	db.logger.Info("starting sharky migration; have patience, this might take a while...")
	var (
		start          = time.Now()
		batch          = new(leveldb.Batch)
		batchSize      = 10000
		batchesCount   = 0
		headerSize     = 16 + postage.StampSize
		compactionRate = 100
		compactionSize = batchSize * compactionRate
	)

	compaction := func(start, end []byte) (time.Duration, error) {
		compactStart := time.Now()
		if err := db.shed.Compact(start, end); err != nil {
			return 0, fmt.Errorf("leveldb compaction failed: %w", err)
		}
		return time.Since(compactStart), nil
	}

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

	db.gcSize, err = db.shed.NewUint64Field("gc-size")
	if err != nil {
		return err
	}

	db.reserveSize, err = db.shed.NewUint64Field("reserve-size")
	if err != nil {
		return err
	}

	var (
		compactionTime           time.Duration
		dirtyLocations           []sharky.Location
		compactStart, compactEnd *shed.Item
	)

	db.logger.Debugf("starting to move entries with batch size %d", batchSize)
	for {
		isBatchEmpty := true

		err = retrievalDataIndex.Iterate(func(item shed.Item) (stop bool, err error) {
			if compactStart == nil {
				compactStart = &item
			}
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
			batchesCount++
			isBatchEmpty = false
			if batchesCount%batchSize == 0 {
				compactEnd = &item
				db.logger.Debugf("collected %d entries; trying to flush...", batchSize)
				return true, nil
			}
			return false, nil
		}, &shed.IterateOptions{
			StartFrom:         compactEnd,
			SkipStartFromItem: func() bool { return compactEnd != nil }(),
		})
		if err != nil {
			return fmt.Errorf("iterate index: %w", err)
		}

		if isBatchEmpty {
			break
		}

		if err := db.shed.WriteBatch(batch); err != nil {
			for _, loc := range dirtyLocations {
				err = multierror.Append(err, db.sharky.Release(context.TODO(), loc))
			}
			return fmt.Errorf("write batch: %w", err)
		}
		dirtyLocations = nil
		db.logger.Debugf("flush ok; progress so far: %d chunks", batchesCount)
		batch.Reset()

		if batchesCount%compactionSize == 0 {
			db.logger.Debugf("starting compaction")

			// the items are references from the iteration so encoding should be error-free
			start, _ := retrievalDataIndex.ItemKey(*compactStart)
			end, _ := retrievalDataIndex.ItemKey(*compactEnd)

			dur, err := compaction(start, end)
			if err != nil {
				return err
			}
			compactionTime += dur
			db.logger.Debugf("compaction done %s", dur)
			compactStart = nil

			rIdxCount, _ := retrievalDataIndex.Count()
			nrIdxCount, _ := newRetrievalDataIndex.Count()
			db.logger.Debugf("retrieval index count: %d newRetrievalIndex count: %d", rIdxCount, nrIdxCount)
		}
	}

	dur, err := compaction(nil, nil)
	if err != nil {
		return err
	}
	compactionTime += dur

	db.logger.Debugf("leveldb compaction took: %v", compactionTime)
	db.logger.Infof("done migrating to sharky; it took me %s to move %d chunks.", time.Since(start), batchesCount)
	return nil
}
