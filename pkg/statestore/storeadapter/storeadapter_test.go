// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/statestore/test"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/pkg/storage/leveldbstore"
)

func TestStateStoreAdapter(t *testing.T) {
	t.Parallel()

	test.Run(t, func(t *testing.T) storage.StateStorer {
		t.Helper()

		store := storage.NewStateStorerAdapter(inmemstore.New())
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
		})

		// The test requires the state store to have
		// a schema, otherwise the delete test fails.
		if err := store.Put("test_schema", "name"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		return store
	})

	test.RunPersist(t, func(t *testing.T, dir string) storage.StateStorer {
		t.Helper()

		leveldb, err := leveldbstore.New(dir, nil)
		if err != nil {
			t.Fatal(err)
		}

		return storage.NewStateStorerAdapter(leveldb)
	})
}
