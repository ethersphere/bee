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
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
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
	// now is the reconciler's clock (Options.Now) and svcNow the service's
	// (rangesync.Options.Now); time.Now if nil.
	now, svcNow func() time.Time
	// streamErr is the recorder's stream error (streamtest.WithStreamError);
	// nil: none.
	streamErr func(swarm.Address, string, string, string) error
	// streamer, if not nil, wraps the recorder the service opens streams
	// with.
	streamer func(p2p.StreamerDisconnecter) p2p.StreamerDisconnecter
	// striper wires the reconciler's Striper of the sessions the service
	// serves (rangesync.Options.Striper), as pkg/node does; largeDiff is
	// the LargeDiff of both (zero: rangesync.LargeDiff).
	striper   bool
	largeDiff uint64
}

func newRoundNode(t *testing.T, a swarm.Address, o roundNodeOptions, peers ...swarm.Address) *roundNode {
	t.Helper()
	if o.validStamp == nil {
		o.validStamp = func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	}
	n := &roundNode{routes: make(map[string]p2p.ProtocolSpec)}
	n.addr = a
	n.store = mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithChunks(o.chunks...), mockstorer.WithPutHook(o.putHook))
	recOpts := []streamtest.Option{streamtest.WithBaseAddr(a), streamtest.WithPeerProtocols(n.routes)}
	if o.streamErr != nil {
		recOpts = append(recOpts, streamtest.WithStreamError(o.streamErr))
	}
	n.rec = streamtest.NewRecorderDisconnecter(streamtest.New(recOpts...))
	var streamer p2p.StreamerDisconnecter = n.rec
	if o.streamer != nil {
		streamer = o.streamer(n.rec)
	}
	n.r = reconciler.New(a, &fakeTopology{peers: peers}, nil, log.Noop, reconciler.Options{RoundInterval: o.roundInterval, Now: o.now, LargeDiff: o.largeDiff})
	so := rangesync.Options{
		LeafBits:     8,
		Scorer:       n.r,
		Wants:        n.r.Wants,
		FetchTimeout: o.fetchTimeout,
		Now:          o.svcNow,
		LargeDiff:    o.largeDiff,
	}
	if o.striper {
		so.Striper = n.r.ServedStriper
	}
	n.svc = rangesync.New(streamer, n.store, a, nil, nil, o.validStamp, log.Noop, so)
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

