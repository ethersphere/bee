// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package persistent_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/addressbook/persistent"
	"github.com/ethersphere/bee/pkg/addressbook/test"
	"github.com/ethersphere/bee/pkg/statestore"
)

func TestPersistent(t *testing.T) {
	test.Run(t, func(t *testing.T) (addressbook.GetPutter, func()) {
		dir, err := ioutil.TempDir("", "statestore_test")
		if err != nil {
			t.Fatal(err)
		}

		store, err := statestore.New(dir)
		if err != nil {
			t.Fatal(err)
		}

		book := persistent.New(store)

		return book, func() {
			os.RemoveAll(dir)
		}
	})
}
