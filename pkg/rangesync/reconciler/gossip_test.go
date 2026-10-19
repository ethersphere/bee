// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// newGossipHarness returns a reconciler that is not started: the test runs
// its rounds (Round) and Status gossip passes (Gossip) itself, at the times
// of the fake clock.
func newGossipHarness(t *testing.T, svc *fakeService, peers ...swarm.Address) *harness {
	t.Helper()
	h := &harness{
		svc:   svc,
		topo:  &fakeTopology{peers: peers},
		clock: &fakeClock{now: time.Unix(1_000_000, 0)},
	}
	h.r = reconciler.New(self, h.topo, svc, log.Noop, reconciler.Options{Now: h.clock.Now, After: h.clock.After})
	t.Cleanup(func() {
		if err := h.r.Close(); err != nil {
			t.Error(err)
		}
	})
	return h
}

// gossipLog records the clock time of every Status sent to each peer.
type gossipLog struct {
	h     *harness
	peers []swarm.Address
	times map[string][]time.Time
}

func newGossipLog(h *harness, peers ...swarm.Address) *gossipLog {
	return &gossipLog{h: h, peers: peers, times: make(map[string][]time.Time)}
}

// record stamps the Statuses sent since the last call with the clock time.
func (g *gossipLog) record() {
	for _, p := range g.peers {
		k := p.ByteString()
		for n := g.h.svc.count(g.h.svc.sent, p); len(g.times[k]) < n; {
			g.times[k] = append(g.times[k], g.h.clock.Now())
		}
	}
}

// assertEvery checks that each of peers received want Statuses, every
// interval apart.
func (g *gossipLog) assertEvery(t *testing.T, want int, interval time.Duration, peers ...swarm.Address) {
	t.Helper()
	for _, p := range peers {
		ts := g.times[p.ByteString()]
		if len(ts) != want {
			t.Fatalf("peer %s: %d statuses, want %d", p, len(ts), want)
		}
		for i := 1; i < len(ts); i++ {
			if d := ts[i].Sub(ts[i-1]); d != interval {
				t.Fatalf("peer %s: status %d sent %v after the previous one, want %v", p, i, d, interval)
			}
		}
	}
}

// TestStatusGossipDebounce: 1000 root changes, one every 50 ms, send one
// Status per StatusMinInterval to each neighbor (after the first contact).
func TestStatusGossipDebounce(t *testing.T) {
	t.Parallel()

	peers := []swarm.Address{lower, addr(0x20, 0), higher}
	svc := newFakeService(status(1))
	for _, p := range peers {
		svc.remote[p.ByteString()] = status(1)
	}
	h := newGossipHarness(t, svc, peers...)
	g := newGossipLog(h, peers...)
	h.r.Round() // first contact
	g.record()

	const (
		changes = 1000
		every   = 50 * time.Millisecond
	)
	for i := range changes {
		h.clock.Add(every)
		svc.set(func() { svc.local = status(uint64(i + 2)) })
		h.r.Gossip()
		g.record()
	}
	g.assertEvery(t, 1+int(changes*every/reconciler.StatusMinInterval), reconciler.StatusMinInterval, peers...)
}

// TestStatusGossipHeartbeat: with no change, each neighbor receives one
// Status per StatusMaxInterval.
func TestStatusGossipHeartbeat(t *testing.T) {
	t.Parallel()

	peers := []swarm.Address{lower, higher}
	svc := newFakeService(status(1))
	for _, p := range peers {
		svc.remote[p.ByteString()] = status(1)
	}
	h := newGossipHarness(t, svc, peers...)
	g := newGossipLog(h, peers...)
	h.r.Round() // first contact
	g.record()
	if got := h.r.Gossip(); got != reconciler.StatusMaxInterval {
		t.Fatalf("next status in %v, want %v", got, reconciler.StatusMaxInterval)
	}

	const minutes = 10
	for range minutes * 60 {
		h.clock.Add(time.Second)
		h.r.Gossip()
		g.record()
	}
	g.assertEvery(t, 1+minutes*int(time.Minute/reconciler.StatusMaxInterval), reconciler.StatusMaxInterval, peers...)
}

