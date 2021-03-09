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

package leveldb

import (
	"errors"
	"fmt"
)

var errMissingCurrentSchema = errors.New("could not find current db schema")
var errMissingTargetSchema = errors.New("could not find target db schema")

const (
	dbSchemaKey = "statestore_schema"

	dbSchemaGrace = "grace"
)

var (
	dbSchemaCurrent = dbSchemaGrace
)

type migration struct {
	name string               // name of the schema
	fn   func(s *store) error // the migration function that needs to be performed in order to get to the current schema name
}

// schemaMigrations contains an ordered list of the database schemes, that is
// in order to run data migrations in the correct sequence
var schemaMigrations = []migration{
	{name: dbSchemaGrace, fn: func(s *store) error { return nil }},
}

func (s *store) migrate(schemaName string) error {
	migrations, err := getMigrations(schemaName, dbSchemaCurrent, schemaMigrations, s)
	if err != nil {
		return fmt.Errorf("error getting migrations for current schema (%s): %w", schemaName, err)
	}

	// no migrations to run
	if migrations == nil {
		return nil
	}

	s.logger.Infof("statestore: need to run %d data migrations to schema %s", len(migrations), schemaName)
	for i := 0; i < len(migrations); i++ {
		err := migrations[i].fn(s)
		if err != nil {
			return err
		}
		err = s.putSchemaName(migrations[i].name) // put the name of the current schema
		if err != nil {
			return err
		}
		schemaName, err = s.getSchemaName()
		if err != nil {
			return err
		}
		s.logger.Infof("statestore: successfully ran migration: id %d current schema: %s", i, schemaName)
	}
	return nil
}

// getMigrations returns an ordered list of migrations that need be executed
// with no errors in order to bring the statestore to the most up-to-date
// schema definition
func getMigrations(currentSchema, targetSchema string, allSchemeMigrations []migration, store *store) (migrations []migration, err error) {
	foundCurrent := false
	foundTarget := false
	if currentSchema == dbSchemaCurrent {
		return nil, nil
	}
	for i, v := range allSchemeMigrations {
		switch v.name {
		case currentSchema:
			if foundCurrent {
				return nil, errors.New("found schema name for the second time when looking for migrations")
			}
			foundCurrent = true
			store.logger.Infof("statestore migration: found current schema %s, migrate to %s, total migrations %d", currentSchema, dbSchemaCurrent, len(allSchemeMigrations)-i)
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
