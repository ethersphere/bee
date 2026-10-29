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
	stabilmock "github.com/ethersphere/bee/v2/pkg/stabilization/mock"
	"github.com/ethersphere/bee/v2/pkg/status"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	topMock "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
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

	service := salud.New(statusM, topM, reserve, log.Noop, stabilmock.NewSubscriber(true), "full", 0.8, 0.8)

	if got := value(t, service.SelfHealthyGauge()); got != 1 {
		t.Fatalf("self healthy gauge before the first cycle: got %v, want 1", got)
	}

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
	if got := value(t, service.SelfHealthyGauge()); got != 1 {
		t.Fatalf("self healthy gauge: got %v, want 1", got)
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

	service := salud.New(statusM, topM, reserve, log.Noop, stabilmock.NewSubscriber(true), "full", 0.8, 0.8)
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
	if got := value(t, service.SelfHealthyGauge()); got != 0 {
		t.Fatalf("self healthy gauge: got %v, want 0", got)
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

	service := salud.New(statusM, topM, reserve, log.Noop, stabilmock.NewSubscriber(true), "full", 0.8, 0.8)
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
	if got := value(t, service.SelfHealthyGauge()); got != 1 {
		t.Fatalf("self healthy gauge: got %v, want 1", got)
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

	service := salud.New(&statusMock{make(map[string]peer)}, topM, mockstorer.NewReserve(), log.Noop, stabilmock.NewSubscriber(true), "full", 0.8, 0.8)

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

	service := salud.New(&statusMock{make(map[string]peer)}, topM, mockstorer.NewReserve(), log.Noop, stabilmock.NewSubscriber(true), "full", 0.8, 0.8)
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

// value reads the current value of a gauge. The prometheus testutil package is
// not used because it needs an additional module in go.mod.
func value(t *testing.T, c prometheus.Metric) float64 {
	t.Helper()

	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatal(err)
	}
	return m.GetGauge().GetValue()
}
