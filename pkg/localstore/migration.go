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
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
)

var errMissingCurrentSchema = errors.New("could not find current db schema")
var errMissingTargetSchema = errors.New("could not find target db schema")

type migration struct {
	schemaName string
	// The migration function that needs to be performed
	// in order to get to the current schema name.
	fn func(db *DB) error
}

// schemaMigrations contains an ordered list of the database schemes, that is
// in order to run data migrations in the correct sequence
var schemaMigrations = []migration{
	{schemaName: DBSchemaCode, fn: func(*DB) error { return nil }},
	{schemaName: DBSchemaYuj, fn: migrateYuj},
	{schemaName: DBSchemaBatchIndex, fn: migrateBatchIndex},
}

func (db *DB) migrate(schemaName string) error {
	migrations, err := getMigrations(schemaName, DBSchemaCurrent, schemaMigrations, db)
	if err != nil {
		return fmt.Errorf("error getting migrations for current schema %q: %w", schemaName, err)
	}

	if len(migrations) == 0 {
		return nil
	}

	db.logger.Infof("localstore migration: need to run %v data migrations on localstore to schema %s", len(migrations), schemaName)
	for i, migration := range migrations {
		if err := migration.fn(db); err != nil {
			return err
		}
		if err = db.schemaName.Put(migration.schemaName); err != nil {
			return err
		}
		if schemaName, err = db.schemaName.Get(); err != nil {
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
	if currentSchema == DBSchemaCurrent {
		return nil, nil
	}

	var (
		foundCurrent = false
		foundTarget  = false
	)
	for i, v := range allSchemeMigrations {
		switch v.schemaName {
		case currentSchema:
			if foundCurrent {
				return nil, errors.New("found schema name for the second time when looking for migrations")
			}
			foundCurrent = true
			db.logger.Infof("localstore migration: found current localstore schema %s, migrate to %s, total migrations %d", currentSchema, DBSchemaCurrent, len(allSchemeMigrations)-i)
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

// truncateIndex truncates the given index for the given db.
func truncateIndex(db *DB, idx shed.Index) (n int, err error) {
	const maxBatchSize = 10000

	batch := db.shed.GetBatch(true)
	if err = idx.Iterate(func(item shed.Item) (stop bool, err error) {
		if err = idx.DeleteInBatch(batch, item); err != nil {
			return true, err
		}
		db.logger.Debugf("truncateIndex: deleted %x", item.Address)

		if n++; n%maxBatchSize == 0 {
			db.logger.Debugf("truncateIndex: writing batch; processed %d", n)
			err := db.shed.WriteBatch(batch)
			if err != nil {
				return true, err
			}
			batch.Reset()
		}

		return false, nil
	}, nil); err != nil {
		return n, err
	}
	return n, db.shed.WriteBatch(batch)
}
