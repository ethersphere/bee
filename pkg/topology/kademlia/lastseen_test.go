// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/addressbook"
	beeCrypto "github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/discovery/mock"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/stabilization"
	mockstate "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology/kademlia"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

type spyBook struct {
	addressbook.Interface
	mu   sync.Mutex
	seen map[string]int
}

func (s *spyBook) Seen(overlays ...swarm.Address) error {
	s.mu.Lock()
	for _, o := range overlays {
		s.seen[o.String()]++
	}
	s.mu.Unlock()
	return s.Interface.Seen(overlays...)
}

func (s *spyBook) count(o swarm.Address) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen[o.String()]
}

// TestMarkConnectedPeersSeen covers the sweep the manage loop runs on every
// lastSeenRefreshInterval tick. A peer we hold a connection to is seen
// continuously, so marking it on the connect event alone would let the
// addressbook pruner evict our longest-lived peers.
func TestMarkConnectedPeersSeen(t *testing.T) {
	t.Parallel()

	detector, err := stabilization.NewDetector(stabilization.Config{
		PeriodDuration:             1 * time.Second,
		NumPeriodsForStabilization: 2,
		StabilizationFactor:        1,
		WarmupTime:                 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	var conns, failed int32
	spy := &spyBook{Interface: addressbook.New(mockstate.NewStateStore()), seen: map[string]int{}}
	base := swarm.RandAddress(t)
	disc := mock.NewDiscovery()

	pk, _ := beeCrypto.GenerateSecp256k1Key()
	signer := beeCrypto.NewDefaultSigner(pk)
	p2ps := p2pMock(t, spy, signer, &conns, &failed)

	bit := -1
	kad, err := kademlia.New(base, spy, disc, p2ps, detector, log.Noop, kademlia.Options{
		BitSuffixLength: &bit,
		ExcludeFunc:     defaultExcludeFunc,
	})
	if err != nil {
		t.Fatal(err)
	}
	p2ps.SetPickyNotifier(kad)
	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)
	kad.SetStorageRadius(0)

	connected := swarm.RandAddress(t)
	connectOne(t, signer, kad, spy, connected, nil)

	if err := kad.MarkConnectedPeersSeen(); err != nil {
		t.Fatal(err)
	}

	if got := spy.count(connected); got != 1 {
		t.Fatalf("connected peer marked seen %d times, want 1", got)
	}
}
