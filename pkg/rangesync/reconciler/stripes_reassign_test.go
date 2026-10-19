// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// errKilled is the error of the fetch handler of a killed peer.
var errKilled = errors.New("peer killed")

// TestTransferStripesPeerKilled kills one of 4 peers after it listed its
// stripe: the stripe is reassigned to its second peer and completes there,
// and every chunk is delivered exactly once.
func TestTransferStripesPeerKilled(t *testing.T) {
	t.Parallel()

	const (
		k         = 4
		perStripe = 12 // streamtest deadlocks past 16 deliveries per Want
	)
	chunks := stripedChunks(2, perStripe)

	emptyAddr := swarm.RandAddress(t)
	routes := make(map[string]p2p.ProtocolSpec)
	peers := make([]*stripeNode, k)
	for i := range peers {
		a := swarm.RandAddress(t)
		peers[i] = newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(a)), a, chunks...)
		spec := peers[i].svc.Protocol()
		if i == 0 { // killed: lists its stripe, then delivers nothing
			for j := range spec.StreamSpecs {
				if spec.StreamSpecs[j].Name == rangesync.FetchStreamName {
					spec.StreamSpecs[j].Handler = func(_ context.Context, _ p2p.Peer, s p2p.Stream) error {
						_ = s.Reset()
						return errKilled
					}
				}
			}
		}
		routes[peers[i].addr.String()] = spec
	}
	empty := newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(emptyAddr), streamtest.WithPeerProtocols(routes)), emptyAddr)
	statuses := make([]reconciler.PeerStatus, 0, k)
	for _, p := range peers {
		p.rec.SetProtocols(empty.svc.Protocol())
		if err := empty.svc.SendStatus(context.Background(), p.addr); err != nil {
			t.Fatal(err)
		}
		if err := p.svc.SendStatus(context.Background(), empty.addr); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "status", func() bool {
			_, a := empty.svc.LastStatus(p.addr)
			_, b := p.svc.LastStatus(empty.addr)
			return a && b
		})
		st, _ := empty.svc.LastStatus(p.addr)
		statuses = append(statuses, reconciler.PeerStatus{Peer: p.addr, Status: st})
	}
	own, err := empty.svc.LocalStatus()
	if err != nil {
		t.Fatal(err)
	}
	stripes := reconciler.PlanStripesRedundancy(own.Root, statuses, 2)
	if len(stripes) != k {
		t.Fatalf("got %d stripes, want %d", len(stripes), k)
	}

	r := reconciler.New(empty.addr, nil, empty.svc, log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })

	killed := 0
	for _, res := range r.TransferStripes(context.Background(), stripes) {
		if res.Err != nil {
			t.Fatalf("stripe %s: %v", res.Range, res.Err)
		}
		if res.Stored != perStripe {
			t.Fatalf("stripe %s: stored %d, want %d", res.Range, res.Stored, perStripe)
		}
		if res.Peer.Equal(peers[0].addr) {
			t.Fatalf("stripe %s completed by the killed peer", res.Range)
		}
		if res.Listed > perStripe {
			killed++
		}
	}
	if killed != 1 {
		t.Fatalf("%d stripes relisted, want 1", killed)
	}
	for _, ch := range chunks {
		id := chunkIDOf(t, ch)
		if has, err := empty.store.ReserveHasID(id.Address, id.StampHash); err != nil || !has {
			t.Fatalf("chunk %s not stored (%v)", ch.Address(), err)
		}
	}
	deliveries := 0
	for _, p := range peers {
		for _, rec := range streamRecords(t, empty, p.addr, rangesync.FetchStreamName) {
			ms, err := protobuf.ReadMessages(bytes.NewReader(rec.Out()), func() protobuf.Message { return new(pb.Delivery) })
			if err != nil {
				t.Fatal(err)
			}
			deliveries += len(ms)
		}
	}
	if deliveries != len(chunks) {
		t.Fatalf("%d deliveries of %d chunks", deliveries, len(chunks))
	}
	if got := r.StripesTotal("reassigned"); got != 1 {
		t.Fatalf("stripes reassigned %v, want 1", got)
	}
	if got := r.StripesTotal("done"); got != k {
		t.Fatalf("stripes done %v, want %d", got, k)
	}
}

