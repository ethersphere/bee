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

// lastSeenSpy counts the addressbook writes hive performs, so that a sighting
// can be told apart from a record update.
type lastSeenSpy struct {
	ab.Interface
	mu    sync.Mutex
	puts  int
	seens int
}

func (s *lastSeenSpy) Put(o swarm.Address, a bzz.Address, v bool) error {
	s.mu.Lock()
	s.puts++
	s.mu.Unlock()
	return s.Interface.Put(o, a, v)
}

func (s *lastSeenSpy) Seen(o ...swarm.Address) error {
	s.mu.Lock()
	s.seens++
	s.mu.Unlock()
	return s.Interface.Seen(o...)
}

func (s *lastSeenSpy) counts() (puts, seens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.puts, s.seens
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

// TestSeenOnRepeatGossip covers the sighting that keeps gossip-only peers
// alive. A peer mints its bzz.Address once and re-presents that same signed
// record for its whole uptime, so there is nothing new to Put. It is still a
// sighting, and last-seen must move, or the pruner evicts a peer we are told
// about constantly.
func TestSeenOnRepeatGossip(t *testing.T) {
	t.Parallel()

	const networkID = uint64(1)

	base := time.Unix(1_700_000_000, 0)
	now := base
	svc, spy := newHiveWithSpy(t, networkID, &now)

	id := newPeerIdentity(t, networkID, "/ip4/10.0.0.1/tcp/1634")
	rec := id.protoAt(t, networkID, base.Unix())

	// first sighting: an unknown peer, stored by Put, which stamps last-seen.
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec}})
	if puts, seens := spy.counts(); puts != 1 || seens != 0 {
		t.Fatalf("first sighting: puts=%d seens=%d, want 1/0", puts, seens)
	}

	// ten days on, the same peer is still gossiped to us with the very same
	// record.
	now = base.Add(10 * 24 * time.Hour)
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec}})

	puts, seens := spy.counts()
	if puts != 1 {
		t.Fatalf("re-sighting stored the record again: puts=%d, want 1", puts)
	}
	if seens != 1 {
		t.Fatalf("re-sighting did not refresh last-seen: seens=%d, want 1", seens)
	}
}

// TestSeenOnNewerRecord asserts that a genuinely newer record still takes the
// Put path, where last-seen is stamped as part of the write.
func TestSeenOnNewerRecord(t *testing.T) {
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

	if puts, _ := spy.counts(); puts != 2 {
		t.Fatalf("newer record was not stored: puts=%d, want 2", puts)
	}
}

// TestSeenSkippedOnInvalidRecord makes sure a record we refuse to parse is not
// taken for a sighting. Only a record carrying the peer's own signature is
// evidence that we heard about that peer at all.
func TestSeenSkippedOnInvalidRecord(t *testing.T) {
	t.Parallel()

	const networkID = uint64(1)

	base := time.Unix(1_700_000_000, 0)
	now := base
	svc, spy := newHiveWithSpy(t, networkID, &now)

	id := newPeerIdentity(t, networkID, "/ip4/10.0.0.1/tcp/1634")
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{id.protoAt(t, networkID, base.Unix())}})

	// the record is signed for a different network, so it does not verify
	// against ours.
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{id.protoAt(t, networkID+1, base.Unix())}})

	if puts, seens := spy.counts(); puts != 1 || seens != 0 {
		t.Fatalf("invalid record touched the addressbook: puts=%d seens=%d, want 1/0", puts, seens)
	}
}
