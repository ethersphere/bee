// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"encoding/binary"
	"fmt"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
)

func (db *DB) Compact(sharkyBasePath string) error {

	s, err := sharky.NewCompaction(sharkyBasePath, sharkyNoOfShards, swarm.SocMaxChunkSize)
	if err != nil {
		return err
	}

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
		return err
	}

	var buf = make([]byte, 5000)

	err = retrievalDataIndex.Iterate(func(item shed.Item) (stop bool, err error) {

		oldLoc, err := sharky.LocationFromBinary(item.Location)
		if err != nil {
			return false, fmt.Errorf("location from binary: %w", err)
		}

		err = db.sharky.Read(oldLoc, buf)
		if err != nil {
			return false, fmt.Errorf("read from sharky: %w", err)
		}

		updated, newLoc, err := s.Write(oldLoc, buf[:oldLoc.Length])
		if err != nil {
			return false, fmt.Errorf("write to sharky: %w", err)
		}

		if !updated {
			return false, nil
		}

		lb, err := newLoc.MarshalBinary()
		if err != nil {
			return false, fmt.Errorf("location marshall binary: %w", err)
		}

		item.Location = lb

		return false, retrievalDataIndex.Put(item)
	}, nil)

	if err != nil {
		return err
	}

	return s.Close()
}
