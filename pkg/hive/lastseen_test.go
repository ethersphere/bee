// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive_test

import (
	"sync"
	"testing"
	"time"

	ab "github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/hive"
	"github.com/ethersphere/bee/v2/pkg/hive/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// lastSeenSpy counts the addressbook writes hive performs, so we can tell a
// re-sighting (UpdateLastSeen) from a record update (Put).
type lastSeenSpy struct {
	ab.Interface
	mu      sync.Mutex
	puts    int
	updates int
}

func (s *lastSeenSpy) Put(o swarm.Address, a bzz.Address, v bool) error {
	s.mu.Lock()
	s.puts++
	s.mu.Unlock()
	return s.Interface.Put(o, a, v)
}

func (s *lastSeenSpy) UpdateLastSeen(o swarm.Address) error {
	s.mu.Lock()
	s.updates++
	s.mu.Unlock()
	return s.Interface.UpdateLastSeen(o)
}

func (s *lastSeenSpy) counts() (puts, updates int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.puts, s.updates
}

func newHiveWithSpy(t *testing.T, networkID uint64, now *time.Time) (*hive.Service, *lastSeenSpy) {
	t.Helper()

	spy := &lastSeenSpy{Interface: ab.New(mock.NewStateStore())}
	svc := hive.New(streamtest.New(), spy, networkID, swarm.RandAddress(t), log.Noop, hive.Options{
		AllowPrivateCIDRs: true,
	})
	svc.SetTimeFunc(func() time.Time { return *now })
	t.Cleanup(func() { _ = svc.Close() })
	return svc, spy
}

// TestGossipRefreshesLastSeenOnRepeatSighting covers the case that keeps
// gossip-only peers alive. A peer mints its bzz.Address once and re-presents
// that same signed record for its whole uptime, so CheckTimestamp rejects it
// as stale and there is nothing to Put. It is still a sighting, and last-seen
// must move, or the pruner eventually evicts a peer we hear about constantly.
func TestGossipRefreshesLastSeenOnRepeatSighting(t *testing.T) {
	t.Parallel()

	const networkID = uint64(1)

	base := time.Unix(1_700_000_000, 0)
	now := base
	svc, spy := newHiveWithSpy(t, networkID, &now)

	id := newPeerIdentity(t, networkID, "/ip4/10.0.0.1/tcp/1634")
	rec := id.protoAt(t, networkID, base.Unix())

	// first sighting: unknown peer, gets stored.
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec}})
	if puts, updates := spy.counts(); puts != 1 || updates != 0 {
		t.Fatalf("first sighting: puts=%d updates=%d, want 1/0", puts, updates)
	}

	// ten days later, the same peer is still being gossiped to us with the very
	// same record.
	now = base.Add(10 * 24 * time.Hour)
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec}})

	puts, updates := spy.counts()
	if puts != 1 {
		t.Fatalf("re-sighting stored the record again: puts=%d, want 1", puts)
	}
	if updates != 1 {
		t.Fatalf("re-sighting did not refresh last-seen: updates=%d, want 1", updates)
	}
}

// TestGossipUpdatesRecordWhenGenuinelyNewer asserts that a record newer than
// existing.Timestamp+MinimumUpdateInterval still takes the Put path, where
// last-seen is stamped as part of the write.
func TestGossipUpdatesRecordWhenGenuinelyNewer(t *testing.T) {
	t.Parallel()

	const networkID = uint64(1)

	base := time.Unix(1_700_000_000, 0)
	now := base
	svc, spy := newHiveWithSpy(t, networkID, &now)

	id := newPeerIdentity(t, networkID, "/ip4/10.0.0.1/tcp/1634")
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{id.protoAt(t, networkID, base.Unix())}})

	// re-minted beyond the minimum update interval.
	newer := base.Add(bzz.MinimumUpdateInterval + time.Second)
	now = newer
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{id.protoAt(t, networkID, newer.Unix())}})

	if puts, updates := spy.counts(); puts != 2 || updates != 0 {
		t.Fatalf("newer record: puts=%d updates=%d, want 2/0", puts, updates)
	}
}

// TestGossipDoesNotRefreshLastSeenOnInvalidRecord makes sure the refresh is
// scoped to records that merely are not newer. A record rejected for any other
// reason is not evidence that the peer is alive.
func TestGossipDoesNotRefreshLastSeenOnInvalidRecord(t *testing.T) {
	t.Parallel()

	const networkID = uint64(1)

	base := time.Unix(1_700_000_000, 0)
	now := base
	svc, spy := newHiveWithSpy(t, networkID, &now)

	id := newPeerIdentity(t, networkID, "/ip4/10.0.0.1/tcp/1634")
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{id.protoAt(t, networkID, base.Unix())}})

	// a record timestamped far in the future is rejected as invalid, not stale.
	future := base.Add(24 * time.Hour)
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{id.protoAt(t, networkID, future.Unix())}})

	if puts, updates := spy.counts(); puts != 1 || updates != 0 {
		t.Fatalf("invalid record touched the addressbook: puts=%d updates=%d, want 1/0", puts, updates)
	}
}
