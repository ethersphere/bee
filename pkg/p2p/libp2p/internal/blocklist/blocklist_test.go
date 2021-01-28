// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocklist_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/blocklist"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestExist(t *testing.T) {
	addr1 := swarm.NewAddress([]byte{0, 1, 2, 3})
	addr2 := swarm.NewAddress([]byte{4, 5, 6, 7})

	bl := blocklist.NewBlocklist(mock.NewStateStore())

	exists, err := bl.Exists(addr1)
	if err != nil {
		t.Fatal(err)
	}

	if exists {
		t.Fatal("got exists, expected not exists")
	}

	// add forever
	if err := bl.Add(addr1, 0); err != nil {
		t.Fatal(err)
	}

	// add for 50 miliseconds
	if err := bl.Add(addr2, time.Millisecond*50); err != nil {
		t.Fatal(err)
	}

	blocklist.SetTimeNow(func() time.Time { return time.Now().Add(100 * time.Millisecond) })
	defer func() { blocklist.SetTimeNow(time.Now) }()

	exists, err = bl.Exists(addr1)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("got not exists, expected exists")
	}

	exists, err = bl.Exists(addr2)
	if err != nil {
		t.Fatal(err)
	}

	if exists {
		t.Fatal("got  exists, expected not exists")
	}

}

func TestPeers(t *testing.T) {
	addr1 := swarm.NewAddress([]byte{0, 1, 2, 3})
	addr2 := swarm.NewAddress([]byte{4, 5, 6, 7})

	bl := blocklist.NewBlocklist(mock.NewStateStore())

	// add forever
	if err := bl.Add(addr1, 0); err != nil {
		t.Fatal(err)
	}

	// add for 50 miliseconds
	if err := bl.Add(addr2, time.Millisecond*50); err != nil {
		t.Fatal(err)
	}

	peers, err := bl.Peers()
	if err != nil {
		t.Fatal(err)
	}
	if !isIn(addr1, peers) {
		t.Fatalf("expected addr1 to exist in peers: %v", addr1)
	}

	if !isIn(addr2, peers) {
		t.Fatalf("expected addr2 to exist in peers: %v", addr2)
	}

	blocklist.SetTimeNow(func() time.Time { return time.Now().Add(100 * time.Millisecond) })
	defer func() { blocklist.SetTimeNow(time.Now) }()

	// now expect just one
	peers, err = bl.Peers()
	if err != nil {
		t.Fatal(err)
	}
	if !isIn(addr1, peers) {
		t.Fatalf("expected addr1 to exist in peers: %v", peers)
	}

	if isIn(addr2, peers) {
		t.Fatalf("expected addr2 to not exist in peers: %v", peers)
	}
}

func isIn(p swarm.Address, peers []p2p.Peer) bool {
	for _, v := range peers {
		if v.Address.Equal(p) {
			return true
		}
	}
	return false
}