// TestTransferStripesRestartOnProgress ends a stripe session with an error
// after it listed and stored chunks: the stripe restarts with the same peer.
func TestTransferStripesRestartOnProgress(t *testing.T) {
	t.Parallel()

	other := addr(0xf1, 0)
	svc := newFakeService(status(0))
	svc.wants[higher.ByteString()] = 3
	svc.sessionErrAfter[higher.ByteString()] = context.DeadlineExceeded // session timeout
	r := reconciler.New(self, nil, svc, log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher, other}}})[0]
	if res.Err != nil || !res.Peer.Equal(higher) {
		t.Fatalf("peer %s, err %v", res.Peer, res.Err)
	}
	if n := len(svc.sessionOpens(higher)); n != 2 {
		t.Fatalf("%d sessions, want 2", n)
	}
	if n := len(svc.sessionOpens(other)); n != 0 {
		t.Fatalf("%d sessions with the second peer", n)
	}
	if got := r.StripesTotal("reassigned"); got != 0 {
		t.Fatalf("stripes reassigned %v", got)
	}
}

// TestTransferStripesWithheld reassigns a stripe whose peer lists IDs but
// delivers none, first to its next peer, then to an eligible peer with a
// Status, and gives up after MaxStripeReassign reassignments.
func TestTransferStripesWithheld(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	t.Run("next peer", func(t *testing.T) {
		t.Parallel()
		other := addr(0xf1, 0)
		svc := newFakeService(status(0))
		svc.wants[higher.ByteString()] = 3
		svc.wants[other.ByteString()] = 3
		svc.withhold[higher.ByteString()] = true
		r := reconciler.New(self, nil, svc, log.Noop, reconciler.Options{})
		t.Cleanup(func() { _ = r.Close() })

		res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher, other}}})[0]
		if res.Err != nil || !res.Peer.Equal(other) || res.Stored != 3 {
			t.Fatalf("peer %s, stored %d, err %v", res.Peer, res.Stored, res.Err)
		}
		if got := r.StripesTotal("reassigned"); got != 1 {
			t.Fatalf("stripes reassigned %v, want 1", got)
		}
	})
	t.Run("eligible peer", func(t *testing.T) {
		t.Parallel()
		other := addr(0xf1, 0)
		svc := newFakeService(status(1))
		svc.remote[higher.ByteString()] = status(2)
		svc.remote[other.ByteString()] = status(2)
		svc.wants[higher.ByteString()] = 3
		svc.wants[other.ByteString()] = 3
		svc.withhold[higher.ByteString()] = true
		h := newHarness(t, svc, higher, other)
		waitFor(t, "status", func() bool { return svc.count(svc.sent, other) > 0 && svc.count(svc.sent, higher) > 0 })
		waitAdmitted(t, h.r, higher, other)

		res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
		if res.Err != nil || !res.Peer.Equal(other) {
			t.Fatalf("peer %s, err %v", res.Peer, res.Err)
		}
	})
	t.Run("bounded", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(status(0))
		ps := make([]swarm.Address, 0, reconciler.MaxStripeReassign+3)
		for i := range reconciler.MaxStripeReassign + 3 {
			p := addr(0xe0, byte(i))
			ps = append(ps, p)
			svc.wants[p.ByteString()] = 3
			svc.withhold[p.ByteString()] = true
		}
		r := reconciler.New(self, nil, svc, log.Noop, reconciler.Options{})
		t.Cleanup(func() { _ = r.Close() })

		res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: ps}})[0]
		if !errors.Is(res.Err, reconciler.ErrStripeWithheld) {
			t.Fatalf("got %v, want %v", res.Err, reconciler.ErrStripeWithheld)
		}
		sessions := 0
		for _, p := range ps {
			sessions += len(svc.sessionOpens(p))
		}
		if sessions != reconciler.MaxStripeReassign+1 {
			t.Fatalf("%d sessions, want %d", sessions, reconciler.MaxStripeReassign+1)
		}
		if got := r.StripesTotal("reassigned"); got != reconciler.MaxStripeReassign {
			t.Fatalf("stripes reassigned %v", got)
		}
	})
}

