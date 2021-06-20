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
	"strings"
)

var (
	errMissingCurrentSchema = errors.New("could not find current db schema")
	errMissingTargetSchema  = errors.New("could not find target db schema")
)

const (
	dbSchemaKey = "statestore_schema"

	dbSchemaGrace         = "grace"
	dbSchemaDrain         = "drain"
	dbSchemaCleanInterval = "clean-interval"
	dbSchemaNoStamp       = "no-stamp"
	dbSchemaIdk           = "idk"
)

var (
	dbSchemaCurrent = dbSchemaIdk
)

type migration struct {
	name string               // name of the schema
	fn   func(s *store) error // the migration function that needs to be performed in order to get to the current schema name
}

// schemaMigrations contains an ordered list of the database schemes, that is
// in order to run data migrations in the correct sequence
var schemaMigrations = []migration{
	{name: dbSchemaGrace, fn: func(s *store) error { return nil }},
	{name: dbSchemaDrain, fn: migrateGrace},
	{name: dbSchemaCleanInterval, fn: migrateGrace},
	{name: dbSchemaNoStamp, fn: migrateStamp},
	{name: dbSchemaIdk, fn: migrateIdk},
}

func migrateIdk(s *store) error {
	collectedKeys, err := collectKeys(s, "blocklist-")
	if err != nil {
		return err
	}
	return deleteKeys(s, collectedKeys)
}

func migrateStamp(s *store) error {
	for _, pfx := range []string{"postage", "batchstore", "addressbook_entry_"} {
		collectedKeys, err := collectKeys(s, pfx)
		if err != nil {
			return err
		}
		if err := deleteKeys(s, collectedKeys); err != nil {
			return err
		}
	}

	return nil
}

func migrateGrace(s *store) error {
	var collectedKeys []string
	mgfn := func(k, v []byte) (bool, error) {
		stk := string(k)
		if strings.Contains(stk, "|") &&
			len(k) > 32 &&
			!strings.Contains(stk, "swap") &&
			!strings.Contains(stk, "peer") {
			s.logger.Debugf("found key designated to deletion %s", k)
			collectedKeys = append(collectedKeys, stk)
		}

		return false, nil
	}

	_ = s.Iterate("", mgfn)

	for _, v := range collectedKeys {
		err := s.Delete(v)
		if err != nil {
			s.logger.Debugf("error deleting key %s", v)
			continue
		}
		s.logger.Debugf("deleted key %s", v)
	}
	s.logger.Debugf("deleted keys: %d", len(collectedKeys))

	return nil
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

	s.logger.Debugf("statestore: need to run %d data migrations to schema %s", len(migrations), schemaName)
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
		s.logger.Debugf("statestore: successfully ran migration: id %d current schema: %s", i, schemaName)
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
			store.logger.Debugf("statestore migration: found current schema %s, migrate to %s, total migrations %d", currentSchema, dbSchemaCurrent, len(allSchemeMigrations)-i)
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

func collectKeys(s *store, prefix string) (keys []string, err error) {
	if err := s.Iterate(prefix, func(k, v []byte) (bool, error) {
		stk := string(k)
		if strings.HasPrefix(stk, prefix) {
			keys = append(keys, stk)
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	return keys, nil
}

func deleteKeys(s *store, keys []string) error {
	for _, v := range keys {
		err := s.Delete(v)
		if err != nil {
			return fmt.Errorf("error deleting key %s: %w", v, err)
		}
		s.logger.Debugf("deleted key %s", v)
	}
	s.logger.Debugf("deleted keys: %d", len(keys))
	return nil
}
