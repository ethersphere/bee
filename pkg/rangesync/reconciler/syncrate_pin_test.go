// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The stripe-registry SyncRate pin (T4.7 part 3): a registered stripe job
// holds SyncRate only while it stores chunks, past a grace, and a peer's
// claim that stores nothing leaves the stripe worker.

// pinService is a fakeService whose stripe sessions (those with an Open) are
// set per peer: a staller's stripe sessions block until their context ends
// (a liar whose stripe stalls); the others list lists IDs, one per batch, in
// the first session over a range and none after (a holder: the transfer,
// then an equal verification), or in every session (every); only over
// ranges inside one of inside, if set (and only those answer Busy, busyN).
// Its fetches report each chunk as they store it, as the Service's
// FetchWithProgress does; those from a gated peer wait for the test
// (calls).
type pinService struct {
	*fakeService

	pmu    sync.Mutex
	stall  map[string]bool
	lists  map[string]int
	every  map[string]bool
	gated  map[string]bool
	inside []fingerprint.Range
	listed map[string]bool // ranges listed once
	calls  chan gatedFetch
}

// gatedFetch is a fetch of a gated peer waiting for the test: closing release
// lets it go on, and done then receives the number of chunks it stored.
type gatedFetch struct {
	release chan struct{}
	done    chan int
}

func newPinService(local rangesync.Status) *pinService {
	return &pinService{
		fakeService: newFakeService(local),
		stall:       make(map[string]bool),
		lists:       make(map[string]int),
		every:       make(map[string]bool),
		gated:       make(map[string]bool),
		listed:      make(map[string]bool),
		calls:       make(chan gatedFetch),
	}
}

