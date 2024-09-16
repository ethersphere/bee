// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocklist_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/blocklist"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestExist(t *testing.T) {
	t.Parallel()

	addr1 := swarm.NewAddress([]byte{0, 1, 2, 3})
	addr2 := swarm.NewAddress([]byte{4, 5, 6, 7})
	ctMock := &currentTimeMock{}

	bl := blocklist.NewBlocklistWithCurrentTimeFn(mock.NewStateStore(), ctMock.Time)

	exists, err := bl.Exists(addr1)
	if err != nil {
		t.Fatal(err)
	}

	if exists {
		t.Fatal("got exists, expected not exists")
	}

	// add forever
	if err := bl.Add(addr1, 0, "", false); err != nil {
		t.Fatal(err)
	}

	// add for 50 milliseconds
	if err := bl.Add(addr2, time.Millisecond*50, "", false); err != nil {
		t.Fatal(err)
	}

	ctMock.SetTime(time.Now().Add(100 * time.Millisecond))

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
	t.Parallel()

	addr1 := swarm.NewAddress([]byte{0, 1, 2, 3})
	addr2 := swarm.NewAddress([]byte{4, 5, 6, 7})
	ctMock := &currentTimeMock{}

	bl := blocklist.NewBlocklistWithCurrentTimeFn(mock.NewStateStore(), ctMock.Time)

	// add forever
	if err := bl.Add(addr1, 0, "r1", true); err != nil {
		t.Fatal(err)
	}

	// add for 50 milliseconds
	if err := bl.Add(addr2, time.Millisecond*50, "r2", true); err != nil {
		t.Fatal(err)
	}

	peers, err := bl.Peers()
	if err != nil {
		t.Fatal(err)
	}
	if !isIn(addr1, peers, "r1", true) {
		t.Fatalf("expected addr1 to exist in peers: %v", addr1)
	}

	if !isIn(addr2, peers, "r2", true) {
		t.Fatalf("expected addr2 to exist in peers: %v", addr2)
	}

	ctMock.SetTime(time.Now().Add(100 * time.Millisecond))

	// now expect just one
	peers, err = bl.Peers()
	if err != nil {
		t.Fatal(err)
	}
	if !isIn(addr1, peers, "r1", true) {
		t.Fatalf("expected addr1 to exist in peers: %v", peers)
	}

	if isIn(addr2, peers, "r2", true) {
		t.Fatalf("expected addr2 to not exist in peers: %v", peers)
	}
}

func isIn(p swarm.Address, peers []p2p.BlockListedPeer, reason string, f bool) bool {
	for _, v := range peers {
		if v.Address.Equal(p) && v.Reason == reason && v.Peer.FullNode == f {
			return true
		}
	}
	return false
}

type currentTimeMock struct {
	time time.Time
}

func (c *currentTimeMock) Time() time.Time {
	return c.time
}

func (c *currentTimeMock) SetTime(t time.Time) {
	c.time = t
}
