// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"bytes"
	"context"
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
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// radiusNet is a node a (overlay 0x00…, initiating) with peers b and c
// (0x80…, 0xc0…, radius 0) holding every chunk, and a's reconciler.
type radiusNet struct {
	a     *stripeNode
	peers []*stripeNode
	r     *reconciler.Reconciler
	topo  *fakeTopology
}

// round triggers a round of a's reconciler (rounds are otherwise hourly,
// since peers answer Busy to sessions faster than PeerSessionInterval).
func (n *radiusNet) round() {
	n.topo.setPeers(n.peers[0].addr, n.peers[1].addr)
}

func newRadiusNet(t *testing.T, radius uint8, own, all []swarm.Chunk) *radiusNet {
	t.Helper()
	n := &radiusNet{}
	routes := make(map[string]p2p.ProtocolSpec)
	for _, b := range []byte{0x80, 0xc0} {
		a := addr(b, 1)
		p := newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(a)), a, all...)
		routes[a.String()] = p.svc.Protocol()
		n.peers = append(n.peers, p)
	}
	aAddr := addr(0x00, 1)
	n.a = newStripeNodeStore(t, streamtest.New(streamtest.WithBaseAddr(aAddr), streamtest.WithPeerProtocols(routes)), aAddr,
		mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithRadius(radius), mockstorer.WithChunks(own...)))
	addrs := make([]swarm.Address, 0, len(n.peers))
	for _, p := range n.peers {
		p.rec.SetProtocols(n.a.svc.Protocol())
		if err := p.svc.SendStatus(context.Background(), n.a.addr); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "status", func() bool { _, ok := n.a.svc.LastStatus(p.addr); return ok })
		addrs = append(addrs, p.addr)
	}
	n.topo = &fakeTopology{peers: addrs}
	n.r = reconciler.New(n.a.addr, n.topo, n.a.svc, log.Noop, reconciler.Options{RoundInterval: time.Hour})
	t.Cleanup(func() { _ = n.r.Close() })
	n.r.Start(context.Background())
	return n
}

// sessions returns the opening ranges a wrote, and the number of Messages
// it wrote, per reconcile session with the peers.
func (n *radiusNet) sessions(t *testing.T) (opens []fingerprint.Range, msgs []int) {
	t.Helper()
	for _, p := range n.peers {
		for _, rec := range streamRecords(t, n.a, p.addr, rangesync.ReconcileStreamName) {
			ms, err := protobuf.ReadMessages(bytes.NewReader(rec.In()), func() protobuf.Message { return new(pb.Message) })
			if err != nil || len(ms) == 0 {
				continue // still running
			}
			fp := ms[0].(*pb.Message).Parts[0].GetFP()
			if fp == nil {
				t.Fatalf("opening without a fingerprint: %v", ms[0])
			}
			rg, err := rangesync.ParseRange(fp.Range)
			if err != nil {
				t.Fatal(err)
			}
			opens = append(opens, rg)
			msgs = append(msgs, len(ms))
		}
	}
	return opens, msgs
}

// deliveries returns the number of chunks delivered to a.
func (n *radiusNet) deliveries(t *testing.T) int {
	t.Helper()
	d := 0
	for _, p := range n.peers {
		for _, rec := range streamRecords(t, n.a, p.addr, rangesync.FetchStreamName) {
			ms, err := protobuf.ReadMessages(bytes.NewReader(rec.Out()), func() protobuf.Message { return new(pb.Delivery) })
			if err != nil {
				t.Fatal(err)
			}
			d += len(ms)
		}
	}
	return d
}

func newStripeNodeStore(t *testing.T, rec *streamtest.Recorder, a swarm.Address, store *mockstorer.ReserveStore) *stripeNode {
	t.Helper()
	n := &stripeNode{addr: a, store: store, rec: streamtest.NewRecorderDisconnecter(rec)}
	accept := func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	n.svc = rangesync.New(n.rec, n.store, n.addr, nil, nil, accept, log.Noop, rangesync.Options{LeafBits: 8})
	t.Cleanup(func() { _ = n.svc.Close() })
	if err := n.svc.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "index", func() bool {
		_, err := n.svc.LocalStatus()
		return err == nil
	})
	return n
}

// waitRoot waits until n's Index is rebuilt at a root of bits.
func waitRoot(t *testing.T, n *stripeNode, bits uint8) {
	t.Helper()
	waitFor(t, "rebuild", func() bool {
		st, err := n.svc.LocalStatus()
		return err == nil && st.Root.Bits == bits
	})
}