// TestStatusGossipNeighborsOnly: root summary changes and heartbeats go to
// eligible neighbors only; a peer with a disjoint root or another mapping
// gets the first contact only, a pullsync-only peer is probed once. A moved
// root (radius change) goes to every rangesync peer at once, even within
// StatusMinInterval of the last Status, and the Index's later signal of the
// same change sends nothing more.
func TestStatusGossipNeighborsOnly(t *testing.T) {
	t.Parallel()

	own := fingerprint.NewRange(addr(0x80, 0), 1)
	withRoot := func(st rangesync.Status, root fingerprint.Range) rangesync.Status {
		st.Root = root
		return st
	}
	neighbor, far, mapped, unsupported := lower, addr(0x11, 0), addr(0x12, 0), addr(0x13, 0)
	svc := newFakeService(withRoot(status(1), own))
	svc.remote[neighbor.ByteString()] = withRoot(status(1), own)
	svc.remote[far.ByteString()] = withRoot(status(1), fingerprint.NewRange(addr(0x00, 0), 1))
	m := withRoot(status(1), own)
	m.Mapping++
	svc.remote[mapped.ByteString()] = m
	svc.unsupported[unsupported.ByteString()] = true
	peers := []swarm.Address{neighbor, far, mapped, unsupported}
	h := newGossipHarness(t, svc, peers...)
	g := newGossipLog(h, peers...)
	h.r.Round() // first contact
	g.record()

	const seconds = 120
	for i := range seconds {
		h.clock.Add(time.Second)
		svc.set(func() { svc.local = withRoot(status(uint64(i+2)), own) })
		h.r.Gossip()
		g.record()
	}
	g.assertEvery(t, 1+seconds*int(time.Second)/int(reconciler.StatusMinInterval), reconciler.StatusMinInterval, neighbor)
	g.assertEvery(t, 1, 0, far, mapped, unsupported)

	// the radius decreases one second after the last push: the next round
	// sends the new root to every peer that speaks rangesync
	h.clock.Add(time.Second)
	svc.set(func() { svc.local = withRoot(svc.local, fingerprint.NewRange(swarm.ZeroAddress, 0)) })
	h.r.Round()
	g.record()
	for _, tc := range []struct {
		peer swarm.Address
		want int
	}{
		{neighbor, 2 + seconds*int(time.Second)/int(reconciler.StatusMinInterval)},
		{far, 2},
		{mapped, 2},
		{unsupported, 1},
	} {
		if got := len(g.times[tc.peer.ByteString()]); got != tc.want {
			t.Fatalf("peer %s: %d statuses after the root moved, want %d", tc.peer, got, tc.want)
		}
	}

	// the Index signals the same change later: nothing new to send
	before := svc.count(svc.sent, neighbor)
	h.clock.Add(reconciler.StatusMinInterval)
	h.r.Gossip()
	if got := svc.count(svc.sent, neighbor); got != before {
		t.Fatalf("%d statuses for an unchanged root summary", got-before)
	}
}

// TestStatusGossipRetry: a Status that could not be sent is retried after
// StatusMinInterval, not before.
func TestStatusGossipRetry(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[lower.ByteString()] = status(1)
	svc.sendErr[lower.ByteString()] = errors.New("stream reset")
	h := newGossipHarness(t, svc, lower)
	h.r.Round() // the first contact fails
	if got := h.r.Gossip(); got != reconciler.StatusMinInterval {
		t.Fatalf("retry in %v, want %v", got, reconciler.StatusMinInterval)
	}
	h.clock.Add(reconciler.StatusMinInterval - time.Second)
	h.r.Gossip()
	if got := svc.count(svc.sent, lower); got != 1 {
		t.Fatalf("%d attempts within StatusMinInterval, want 1", got)
	}
	svc.set(func() { delete(svc.sendErr, lower.ByteString()) })
	h.clock.Add(time.Second)
	h.r.Gossip()
	if got := svc.count(svc.sent, lower); got != 2 {
		t.Fatalf("%d attempts after StatusMinInterval, want 2", got)
	}
	if got := h.r.Gossip(); got != reconciler.StatusMaxInterval {
		t.Fatalf("next status in %v after a success, want %v", got, reconciler.StatusMaxInterval)
	}
}

// TestStatusGossipResend: while this node has no Status of a peer (it may
// have lost this node's on a quick reconnect), each round asks for this
// node's Status to be sent again, at most every StatusMinInterval.
func TestStatusGossipResend(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1)) // no Status of lower yet
	h := newGossipHarness(t, svc, lower)
	h.r.Round() // first contact
	h.clock.Add(time.Second)
	h.r.Round()
	if got := svc.count(svc.sent, lower); got != 1 {
		t.Fatalf("%d statuses within StatusMinInterval, want 1", got)
	}
	h.clock.Add(reconciler.StatusMinInterval - time.Second)
	h.r.Gossip() // the resend the round asked for
	h.r.Gossip()
	if got := svc.count(svc.sent, lower); got != 2 {
		t.Fatalf("%d statuses, want 2", got)
	}

	// with the peer's Status, rounds stop asking
	svc.set(func() { svc.remote[lower.ByteString()] = status(1) })
	h.clock.Add(reconciler.StatusMinInterval)
	h.r.Round()
	h.r.Gossip()
	if got := svc.count(svc.sent, lower); got != 2 {
		t.Fatalf("%d statuses after the peer's Status arrived, want 2", got)
	}
}

// TestStatusGossipLoop: the started reconciler pushes its Status on the
// Index's root change signal and sends the heartbeat on its own.
func TestStatusGossipLoop(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[lower.ByteString()] = status(1)
	clock := &fakeClock{now: time.Unix(1_000_000, 0)}
	r := reconciler.New(self, &fakeTopology{peers: []swarm.Address{lower}}, svc, log.Noop, reconciler.Options{
		Now:           clock.Now,
		After:         clock.After,
		RoundInterval: time.Hour, // only the first round runs
	})
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	})
	r.Start(context.Background())
	waitFor(t, "first contact", func() bool { return svc.count(svc.sent, lower) == 1 })

	clock.Add(reconciler.StatusMinInterval)
	svc.set(func() { svc.local = status(2) })
	svc.rootChange()
	waitFor(t, "push", func() bool { return svc.count(svc.sent, lower) == 2 })

	start := clock.Now()
	waitFor(t, "heartbeat", func() bool {
		clock.Add(time.Second)
		return svc.count(svc.sent, lower) >= 3
	})
	if d := clock.Now().Sub(start); d < reconciler.StatusMaxInterval {
		t.Fatalf("heartbeat %v after the push, want at least %v", d, reconciler.StatusMaxInterval)
	}
	if got := svc.count(svc.sent, lower); got != 3 {
		t.Fatalf("%d statuses, want 3", got)
	}
}
