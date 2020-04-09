// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.package storage

package persistent_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/statestore/persistent"
	"github.com/ethersphere/bee/pkg/statestore/test"
	"github.com/ethersphere/bee/pkg/storage"
)

func TestPersistentStateStore(t *testing.T) {
	test.Run(t, func(t *testing.T) (storage.StateStorer, func()) {
		dir, err := ioutil.TempDir("", "statestore_test")
		if err != nil {
			t.Fatal(err)
		}

		store, err := persistent.NewStateStore(dir)
		if err != nil {
			t.Fatal(err)
		}

		return store, func() { os.RemoveAll(dir) }
	})

	test.RunPersist(t, func(t *testing.T, dir string) storage.StateStorer {

		store, err := persistent.NewStateStore(dir)
		if err != nil {
			t.Fatal(err)
		}

		return store
	})
}
