// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldb_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/pkg/statestore/test"
	"github.com/ethersphere/bee/pkg/storage"
)

func TestPersistentStateStore(t *testing.T) {
	test.Run(t, func(t *testing.T) storage.StateStorer {
		dir := t.TempDir()

		store, err := leveldb.NewStateStore(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
		})

		return store
	})

	test.RunPersist(t, func(t *testing.T, dir string) storage.StateStorer {
		store, err := leveldb.NewStateStore(dir, nil)
		if err != nil {
			t.Fatal(err)
		}

		return store
	})
}

func TestGetSchemaName(t *testing.T) {
	dir := t.TempDir()

	store, err := leveldb.NewStateStore(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	})
	n, err := store.GetSchemaName() // expect current
	if err != nil {
		t.Fatal(err)
	}
	if n != leveldb.DbSchemaCurrent {
		t.Fatalf("wanted current db schema but got '%s'", n)
	}
}