func (s *pinService) ReconcileWith(ctx context.Context, peer swarm.Address, opts rangesync.ReconcileOptions) ([]storer.ChunkID, error) {
	if len(opts.Open) == 0 {
		return s.fakeService.ReconcileWith(ctx, peer, opts)
	}
	k := peer.ByteString()
	s.pmu.Lock()
	in := len(s.inside) == 0 || slices.ContainsFunc(s.inside, func(r fingerprint.Range) bool { return r.ContainsRange(opts.Open[0]) })
	s.pmu.Unlock()
	s.mu.Lock()
	s.opens[k] = append(s.opens[k], opts.Open)
	busy := in && s.busyN[k] > 0
	if busy {
		s.busyN[k]--
	}
	s.mu.Unlock()
	if busy {
		return nil, rangesync.ErrPeerBusy
	}
	s.pmu.Lock()
	stall, n := s.stall[k], 0
	if rk := opts.Open[0].String(); !stall && in && s.lists[k] > 0 && (s.every[k] || !s.listed[rk]) {
		s.listed[rk], n = true, s.lists[k]
	}
	s.pmu.Unlock()
	if stall {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	for range n {
		opts.Wants(make([]storer.ChunkID, 1))
	}
	return nil, ctx.Err()
}

// FetchWithResult is FetchWithProgress without the reports.
func (s *pinService) FetchWithResult(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (rangesync.FetchResult, error) {
	return s.FetchWithProgress(ctx, peer, ids, nil)
}

// FetchWithProgress fetches as fakeService.Fetch, after the test released
// it for a gated peer, and calls stored once per chunk stored before it
// reports to the test, so that the reconciler counted the chunks when drip
// returns.
func (s *pinService) FetchWithProgress(ctx context.Context, peer swarm.Address, ids []storer.ChunkID, stored func()) (rangesync.FetchResult, error) {
	s.pmu.Lock()
	gated := s.gated[peer.ByteString()]
	s.pmu.Unlock()
	report := func(n int) {
		for range n {
			if stored != nil {
				stored()
			}
		}
	}
	if !gated {
		n, err := s.Fetch(ctx, peer, ids)
		report(n)
		return rangesync.FetchResult{Stored: n}, err
	}
	if err := ctx.Err(); err != nil {
		return rangesync.FetchResult{}, err
	}
	call := gatedFetch{release: make(chan struct{}), done: make(chan int, 1)}
	select {
	case s.calls <- call:
	case <-ctx.Done():
		return rangesync.FetchResult{}, ctx.Err()
	}
	select {
	case <-call.release:
	case <-ctx.Done():
	}
	if err := ctx.Err(); err != nil { // a job that ended stores nothing
		call.done <- 0
		return rangesync.FetchResult{}, err
	}
	n, err := s.Fetch(ctx, peer, ids)
	report(n)
	call.done <- n
	return rangesync.FetchResult{Stored: n}, err
}

// drip lets one gated fetch store its chunk: it releases the gated fetches
// as they come until one stores (a fetch of a job that ended stores
// nothing).
func (s *pinService) drip(t *testing.T) {
	t.Helper()
	for {
		select {
		case call := <-s.calls:
			close(call.release)
			if <-call.done > 0 {
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for a gated fetch")
		}
	}
}

// stripeSessions returns the stripe sessions (one Open range) with peer
// inside rg.
func stripeSessions(svc *fakeService, peer swarm.Address, rg fingerprint.Range) int {
	return countInside(svc, rg, peer)
}

// newPinHarness returns a started reconciler with overlay self over svc and
// fast rounds on the real clock; its start-up guard ends a millisecond after
// Start on the fake clock, which the harness moves past it. Stripe retries
// wait a millisecond and pass at once (advancingAfter).
func newPinHarness(t *testing.T, svc reconciler.Service, fake *fakeService, largeDiff uint64, peers ...swarm.Address) *harness {
	t.Helper()
	h := &harness{
		svc:   fake,
		topo:  &fakeTopology{peers: peers},
		clock: &fakeClock{now: time.Unix(1_000_000, 0)},
	}
	h.r = reconciler.New(self, h.topo, svc, log.Noop, reconciler.Options{
		Now:                 h.clock.Now,
		After:               advancingAfter(h.clock, time.Millisecond),
		RoundInterval:       10 * time.Millisecond,
		StartupGuardTimeout: time.Millisecond,
		LargeDiff:           largeDiff,
	})
	h.r.SetStripeRetryInterval(time.Millisecond)
	t.Cleanup(func() {
		if err := h.r.Close(); err != nil {
			t.Error(err)
		}
	})
	h.r.Start(context.Background())
	h.clock.Add(time.Millisecond)
	return h
}

// TestSyncRateClaimPin is the attack of the PRE-TESTNET BLOCKER (T4.5 part 2c
// round 2): a higher-overlay peer claims a count more than LargeDiff above
// this node's, through its Status (the rounds' session-root check) or a
// session's opening (the Striper), and stalls its stripe. SyncRate reads 0
// once the catch-up grace is spent, while the claim is registered and its
// transfer in flight; once the claim's job stored nothing for
// ClaimIdleTimeout it leaves the stripe worker (stripes_total{idle}), and a
// claim queued behind it runs. The liar registers its claim again and
// SyncRate stays 0. No peer is scored. Before, the registry held SyncRate at
// StartupGuardRate for as long as a claim was registered, and the liar's
// claim held the worker until its stripe's timeout (15 minutes), then its
// reassignments and verification.
func TestSyncRateClaimPin(t *testing.T) {
	t.Parallel()

	const largeDiff = 100
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	liar, honest := higher, addr(0xf1, 0)
	zero := func(t *testing.T, h *harness, when string) {
		t.Helper()
		if got := h.r.SyncRate(); got != 0 {
			t.Fatalf("sync rate %v %s, want 0", got, when)
		}
	}
	noScores := func(t *testing.T, h *harness) {
		t.Helper()
		for _, p := range []swarm.Address{liar, honest} {
			if n := h.r.Score(p); n != 0 {
				t.Fatalf("%s scored %d", p, n)
			}
		}
	}

	t.Run("status", func(t *testing.T) {
		t.Parallel()
		svc := newPinService(status(1000))
		svc.remote[liar.ByteString()] = status(1000)
		svc.remote[honest.ByteString()] = status(1000)
		svc.stall[liar.ByteString()] = true
		h := newPinHarness(t, svc, svc.fakeService, largeDiff, liar, honest)
		waitFor(t, "guard released", h.r.GuardReleased)
		zero(t, h, "in sync")

		// the liar's Status claims more than LargeDiff above this node's
		svc.set(func() { svc.remote[liar.ByteString()] = status(1000 + largeDiff + 1) })
		svc.statusChange()
		waitFor(t, "the liar's stripe in flight", func() bool { return stripeSessions(svc.fakeService, liar, root) > 0 })
		if n := h.r.PeerStripes(liar); n != 1 {
			t.Fatalf("%d claims of the liar, want 1", n)
		}
		if got := h.r.SyncRate(); got < reconciler.StartupGuardRate {
			t.Fatalf("sync rate %v within the catch-up grace, want at least %v", got, reconciler.StartupGuardRate)
		}
		h.clock.Add(reconciler.CatchUpGrace)
		zero(t, h, "past the grace, the liar's claim registered and in flight")

		// the claim leaves the worker once it stored nothing for the idle
		// timeout; the liar's Status registers it again
		sessions := stripeSessions(svc.fakeService, liar, root)
		h.clock.Add(reconciler.ClaimIdleTimeout)
		waitFor(t, "claim ended", func() bool { return h.r.StripesTotal("idle") >= 1 })
		waitFor(t, "claim registered again", func() bool { return stripeSessions(svc.fakeService, liar, root) > sessions })
		zero(t, h, "with the claim registered again")
		h.clock.Add(reconciler.ClaimIdleTimeout)
		waitFor(t, "claim ended again", func() bool { return h.r.StripesTotal("idle") >= 2 })
		zero(t, h, "after two claims")
		noScores(t, h)
	})

	t.Run("opening", func(t *testing.T) {
		t.Parallel()
		claimed, other := root.Child(2, 0), root.Child(2, 3)
		svc := newPinService(status(1000))
		svc.remote[liar.ByteString()] = status(1001) // within LargeDiff: round sessions run
		svc.remote[honest.ByteString()] = status(1000)
		svc.offer[liar.ByteString()] = claimed // its sessions' opening claims far more there
		svc.stall[liar.ByteString()] = true
		h := newPinHarness(t, svc, svc.fakeService, largeDiff, liar, honest)
		waitFor(t, "the liar's stripe in flight", func() bool { return stripeSessions(svc.fakeService, liar, claimed) > 0 })
		waitFor(t, "guard released", h.r.GuardReleased)
		h.clock.Add(reconciler.CatchUpGrace)
		zero(t, h, "past the grace, the liar's claim registered and in flight")

		// an honest peer's claim waits behind the liar's
		if !h.r.EnqueuePeerStripes(honest, []fingerprint.Range{other}) {
			t.Fatal("honest claim not registered")
		}
		settle()
		if n := stripeSessions(svc.fakeService, honest, other); n != 0 {
			t.Fatalf("%d stripe sessions of the queued claim", n)
		}
		zero(t, h, "with two claims registered")

		h.clock.Add(reconciler.ClaimIdleTimeout)
		waitFor(t, "claim ended", func() bool { return h.r.StripesTotal("idle") >= 1 })
		waitFor(t, "the queued claim runs", func() bool { return stripeSessions(svc.fakeService, honest, other) > 0 })
		zero(t, h, "after the liar's claim ended")
		noScores(t, h)
	})
}

// TestSyncRateClaimDrip: a liar that drips one real chunk per interval longer
// than two SyncRate windows (one per 25 minutes) holds SyncRate above 0 only
// as long as those chunks are in the rate window, as an honest slow peer's
// would, and not in between, although its claim of this node's root is
// registered again and in flight throughout. Before, the registry held
// SyncRate at StartupGuardRate throughout.
func TestSyncRateClaimDrip(t *testing.T) {
	t.Parallel()

	const (
		largeDiff = 100
		interval  = 25 * time.Minute
	)
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	liar, honest := higher, addr(0xf1, 0)
	svc := newPinService(status(1000))
	svc.remote[liar.ByteString()] = status(1000 + largeDiff + 1)
	svc.remote[honest.ByteString()] = status(1000)
	svc.lists[liar.ByteString()] = 1000
	svc.every[liar.ByteString()] = true
	svc.gated[liar.ByteString()] = true
	h := newPinHarness(t, svc, svc.fakeService, largeDiff, liar, honest)
	waitFor(t, "the liar's stripe in flight", func() bool { return stripeSessions(svc.fakeService, liar, root) > 0 })
	h.clock.Add(reconciler.StartupGuardTimeout) // the liar's lack keeps the guard until its timeout

	for i := range 3 {
		svc.drip(t) // the claim in flight stores a chunk
		if got := h.r.SyncRate(); got <= 0 {
			t.Fatalf("drip %d: sync rate %v after a chunk stored", i, got)
		}
		// the claim leaves the worker once it stores nothing, and the
		// liar's Status registers it again; the idle clock's timer may
		// fire up to one step late on the fake clock (stripeClock re-arms
		// from the clock it then reads)
		idle := h.r.StripesTotal("idle")
		elapsed := time.Duration(0)
		for h.r.StripesTotal("idle") <= idle {
			if elapsed > 2*reconciler.ClaimIdleTimeout {
				t.Fatalf("drip %d: the claim did not leave the worker %v after its chunk", i, elapsed)
			}
			h.clock.Add(time.Minute)
			elapsed += time.Minute
			time.Sleep(10 * time.Millisecond)
		}
		for ; elapsed < interval; elapsed += time.Minute {
			h.clock.Add(time.Minute)
			if elapsed >= 2*reconciler.SyncRateWindow {
				if got := h.r.SyncRate(); got != 0 {
					t.Fatalf("drip %d: sync rate %v %v after it, want 0", i, got, elapsed+time.Minute)
				}
			}
		}
	}
	if n := h.r.Score(honest); n != 0 {
		t.Fatalf("honest peer scored %d", n)
	}
}

// storeSlowly lets n gated fetches store a chunk each, step apart on the
// clock of h, and checks that SyncRate stays above 0 throughout.
func storeSlowly(t *testing.T, h *harness, svc *pinService, n int, step time.Duration) {
	t.Helper()
	for i := range n {
		svc.drip(t)
		if got := h.r.SyncRate(); got <= 0 {
			t.Fatalf("chunk %d: sync rate %v once stored", i, got)
		}
		h.clock.Add(step)
		if got := h.r.SyncRate(); got <= 0 {
			t.Fatalf("chunk %d: sync rate %v %v after it", i, got, step)
		}
	}
}

// TestSyncRateHonestChurn: honest striped transfers keep SyncRate above 0
// while they store, through brief stalls (Busy answers, the verification
// round) within the grace, and are not ended: a peer's claim whose chunks
// come 90 seconds apart (within ClaimIdleTimeout), and this node's own
// bootstrap and radius half whose chunks come 3 minutes apart (past
// ClaimIdleTimeout, which own transfers are not subject to). Before their
// first chunk, Busy answers on the clock within the own-job grace do not
// drop the rate of an own transfer. No peer is scored; past the window
// after a transfer ends, SyncRate reads 0 (the reserve worker lowers the
// radius again).
func TestSyncRateHonestChurn(t *testing.T) {
	t.Parallel()

	const (
		largeDiff = 100
		perStripe = 3
	)
	a, b := higher, addr(0xf1, 0)
	// newChurn returns a pinService whose peers a and b list perStripe IDs
	// in their stripe sessions and gate their fetches, and a answers Busy
	// to its first opens.
	newChurn := func(local, remote rangesync.Status) *pinService {
		svc := newPinService(local)
		for _, p := range []swarm.Address{a, b} {
			svc.remote[p.ByteString()] = remote
			svc.lists[p.ByteString()] = perStripe
			svc.gated[p.ByteString()] = true
		}
		svc.busyN[a.ByteString()] = 3
		return svc
	}
	running := func(t *testing.T, h *harness, own bool) {
		t.Helper()
		waitFor(t, "transfer in flight", func() bool {
			_, o, ok := h.r.RunningStripe()
			return ok && o == own && h.r.CatchUpFetches() > 0
		})
	}
	ended := func(t *testing.T, h *harness) {
		t.Helper()
		waitFor(t, "transfer ended", func() bool { return h.r.StripeRegistry() == 0 })
		if n := h.r.StripesTotal("idle"); n != 0 {
			t.Fatalf("%v transfers ended for storing nothing", n)
		}
		for _, p := range []swarm.Address{a, b} {
			if n := h.r.Score(p); n != 0 {
				t.Fatalf("%s scored %d", p, n)
			}
		}
	}

	t.Run("claim", func(t *testing.T) {
		t.Parallel()
		// both peers hold more than LargeDiff above this node, which is not
		// bootstrapping: the round registers a's claim of the root
		svc := newChurn(status(1000), status(1000+largeDiff+1))
		h := newPinHarness(t, svc, svc.fakeService, largeDiff, a, b)
		running(t, h, false)
		if got := h.r.SyncRate(); got < reconciler.StartupGuardRate {
			t.Fatalf("sync rate %v within the catch-up grace", got)
		}
		storeSlowly(t, h, svc, 2*perStripe, 90*time.Second)
		svc.set(func() { svc.local = status(1000 + largeDiff + 1) }) // caught up
		ended(t, h)
		if got := h.r.StripesTotal("done"); got < 2 {
			t.Fatalf("%v stripes done, want 2", got)
		}
		pastWindow(h)
		if got := h.r.SyncRate(); got != 0 {
			t.Fatalf("sync rate %v past the window, want 0", got)
		}
	})

	t.Run("bootstrap", func(t *testing.T) {
		t.Parallel()
		svc := newChurn(status(0), status(1000))
		h := newPinHarness(t, svc, svc.fakeService, largeDiff, a, b)
		running(t, h, true)
		waitFor(t, "guard released", h.r.GuardReleased)
		h.clock.Add(reconciler.OwnJobGrace / 2) // a stall before the first chunk
		if got := h.r.SyncRate(); got < reconciler.StartupGuardRate {
			t.Fatalf("sync rate %v within the own-job grace", got)
		}
		storeSlowly(t, h, svc, 2*perStripe, 3*time.Minute)
		svc.set(func() { svc.local = status(1000) }) // caught up
		ended(t, h)
		pastWindow(h)
		if got := h.r.SyncRate(); got != 0 {
			t.Fatalf("sync rate %v past the window, want 0", got)
		}
	})

	t.Run("radius half", func(t *testing.T) {
		t.Parallel()
		svc := newChurn(selfRoot(2, 5), selfRoot(0, 10))
		svc.busyN[a.ByteString()] = 0
		svc.inside = []fingerprint.Range{sibling(2)} // the old half's sessions list nothing
		h := newPinHarness(t, svc, svc.fakeService, largeDiff, a, b)
		waitFor(t, "guard released", h.r.GuardReleased)
		svc.set(func() { svc.busyN[a.ByteString()] = 3 })
		if !reserveWorker(h) {
			t.Fatalf("no decrease in sync, sync rate %v", h.r.SyncRate())
		}
		running(t, h, true)
		h.clock.Add(reconciler.OwnJobGrace / 2)
		if reserveWorker(h) {
			t.Fatal("radius decreased within the own-job grace")
		}
		for range 2 * perStripe {
			svc.drip(t)
			h.clock.Add(3 * time.Minute)
			if reserveWorker(h) {
				t.Fatal("radius decreased while the new half stores")
			}
		}
		ended(t, h)
		if h.r.RadiusPending() {
			t.Fatal("half still pending")
		}
		pastWindow(h)
		if !reserveWorker(h) {
			t.Fatalf("no decrease past the window, sync rate %v", h.r.SyncRate())
		}
	})
}

// TestSyncRateOwnJobGrace: the own-job grace is one budget for the node
// (T4.7 part 3). A bootstrap whose stripes stall holds SyncRate above 0 for
// OwnJobGrace, then reads 0; when it fails and the next round registers the
// bootstrap again (as a Sybil majority of the peers sharing this node's root
// could make it, IsBootstrap), the new transfer gets no new grace: SyncRate
// stays 0 until an own transfer stores a chunk. Before, each registered
// transfer held SyncRate at StartupGuardRate for as long as it ran.
func TestSyncRateOwnJobGrace(t *testing.T) {
	t.Parallel()

	a, b := higher, addr(0xf1, 0)
	svc := newPinService(status(0))
	for _, p := range []swarm.Address{a, b} {
		svc.remote[p.ByteString()] = status(1000)
		svc.stall[p.ByteString()] = true
	}
	h := newPinHarness(t, svc, svc.fakeService, 0, a, b)
	waitFor(t, "bootstrap in flight", func() bool {
		_, own, ok := h.r.RunningStripe()
		return ok && own
	})
	h.clock.Add(reconciler.StartupGuardTimeout) // no first wants: the guard ends at its timeout
	if got := h.r.SyncRate(); got < reconciler.StartupGuardRate {
		t.Fatalf("sync rate %v within the own-job grace", got)
	}
	h.clock.Add(reconciler.OwnJobGrace)
	if got := h.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v past the own-job grace, nothing stored, want 0", got)
	}

	// the stalled stripes time out, are reassigned and time out again: the
	// bootstrap fails, and a later round registers it again
	sessions := len(svc.sessionOpens(a)) + len(svc.sessionOpens(b))
	for i := 0; !h.r.Bootstrapping() || len(svc.sessionOpens(a))+len(svc.sessionOpens(b)) <= sessions+2; i++ {
		if i == 100 {
			t.Fatal("the bootstrap was not registered again")
		}
		h.clock.Add(reconciler.StripeTimeout)
		time.Sleep(10 * time.Millisecond)
		if got := h.r.SyncRate(); got != 0 {
			t.Fatalf("sync rate %v with a stalled bootstrap registered again, want 0", got)
		}
	}

	// the peers deliver: the bootstrap stores and the rate rises
	svc.set(func() {})
	svc.pmu.Lock()
	for _, p := range []swarm.Address{a, b} {
		svc.stall[p.ByteString()] = false
		svc.lists[p.ByteString()] = 3
	}
	svc.pmu.Unlock()
	h.clock.Add(reconciler.StripeTimeout) // the stalled attempts end
	waitFor(t, "chunks stored", func() bool { return h.r.SyncRate() > 0 })
}

// TestClaimStoringNotEnded is an honest claim over real services (T4.7 part
// 3): node a lacks more than LargeDiff of what node b holds in their root
// (not a bootstrap), so its round registers the root as b's claim, and the
// claim's stripe fetches the IDs b lists, its chunks 30 seconds apart on the
// clock: each stored chunk restarts the claim's idle clock as it is stored
// (the Service's FetchWithProgress, not the end of the fetch), so a fetch
// that lasts three ClaimIdleTimeouts while it stores is not ended. The claim
// completes, no peer is scored.
func TestClaimStoringNotEnded(t *testing.T) {
	t.Parallel()

	// at most 16 deliveries in one fetch: streamtest deadlocks when its
	// writer runs further ahead of a lagging reader
	const (
		largeDiff = 8
		extra     = 12
	)
	clock := &fakeClock{now: time.Unix(1_000_000, 0)}
	base := chunktesting.GenerateTestRandomChunks(3 * extra)
	more := chunktesting.GenerateTestRandomChunks(extra)
	aAddr, bAddr := addr(0x10, 0), addr(0xa0, 0)
	a := newRoundNode(t, aAddr, roundNodeOptions{
		roundInterval: 20 * time.Millisecond,
		chunks:        base,
		now:           clock.Now,
		after:         clock.After,
		largeDiff:     largeDiff,
		striper:       true,
		putHook: func(swarm.Chunk) error {
			clock.Add(30 * time.Second) // each chunk takes 30 seconds
			return nil
		},
	}, bAddr)
	b := newRoundNode(t, bAddr, roundNodeOptions{
		roundInterval: time.Hour,
		chunks:        append(base[:len(base):len(base)], more...),
		largeDiff:     largeDiff,
		striper:       true,
	}, aAddr)
	b.start(t, []*stripeNode{&a.stripeNode}, nil)
	a.start(t, []*stripeNode{&b.stripeNode}, nil)

	waitFor(t, "the claim transferred", func() bool {
		for _, ch := range more {
			if !a.has(t, ch) {
				return false
			}
		}
		return a.r.StripeRegistry() == 0
	})
	if got := a.r.StripesTotal("idle"); got != 0 {
		t.Fatalf("%v claims ended for storing nothing while they stored", got)
	}
	if got := a.r.StripesTotal("done"); got < 1 {
		t.Fatal("no stripe done")
	}
	if took := clock.Now().Sub(time.Unix(1_000_000, 0)); took < 2*reconciler.ClaimIdleTimeout {
		t.Fatalf("the fetch took %v on the clock, want it past the idle timeout", took)
	}
	if score := a.r.Score(bAddr); score != 0 {
		t.Fatalf("peer scored %d", score)
	}
}
