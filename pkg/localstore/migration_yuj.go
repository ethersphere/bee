// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
)

// DBSchemaYuj is the bee schema identifier for storage incentives initial iteration.
const DBSchemaYuj = "yuj"

// migrateYuj removes all existing database content, unless
// pinned content is detected, in which case it aborts the
// operation for the user to resolve.
func migrateYuj(db *DB) error {
	pinIndex, err := db.shed.NewIndex("Hash->PinCounter", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			b := make([]byte, 8)
			binary.BigEndian.PutUint64(b[:8], fields.PinCounter)
			return b, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.PinCounter = binary.BigEndian.Uint64(value[:8])
			return e, nil
		},
	})
	if err != nil {
		return err
	}

	hasPins := false
	_ = pinIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		hasPins = true
		return true, nil
	}, nil)
	if hasPins {
		return errors.New("failed to update your node due to the existence of pinned content; please refer to the release notes on how to safely migrate your pinned content")
	}

	// Define the old indexes from the previous schema and swipe them clean.

	retrievalDataIndex, err := db.shed.NewIndex("Address->StoreTimestamp|BinID|Data", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			b := make([]byte, 16)
			binary.BigEndian.PutUint64(b[:8], fields.BinID)
			binary.BigEndian.PutUint64(b[8:16], uint64(fields.StoreTimestamp))
			value = append(b, fields.Data...)
			return value, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.StoreTimestamp = int64(binary.BigEndian.Uint64(value[8:16]))
			e.BinID = binary.BigEndian.Uint64(value[:8])
			e.Data = value[16:]
			return e, nil
		},
	})
	if err != nil {
		return err
	}

	retrievalAccessIndex, err := db.shed.NewIndex("Address->AccessTimestamp", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			b := make([]byte, 8)
			binary.BigEndian.PutUint64(b, uint64(fields.AccessTimestamp))
			return b, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.AccessTimestamp = int64(binary.BigEndian.Uint64(value))
			return e, nil
		},
	})
	if err != nil {
		return err
	}
	// pull index allows history and live syncing per po bin
	pullIndex, err := db.shed.NewIndex("PO|BinID->Hash|Tag", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 41)
			key[0] = db.po(swarm.NewAddress(fields.Address))
			binary.BigEndian.PutUint64(key[1:9], fields.BinID)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.BinID = binary.BigEndian.Uint64(key[1:9])
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			value = make([]byte, 36) // 32 bytes address, 4 bytes tag
			copy(value, fields.Address)

			if fields.Tag != 0 {
				binary.BigEndian.PutUint32(value[32:], fields.Tag)
			}

			return value, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.Address = value[:32]
			if len(value) > 32 {
				e.Tag = binary.BigEndian.Uint32(value[32:])
			}
			return e, nil
		},
	})
	if err != nil {
		return err
	}
	// create a vector for bin IDs
	binIDs, err := db.shed.NewUint64Vector("bin-ids")
	if err != nil {
		return err
	}
	pushIndex, err := db.shed.NewIndex("StoreTimestamp|Hash->Tags", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 40)
			binary.BigEndian.PutUint64(key[:8], uint64(fields.StoreTimestamp))
			copy(key[8:], fields.Address)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key[8:]
			e.StoreTimestamp = int64(binary.BigEndian.Uint64(key[:8]))
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			tag := make([]byte, 4)
			binary.BigEndian.PutUint32(tag, fields.Tag)
			return tag, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			if len(value) == 4 { // only values with tag should be decoded
				e.Tag = binary.BigEndian.Uint32(value)
			}
			return e, nil
		},
	})
	if err != nil {
		return err
	}
	gcIndex, err := db.shed.NewIndex("AccessTimestamp|BinID|Hash->nil", shed.IndexFuncs{
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
			return nil, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			return e, nil
		},
	})
	if err != nil {
		return err
	}

	// Create a index structure for excluding pinned chunks from gcIndex
	gcExcludeIndex, err := db.shed.NewIndex("Hash->nil", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			return nil, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			return e, nil
		},
	})
	if err != nil {
		return err
	}

	start := time.Now()
	db.logger.Debug("truncating indexes")

	var count int
	for _, v := range []struct {
		name string
		idx  shed.Index
	}{
		{"pullsync", pullIndex},
		{"pushsync", pushIndex},
		{"gc", gcIndex},
		{"gcExclude", gcExcludeIndex},
		{"retrievalAccess", retrievalAccessIndex},
		{"retrievalData", retrievalDataIndex},
	} {
		db.logger.Debugf("truncating %s index", v.name)
		n, err := truncateIndex(db, v.idx)
		if err != nil {
			return fmt.Errorf("truncate %s index: %w", v.name, err)
		}
		count += n
		db.logger.Debugf("truncated %d %s index entries", count, v.name)
	}

	gcSize, err := db.shed.NewUint64Field("gc-size")
	if err != nil {
		return fmt.Errorf("gc size index: %w", err)
	}

	err = gcSize.Put(0)
	if err != nil {
		return fmt.Errorf("put gcsize: %w", err)
	}

	for i := 0; i < int(swarm.MaxBins); i++ {
		if err := binIDs.Put(uint64(i), 0); err != nil {
			return fmt.Errorf("zero binsIDs: %w", err)
		}
	}

	db.logger.Debugf("done truncating indexes. took %s", time.Since(start))
	return nil
}
