package localstore

const DBSchemaPinAll = "pin-all"

// func migratePinAll(db *DB) error {
// 	start := time.Now()
// 	db.logger.Debug("purging the cache, then pinning everything else to pin counter 1")
// 	batch := new(leveldb.Batch)
// 	headerSize := 16 + postage.StampSize
// 	retrievalDataIndex, err := db.shed.NewIndex("Address->StoreTimestamp|BinID|BatchID|BatchIndex|Sig|Data", shed.IndexFuncs{
// 		EncodeKey: func(fields shed.Item) (key []byte, err error) {
// 			return fields.Address, nil
// 		},
// 		DecodeKey: func(key []byte) (e shed.Item, err error) {
// 			e.Address = key
// 			return e, nil
// 		},
// 		EncodeValue: func(fields shed.Item) (value []byte, err error) {
// 			b := make([]byte, headerSize)
// 			binary.BigEndian.PutUint64(b[:8], fields.BinID)
// 			binary.BigEndian.PutUint64(b[8:16], uint64(fields.StoreTimestamp))
// 			stamp, err := postage.NewStamp(fields.BatchID, fields.Index, fields.Timestamp, fields.Sig).MarshalBinary()
// 			if err != nil {
// 				return nil, err
// 			}
// 			copy(b[16:], stamp)
// 			value = append(b, fields.Data...)
// 			return value, nil
// 		},
// 		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
// 			e.StoreTimestamp = int64(binary.BigEndian.Uint64(value[8:16]))
// 			e.BinID = binary.BigEndian.Uint64(value[:8])
// 			stamp := new(postage.Stamp)
// 			if err = stamp.UnmarshalBinary(value[16:headerSize]); err != nil {
// 				return e, err
// 			}
// 			e.BatchID = stamp.BatchID()
// 			e.Index = stamp.Index()
// 			e.Timestamp = stamp.Timestamp()
// 			e.Sig = stamp.Sig()
// 			e.Data = value[headerSize:]
// 			return e, nil
// 		},
// 	})
// 	if err != nil {
// 		return err
// 	}

// 	pinIndex, err := db.shed.NewIndex("Hash->PinCounter", shed.IndexFuncs{
// 		EncodeKey: func(fields shed.Item) (key []byte, err error) {
// 			return fields.Address, nil
// 		},
// 		DecodeKey: func(key []byte) (e shed.Item, err error) {
// 			e.Address = key
// 			return e, nil
// 		},
// 		EncodeValue: func(fields shed.Item) (value []byte, err error) {
// 			b := make([]byte, 8)
// 			binary.BigEndian.PutUint64(b[:8], fields.PinCounter)
// 			return b, nil
// 		},
// 		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
// 			e.PinCounter = binary.BigEndian.Uint64(value[:8])
// 			return e, nil
// 		},
// 	})
// 	if err != nil {
// 		return err
// 	}

// 	pushIndex, err := db.shed.NewIndex("StoreTimestamp|Hash->Tags", shed.IndexFuncs{
// 		EncodeKey: func(fields shed.Item) (key []byte, err error) {
// 			key = make([]byte, 40)
// 			binary.BigEndian.PutUint64(key[:8], uint64(fields.StoreTimestamp))
// 			copy(key[8:], fields.Address)
// 			return key, nil
// 		},
// 		DecodeKey: func(key []byte) (e shed.Item, err error) {
// 			e.Address = key[8:]
// 			e.StoreTimestamp = int64(binary.BigEndian.Uint64(key[:8]))
// 			return e, nil
// 		},
// 		EncodeValue: func(fields shed.Item) (value []byte, err error) {
// 			tag := make([]byte, 4)
// 			binary.BigEndian.PutUint32(tag, fields.Tag)
// 			return tag, nil
// 		},
// 		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
// 			if len(value) == 4 { // only values with tag should be decoded
// 				e.Tag = binary.BigEndian.Uint32(value)
// 			}
// 			return e, nil
// 		},
// 	})
// 	if err != nil {
// 		return err
// 	}
// 	const limit = 10000
// 	i := 0

// 	err = db.gcIndex.Iterate(func(item shed.Item) (stop bool, err error) {
// 		i++
// 		// delete from retrieve, pull, gc
// 		err = db.retrievalDataIndex.DeleteInBatch(batch, item)
// 		if err != nil {
// 			return false, err
// 		}
// 		err = db.retrievalAccessIndex.DeleteInBatch(batch, item)
// 		if err != nil {
// 			return false, err
// 		}
// 		err = db.pushIndex.DeleteInBatch(batch, item)
// 		if err != nil {
// 			return false, err
// 		}
// 		err = db.pullIndex.DeleteInBatch(batch, item)
// 		if err != nil {
// 			return false, err
// 		}
// 		err = db.gcIndex.DeleteInBatch(batch, item)
// 		if err != nil {
// 			return false, err
// 		}
// 		err = db.postageChunksIndex.DeleteInBatch(batch, item)
// 		if err != nil {
// 			return false, err
// 		}
// 		err = db.postageIndexIndex.DeleteInBatch(batch, item)
// 		if err != nil {
// 			return false, err
// 		}

// 		if i == limit {
// 			err = db.shed.WriteBatch(batch)
// 			if err != nil {
// 				return true, fmt.Errorf("write batch: %w", err)
// 			}
// 			i = 0
// 			batch.Reset()
// 		}

// 		return false, nil
// 	}, nil)
// 	if err != nil {
// 		return err
// 	}

// 	err = db.shed.WriteBatch(batch)
// 	if err != nil {
// 		return fmt.Errorf("write last batch: %w", err)
// 	}

// 	return nil
// }
