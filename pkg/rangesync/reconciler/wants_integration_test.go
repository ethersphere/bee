// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// roundNode is a rangesync service over a mock reserve with a reconciler,
// the Service's PeerScorer and Wants, as pkg/node wires them. Its streams
// to each peer go to that peer's protocol (routes, filled before start).
type roundNode struct {
	stripeNode
	r      *reconciler.Reconciler
	routes map[string]p2p.ProtocolSpec
}

// roundNodeOptions configures a roundNode.
type roundNodeOptions struct {
	validStamp    func(swarm.Chunk) (swarm.Chunk, error)
	putHook       func(swarm.Chunk) error
	fetchTimeout  time.Duration
	roundInterval time.Duration
	chunks        []swarm.Chunk
}

func newRoundNode(t *testing.T, a swarm.Address, o roundNodeOptions, peers ...swarm.Address) *roundNode {
	t.Helper()
	if o.validStamp == nil {
		o.validStamp = func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	}
	n := &roundNode{routes: make(map[string]p2p.ProtocolSpec)}
	n.addr = a
	n.store = mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithChunks(o.chunks...), mockstorer.WithPutHook(o.putHook))
	n.rec = streamtest.NewRecorderDisconnecter(streamtest.New(streamtest.WithBaseAddr(a), streamtest.WithPeerProtocols(n.routes)))
	n.r = reconciler.New(a, &fakeTopology{peers: peers}, nil, log.Noop, reconciler.Options{RoundInterval: o.roundInterval})
	n.svc = rangesync.New(n.rec, n.store, a, nil, nil, o.validStamp, log.Noop, rangesync.Options{
		LeafBits:     8,
		Scorer:       n.r,
		Wants:        n.r.Wants,
		FetchTimeout: o.fetchTimeout,
	})
	n.r.SetService(n.svc)
	t.Cleanup(func() {
		if err := n.r.Close(); err != nil {
			t.Error(err)
		}
		if err := n.svc.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := n.svc.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "index", func() bool {
		_, err := n.svc.LocalStatus()
		return err == nil
	})
	return n
}

// connect routes n's streams to each peer through spec (the peer's protocol
// if nil) and exchanges Statuses, then starts n's reconciler.
func (n *roundNode) start(t *testing.T, peers []*stripeNode, specs map[string]p2p.ProtocolSpec) {
	t.Helper()
	for _, p := range peers {
		spec, ok := specs[p.addr.String()]
		if !ok {
			spec = p.svc.Protocol()
		}
		n.routes[p.addr.String()] = spec
		p.rec.SetProtocols(n.svc.Protocol())
		if err := p.svc.SendStatus(context.Background(), n.addr); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "status", func() bool {
			_, ok := n.svc.LastStatus(p.addr)
			return ok
		})
	}
	n.r.Start(context.Background())
}

func (n *roundNode) has(t *testing.T, ch swarm.Chunk) bool {
	t.Helper()
	id := chunkIDOf(t, ch)
	has, err := n.store.ReserveHasID(id.Address, id.StampHash)
	if err != nil {
		t.Fatal(err)
	}
	return has
}

// wantsTo returns the IDs of each Want n sent to peer, in order.
func (n *roundNode) wantsTo(t *testing.T, peer swarm.Address) [][]swarm.Address {
	t.Helper()
	var out [][]swarm.Address
	for _, rec := range streamRecords(t, &n.stripeNode, peer, rangesync.FetchStreamName) {
		ms, err := protobuf.ReadMessages(bytes.NewReader(rec.In()), func() protobuf.Message { return new(pb.Want) })
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			var addrs []swarm.Address
			for _, id := range m.(*pb.Want).IDs {
				addrs = append(addrs, swarm.NewAddress(id.Address))
			}
			out = append(out, addrs)
		}
	}
	return out
}

// deliveriesOf returns the number of times peers delivered ch to n.
func (n *roundNode) deliveriesOf(t *testing.T, ch swarm.Chunk, peers ...swarm.Address) int {
	t.Helper()
	count := 0
	for _, p := range peers {
		for _, rec := range streamRecords(t, &n.stripeNode, p, rangesync.FetchStreamName) {
			ms, err := protobuf.ReadMessages(bytes.NewReader(rec.Out()), func() protobuf.Message { return new(pb.Delivery) })
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range ms {
				if bytes.Equal(m.(*pb.Delivery).Address, ch.Address().Bytes()) {
					count++
				}
			}
		}
	}
	return count
}

// wrapStream returns spec with the handler of stream name replaced by wrap
// of it.
func wrapStream(spec p2p.ProtocolSpec, name string, wrap func(p2p.HandlerFunc) p2p.HandlerFunc) p2p.ProtocolSpec {
	specs := make([]p2p.StreamSpec, len(spec.StreamSpecs))
	copy(specs, spec.StreamSpecs)
	for i := range specs {
		if specs[i].Name == name {
			specs[i].Handler = wrap(specs[i].Handler)
		}
	}
	spec.StreamSpecs = specs
	return spec
}