// TestEnqueueStripesDedup checks the stripe registry: a range overlapping a
// queued or in-flight range is not registered again.
func TestEnqueueStripesDedup(t *testing.T) {
	t.Parallel()

	r := reconciler.New(self, nil, newFakeService(status(0)), log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	half := root.Child(1, 0)
	if !r.EnqueueStripes([]fingerprint.Range{half}) {
		t.Fatal("half not registered")
	}
	if r.EnqueueStripes([]fingerprint.Range{half.Child(2, 1)}) {
		t.Fatal("sub-range of a registered range registered")
	}
	if r.EnqueueStripes([]fingerprint.Range{root}) {
		t.Fatal("super-range of a registered range registered")
	}
	if !r.EnqueueStripes([]fingerprint.Range{root.Child(1, 1)}) {
		t.Fatal("disjoint range not registered")
	}
}

// TestRoundBootstrapStripes checks the bootstrap trigger: an empty node
// transfers its root striped across its peers instead of one session per
// peer over the whole root.
func TestRoundBootstrapStripes(t *testing.T) {
	t.Parallel()

	other := addr(0xf1, 0)
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	svc := newFakeService(status(0))
	svc.remote[higher.ByteString()] = status(5)
	svc.remote[other.ByteString()] = status(5)
	newHarness(t, svc, higher, other)

	waitFor(t, "stripe sessions", func() bool {
		got := make(map[string]bool)
		for _, p := range []swarm.Address{higher, other} {
			for _, open := range svc.sessionOpens(p) {
				if len(open) == 1 {
					got[open[0].String()] = true
				}
			}
		}
		return got[root.Child(1, 0).String()] && got[root.Child(1, 1).String()]
	})
	for _, p := range []swarm.Address{higher, other} {
		for _, open := range svc.sessionOpens(p) {
			if len(open) == 0 {
				t.Fatalf("root session with %s while bootstrapping", p)
			}
		}
	}
}

// TestTransferStripesTrickle bounds the same-peer restarts of a peer that
// stores a few chunks per attempt and then fails.
func TestTransferStripesTrickle(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(0))
	svc.wants[higher.ByteString()] = 1
	svc.sessionErrAfter[higher.ByteString()] = context.DeadlineExceeded
	svc.sessionErrSticky = true
	r := reconciler.New(self, nil, svc, log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
	if res.Err == nil {
		t.Fatal("trickling stripe done")
	}
	if n := len(svc.sessionOpens(higher)); n != reconciler.MaxStripeRestarts+1 {
		t.Fatalf("%d sessions, want %d", n, reconciler.MaxStripeRestarts+1)
	}
}

// TestTransferStripesPartialShortfall shows the cost of a false withholding
// positive (IDs listed but not stored, e.g. stamp rejects): the stripe is
// reassigned at most MaxStripeReassign times.
func TestTransferStripesPartialShortfall(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(0))
	ps := make([]swarm.Address, 0, reconciler.MaxStripeReassign+3)
	for i := range reconciler.MaxStripeReassign + 3 {
		p := addr(0xe0, byte(i))
		ps = append(ps, p)
		svc.wants[p.ByteString()] = 3
		svc.withholdN[p.ByteString()] = 1
	}
	r := reconciler.New(self, nil, svc, log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: ps}})[0]
	if !errors.Is(res.Err, reconciler.ErrStripeWithheld) {
		t.Fatalf("got %v, want %v", res.Err, reconciler.ErrStripeWithheld)
	}
	sessions := 0
	for _, p := range ps {
		sessions += len(svc.sessionOpens(p))
	}
	if sessions != reconciler.MaxStripeReassign+1 {
		t.Fatalf("%d sessions, want %d", sessions, reconciler.MaxStripeReassign+1)
	}
}