// TestRadiusDecreaseStripesNewHalf lowers the radius of one of 3 nodes: only
// the new sibling half is transferred (striped), and the old half costs one
// fingerprint exchange per peer.
func TestRadiusDecreaseStripesNewHalf(t *testing.T) {
	t.Parallel()

	const perQuarter = 6
	all := stripedChunks(2, perQuarter)
	var own, fresh []swarm.Chunk
	for _, ch := range all {
		if ch.Address().Bytes()[0] < 0x80 {
			own = append(own, ch)
		} else {
			fresh = append(fresh, ch)
		}
	}
	n := newRadiusNet(t, 1, own, all)
	oldHalf := fingerprint.NewRange(swarm.ZeroAddress, 1)
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	waitFor(t, "first sessions", func() bool { o, _ := n.sessions(t); return len(o) >= 2 })

	n.a.store.SetStorageRadius(0)
	waitRoot(t, n.a, 0)
	n.round()
	waitFor(t, "new half stored", func() bool {
		for _, ch := range fresh {
			if has, _ := n.a.store.ReserveHasID(chunkIDOf(t, ch).Address, chunkIDOf(t, ch).StampHash); !has {
				return false
			}
		}
		return true
	})
	// a round session that raced a stripe session for the peer's single
	// session slot is retried by the next round
	waitFor(t, "verification", func() bool { return n.r.VerificationDiffCount() >= 2 })
	n.round()
	waitFor(t, "old-half sessions after the decrease", func() bool {
		o, _ := n.sessions(t)
		c := 0
		for _, rg := range o {
			if rg.Equal(oldHalf) {
				c++
			}
		}
		return c >= 3 // 2 before, at least 1 after
	})

	if d := n.deliveries(t); d != len(fresh) {
		t.Fatalf("%d deliveries, want %d (new half only)", d, len(fresh))
	}
	opens, msgs := n.sessions(t)
	for i, rg := range opens {
		switch {
		case rg.Equal(root):
			t.Fatalf("session over the whole new root")
		case rg.Equal(oldHalf) && msgs[i] != 1:
			t.Fatalf("old-half session wrote %d messages, want 1", msgs[i])
		case !oldHalf.ContainsRange(rg) && !root.Child(1, 1).ContainsRange(rg):
			t.Fatalf("unexpected session range %s", rg)
		}
	}
}

// TestRadiusIncreaseNoFetch raises the radius of a node holding every chunk:
// the root narrows and nothing is transferred.
func TestRadiusIncreaseNoFetch(t *testing.T) {
	t.Parallel()

	all := stripedChunks(2, 4)
	n := newRadiusNet(t, 0, all, all) // same digests: no session yet

	n.a.store.SetStorageRadius(1)
	waitRoot(t, n.a, 1)
	n.round()
	waitFor(t, "sessions at the new root", func() bool {
		o, _ := n.sessions(t)
		c := 0
		for _, rg := range o {
			if rg.Bits == 1 {
				c++
			}
		}
		return c >= 2
	})
	settle()
	if d := n.deliveries(t); d != 0 {
		t.Fatalf("%d deliveries after a radius increase", d)
	}
	if got := n.r.StripesTotal("done"); got != 0 {
		t.Fatalf("%v stripes after a radius increase", got)
	}
}

// TestRadiusDecreaseBusyRetried: during a radius decrease the local index
// and the peers' are rebuilding; Busy is retried, not scored or reassigned,
// and the new half is striped while the sessions open only the old half.
func TestRadiusDecreaseBusyRetried(t *testing.T) {
	t.Parallel()

	other := addr(0xf1, 0)
	half := func(bits uint8, count uint64) rangesync.Status {
		st := status(count)
		st.Root = fingerprint.NewRange(self, bits)
		return st
	}
	svc := newFakeService(half(1, 5))
	for _, p := range []swarm.Address{higher, other} {
		svc.remote[p.ByteString()] = half(0, 10)
	}
	h := newHarness(t, svc, higher, other)
	waitFor(t, "first sessions", func() bool { return len(svc.sessionOpens(higher)) > 0 })

	root := fingerprint.NewRange(self, 0)
	oldHalf := fingerprint.NewRange(self, 1)
	newHalf := root.Child(1, 1-root.ChildIndex(self, 1))
	svc.set(func() {
		svc.localErr = fingerprint.ErrBusy // rebuilding
		for _, p := range []swarm.Address{higher, other} {
			svc.reconErr[p.ByteString()] = rangesync.ErrPeerBusy
			svc.wants[p.ByteString()] = 2
		}
	})
	settle()
	before := make(map[string]int)
	svc.set(func() {
		for _, p := range []swarm.Address{higher, other} {
			before[p.ByteString()] = len(svc.opens[p.ByteString()])
		}
		svc.localErr = nil
		svc.local = half(0, 5)
		for _, p := range []swarm.Address{higher, other} {
			svc.reconErr[p.ByteString()] = rangesync.ErrPeerBusy // once more
		}
	})

	waitFor(t, "new half striped", func() bool { return h.r.StripesTotal("done") >= 2 })
	waitFor(t, "old-half session", func() bool {
		for _, p := range []swarm.Address{higher, other} {
			for _, open := range svc.sessionOpens(p) {
				if len(open) == 1 && open[0].Equal(oldHalf) {
					return true
				}
			}
		}
		return false
	})
	if got := h.r.StripesTotal("reassigned"); got != 0 {
		t.Fatalf("stripes reassigned %v after Busy, want 0", got)
	}
	for _, p := range []swarm.Address{higher, other} {
		if s := h.r.Score(p); s != 0 {
			t.Fatalf("score of %s = %d after Busy", p, s)
		}
		// until the peer's last stripe or verification session (the
		// transfer ends after it and root sessions resume), every session
		// opens the old half or a part of the new half
		opens := svc.sessionOpens(p)[before[p.ByteString()]:]
		last := -1
		for i, open := range opens {
			if len(open) == 1 && newHalf.ContainsRange(open[0]) {
				last = i
			}
		}
		for _, open := range opens[:last+1] {
			if len(open) == 0 {
				t.Fatalf("root session with %s during the radius decrease", p)
			}
			for _, rg := range open {
				if !oldHalf.ContainsRange(rg) && !newHalf.ContainsRange(rg) {
					t.Fatalf("session over %s during the radius decrease", rg)
				}
			}
		}
	}
}
