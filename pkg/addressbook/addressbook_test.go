// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package addressbook_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/statestore/persistent"
	"github.com/ethersphere/bee/pkg/swarm"

	ma "github.com/multiformats/go-multiaddr"
)

type bookFunc func(t *testing.T) (book addressbook.GetPutter, cleanup func())

func TestInMem(t *testing.T) {
	run(t, func(t *testing.T) (addressbook.GetPutter, func()) {
		store := mock.NewStateStore()
		book := addressbook.New(store)

		return book, func() {}
	})
}

func TestPersistent(t *testing.T) {
	run(t, func(t *testing.T) (addressbook.GetPutter, func()) {
		dir, err := ioutil.TempDir("", "statestore_test")
		if err != nil {
			t.Fatal(err)
		}

		store, err := persistent.NewStateStore(dir)
		if err != nil {
			t.Fatal(err)
		}

		book := addressbook.New(store)

		return book, func() {
			os.RemoveAll(dir)
		}
	})
}

func run(t *testing.T, f bookFunc) {
	store, cleanup := f(t)
	defer cleanup()

	addr1 := swarm.NewAddress([]byte{0, 1, 2, 3})
	addr2 := swarm.NewAddress([]byte{0, 1, 2, 4})
	multiaddr, err := ma.NewMultiaddr("/ip4/1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}

	err = store.Put(addr1, multiaddr)
	if err != nil {
		t.Fatal(err)
	}

	v, err := store.Get(addr1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Get(addr2)
	if err == nil {
		t.Fatal("value found in store but should not have been")
	}

	if multiaddr.String() != v.String() {
		t.Fatalf("value retrieved from store not equal to original stored address: %v, want %v", v, multiaddr)
	}
}
