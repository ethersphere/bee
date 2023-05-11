// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package salud_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/salud"
	"github.com/ethersphere/bee/pkg/spinlock"
	"github.com/ethersphere/bee/pkg/status"
	"github.com/ethersphere/bee/pkg/swarm"
	topMock "github.com/ethersphere/bee/pkg/topology/mock"
)

type peer struct {
	addr    swarm.Address
	status  *status.Snapshot
	waitDur time.Duration
	health  bool
}

func TestSalud(t *testing.T) {

	peers := []peer{
		// fully healhy
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8}, 1, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8}, 1, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8}, 1, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8}, 1, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8}, 1, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8}, 1, true},

		// healthy since radius >= most common radius -  1
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 7}, 1, true},

		// not healthy radius too low
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 6}, 1, false},

		// dur too long
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8}, 2, false},

		// connections not enough
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 90, StorageRadius: 8}, 1, false},
	}

	statusM := &statusMock{make(map[string]peer)}

	var addrs []swarm.Address
	for _, p := range peers {
		addrs = append(addrs, p.addr)
		statusM.peers[p.addr.ByteString()] = p
	}

	topM := topMock.NewTopologyDriver(topMock.WithPeers(addrs...))

	_ = salud.New(statusM, topM, log.Noop, 0)

	spinlock.Wait(time.Minute, func() bool {
		return len(topM.PeersHealth()) == len(peers)
	})

	for _, p := range peers {
		if want, got := p.health, topM.PeersHealth()[p.addr.ByteString()]; want != got {
			t.Fatalf("got health %v, want %v for peer %s, %v", got, want, p.addr, p.status)
		}
	}
}

type statusMock struct {
	peers map[string]peer
}

func (p *statusMock) PeerSnapshot(ctx context.Context, peer swarm.Address) (*status.Snapshot, error) {
	if peer, ok := p.peers[peer.ByteString()]; ok {
		time.Sleep(peer.waitDur * time.Second)
		return peer.status, nil
	}
	return nil, errors.New("peer not found")
}