// start routes n's streams to each peer through spec (the peer's protocol
// if nil) and exchanges Statuses, then starts n's reconciler.
//
// Each peer holds n's Status before the reconciler starts. streamtest's
// FullClose returns before the remote handler has stored a Status (libp2p's
// waits for it), so the Status of the first round's probe may not have
// arrived when that round's sessions open, which the peer would answer Busy
// (no Status of n), and the next round is an hour away.
func (n *roundNode) start(t *testing.T, peers []*stripeNode, specs map[string]p2p.ProtocolSpec) {
	t.Helper()
	for _, p := range peers {
		spec, ok := specs[p.addr.String()]
		if !ok {
			spec = p.svc.Protocol()
		}
		n.routes[p.addr.String()] = spec
		p.rec.SetProtocols(n.svc.Protocol())
		exchangeStatus(t, n, p)
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
	// started is taken before A's reconciler starts, so before any fetch
	// (and its fetch timeout) starts.
	started time.Time
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
	s.started = time.Now()
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
	waitFor(t, "z from B2", func() bool { return s.a.has(t, s.z) })
	if w := s.a.wantsTo(t, s.b2.addr); len(w) != 1 || len(w[0]) != 1 || !w[0][0].Equal(s.z.Address()) {
		t.Fatalf("wants to B2 %v, want one of z (x denied)", w)
	}

	waitFor(t, "x from B2", func() bool { return s.a.has(t, s.x) })
	// B1's fetch, and its timeout, started after s.started: x can be stored
	// no sooner than the timeout after it
	if elapsed := time.Since(s.started); elapsed < fetchTimeout {
		t.Fatalf("x fetched %s after the start, before B1's fetch timeout %s", elapsed, fetchTimeout)
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

// badSignature returns ch with a stamp whose signature does not recover (an
// invalid recovery code).
func badSignature(ch swarm.Chunk) swarm.Chunk {
	st := ch.Stamp()
	sig := append([]byte(nil), st.Sig()...)
	sig[len(sig)-1] = 0
	return swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(postage.NewStamp(st.BatchID(), st.Index(), st.Timestamp(), sig))
}

// recoverStamp is a stamp validator that fails as postage.ValidStamp does
// when a signature does not recover: with the raw recovery error.
func recoverStamp(ch swarm.Chunk) (swarm.Chunk, error) {
	if _, err := postage.RecoverBatchOwner(ch.Address(), ch.Stamp()); err != nil {
		return nil, err
	}
	return ch, nil
}

// TestStripeBadSignatures (T4.4 fix round 2): a stripe peer that lists IDs
// it made up on a real batch and delivers them with stamps whose signatures
// do not recover cannot pin the stripe. The IDs are rejected, not deferred:
// the stripe is not asked for them again, completes in one session (and so
// can be verified), and the valid chunks are stored. Since T4.5 part 2b such
// stamps count against the peer (rangesync's fetch shortfall): up to the
// tolerance (4 here) it is not scored (TestStripeBadSignaturesScored).
func TestStripeBadSignatures(t *testing.T) {
	t.Parallel()

	const bad = 4 // the fetch's shortfall tolerance
	a, bAddr, chunks, good := badSignatureStripe(t, bad)

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := a.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{bAddr}}})[0]
	if res.Err != nil {
		t.Fatalf("stripe failed: %v", res.Err)
	}
	if res.Listed != len(chunks) || res.Stored != good || res.Undelivered != 0 || res.Deferred != 0 {
		t.Fatalf("listed %d, stored %d, undelivered %d, deferred %d", res.Listed, res.Stored, res.Undelivered, res.Deferred)
	}
	if n := len(streamRecords(t, &a.stripeNode, bAddr, rangesync.ReconcileStreamName)); n != 1 {
		t.Fatalf("%d sessions with the peer, want 1", n)
	}
	if n := len(a.wantsTo(t, bAddr)); n != 1 {
		t.Fatalf("%d wants to the peer, want 1 (no IDs asked for again)", n)
	}
	for i, ch := range chunks {
		if got, want := a.has(t, ch), i < good; got != want {
			t.Fatalf("chunk %d stored %v, want %v", i, got, want)
		}
	}
	if got := a.r.Score(bAddr); got != 0 {
		t.Fatalf("peer score %d", got)
	}
}

// TestStripeBadSignaturesScored (T4.5 part 2b): past the fetch's shortfall
// tolerance, a stripe peer's stamps that are invalid on every node are an
// offense of its fetch: the peer is scored once, through the single
// PeerScorer, without a disconnect (a first offense), and its IDs are
// rejected, so not requested again. The stripe's own withholding rule adds
// no score.
func TestStripeBadSignaturesScored(t *testing.T) {
	t.Parallel()

	a, bAddr, chunks, good := badSignatureStripe(t, 8)

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := a.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{bAddr}}})[0]
	if res.Stored != good {
		t.Fatalf("stored %d, want %d", res.Stored, good)
	}
	if got := a.r.Score(bAddr); got != 1 {
		t.Fatalf("peer score %d, want 1", got)
	}
	if a.rec.IsDisconnected(bAddr) {
		t.Fatal("peer disconnected on a first offense")
	}
	if n := len(a.wantsTo(t, bAddr)); n != 1 {
		t.Fatalf("%d wants to the peer, want 1 (rejected IDs not asked for again)", n)
	}
	for i, ch := range chunks {
		if got, want := a.has(t, ch), i < good; got != want {
			t.Fatalf("chunk %d stored %v, want %v", i, got, want)
		}
	}
}