// waitThen returns a wrapper that runs the handler once c is closed (or
// done is).
func waitThen(c, done <-chan struct{}) func(p2p.HandlerFunc) p2p.HandlerFunc {
	return func(h p2p.HandlerFunc) p2p.HandlerFunc {
		return func(ctx context.Context, p p2p.Peer, s p2p.Stream) error {
			select {
			case <-c:
			case <-done:
				return s.Reset()
			}
			return h(ctx, p, s)
		}
	}
}

// signalThen returns a wrapper that closes c (once) when the handler is
// called, then runs it once proceed is closed, or resets the stream once
// done is. With proceed nil it stalls instead: it reads the stream until the
// requester resets it, and serves nothing.
func signalThen(c chan struct{}, proceed, done <-chan struct{}) func(p2p.HandlerFunc) p2p.HandlerFunc {
	var once sync.Once
	return func(h p2p.HandlerFunc) p2p.HandlerFunc {
		return func(ctx context.Context, p p2p.Peer, s p2p.Stream) error {
			once.Do(func() { close(c) })
			if proceed == nil {
				_, _ = io.Copy(io.Discard, s)
				return nil
			}
			select {
			case <-proceed:
			case <-done:
				return s.Reset()
			}
			return h(ctx, p, s)
		}
	}
}

// baseChunks returns chunks every node of a test holds, so that a node
// lacking a few chunks is not bootstrapping (reconciler.IsBootstrap) and
// runs per-peer round sessions.
func baseChunks() []swarm.Chunk {
	return chunktesting.GenerateTestRandomChunks(8)
}

// srep sets up §7.8 with real services: node A, whose overlay is the lowest
// so that it initiates, and B1 holding {x, y}, B2 holding {x, z}. B1's fetch
// handler signals b1Wanted when A's Want arrives and serves it once
// b1Proceed is closed, or never when B1 stalls; B2's sessions wait for
// b1Wanted, so that B1's fetch holds the claims when B2's session lists x.
type srep struct {
	a                   *roundNode
	b1, b2              *stripeNode
	x, y, z             swarm.Chunk
	b1Wanted, b1Proceed chan struct{}
}

func newSREP(t *testing.T, fetchTimeout time.Duration, stall bool) *srep {
	t.Helper()
	chs := chunktesting.GenerateTestRandomChunks(3)
	s := &srep{x: chs[0], y: chs[1], z: chs[2], b1Wanted: make(chan struct{})}
	if !stall {
		s.b1Proceed = make(chan struct{})
	}
	done := make(chan struct{})
	b1Addr, b2Addr := addr(0xa0, 0), addr(0xb0, 0)
	base := baseChunks()
	s.b1 = newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(b1Addr)), b1Addr, append(base, s.x, s.y)...)
	s.b2 = newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(b2Addr)), b2Addr, append(base, s.x, s.z)...)
	s.a = newRoundNode(t, addr(0x10, 0), roundNodeOptions{fetchTimeout: fetchTimeout, roundInterval: time.Hour, chunks: base}, b1Addr, b2Addr)
	s.a.start(t, []*stripeNode{s.b1, s.b2}, map[string]p2p.ProtocolSpec{
		b1Addr.String(): wrapStream(s.b1.svc.Protocol(), rangesync.FetchStreamName, signalThen(s.b1Wanted, s.b1Proceed, done)),
		b2Addr.String(): wrapStream(s.b2.svc.Protocol(), rangesync.ReconcileStreamName, waitThen(s.b1Wanted, done)),
	})
	t.Cleanup(func() { close(done) }) // before the nodes close
	return s
}

// TestSREPFetchedOnce is §7.8 with real services: B1 and B2 both have x. The
// round's sessions with both find x; B1's fetch claims it first, so B2's is
// denied it and wants only z. x is fetched once.
func TestSREPFetchedOnce(t *testing.T) {
	t.Parallel()

	s := newSREP(t, 0, false)
	waitClosed(t, "B1's Want", s.b1Wanted)
	waitFor(t, "z from B2", func() bool { return s.a.has(t, s.z) })
	if s.a.has(t, s.x) {
		t.Fatal("x stored before B1 delivered")
	}
	close(s.b1Proceed)
	waitFor(t, "x and y from B1", func() bool { return s.a.has(t, s.x) && s.a.has(t, s.y) })

	if n := s.a.deliveriesOf(t, s.x, s.b1.addr, s.b2.addr); n != 1 {
		t.Fatalf("x delivered %d times, want 1", n)
	}
	if n := s.a.deliveriesOf(t, s.x, s.b1.addr); n != 1 {
		t.Fatalf("x delivered %d times by B1, want 1", n)
	}
	wants := s.a.wantsTo(t, s.b2.addr)
	if len(wants) != 1 || len(wants[0]) != 1 || !wants[0][0].Equal(s.z.Address()) {
		t.Fatalf("wants to B2 %v, want one of z", wants)
	}
	if got := s.a.r.Score(s.b1.addr); got != 0 {
		t.Fatalf("B1 score %d", got)
	}
}

