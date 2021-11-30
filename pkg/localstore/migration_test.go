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
	"io"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
)

func TestOneMigration(t *testing.T) {
	defer func(v []migration, s string) {
		schemaMigrations = v
		DBSchemaCurrent = s
	}(schemaMigrations, DBSchemaCurrent)

	DBSchemaCurrent = DBSchemaCode
	dbSchemaNext := "dbSchemaNext"

	ran := false
	shouldNotRun := false
	schemaMigrations = []migration{
		{schemaName: DBSchemaCode, fn: func(db *DB) error {
			shouldNotRun = true // this should not be executed
			return nil
		}},
		{schemaName: dbSchemaNext, fn: func(db *DB) error {
			ran = true
			return nil
		}},
	}

	dir, err := os.MkdirTemp("", "localstore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	baseKey := make([]byte, 32)
	if _, err := rand.Read(baseKey); err != nil {
		t.Fatal(err)
	}

	logger := logging.New(io.Discard, 0)

	// start the fresh localstore with the sanctuary schema name
	db, err := New(dir, baseKey, nil, nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	DBSchemaCurrent = dbSchemaNext

	// start the existing localstore and expect the migration to run
	db, err = New(dir, baseKey, nil, nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	schemaName, err := db.schemaName.Get()
	if err != nil {
		t.Fatal(err)
	}

	if schemaName != dbSchemaNext {
		t.Errorf("schema name mismatch. got '%s', want '%s'", schemaName, dbSchemaNext)
	}

	if !ran {
		t.Errorf("expected migration did not run")
	}

	if shouldNotRun {
		t.Errorf("migration ran but shouldnt have")
	}

	err = db.Close()
	if err != nil {
		t.Error(err)
	}
}

func TestManyMigrations(t *testing.T) {
	defer func(v []migration, s string) {
		schemaMigrations = v
		DBSchemaCurrent = s
	}(schemaMigrations, DBSchemaCurrent)

	DBSchemaCurrent = DBSchemaCode

	shouldNotRun := false
	executionOrder := []int{-1, -1, -1, -1}

	schemaMigrations = []migration{
		{schemaName: DBSchemaCode, fn: func(db *DB) error {
			shouldNotRun = true // this should not be executed
			return nil
		}},
		{schemaName: "keju", fn: func(db *DB) error {
			executionOrder[0] = 0
			return nil
		}},
		{schemaName: "coconut", fn: func(db *DB) error {
			executionOrder[1] = 1
			return nil
		}},
		{schemaName: "mango", fn: func(db *DB) error {
			executionOrder[2] = 2
			return nil
		}},
		{schemaName: "salvation", fn: func(db *DB) error {
			executionOrder[3] = 3
			return nil
		}},
	}

	dir, err := os.MkdirTemp("", "localstore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	baseKey := make([]byte, 32)
	if _, err := rand.Read(baseKey); err != nil {
		t.Fatal(err)
	}
	logger := logging.New(io.Discard, 0)

	// start the fresh localstore with the sanctuary schema name
	db, err := New(dir, baseKey, nil, nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	DBSchemaCurrent = "salvation"

	// start the existing localstore and expect the migration to run
	db, err = New(dir, baseKey, nil, nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	schemaName, err := db.schemaName.Get()
	if err != nil {
		t.Fatal(err)
	}

	if schemaName != "salvation" {
		t.Errorf("schema name mismatch. got '%s', want '%s'", schemaName, "salvation")
	}

	if shouldNotRun {
		t.Errorf("migration ran but shouldnt have")
	}

	for i, v := range executionOrder {
		if i != v && i != len(executionOrder)-1 {
			t.Errorf("migration did not run in sequence, slot %d value %d", i, v)
		}
	}

	err = db.Close()
	if err != nil {
		t.Error(err)
	}
}

// TestMigrationErrorFrom checks that local store boot should fail when the schema we're migrating from cannot be found
func TestMigrationErrorFrom(t *testing.T) {
	defer func(v []migration, s string) {
		schemaMigrations = v
		DBSchemaCurrent = s
	}(schemaMigrations, DBSchemaCurrent)

	DBSchemaCurrent = "koo-koo-schema"

	shouldNotRun := false
	schemaMigrations = []migration{
		{schemaName: "langur", fn: func(db *DB) error {
			shouldNotRun = true
			return nil
		}},
		{schemaName: "coconut", fn: func(db *DB) error {
			shouldNotRun = true
			return nil
		}},
		{schemaName: "chutney", fn: func(db *DB) error {
			shouldNotRun = true
			return nil
		}},
	}

	dir, err := os.MkdirTemp("", "localstore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	baseKey := make([]byte, 32)
	if _, err := rand.Read(baseKey); err != nil {
		t.Fatal(err)
	}
	logger := logging.New(io.Discard, 0)

	// start the fresh localstore with the sanctuary schema name
	db, err := New(dir, baseKey, nil, nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	DBSchemaCurrent = "foo"

	// start the existing localstore and expect the migration to run
	_, err = New(dir, baseKey, nil, nil, logger)
	if !strings.Contains(err.Error(), errMissingCurrentSchema.Error()) {
		t.Fatalf("expected errCannotFindSchema but got %v", err)
	}

	if shouldNotRun {
		t.Errorf("migration ran but shouldnt have")
	}
}

// TestMigrationErrorTo checks that local store boot should fail when the schema we're migrating to cannot be found
func TestMigrationErrorTo(t *testing.T) {
	defer func(v []migration, s string) {
		schemaMigrations = v
		DBSchemaCurrent = s
	}(schemaMigrations, DBSchemaCurrent)

	DBSchemaCurrent = "langur"

	shouldNotRun := false
	schemaMigrations = []migration{
		{schemaName: "langur", fn: func(db *DB) error {
			shouldNotRun = true
			return nil
		}},
		{schemaName: "coconut", fn: func(db *DB) error {
			shouldNotRun = true
			return nil
		}},
		{schemaName: "chutney", fn: func(db *DB) error {
			shouldNotRun = true
			return nil
		}},
	}

	dir, err := os.MkdirTemp("", "localstore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	baseKey := make([]byte, 32)
	if _, err := rand.Read(baseKey); err != nil {
		t.Fatal(err)
	}

	logger := logging.New(io.Discard, 0)

	// start the fresh localstore with the sanctuary schema name
	db, err := New(dir, baseKey, nil, nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	DBSchemaCurrent = "foo"

	// start the existing localstore and expect the migration to run
	_, err = New(dir, baseKey, nil, nil, logger)
	if !strings.Contains(err.Error(), errMissingTargetSchema.Error()) {
		t.Fatalf("expected errMissingTargetSchema but got %v", err)
	}

	if shouldNotRun {
		t.Errorf("migration ran but shouldnt have")
	}
}