// badSignatureStripe returns a node a whose stamp validator recovers
// signatures, and a stripe peer at bAddr, with exchanged Statuses, that holds
// 2 valid chunks (the first good of chunks) and bad chunks whose stamps'
// signatures do not recover.
func badSignatureStripe(t *testing.T, bad int) (a *roundNode, bAddr swarm.Address, chunks []swarm.Chunk, good int) {
	t.Helper()
	good = 2 // streamtest deadlocks past 16 deliveries per Want
	chunks = chunktesting.GenerateTestRandomChunks(good)
	for _, ch := range chunktesting.GenerateTestRandomChunks(bad) {
		chunks = append(chunks, badSignature(ch))
	}
	bAddr = addr(0xa0, 0)
	b := newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(bAddr)), bAddr, chunks...)
	a = newRoundNode(t, addr(0x10, 0), roundNodeOptions{validStamp: recoverStamp, roundInterval: time.Hour}, bAddr)
	a.routes[bAddr.String()] = b.svc.Protocol()
	b.rec.SetProtocols(a.svc.Protocol())
	exchangeStatus(t, a, b)
	a.r.SetStripeRetryInterval(time.Millisecond)
	return a, bAddr, chunks, good
}

// exchangeStatus has a and b send each other their Status and waits until
// each holds the other's.
func exchangeStatus(t *testing.T, a *roundNode, b *stripeNode) {
	t.Helper()
	if err := a.svc.SendStatus(context.Background(), b.addr); err != nil {
		t.Fatal(err)
	}
	if err := b.svc.SendStatus(context.Background(), a.addr); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "statuses", func() bool {
		_, ab := b.svc.LastStatus(a.addr)
		_, ba := a.svc.LastStatus(b.addr)
		return ab && ba
	})
}

// TestSyncRateCatchUpStoring (T4.5 part 2c fix round 2) is the honest side
// of the catch-up grace, over a real service: a catch-up that stores chunks
// keeps SyncRate above 0 past the grace, however slowly it stores, because
// each chunk counts towards the rate when it is stored, not when the fetch
// returns. A catch-up that stops storing for longer than the window reads 0,
// although its fetch is still in flight, and rises again once it stores.
//
// The session listed CatchUpMinDiff IDs, of which this node lacks missing:
// streamtest's streams deadlock when a reader lags many writes behind.
func TestSyncRateCatchUpStoring(t *testing.T) {
	t.Parallel()

	const missing = 6
	clock := &fakeClock{now: time.Unix(1_000_000, 0)}
	chs := chunktesting.GenerateTestRandomChunks(reconciler.CatchUpMinDiff)
	var puts atomic.Int64
	gate := make(chan struct{})
	a := newRoundNode(t, addr(0x10, 0), roundNodeOptions{
		now:           clock.Now,
		roundInterval: time.Hour,
		chunks:        chs[missing:],
		putHook: func(swarm.Chunk) error {
			if puts.Add(1) == 2 {
				<-gate // the second chunk waits
			}
			return nil
		},
	})
	var once sync.Once
	open := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(open) // before the node's cleanup, which waits for the fetch
	bAddr := addr(0xa0, 0)
	b := newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(bAddr)), bAddr, chs...)
	a.routes[b.addr.String()] = b.svc.Protocol()
	b.rec.SetProtocols(a.svc.Protocol())
	exchangeStatus(t, a, b)
	a.r.Start(context.Background())
	clock.Add(reconciler.StartupGuardTimeout) // no neighbor: the guard ends at its timeout
	if got := a.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v before the catch-up", got)
	}

	ids := make([]storer.ChunkID, len(chs))
	for i, ch := range chs {
		ids[i] = chunkIDOf(t, ch)
	}
	a.r.Wants(b.addr, ids) // a session a served listed them
	waitFor(t, "first chunk stored", func() bool { return puts.Load() == 2 })
	clock.Add(10 * reconciler.CatchUpGrace)
	if got := a.r.SyncRate(); got < reconciler.StartupGuardRate {
		t.Fatalf("sync rate %v past the grace while storing, want at least %v", got, reconciler.StartupGuardRate)
	}
	clock.Add(reconciler.SyncRateWindow)
	if got := a.r.SyncRate(); got <= 0 {
		t.Fatalf("sync rate %v within the window of the last chunk", got)
	}
	clock.Add(reconciler.SyncRateWindow + time.Second)
	if n := a.r.CatchUpFetches(); n != 1 {
		t.Fatalf("%d catch-up fetches in flight, want 1", n)
	}
	if got := a.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v with nothing stored for two windows, want 0", got)
	}

	open()
	waitFor(t, "catch-up done", func() bool { return a.r.CatchUpFetches() == 0 })
	for _, ch := range chs {
		if !a.has(t, ch) {
			t.Fatalf("chunk %s not stored", ch.Address())
		}
	}
	if got := a.r.SyncRate(); got <= 0 {
		t.Fatalf("sync rate %v after the catch-up stored", got)
	}
	clock.Add(2*reconciler.SyncRateWindow + time.Second)
	if got := a.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v past the window, want 0", got)
	}
	if score := a.r.Score(b.addr); score != 0 {
		t.Fatalf("peer scored %d", score)
	}
}

