// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The per-peer fetch credit (T4.7 part 4, rangesync's credit.go) in the
// reconciler: the IDs a peer was not asked for because its credit is spent
// are retried from other peers, a stripe whose peer's credit is spent is
// reassigned at once, a peer with no credit holds no SyncRate, and the
// stripe worker runs the claims of the peers with the most credit first.

// TestWantSetRetryNoCredit: an ID a peer was not asked for because its fetch
// credit is spent (FetchResult.NoCredit) is retried from the next peer known
// to have it, as one the peer did not deliver (TestWantSetRetryChain).
func TestWantSetRetryNoCredit(t *testing.T) {
	t.Parallel()

	x := testChunkID(1)
	svc := newWantService(status(1))
	svc.lists(higher, x)
	svc.lists(higher2, x)
	svc.lists(higher3, x)
	svc.creditLeft[higher2.ByteString()] = 0
	b1Claimed := svc.claimSignal(higher)
	stall := make(chan struct{})
	svc.stall[higher.ByteString()] = stall
	gate2, gate3 := make(chan struct{}), make(chan struct{})
	svc.set(func() {
		svc.peerGate[higher2.ByteString()] = gate2
		svc.peerGate[higher3.ByteString()] = gate3
	})
	h := newWantHarness(t, svc, higher, higher2, higher3)
	h.r.Start(context.Background())

	waitClosed(t, "B1 claimed", b1Claimed)
	close(gate2)
	waitFor(t, "B2 denied x", func() bool { return len(svc.fetchesFrom(higher2)) == 1 })
	close(gate3)
	waitFor(t, "B3 denied x", func() bool { return len(svc.fetchesFrom(higher3)) == 1 })
	close(stall) // B1's fetch fails: x goes to B2, whose credit is spent, then to B3
	waitDelivered(t, svc, x)
	assertDelivered(t, svc, x, higher3)
	if n := len(svc.fetchesFrom(higher2)); n != 2 {
		t.Fatalf("%d fetches from B2, want 2", n)
	}
	if n := h.r.Score(higher2); n != 0 {
		t.Fatalf("B2 scored %d", n)
	}
}

// TestStripeNoCreditReassigned: a stripe whose peer's fetch credit runs out
// is reassigned at once, although the peer stored chunks first (progress
// would restart it with the same peer), and nobody is blamed: the peer it
// goes to delivers the rest. A peer that lists junk does not hold a stripe
// for its inactivity timeout, and not for restarts.
func TestStripeNoCreditReassigned(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	ids := []storer.ChunkID{testChunkID(1), testChunkID(2), testChunkID(3), testChunkID(4)}
	svc := newWantService(status(1))
	svc.lists(higher, ids...)
	svc.lists(higher2, ids...)
	svc.creditLeft[higher.ByteString()] = 2
	r := newStripeReconciler(t, svc)
	res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher, higher2}}})[0]
	if res.Err != nil {
		t.Fatalf("stripe failed: %v", res.Err)
	}
	if !res.Peer.Equal(higher2) || res.Stored != len(ids) {
		t.Fatalf("stripe ended with %s, stored %d; want %s, %d", res.Peer, res.Stored, higher2, len(ids))
	}
	if got := r.StripesTotal("reassigned"); got != 1 {
		t.Fatalf("stripes reassigned %v, want 1", got)
	}
	if n := len(svc.fetchesFrom(higher)); n != 1 {
		t.Fatalf("%d fetches from the peer with no credit, want 1: restarted", n)
	}
	for i, id := range ids {
		by := higher2
		if i < 2 {
			by = higher
		}
		assertDelivered(t, svc, id, by)
	}
	for _, p := range []swarm.Address{higher, higher2} {
		if n := r.Score(p); n != 0 {
			t.Fatalf("%s scored %d", p, n)
		}
	}
}

