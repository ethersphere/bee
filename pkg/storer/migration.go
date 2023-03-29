// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	epochKey = "localstore-epoch"
)

func (db *DB) EpochMigration(stateStore storage.StateStorer, validStamp postage.ValidStampFn, path string, logger log.Logger) error {

	var ts uint64
	err := stateStore.Get(epochKey, &ts)
	if err == nil {
		return nil
	}

	if !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("get epoch key: %w", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) { // fresh node
		return nil
	}

	dbshed, err := shed.NewDB(path, nil)
	if err != nil {
		return fmt.Errorf("shed: %w", err)
	}

	// pull index allows history and live syncing per po bin
	pullIndex, err := dbshed.NewIndex("PO|BinID->Hash", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 9)
			key[0] = 0
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
		return err
	}

	// Index storing actual chunk address, data and bin id.
	headerSize := 16 + postage.StampSize
	retrievalDataIndex, err := dbshed.NewIndex("Address->StoreTimestamp|BinID|BatchID|BatchIndex|Sig|Location", shed.IndexFuncs{
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

	ctx := context.Background()

	err = pullIndex.Iterate(func(i shed.Item) (stop bool, err error) {

		addr := swarm.NewAddress(i.Address)
		_ = pullIndex.Delete(i)

		item, err := retrievalDataIndex.Get(i)
		if err != nil {
			logger.Debug("epoch migration: retrieval data index read failed", "chunk_address", addr, "error", err)
			return false, nil //continue
		}

		l, err := sharky.LocationFromBinary(item.Location)
		if err != nil {
			return false, err
		}

		item.Data = make([]byte, l.Length)
		err = db.sharky.Read(ctx, l, item.Data)
		if err != nil {
			logger.Debug("epoch migration: sharky read failed", "chunk_address", addr, "error", err)
			return false, err
		}

		ch := swarm.NewChunk(addr, item.Data).WithStamp(postage.NewStamp(item.BatchID, item.Index, item.Timestamp, item.Sig))

		err = db.sharky.Release(ctx, l)
		if err != nil {
			logger.Debug("epoch migration: release failed", "chunk_address", ch.Address(), "error", err)
			return false, err
		}

		if !cac.Valid(ch) && !soc.Valid(ch) {
			logger.Debug("epoch migration: invalid chunk", "chunk", ch)
			return false, nil //continue
		}

		if _, err := validStamp(ch); err != nil {
			logger.Debug("epoch migration: invalid stamp", "chunk_address", ch.Address(), "error", err)
			return false, nil //continue
		}

		return false, db.ReservePut(ctx, ch)
	}, nil)
	if err != nil {
		return err
	}

	matches, err := filepath.Glob(filepath.Join(path, "*"))
	if err != nil {
		return err
	}

	for _, m := range matches {
		if m != indexPath && m != sharkyPath {
			err = os.Remove(m)
			if err != nil {
				return err
			}
		}
	}

	return stateStore.Put(epochKey, time.Now().Unix())
}