// TestSREPStallRetry is §7.8 with real services: B1 wins the claims on x and
// y and stalls. Once A's fetch timeout (ClaimDeadline/2 by default) ends B1's
// fetch, the claim-and-stall is reported to the reconciler's failure score
// and x is fetched from B2, which the want set denied it before. y, which
// only B1 listed, is left to a later round.
func TestSREPStallRetry(t *testing.T) {
	t.Parallel()

	const fetchTimeout = time.Second
	s := newSREP(t, fetchTimeout, true)
	waitClosed(t, "B1's Want", s.b1Wanted)
	wanted := time.Now()
	waitFor(t, "z from B2", func() bool { return s.a.has(t, s.z) })
	if w := s.a.wantsTo(t, s.b2.addr); len(w) != 1 || len(w[0]) != 1 || !w[0][0].Equal(s.z.Address()) {
		t.Fatalf("wants to B2 %v, want one of z (x denied)", w)
	}

	waitFor(t, "x from B2", func() bool { return s.a.has(t, s.x) })
	if elapsed := time.Since(wanted); elapsed < fetchTimeout {
		t.Fatalf("x fetched %s after B1's Want, before the fetch timeout %s", elapsed, fetchTimeout)
	}
	if n := s.a.deliveriesOf(t, s.x, s.b1.addr, s.b2.addr); n != 1 {
		t.Fatalf("x delivered %d times, want 1", n)
	}
	wants := s.a.wantsTo(t, s.b2.addr)
	if len(wants) != 2 || len(wants[1]) != 1 || !wants[1][0].Equal(s.x.Address()) {
		t.Fatalf("wants to B2 %v, want z then x", wants)
	}
	if s.a.has(t, s.y) {
		t.Fatal("y stored: only the stalled B1 has it")
	}
	if got := s.a.r.Score(s.b1.addr); got != 1 {
		t.Fatalf("B1 score %d, want 1 (claim and stall)", got)
	}
	if !s.a.rec.IsDisconnected(s.b1.addr) {
		t.Fatal("B1 not disconnected")
	}
	if got := s.a.r.Score(s.b2.addr); got != 0 {
		t.Fatalf("B2 score %d", got)
	}
}

// TestRejectedNotRefetchedEachRound: a chunk rejected by this node is fetched
// once, not every round, while a peer keeps listing it (§8.2 rejected cache):
// a stale stamp (§7.7: this node holds a newer stamp, Put returns
// ErrOverwriteNewerChunk) and a stamp this node's stamp validator rejects
// (an expired batch this node has evicted while the peer has not).
func TestRejectedNotRefetchedEachRound(t *testing.T) {
	t.Parallel()

	for _, stale := range []bool{true, false} {
		name := "invalid stamp"
		if stale {
			name = "stale stamp"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			old := chunktesting.GenerateTestRandomChunk()
			oldID := chunkIDOf(t, old)
			base := baseChunks()
			o := roundNodeOptions{roundInterval: 20 * time.Millisecond, chunks: base}
			if stale {
				newer := swarm.NewChunk(old.Address(), old.Data()).WithStamp(postagetesting.MustNewStamp())
				o.chunks = append(base, newer)
				o.putHook = func(ch swarm.Chunk) error {
					if sh, err := ch.Stamp().Hash(); err == nil && bytes.Equal(sh, oldID.StampHash) {
						return storage.ErrOverwriteNewerChunk
					}
					return nil
				}
			} else {
				o.validStamp = func(ch swarm.Chunk) (swarm.Chunk, error) {
					if ch.Address().Equal(old.Address()) {
						return nil, fmt.Errorf("batchstore get: %w", postage.ErrNotFound) // batch expired
					}
					return ch, nil
				}
			}
			bAddr := addr(0xa0, 0)
			b := newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(bAddr)), bAddr, append(base, old)...)
			a := newRoundNode(t, addr(0x10, 0), o, bAddr)
			a.start(t, []*stripeNode{b}, nil)

			const rounds = 4 // the peer's session burst (PeerSessionBurst)
			waitFor(t, "sessions", func() bool {
				return len(streamRecords(t, &a.stripeNode, bAddr, rangesync.ReconcileStreamName)) >= rounds
			})
			settle() // let the last session's fetch run
			if w := a.wantsTo(t, bAddr); len(w) != 1 {
				t.Fatalf("%d wants to the peer in %d sessions, want 1", len(w), rounds)
			}
			if a.has(t, old) {
				t.Fatal("rejected chunk stored")
			}
		})
	}
}
