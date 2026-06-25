// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestGossipDedup(t *testing.T) {
	t.Parallel()

	const ttl = 20 * time.Millisecond

	d := newGossipDedupCache(ttl, 0)
	t.Cleanup(func() { d.close() })

	addressee := swarm.RandAddress(t)
	peer := swarm.RandAddress(t)

	if d.contains(addressee, peer) {
		t.Fatal("unexpected hit on empty cache")
	}

	d.add(addressee, peer)
	if !d.contains(addressee, peer) {
		t.Fatal("want cache hit after add")
	}

	time.Sleep(ttl + 5*time.Millisecond)
	if d.contains(addressee, peer) {
		t.Fatal("want cache miss after ttl")
	}

	d.add(addressee, peer)
	d.clearAddressee(addressee)
	if d.contains(addressee, peer) {
		t.Fatal("want cache miss after clear")
	}
}

func TestGossipDedupPrune(t *testing.T) {
	t.Parallel()

	const ttl = 20 * time.Millisecond

	d := newGossipDedupCache(ttl, 0)
	t.Cleanup(func() { d.close() })

	addressee := swarm.RandAddress(t)
	peer := swarm.RandAddress(t)

	d.add(addressee, peer)
	time.Sleep(2 * ttl)
	d.prune()

	if d.contains(addressee, peer) {
		t.Fatal("want expired entry pruned")
	}
}
