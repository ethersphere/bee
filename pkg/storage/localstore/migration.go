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

	"github.com/ethersphere/swarm/chunk"
	"github.com/ethersphere/swarm/log"
	"github.com/ethersphere/swarm/shed"
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
	{name: DbSchemaPurity, fn: func(db *DB) error { return nil }},
	{name: DbSchemaHalloween, fn: func(db *DB) error { return nil }},
	{name: DbSchemaSanctuary, fn: func(db *DB) error { return nil }},
	{name: DbSchemaDiwali, fn: migrateSanctuary},
}

func (db *DB) migrate(schemaName string) error {
	migrations, err := getMigrations(schemaName, DbSchemaCurrent, schemaMigrations)
	if err != nil {
		return fmt.Errorf("error getting migrations for current schema (%s): %v", schemaName, err)
	}

	// no migrations to run
	if migrations == nil {
		return nil
	}

	log.Info("need to run data migrations on localstore", "numMigrations", len(migrations), "schemaName", schemaName)
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
		log.Info("successfully ran migration", "migrationId", i, "currentSchema", schemaName)
	}
	return nil
}

// migrationFn is a function that takes a localstore.DB and
// returns an error if a migration has failed
type migrationFn func(db *DB) error

// getMigrations returns an ordered list of migrations that need be executed
// with no errors in order to bring the localstore to the most up-to-date
// schema definition
func getMigrations(currentSchema, targetSchema string, allSchemeMigrations []migration) (migrations []migration, err error) {
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
			log.Info("found current localstore schema", "currentSchema", currentSchema, "migrateTo", DbSchemaCurrent, "total migrations", len(allSchemeMigrations)-i)
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

// this function migrates Sanctuary schema to the Diwali schema
func migrateSanctuary(db *DB) error {
	// just rename the pull index
	renamed, err := db.shed.RenameIndex("PO|BinID->Hash", "PO|BinID->Hash|Tag")
	if err != nil {
		return err
	}
	if !renamed {
		return errors.New("pull index was not successfully renamed")
	}

	if db.tags == nil {
		return errors.New("had an error accessing the tags object")
	}

	batch := new(leveldb.Batch)
	db.batchMu.Lock()
	defer db.batchMu.Unlock()

	// since pullIndex points to the Tag value, we should eliminate possible
	// pushIndex leak due to items that were used by previous pull sync tag
	// increment logic. we need to build the index first since db object is
	// still not initialised at this stage
	db.pushIndex, err = db.shed.NewIndex("StoreTimestamp|Hash->Tags", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 40)
			binary.BigEndian.PutUint64(key[:8], uint64(fields.StoreTimestamp))
			copy(key[8:], fields.Address[:])
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
			if value != nil {
				e.Tag = binary.BigEndian.Uint32(value)
			}
			return e, nil
		},
	})

	err = db.pushIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		tag, err := db.tags.Get(item.Tag)
		if err != nil {
			if err == chunk.TagNotFoundErr {
				return false, nil
			}
			return true, err
		}

		// anonymous tags should no longer appear in pushIndex
		if tag != nil && tag.Anonymous {
			db.pushIndex.DeleteInBatch(batch, item)
		}
		return false, nil
	}, nil)
	if err != nil {
		return err
	}

	return db.shed.WriteBatch(batch)
}