// TestSyncRateNoCredit: a peer whose fetch credit is spent holds no SyncRate
// with the catch-ups it lists (over a real service): its catch-up fetch is
// refused at once, without a stream, so no catch-up fetch stays in flight,
// and SyncRate reads 0 at once, the node-wide catch-up grace unspent but
// for an instant.
func TestSyncRateNoCredit(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Unix(1_000_000, 0)}
	var opens countFetchOpens
	a := newRoundNode(t, addr(0x10, 0), roundNodeOptions{
		roundInterval: time.Hour,
		now:           clock.Now,
		svcNow:        clock.Now,
		streamer: func(s p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
			opens.StreamerDisconnecter = s
			return &opens
		},
	})
	bAddr := addr(0xa0, 0)
	b := newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(bAddr)), bAddr)
	a.routes[b.addr.String()] = b.svc.Protocol()
	b.rec.SetProtocols(a.svc.Protocol())
	exchangeStatus(t, a, b)
	a.r.Start(context.Background())
	clock.Add(reconciler.StartupGuardTimeout) // no neighbor: the guard ends at its timeout

	junk := func(n int) []storer.ChunkID {
		ids := make([]storer.ChunkID, n)
		for i := range ids {
			ids[i] = chunkIDOf(t, chunktesting.GenerateTestRandomChunk())
		}
		return ids
	}
	// small fresh junk, unscored, until the peer's credit is spent
	for i := 0; ; i++ {
		_, err := a.svc.FetchWithResult(context.Background(), b.addr, junk(rangesync.MaxWantIDs/4))
		if errors.Is(err, rangesync.ErrFetchCredit) {
			break
		}
		if err != nil || i > 4 {
			t.Fatalf("fetch %d: %v", i, err)
		}
	}
	clock.Add(reconciler.CatchUpGrace)
	if got := a.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v, want 0", got)
	}
	opened := opens.n.Load()
	for range 5 {
		a.r.Wants(b.addr, junk(reconciler.CatchUpMinDiff))
		waitFor(t, "the catch-up fetch refused", func() bool { return a.r.CatchUpFetches() == 0 })
		if got := a.r.SyncRate(); got != 0 {
			t.Fatalf("sync rate %v with the catch-up refused, want 0", got)
		}
	}
	if n := opens.n.Load() - opened; n != 0 {
		t.Fatalf("%d fetch streams opened to the peer with no credit", n)
	}
	if score := a.r.Score(b.addr); score != 0 {
		t.Fatalf("peer scored %d", score)
	}
}

// countFetchOpens is a streamer that counts the fetch streams it opens.
type countFetchOpens struct {
	p2p.StreamerDisconnecter
	n atomic.Int64
}

func (c *countFetchOpens) NewStream(ctx context.Context, addr swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	if stream == rangesync.FetchStreamName {
		c.n.Add(1)
	}
	return c.StreamerDisconnecter.NewStream(ctx, addr, h, protocol, version, stream)
}

// creditPinService is a pinService whose peers have the fetch credit in
// credit (rangesync.Service.FetchCredit).
type creditPinService struct {
	*pinService
	credit map[string]int
}

func (s *creditPinService) FetchCredit(peer swarm.Address) int { return s.credit[peer.ByteString()] }

// TestClaimOrderByCredit is part 3's hand-over (T4.7 part 3 fix round 1, review
// Low 2): an honest claim queued behind the claims of peers whose claims
// store nothing waited claimIdleTimeout for each of them, with SyncRate at
// 0. The stripe worker now runs the claim of the peer with the most fetch
// credit next, the per-overlay record kept across reconnects: the honest
// neighbor's claim runs right after the liar's claim in flight ends, before
// the other liar's claim queued before it. Without a credit (a service
// without one), the claims run in the order queued.
func TestClaimOrderByCredit(t *testing.T) {
	t.Parallel()

	const largeDiff = 100
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	claimed, liars, honests := root.Child(2, 0), root.Child(2, 1), root.Child(2, 3)
	liar, liar2, honest := higher, higher2, addr(0xf1, 0)
	next := func(t *testing.T, credit bool) fingerprint.Range {
		t.Helper()
		svc := newPinService(status(1000))
		for _, p := range []swarm.Address{liar, liar2, honest} {
			svc.remote[p.ByteString()] = status(1000)
		}
		svc.remote[liar.ByteString()] = status(1001) // within LargeDiff: round sessions run
		svc.offer[liar.ByteString()] = claimed       // its sessions' opening claims far more there
		svc.stall[liar.ByteString()] = true
		svc.stall[liar2.ByteString()] = true
		var s reconciler.Service = svc
		if credit {
			s = &creditPinService{pinService: svc, credit: map[string]int{honest.ByteString(): 1000}}
		}
		h := newPinHarness(t, s, svc.fakeService, largeDiff, liar, liar2, honest)
		waitFor(t, "the liar's stripe in flight", func() bool { return stripeSessions(svc.fakeService, liar, claimed) > 0 })
		// queued behind it: the other liar's claim, then the honest one's
		if !h.r.EnqueuePeerStripes(liar2, []fingerprint.Range{liars}) || !h.r.EnqueuePeerStripes(honest, []fingerprint.Range{honests}) {
			t.Fatal("claims not registered")
		}
		h.clock.Add(reconciler.ClaimIdleTimeout)
		waitFor(t, "claim ended", func() bool { return h.r.StripesTotal("idle") >= 1 })
		var rg fingerprint.Range
		waitFor(t, "the next claim runs", func() bool {
			r, _, ok := h.r.RunningStripe()
			rg = r
			return ok && !r.Equal(claimed)
		})
		for _, p := range []swarm.Address{liar, liar2, honest} {
			if n := h.r.Score(p); n != 0 {
				t.Fatalf("%s scored %d", p, n)
			}
		}
		return rg
	}
	if got := next(t, true); !got.Equal(honests) {
		t.Fatalf("with the credit, %s ran next, want the honest claim %s", got, honests)
	}
	if got := next(t, false); !got.Equal(liars) {
		t.Fatalf("without a credit, %s ran next, want the first queued %s", got, liars)
	}
}
