// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/kademlia/internal/shed"
	"github.com/syndtr/goleveldb/leveldb"
)

// DBSchemaResidue is the bee schema identifier for residual migration.
const DBSchemaResidue = "residue"

// migrateResidue sanitizes the pullIndex by removing any entries that are present
// in gcIndex from the pullIndex.
func migrateResidue(db *DB) error {
	db.logger.Info("starting residual migration")
	start := time.Now()
	var err error

	gcIndex, err := db.shed.NewIndex("AccessTimestamp|BinID|Hash->BatchID|BatchIndex", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			b := make([]byte, 16, 16+len(fields.Address))
			binary.BigEndian.PutUint64(b[:8], uint64(fields.AccessTimestamp))
			binary.BigEndian.PutUint64(b[8:16], fields.BinID)
			key = append(b, fields.Address...)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.AccessTimestamp = int64(binary.BigEndian.Uint64(key[:8]))
			e.BinID = binary.BigEndian.Uint64(key[8:16])
			e.Address = key[16:]
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			value = make([]byte, 40)
			copy(value, fields.BatchID)
			copy(value[32:], fields.Index)
			return value, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.BatchID = make([]byte, 32)
			copy(e.BatchID, value[:32])
			e.Index = make([]byte, postage.IndexSize)
			copy(e.Index, value[32:])
			return e, nil
		},
	})
	if err != nil {
		return fmt.Errorf("failed to instantiate gcIndex: %w", err)
	}

	headerSize := 16 + postage.StampSize
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
		return fmt.Errorf("failed to instantiate retrievalDataIndex: %w", err)
	}

	pullIndex, err := db.shed.NewIndex("PO|BinID->Hash", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 9)
			key[0] = db.po(swarm.NewAddress(fields.Address))
			binary.BigEndian.PutUint64(key[1:9], fields.BinID)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.BinID = binary.BigEndian.Uint64(key[1:9])
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			value = make([]byte, 64) // 32 bytes address, 32 bytes batch id
			copy(value, fields.Address)
			copy(value[32:], fields.BatchID)
			return value, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.Address = value[:32]
			e.BatchID = value[32:64]
			return e, nil
		},
	})
	if err != nil {
		return fmt.Errorf("failed to instantiate pullIndex: %w", err)
	}

	updateBatch := new(leveldb.Batch)
	updatedCount := 0

	err = gcIndex.Iterate(func(item shed.Item) (bool, error) {
		sItem, err := retrievalDataIndex.Get(item)
		switch {
		case errors.Is(err, leveldb.ErrNotFound):
			// continue iteration on error
			return false, nil
		case err != nil:
			return true, fmt.Errorf("retrievalIndex not found: %w", err)
		}

		if found, err := pullIndex.Has(sItem); err == nil && found {
			err = pullIndex.DeleteInBatch(updateBatch, sItem)
			if err != nil {
				return true, fmt.Errorf("failed to add to batch: %w", err)
			}
			updatedCount++
		}
		return false, nil
	}, nil)
	if err != nil {
		return err
	}

	err = db.shed.WriteBatch(updateBatch)
	if err != nil {
		return fmt.Errorf("failed to update entries: %w", err)
	}

	db.logger.Info("residual migration done", "elapsed", time.Since(start), "cleaned_pull_indexes", updatedCount)
	return nil
}
