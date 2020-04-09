// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/swarm"

	ma "github.com/multiformats/go-multiaddr"
)

type bookFunc func(t *testing.T) (book addressbook.GetPutter, cleanup func())

func Run(t *testing.T, f bookFunc) {
	testGetPut(t, f)
}

func testGetPut(t *testing.T, f bookFunc) {
	store, cleanup := f(t)
	defer cleanup()

	addr1 := swarm.NewAddress([]byte{0, 1, 2, 3})
	addr2 := swarm.NewAddress([]byte{0, 1, 2, 4})
	multiaddr, err := ma.NewMultiaddr("/ip4/1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}

	exists, err := store.Put(addr1, multiaddr)
	if err != nil {
		t.Fatal(err)
	}

	if exists {
		t.Fatal("object exists in store but shouldnt")
	}

	v, err = store.Get(addr1)
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
