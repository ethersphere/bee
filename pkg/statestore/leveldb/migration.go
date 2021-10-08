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
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/storage"
)

var (
	errMissingCurrentSchema = errors.New("could not find current db schema")
	errMissingTargetSchema  = errors.New("could not find target db schema")
)

const (
	dbSchemaKey = "statestore_schema"

	dbSchemaGrace           = "grace"
	dbSchemaDrain           = "drain"
	dbSchemaCleanInterval   = "clean-interval"
	dbSchemaNoStamp         = "no-stamp"
	dbSchemaFlushBlock      = "flushblock"
	dbSchemaSwapAddr        = "swapaddr"
	dBSchemaKademliaMetrics = "kademlia-metrics"
)

var (
	dbSchemaCurrent = dBSchemaKademliaMetrics
)

type migration struct {
	name string               // name of the schema
	fn   func(s *Store) error // the migration function that needs to be performed in order to get to the current schema name
}

// schemaMigrations contains an ordered list of the database schemes, that is
// in order to run data migrations in the correct sequence
var schemaMigrations = []migration{
	{name: dbSchemaGrace, fn: func(s *Store) error { return nil }},
	{name: dbSchemaDrain, fn: migrateGrace},
	{name: dbSchemaCleanInterval, fn: migrateGrace},
	{name: dbSchemaNoStamp, fn: migrateStamp},
	{name: dbSchemaFlushBlock, fn: migrateFB},
	{name: dbSchemaSwapAddr, fn: migrateSwap},
	{name: dBSchemaKademliaMetrics, fn: migrateKademliaMetrics},
}

func migrateFB(s *Store) error {
	collectedKeys, err := collectKeys(s, "blocklist-")
	if err != nil {
		return err
	}
	return deleteKeys(s, collectedKeys)
}

func migrateStamp(s *Store) error {
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

func migrateGrace(s *Store) error {
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

func migrateSwap(s *Store) error {
	migratePrefix := func(prefix string) error {
		keys, err := collectKeys(s, prefix)
		if err != nil {
			return err
		}

		for _, key := range keys {
			split := strings.SplitAfter(key, prefix)
			if len(split) != 2 {
				return errors.New("no peer in key")
			}

			if len(split[1]) != 20 {
				s.logger.Debugf("skipping already migrated key %s", key)
				continue
			}

			addr := common.BytesToAddress([]byte(split[1]))
			fixed := fmt.Sprintf("%s%x", prefix, addr)

			var val string
			if err = s.Get(fixed, &val); err == nil {
				s.logger.Debugf("skipping duplicate key %s", key)
				if err = s.Delete(key); err != nil {
					return err
				}
				continue
			}
			if !errors.Is(err, storage.ErrNotFound) {
				return err
			}

			if err = s.Get(key, &val); err != nil {
				return err
			}

			if err = s.Put(fixed, val); err != nil {
				return err
			}

			if err = s.Delete(key); err != nil {
				return err
			}
		}
		return nil
	}

	if err := migratePrefix("swap_peer_chequebook_"); err != nil {
		return err
	}

	return migratePrefix("swap_beneficiary_peer_")
}

// migrateKademliaMetrics removes all old existing
// kademlia metrics database content.
func migrateKademliaMetrics(s *Store) error {
	for _, prefix := range []string{"peer-last-seen-timestamp", "peer-total-connection-duration"} {
		start := time.Now()
		s.logger.Debugf("removing kademlia %q metrics", prefix)

		keys, err := collectKeys(s, prefix)
		if err != nil {
			return err
		}

		if err := deleteKeys(s, keys); err != nil {
			return err
		}

		s.logger.Debugf("removing kademlia %q metrics took %s", prefix, time.Since(start))
	}
	return nil
}

func (s *Store) migrate(schemaName string) error {
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
func getMigrations(currentSchema, targetSchema string, allSchemeMigrations []migration, store *Store) (migrations []migration, err error) {
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

func collectKeysExcept(s *Store, prefix []string) (keys []string, err error) {
	if err := s.Iterate("", func(k, v []byte) (bool, error) {
		stk := string(k)
		has := false
		for _, v := range prefix {
			if strings.HasPrefix(stk, v) {
				has = true
				break
			}
		}
		if !has {
			keys = append(keys, stk)
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	return keys, nil
}

func collectKeys(s *Store, prefix string) (keys []string, err error) {
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

func deleteKeys(s *Store, keys []string) error {
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

// Nuke the store so that only the bare essential entries are
// left. Careful!
func (s *Store) Nuke() error {
	var (
		keys     []string
		prefixes = []string{"accounting", "pseudosettle", "swap", "non-mineable-overlay"}
		err      error
	)
	keys, err = collectKeysExcept(s, prefixes)
	if err != nil {
		return fmt.Errorf("collect keys except: %w", err)
	}
	return deleteKeys(s, keys)
}
