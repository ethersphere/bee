// Copyright 2019 The Swarm Authors
// This file is part of the Swarm library.
//
// The Swarm library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Swarm library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Swarm library. If not, see <http://www.gnu.org/licenses/>.

package localstore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

var errMissingCurrentSchema = errors.New("could not find current db schema")
var errMissingTargetSchema = errors.New("could not find target db schema")

type migration struct {
	name string             // name of the schema
	fn   func(db *DB) error // the migration function that needs to be performed in order to get to the current schema name
}

// schemaMigrations contains an ordered list of the database schemes, that is
// in order to run data migrations in the correct sequence
var schemaMigrations = []migration{
	{name: DbSchemaCode, fn: func(_ *DB) error { return nil }},
	{name: DbSchemaYuj, fn: migrateYuj},
}

func (db *DB) migrate(schemaName string) error {
	migrations, err := getMigrations(schemaName, DbSchemaCurrent, schemaMigrations, db)
	if err != nil {
		return fmt.Errorf("error getting migrations for current schema (%s): %v", schemaName, err)
	}

	// no migrations to run
	if migrations == nil {
		return nil
	}

	db.logger.Infof("localstore migration: need to run %v data migrations on localstore to schema %s", len(migrations), schemaName)
	for i := 0; i < len(migrations); i++ {
		err := migrations[i].fn(db)
		if err != nil {
			return err
		}
		err = db.schemaName.Put(migrations[i].name) // put the name of the current schema
		if err != nil {
			return err
		}
		schemaName, err = db.schemaName.Get()
		if err != nil {
			return err
		}
		db.logger.Infof("localstore migration: successfully ran migration: id %v current schema: %s", i, schemaName)
	}
	return nil
}

// getMigrations returns an ordered list of migrations that need be executed
// with no errors in order to bring the localstore to the most up-to-date
// schema definition
func getMigrations(currentSchema, targetSchema string, allSchemeMigrations []migration, db *DB) (migrations []migration, err error) {
	foundCurrent := false
	foundTarget := false
	if currentSchema == DbSchemaCurrent {
		return nil, nil
	}
	for i, v := range allSchemeMigrations {
		switch v.name {
		case currentSchema:
			if foundCurrent {
				return nil, errors.New("found schema name for the second time when looking for migrations")
			}
			foundCurrent = true
			db.logger.Infof("localstore migration: found current localstore schema %s, migrate to %s, total migrations %d", currentSchema, DbSchemaCurrent, len(allSchemeMigrations)-i)
			continue // current schema migration should not be executed (already has been when schema was migrated to)
		case targetSchema:
			foundTarget = true
		}
		if foundCurrent {
			migrations = append(migrations, v)
		}
	}
	if !foundCurrent {
		return nil, errMissingCurrentSchema
	}
	if !foundTarget {
		return nil, errMissingTargetSchema
	}
	return migrations, nil
}

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

	hasPinned := false
	_ = pinIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		hasPinned = true
		return true, nil
	}, nil)
	if hasPinned {
		return errors.New("failed to update your node due to the existence of pinned content. Please refer to the release notes on how to safely migrate your pinned content.")
	}

	// define the old indexes from the previous schema
	// and swipe them clean.

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

	var lim = 10000
	count := 0

	truncate := func(i shed.Index) error {
		batch := new(leveldb.Batch)
		count = 0

		err := i.Iterate(func(item shed.Item) (stop bool, err error) {
			if err = i.DeleteInBatch(batch, item); err != nil {
				return true, err
			}
			count++
			if count%lim == 0 {
				db.logger.Debugf("truncate writing batch. processed %d", count)
				err := db.shed.WriteBatch(batch)
				if err != nil {
					return true, err
				}
				batch = new(leveldb.Batch)
			}
			return false, nil
		}, nil)
		if err != nil {
			return err
		}
		return db.shed.WriteBatch(batch)
	}
	start := time.Now()
	db.logger.Debug("truncating indexes")

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
		err := truncate(v.idx)
		if err != nil {
			return fmt.Errorf("truncate %s index: %w", v.name, err)
		}
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
