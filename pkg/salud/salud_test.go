// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package salud_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/salud"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/status"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	topMock "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

type peer struct {
	addr    swarm.Address
	status  *status.Snapshot
	waitDur int
	health  bool
}

func TestSalud(t *testing.T) {
	t.Parallel()
	peers := []peer{
		// fully healhy
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", BatchCommitment: 50, ReserveSize: 100, CommittedDepth: 8}, 1, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", BatchCommitment: 50, ReserveSize: 100, CommittedDepth: 8}, 1, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", BatchCommitment: 50, ReserveSize: 100, CommittedDepth: 8}, 1, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", BatchCommitment: 50, ReserveSize: 100, CommittedDepth: 8}, 1, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", BatchCommitment: 50, ReserveSize: 100, CommittedDepth: 8}, 1, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", BatchCommitment: 50, ReserveSize: 100, CommittedDepth: 8}, 1, true},

		// healthy since radius >= most common radius - 2
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 7, BeeMode: "full", BatchCommitment: 50, ReserveSize: 100, CommittedDepth: 7}, 1, true},

		// radius too low
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 5, BeeMode: "full", BatchCommitment: 50, ReserveSize: 100, CommittedDepth: 5}, 1, false},

		// dur too long
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", BatchCommitment: 50, ReserveSize: 100, CommittedDepth: 8}, 2, false},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", BatchCommitment: 50, ReserveSize: 100, CommittedDepth: 8}, 2, false},

		// connections not enough
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 90, StorageRadius: 8, BeeMode: "full", BatchCommitment: 50, ReserveSize: 100, CommittedDepth: 8}, 1, false},

		// commitment wrong
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", BatchCommitment: 35, ReserveSize: 100, CommittedDepth: 8}, 1, false},
	}

	statusM := &statusMock{make(map[string]peer)}

	addrs := make([]swarm.Address, 0, len(peers))
	for _, p := range peers {
		addrs = append(addrs, p.addr)
		statusM.peers[p.addr.ByteString()] = p
	}

	topM := topMock.NewTopologyDriver(topMock.WithPeers(addrs...))

	reserve := mockstorer.NewReserve(
		mockstorer.WithRadius(6),
		mockstorer.WithReserveSize(100),
		mockstorer.WithCapacityDoubling(2),
	)

	service := salud.New(statusM, topM, reserve, log.Noop, -1, "full", 0, 0.8, 0.8)

	err := spinlock.Wait(time.Minute, func() bool {
		return len(topM.PeersHealth()) == len(peers)
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range peers {
		if want, got := p.health, topM.PeersHealth()[p.addr.ByteString()]; want != got {
			t.Fatalf("got health %v, want %v for peer %s, %v", got, want, p.addr, p.status)
		}
	}

	if !service.IsHealthy() {
		t.Fatalf("self should be healthy")
	}

	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSelfUnhealthyRadius(t *testing.T) {
	t.Parallel()
	peers := []peer{
		// fully healhy
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full"}, 0, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full"}, 0, true},
	}

	statusM := &statusMock{make(map[string]peer)}
	addrs := make([]swarm.Address, 0, len(peers))
	for _, p := range peers {
		addrs = append(addrs, p.addr)
		statusM.peers[p.addr.ByteString()] = p
	}

	topM := topMock.NewTopologyDriver(topMock.WithPeers(addrs...))

	reserve := mockstorer.NewReserve(
		mockstorer.WithRadius(7),
		mockstorer.WithReserveSize(100),
		mockstorer.WithCapacityDoubling(0),
	)

	service := salud.New(statusM, topM, reserve, log.Noop, -1, "full", 0, 0.8, 0.8)
	testutil.CleanupCloser(t, service)

	err := spinlock.Wait(time.Minute, func() bool {
		return len(topM.PeersHealth()) == len(peers)
	})
	if err != nil {
		t.Fatal(err)
	}

	if service.IsHealthy() {
		t.Fatalf("self should NOT be healthy")
	}
}

func TestSelfHealthyCapacityDoubling(t *testing.T) {
	t.Parallel()
	peers := []peer{
		// fully healhy
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", CommittedDepth: 8}, 0, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", CommittedDepth: 8}, 0, true},
	}

	statusM := &statusMock{make(map[string]peer)}
	addrs := make([]swarm.Address, 0, len(peers))
	for _, p := range peers {
		addrs = append(addrs, p.addr)
		statusM.peers[p.addr.ByteString()] = p
	}

	topM := topMock.NewTopologyDriver(topMock.WithPeers(addrs...))

	reserve := mockstorer.NewReserve(
		mockstorer.WithRadius(6),
		mockstorer.WithReserveSize(100),
		mockstorer.WithCapacityDoubling(2),
	)

	service := salud.New(statusM, topM, reserve, log.Noop, -1, "full", 0, 0.8, 0.8)
	testutil.CleanupCloser(t, service)

	err := spinlock.Wait(time.Minute, func() bool {
		return len(topM.PeersHealth()) == len(peers)
	})
	if err != nil {
		t.Fatal(err)
	}

	if !service.IsHealthy() {
		t.Fatalf("self should be healthy")
	}
}

func TestSubToRadius(t *testing.T) {
	t.Parallel()
	peers := []peer{
		// fully healhy
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", ReserveSize: 100}, 0, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", ReserveSize: 100}, 0, true},
	}

	addrs := make([]swarm.Address, 0, len(peers))
	for _, p := range peers {
		addrs = append(addrs, p.addr)
	}

	topM := topMock.NewTopologyDriver(topMock.WithPeers(addrs...))

	service := salud.New(&statusMock{make(map[string]peer)}, topM, mockstorer.NewReserve(), log.Noop, -1, "full", 0, 0.8, 0.8)

	c, unsub := service.SubscribeNetworkStorageRadius()
	t.Cleanup(unsub)

	select {
	case radius := <-c:
		if radius != 8 {
			t.Fatalf("wanted radius 8, got %d", radius)
		}
	case <-time.After(time.Second):
	}

	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestUnsub(t *testing.T) {
	t.Parallel()
	peers := []peer{
		// fully healhy
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", ReserveSize: 100}, 0, true},
		{swarm.RandAddress(t), &status.Snapshot{ConnectedPeers: 100, StorageRadius: 8, BeeMode: "full", ReserveSize: 100}, 0, true},
	}

	addrs := make([]swarm.Address, 0, len(peers))
	for _, p := range peers {
		addrs = append(addrs, p.addr)
	}

	topM := topMock.NewTopologyDriver(topMock.WithPeers(addrs...))

	service := salud.New(&statusMock{make(map[string]peer)}, topM, mockstorer.NewReserve(), log.Noop, -1, "full", 0, 0.8, 0.8)
	testutil.CleanupCloser(t, service)

	c, unsub := service.SubscribeNetworkStorageRadius()
	unsub()

	select {
	case <-c:
		t.Fatal("should not have received an address")
	case <-time.After(time.Second):
	}
}

type statusMock struct {
	peers map[string]peer
}

func (p *statusMock) PeerSnapshot(ctx context.Context, peer swarm.Address) (*status.Snapshot, error) {
	if peer, ok := p.peers[peer.ByteString()]; ok {
		time.Sleep(time.Duration(peer.waitDur) * time.Millisecond * 100)
		return peer.status, nil
	}
	return nil, errors.New("peer not found")
}