// gatedFetchOpens is a streamer whose first fetch stream open waits until
// release is closed; entered is closed when it starts.
type gatedFetchOpens struct {
	p2p.StreamerDisconnecter
	entered, release chan struct{}
	first            sync.Once
}

func (g *gatedFetchOpens) NewStream(ctx context.Context, addr swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	if stream == rangesync.FetchStreamName {
		first := false
		g.first.Do(func() { first = true })
		if first {
			close(g.entered)
			select {
			case <-g.release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return g.StreamerDisconnecter.NewStream(ctx, addr, h, protocol, version, stream)
}

// TestWantsServedChurn (T4.5 part 2c fix round 2) is honest churn in the
// sessions this node serves: a peer lists IDs in one session and again in
// the next, which completes while this node's fetch of the first list runs;
// meanwhile the peer evicted them, so it delivers none. The IDs of the
// running batch are not queued again, so the peer is asked for them once
// and not scored: asked again, it would leave them missing again, which
// counts as junk listed again (the shortfall rules), an offense, and a
// second one within the shortfall window disconnects it.
func TestWantsServedChurn(t *testing.T) {
	t.Parallel()

	gate := &gatedFetchOpens{entered: make(chan struct{}), release: make(chan struct{})}
	a := newRoundNode(t, addr(0x10, 0), roundNodeOptions{
		roundInterval: time.Hour,
		streamer: func(s p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
			gate.StreamerDisconnecter = s
			return gate
		},
	})
	var once sync.Once
	release := func() { once.Do(func() { close(gate.release) }) }
	t.Cleanup(release)
	sentinel := chunktesting.GenerateTestRandomChunk()
	bAddr := addr(0xa0, 0)
	b := newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(bAddr)), bAddr, sentinel)
	a.routes[b.addr.String()] = b.svc.Protocol()
	b.rec.SetProtocols(a.svc.Protocol())
	exchangeStatus(t, a, b)
	a.r.Start(context.Background())

	evicted := make([]storer.ChunkID, 10) // above the shortfall tolerance
	isEvicted := make(map[string]bool, len(evicted))
	for i := range evicted {
		evicted[i] = chunkIDOf(t, chunktesting.GenerateTestRandomChunk())
		isEvicted[evicted[i].Address.ByteString()] = true
	}
	a.r.Wants(b.addr, evicted)
	waitClosed(t, "the first fetch's open", gate.entered)
	a.r.Wants(b.addr, evicted)                                  // the next session listed them again
	a.r.Wants(b.addr, []storer.ChunkID{chunkIDOf(t, sentinel)}) // fetched after them
	release()
	waitFor(t, "the sentinel", func() bool { return a.has(t, sentinel) })

	asked := 0
	for _, want := range a.wantsTo(t, b.addr) {
		for _, id := range want {
			if isEvicted[id.ByteString()] {
				asked++
			}
		}
	}
	if asked != len(evicted) {
		t.Fatalf("the evicted IDs were asked for %d times in all, want %d (once each)", asked, len(evicted))
	}
	if score := a.r.Score(b.addr); score != 0 {
		t.Fatalf("peer scored %d", score)
	}
	if a.rec.IsDisconnected(b.addr) {
		t.Fatal("peer disconnected")
	}
}