// TestEnqueueStripesBound drops ranges past MaxStripeRegistry.
func TestEnqueueStripesBound(t *testing.T) {
	t.Parallel()

	r := reconciler.New(self, nil, newFakeService(status(0)), log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	for i := range reconciler.MaxStripeRegistry {
		if !r.EnqueueStripes([]fingerprint.Range{root.Child(11, uint64(i))}) {
			t.Fatalf("range %d not registered", i)
		}
	}
	if r.EnqueueStripes([]fingerprint.Range{root.Child(11, reconciler.MaxStripeRegistry)}) {
		t.Fatal("range past the bound registered")
	}
	if got := r.StripesTotal("dropped"); got != 1 {
		t.Fatalf("stripes dropped %v, want 1", got)
	}
}

// rootSessions reports whether a session over the whole root (no Open) ran
// with any of peers.
func rootSessions(svc *fakeService, peers ...swarm.Address) bool {
	for _, p := range peers {
		for _, open := range svc.sessionOpens(p) {
			if len(open) == 0 {
				return true
			}
		}
	}
	return false
}

// TestRoundBootstrapOverlap: when the root overlaps a range already in a
// striped transfer, the round does not bootstrap; it runs the per-peer
// sessions, and the start-up guard waits for them.
func TestRoundBootstrapOverlap(t *testing.T) {
	t.Parallel()

	other := addr(0xf1, 0)
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	svc := newFakeService(status(0))
	svc.remote[higher.ByteString()] = status(5)
	svc.remote[other.ByteString()] = status(5)
	svc.reconcileGate = make(chan struct{}) // every session blocks
	topo := &fakeTopology{peers: []swarm.Address{higher, other}}
	r := reconciler.New(self, topo, svc, log.Noop, reconciler.Options{RoundInterval: 10 * time.Millisecond})
	t.Cleanup(func() {
		close(svc.reconcileGate)
		_ = r.Close()
	})
	r.HoldStripe(root.Child(1, 0)) // in flight
	r.Start(context.Background())

	waitFor(t, "root session", func() bool { return rootSessions(svc, higher, other) })
	settle()
	for _, p := range []swarm.Address{higher, other} {
		for _, open := range svc.sessionOpens(p) {
			if len(open) != 0 {
				t.Fatalf("stripe session %v with %s", open, p)
			}
		}
	}
	if r.GuardReleased() {
		t.Fatal("guard released with no transfer making progress")
	}
}

// TestRoundBootstrapFailedResumesSessions: after a bootstrap transfer that
// did not complete, the per-peer sessions run.
func TestRoundBootstrapFailedResumesSessions(t *testing.T) {
	t.Parallel()

	other := addr(0xf1, 0)
	svc := newFakeService(status(0))
	for _, p := range []swarm.Address{higher, other} {
		svc.remote[p.ByteString()] = status(5)
		svc.wants[p.ByteString()] = 3
		svc.withhold[p.ByteString()] = true
	}
	newHarness(t, svc, higher, other)

	waitFor(t, "stripe session", func() bool {
		for _, p := range []swarm.Address{higher, other} {
			for _, open := range svc.sessionOpens(p) {
				if len(open) == 1 {
					return true
				}
			}
		}
		return false
	})
	waitFor(t, "root session", func() bool { return rootSessions(svc, higher, other) })
}

// TestRoundBootstrapNoStripePeer: a plan with no peer status yields a
// failed stripe (ErrNoStripePeer), which marks a bootstrap failed.
func TestRoundBootstrapNoStripePeer(t *testing.T) {
	t.Parallel()

	r := reconciler.New(self, nil, newFakeService(status(0)), log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })
	res := r.TransferStripes(context.Background(), reconciler.PlanStripes(fingerprint.NewRange(swarm.ZeroAddress, 0), nil))
	if len(res) == 0 || !errors.Is(res[0].Err, reconciler.ErrNoStripePeer) {
		t.Fatalf("got %v, want a failed stripe", res)
	}
}
