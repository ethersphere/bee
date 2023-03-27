// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/shed"
)

// DBSchemaCatharsis is the bee schema identifier for resetting gcIndex.
const DBSchemaCatharsis = "catharsis"

// migrateCatharsis resets the gcSize to the count of gcIndex items.
func migrateCatharsis(db *DB) error {
	db.logger.Info("starting catharsis migration")
	start := time.Now()
	var err error

	db.gcSize, err = db.shed.NewUint64Field("gc-size")
	if err != nil {
		return err
	}

	db.gcIndex, err = db.shed.NewIndex("AccessTimestamp|BinID|Hash->BatchID|BatchIndex", shed.IndexFuncs{
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
		return err
	}

	count, err := db.gcIndex.Count()
	if err != nil {
		return err
	}

	err = db.gcSize.Put(uint64(count))
	if err != nil {
		return fmt.Errorf("failed updating gcSize: %w", err)
	}

	db.logger.Info("catharsis done", "elapsed", time.Since(start), "new_gc_size", count)
	return nil
}
