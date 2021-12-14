package localstore

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/syndtr/goleveldb/leveldb"
)

const DBSchemaPinAll = "pin-all"

func migratePinAll(db *DB) error {
	start := time.Now()
	db.logger.Debug("purging the cache, then pinning everything else to pin counter 1")
	batch := new(leveldb.Batch)
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
		return err
	}

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

	// Persist gc size.
	gcSize, err := db.shed.NewUint64Field("gc-size")
	if err != nil {
		return err
	}

	// reserve size
	reserveSize, err := db.shed.NewUint64Field("reserve-size")
	if err != nil {
		return err
	}

	const limit = 10000
	i := 0

	err = gcIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		i++
		// delete from retrieve, pull, gc
		err = db.retrievalDataIndex.DeleteInBatch(batch, item)
		if err != nil {
			return false, err
		}
		err = db.retrievalAccessIndex.DeleteInBatch(batch, item)
		if err != nil {
			return false, err
		}
		err = db.pushIndex.DeleteInBatch(batch, item)
		if err != nil {
			return false, err
		}
		err = db.pullIndex.DeleteInBatch(batch, item)
		if err != nil {
			return false, err
		}
		err = db.gcIndex.DeleteInBatch(batch, item)
		if err != nil {
			return false, err
		}
		err = db.postageChunksIndex.DeleteInBatch(batch, item)
		if err != nil {
			return false, err
		}
		err = db.postageIndexIndex.DeleteInBatch(batch, item)
		if err != nil {
			return false, err
		}

		if i == limit {
			err = db.shed.WriteBatch(batch)
			if err != nil {
				return true, fmt.Errorf("write batch: %w", err)
			}
			i = 0
			batch.Reset()
		}

		return false, nil
	}, nil)
	if err != nil {
		return err
	}

	if err = db.shed.WriteBatch(batch); err != nil {
		return fmt.Errorf("write last batch: %w", err)
	}
	batch.Reset()

	if err = gcSize.Put(0); err != nil {
		return err
	}

	i = 0
	ii := uint64(0)
	err = pinIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		i++
		ii++
		if item.PinCounter > 1 {
			item.PinCounter = 1
		}

		if err = pinIndex.PutInBatch(batch, item); err != nil {
			return true, err
		}

		if i == limit {
			err = db.shed.WriteBatch(batch)
			if err != nil {
				return true, fmt.Errorf("write batch: %w", err)
			}
			i = 0
			batch.Reset()
		}
		return false, nil
	}, nil)
	if err != nil {
		return err
	}

	if err = db.shed.WriteBatch(batch); err != nil {
		return fmt.Errorf("write last batch: %w", err)
	}

	if err = reserveSize.Put(ii); err != nil {
		return err
	}

	db.logger.Infof("migration done. took %s", time.Since(start))

	return nil
}
