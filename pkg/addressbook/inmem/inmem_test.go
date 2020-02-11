// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmem

import (
	"testing"

	"github.com/ethersphere/bee/pkg/swarm"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

func TestInMemStore(t *testing.T) {
	mem := New()
	addr1 := swarm.NewAddress([]byte{0, 1, 2, 3})
	addr2 := swarm.NewAddress([]byte{0, 1, 2, 4})
	under := libp2ppeer.ID("bcd")

	exists := mem.Put(under, addr1)
	if exists {
		t.Fatal("object exists in store but shouldnt")
	}

	_, exists = mem.Get(addr2)
	if exists {
		t.Fatal("value found in store but should not have been")
	}

	v, exists := mem.Get(addr1)
	if !exists {
		t.Fatal("value not found in store but should have been")
	}

	if under != v {
		t.Fatalf("value retrieved from store not equal to original stored address: %v, want %v", v, under)
	}
}
