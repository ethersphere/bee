// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	higher2 = addr(0xe0, 0) // self initiates
	higher3 = addr(0xd0, 0) // self initiates
)

// errStalled ends the fetch of a stalling peer of wantService, as the fetch
// timeout of rangesync.Service does.
var errStalled = errors.New("stalled")

// wantService is a fakeService whose fetches claim through a real want set
// and report their outcome as rangesync.Service.FetchWithResult does: an ID
// stored already is skipped, a held claim is Held, a rejected ID Rejected; a
// granted ID is stored when the peer holds it and Undelivered otherwise.
type wantService struct {
	*fakeService
	ws *wantset.Set

	wmu sync.Mutex
	// holds, by peer, are the IDs the peer delivers.
	holds map[string]map[fingerprint.ID]bool
	// stall, by peer, blocks the peer's fetches once they claimed until the
	// channel is closed; they then fail, with their claims Undelivered.
	// pause, by peer, blocks them the same way, after which they deliver.
	stall map[string]chan struct{}
	pause map[string]chan struct{}
	// callGate, if not nil, receives a channel from each fetch, which then
	// waits until that channel is closed before it claims.
	callGate chan chan struct{}
	// afterFetch, if not nil, is called after each fetch with its peer.
	afterFetch func(peer swarm.Address)
	// unverified holds the IDs whose stamp check fails on this node when
	// delivered (FetchResult.Unverified).
	unverified map[fingerprint.ID]bool
	// claimed, by peer, is closed when a fetch from the peer first claims.
	claimed map[string]chan struct{}
	// stored holds the IDs stored; deliveries the peers that delivered
	// each ID; calls the IDs of each fetch, by peer.
	stored     map[fingerprint.ID]bool
	deliveries map[fingerprint.ID][]swarm.Address
	calls      map[string][][]storer.ChunkID
}

func newWantService(local rangesync.Status) *wantService {
	ws := wantset.New(rangesync.ClaimDeadline, rangesync.RejectTTL)
	return &wantService{
		fakeService: newFakeService(local),
		ws:          ws,
		holds:       make(map[string]map[fingerprint.ID]bool),
		stall:       make(map[string]chan struct{}),
		pause:       make(map[string]chan struct{}),
		unverified:  make(map[fingerprint.ID]bool),
		claimed:     make(map[string]chan struct{}),
		stored:      make(map[fingerprint.ID]bool),
		deliveries:  make(map[fingerprint.ID][]swarm.Address),
		calls:       make(map[string][][]storer.ChunkID),
	}
}

var _ reconciler.Service = (*wantService)(nil)

// lists makes peer list ids in its sessions with self and hold them.
func (s *wantService) lists(peer swarm.Address, ids ...storer.ChunkID) {
	s.set(func() { s.wantIDs[peer.ByteString()] = ids })
	s.wmu.Lock()
	defer s.wmu.Unlock()
	h := make(map[fingerprint.ID]bool, len(ids))
	for _, id := range ids {
		h[fpID(id)] = true
	}
	s.holds[peer.ByteString()] = h
}

// withholds makes peer not deliver ids it lists.
func (s *wantService) withholds(peer swarm.Address, ids ...storer.ChunkID) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	for _, id := range ids {
		delete(s.holds[peer.ByteString()], fpID(id))
	}
}

// claimSignal returns a channel closed when a fetch from peer first claims.
func (s *wantService) claimSignal(peer swarm.Address) <-chan struct{} {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	c := make(chan struct{})
	s.claimed[peer.ByteString()] = c
	return c
}

func (s *wantService) FetchWithResult(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (rangesync.FetchResult, error) {
	res, err := s.fetchWithResult(ctx, peer, ids)
	s.wmu.Lock()
	hook := s.afterFetch
	s.wmu.Unlock()
	if hook != nil {
		hook(peer)
	}
	return res, err
}

func (s *wantService) fetchWithResult(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (rangesync.FetchResult, error) {
	var (
		res     rangesync.FetchResult
		granted []storer.ChunkID
	)
	k := peer.ByteString()
	s.wmu.Lock()
	gate := s.callGate
	s.wmu.Unlock()
	if gate != nil {
		release := make(chan struct{})
		select {
		case gate <- release:
		case <-ctx.Done():
			return res, ctx.Err()
		}
		select {
		case <-release:
		case <-ctx.Done():
			return res, ctx.Err()
		}
	}
	s.wmu.Lock()
	s.calls[k] = append(s.calls[k], slices.Clone(ids))
	for _, c := range ids {
		id := fpID(c)
		if s.stored[id] {
			continue
		}
		switch s.ws.Claim(id, peer) {
		case wantset.Granted:
			granted = append(granted, c)
		case wantset.Held:
			res.Held = append(res.Held, c)
		case wantset.Full, wantset.PeerFull:
			res.Deferred = append(res.Deferred, c)
		case wantset.Rejected:
			res.Rejected++
		}
	}
	stall, pause, sig := s.stall[k], s.pause[k], s.claimed[k]
	if sig != nil && len(granted) > 0 {
		close(sig)
		delete(s.claimed, k)
	}
	s.wmu.Unlock()

	if pause != nil && len(granted) > 0 {
		select {
		case <-pause:
		case <-ctx.Done():
		}
	}
	if stall != nil && len(granted) > 0 {
		select {
		case <-stall:
		case <-ctx.Done():
		}
		for _, c := range granted {
			s.ws.Release(fpID(c), peer)
		}
		res.Undelivered = granted
		return res, errStalled
	}

	s.wmu.Lock()
	defer s.wmu.Unlock()
	for _, c := range granted {
		id := fpID(c)
		if s.holds[k][id] && s.unverified[id] {
			s.ws.Release(id, peer)
			res.Unverified = append(res.Unverified, c)
			continue
		}
		if !s.holds[k][id] {
			s.ws.Release(id, peer)
			res.Undelivered = append(res.Undelivered, c)
			continue
		}
		s.stored[id] = true
		s.deliveries[id] = append(s.deliveries[id], peer)
		s.ws.Done(id)
		res.Stored++
	}
	return res, nil
}

// storedBy records id as stored by another fetch, which ends its claim.
func (s *wantService) storedBy(id storer.ChunkID) {
	s.wmu.Lock()
	s.stored[fpID(id)] = true
	s.wmu.Unlock()
	s.ws.Done(fpID(id))
}

// onFirstFetch calls f once, after the first fetch from peer.
func (s *wantService) onFirstFetch(peer swarm.Address, f func()) {
	var once sync.Once
	s.wmu.Lock()
	defer s.wmu.Unlock()
	s.afterFetch = func(p swarm.Address) {
		if p.Equal(peer) {
			once.Do(f)
		}
	}
}

// delivered returns the peers that delivered id.
func (s *wantService) delivered(id storer.ChunkID) []swarm.Address {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return slices.Clone(s.deliveries[fpID(id)])
}

// fetchesFrom returns the IDs of each fetch from peer.
func (s *wantService) fetchesFrom(peer swarm.Address) [][]storer.ChunkID {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return slices.Clone(s.calls[peer.ByteString()])
}

func fpID(c storer.ChunkID) fingerprint.ID {
	return fingerprint.NewID(c.Address, c.StampHash)
}

// testChunkID returns a distinct ChunkID for i.
func testChunkID(i byte) storer.ChunkID {
	a := make([]byte, swarm.HashSize)
	a[0], a[1] = 0x55, i
	sh := make([]byte, swarm.HashSize)
	sh[0] = i
	return storer.ChunkID{Address: swarm.NewAddress(a), StampHash: sh}
}

// newWantHarness returns a started reconciler with overlay self over svc,
// whose rounds run only at Start: a topology change follows the peers at once
// but defers its round by the hour on the fake clock.
func newWantHarness(t *testing.T, svc *wantService, peers ...swarm.Address) *harness {
	t.Helper()
	for _, p := range peers {
		svc.remote[p.ByteString()] = status(2)
	}
	h := &harness{
		svc:   svc.fakeService,
		topo:  &fakeTopology{peers: peers},
		clock: &fakeClock{now: time.Unix(1_000_000, 0)},
	}
	h.r = reconciler.New(self, h.topo, svc, log.Noop, reconciler.Options{
		Now: h.clock.Now,
		// a test that sets a stripe retry interval of a millisecond has
		// each of its waits pass at once
		After:         advancingAfter(h.clock, time.Millisecond),
		RoundInterval: time.Hour, // rounds run at Start only
		// the guard ends a second after Start on the fake clock
		StartupGuardTimeout: time.Millisecond,
	})
	t.Cleanup(func() {
		if err := h.r.Close(); err != nil {
			t.Error(err)
		}
		_ = svc.ws.Close()
	})
	return h
}

func assertDelivered(t *testing.T, svc *wantService, id storer.ChunkID, by ...swarm.Address) {
	t.Helper()
	got := svc.delivered(id)
	if len(got) != len(by) {
		t.Fatalf("%s delivered by %v, want %v", id.Address, got, by)
	}
	for i := range by {
		if !got[i].Equal(by[i]) {
			t.Fatalf("%s delivered by %v, want %v", id.Address, got, by)
		}
	}
}

// waitClosed waits until c is closed.
func waitClosed(t *testing.T, what string, c <-chan struct{}) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// waitDelivered waits until id was delivered.
func waitDelivered(t *testing.T, svc *wantService, id storer.ChunkID) {
	t.Helper()
	waitFor(t, "delivery of "+id.Address.String(), func() bool { return len(svc.delivered(id)) > 0 })
}

// TestWantSetFetchedOnce is §7.8, first half: B1 lists {x, y}, B2 lists {x,
// z}. B1's fetch claims x first, so B2's is denied x and requests only z; B1
// delivers x. Every chunk is fetched once, and the denied peer is not asked
// for x again.
func TestWantSetFetchedOnce(t *testing.T) {
	t.Parallel()

	x, y, z := testChunkID(1), testChunkID(2), testChunkID(3)
	svc := newWantService(status(1))
	svc.lists(higher, x, y)
	svc.lists(higher2, x, z)
	// B2's session waits for B1's fetch to hold its claims, and B1 delivers
	// once B2's fetch is done
	b1Claimed := svc.claimSignal(higher)
	pause := make(chan struct{})
	svc.pause[higher.ByteString()] = pause
	gate := make(chan struct{})
	svc.set(func() { svc.peerGate[higher2.ByteString()] = gate })
	h := newWantHarness(t, svc, higher, higher2)
	h.r.Start(context.Background())

	waitClosed(t, "B1 claimed", b1Claimed)
	close(gate)
	waitDelivered(t, svc, z)
	assertDelivered(t, svc, x) // denied to B2
	close(pause)
	waitDelivered(t, svc, y)
	assertDelivered(t, svc, x, higher)
	assertDelivered(t, svc, y, higher)
	assertDelivered(t, svc, z, higher2)
	settle()
	if calls := svc.fetchesFrom(higher2); len(calls) != 1 || len(calls[0]) != 2 {
		t.Fatalf("fetches from B2: %v, want one of x and z", calls)
	}
	if n := len(svc.fetchesFrom(higher)); n != 1 {
		t.Fatalf("%d fetches from B1, want 1", n)
	}
}

// TestWantSetStallRetry is §7.8, second half: B1 wins the claims on x and y
// and stalls; B2 lists x and z and is denied x. When B1's fetch fails (its
// fetch timeout), x is fetched from B2, the peer known this round to have it;
// y, which no other peer listed, is left to a later round.
func TestWantSetStallRetry(t *testing.T) {
	t.Parallel()

	x, y, z := testChunkID(1), testChunkID(2), testChunkID(3)
	svc := newWantService(status(1))
	svc.lists(higher, x, y)
	svc.lists(higher2, x, z)
	b1Claimed := svc.claimSignal(higher)
	stall := make(chan struct{})
	svc.stall[higher.ByteString()] = stall
	gate := make(chan struct{})
	svc.set(func() { svc.peerGate[higher2.ByteString()] = gate })
	h := newWantHarness(t, svc, higher, higher2)
	h.r.Start(context.Background())

	waitClosed(t, "B1 claimed", b1Claimed)
	close(gate)
	waitDelivered(t, svc, z)
	assertDelivered(t, svc, x)

	close(stall) // B1's fetch times out
	waitDelivered(t, svc, x)
	assertDelivered(t, svc, x, higher2)
	assertDelivered(t, svc, y)
	calls := svc.fetchesFrom(higher2)
	if len(calls) != 2 || len(calls[1]) != 1 || !calls[1][0].Address.Equal(x.Address) {
		t.Fatalf("fetches from B2: %v, want its session's and a retry of x", calls)
	}
	if n := len(svc.fetchesFrom(higher)); n != 1 {
		t.Fatalf("%d fetches from B1, want 1", n)
	}
}

// TestWantSetRetryChain: x is retried from the next peer known to have it
// when a retry fails too, and never from a peer that left.
func TestWantSetRetryChain(t *testing.T) {
	t.Parallel()

	x := testChunkID(1)
	svc := newWantService(status(1))
	svc.lists(higher, x)
	svc.lists(higher2, x)
	svc.lists(higher3, x)
	svc.withholds(higher2, x)
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
	close(stall)
	waitDelivered(t, svc, x)
	assertDelivered(t, svc, x, higher3)
	if n := len(svc.fetchesFrom(higher2)); n != 2 {
		t.Fatalf("%d fetches from B2, want 2", n)
	}
}

// TestWantSetRetryLeftPeer: a peer denied x that has left since is not asked
// for x.
func TestWantSetRetryLeftPeer(t *testing.T) {
	t.Parallel()

	x := testChunkID(1)
	svc := newWantService(status(1))
	svc.lists(higher, x)
	svc.lists(higher2, x)
	b1Claimed := svc.claimSignal(higher)
	stall := make(chan struct{})
	svc.stall[higher.ByteString()] = stall
	gate := make(chan struct{})
	svc.set(func() { svc.peerGate[higher2.ByteString()] = gate })
	h := newWantHarness(t, svc, higher, higher2)
	h.r.Start(context.Background())

	waitClosed(t, "B1 claimed", b1Claimed)
	close(gate)
	waitFor(t, "B2 denied x", func() bool { return len(svc.fetchesFrom(higher2)) == 1 })
	h.topo.setPeers(higher)
	waitFor(t, "B2 left", func() bool { return !h.r.KnowsPeer(higher2) })
	close(stall)
	waitFor(t, "B1 fetch failed", func() bool { return svc.ws.Len() == 0 })
	settle()
	if n := len(svc.fetchesFrom(higher2)); n != 1 {
		t.Fatalf("%d fetches from B2 after it left, want 1", n)
	}
	assertDelivered(t, svc, x)
}

// TestWantSetRetryLongFetch: the source of x recorded while B1's fetch held
// its claim is kept until that fetch's result arrives, however long the
// fetch runs (results come per fetch, which may outlast the claim
// deadline): x is retried from B2 although B2 was denied it more than the
// claim deadline ago.
func TestWantSetRetryLongFetch(t *testing.T) {
	t.Parallel()

	x := testChunkID(1)
	svc := newWantService(status(1))
	svc.lists(higher, x)
	svc.lists(higher2, x)
	b1Claimed := svc.claimSignal(higher)
	stall := make(chan struct{})
	svc.stall[higher.ByteString()] = stall
	gate := make(chan struct{})
	svc.set(func() { svc.peerGate[higher2.ByteString()] = gate })
	h := newWantHarness(t, svc, higher, higher2)
	h.r.Start(context.Background())

	waitClosed(t, "B1 claimed", b1Claimed)
	close(gate)
	waitFor(t, "B2 denied x", func() bool { return len(svc.fetchesFrom(higher2)) == 1 })
	h.clock.Add(2 * rangesync.ClaimDeadline)
	if n := h.r.SourceIDs(); n != 1 {
		t.Fatalf("%d source IDs while B1's fetch runs, want 1", n)
	}
	close(stall)
	waitDelivered(t, svc, x)
	assertDelivered(t, svc, x, higher2)
}

// TestWantSetSourcesPruned: once no fetch that started before a denial is
// running, its source is dropped the claim deadline after the denial.
func TestWantSetSourcesPruned(t *testing.T) {
	t.Parallel()

	x, z := testChunkID(1), testChunkID(3)
	svc := newWantService(status(1))
	svc.lists(higher, x)
	svc.lists(higher2, x, z)
	b1Claimed := svc.claimSignal(higher)
	pause := make(chan struct{})
	svc.pause[higher.ByteString()] = pause
	gate := make(chan struct{})
	svc.set(func() { svc.peerGate[higher2.ByteString()] = gate })
	h := newWantHarness(t, svc, higher, higher2)
	h.r.Start(context.Background())

	waitClosed(t, "B1 claimed", b1Claimed)
	close(gate)
	waitDelivered(t, svc, z)
	close(pause)
	waitDelivered(t, svc, x)
	waitFor(t, "fetches done", func() bool { return svc.ws.Len() == 0 })
	settle()
	if n := h.r.SourceIDs(); n != 1 {
		t.Fatalf("%d source IDs within the window, want 1", n)
	}
	h.clock.Add(rangesync.ClaimDeadline)
	if n := h.r.SourceIDs(); n != 0 {
		t.Fatalf("%d source IDs after the window, want 0", n)
	}
}

// TestWantSetCatchUpRetry: the retry of a catch-up fetch counts as a catch-up
// fetch in flight: SyncRate stays at least StartupGuardRate until it ends.
func TestWantSetCatchUpRetry(t *testing.T) {
	t.Parallel()

	ids := make([]storer.ChunkID, reconciler.CatchUpMinDiff)
	for i := range ids {
		ids[i] = testChunkID(byte(i))
		ids[i].StampHash[1] = byte(i >> 8)
	}
	svc := newWantService(status(1))
	svc.lists(higher, ids...)
	svc.lists(higher2, ids...)
	b1Claimed := svc.claimSignal(higher)
	stall := make(chan struct{})
	svc.stall[higher.ByteString()] = stall
	gate := make(chan struct{})
	svc.set(func() { svc.peerGate[higher2.ByteString()] = gate })
	h := newWantHarness(t, svc, higher, higher2)
	h.r.Start(context.Background())

	waitClosed(t, "B1 claimed", b1Claimed)
	close(gate)
	waitFor(t, "B2 denied", func() bool { return len(svc.fetchesFrom(higher2)) == 1 })
	// the retry from B2 waits in its fetch until released
	calls := make(chan chan struct{})
	svc.wmu.Lock()
	svc.callGate = calls
	svc.wmu.Unlock()
	close(stall)
	var release chan struct{}
	select {
	case release = <-calls:
	case <-time.After(5 * time.Second):
		t.Fatal("no retry")
	}
	h.clock.Add(time.Second) // past the start-up guard
	if got := h.r.SyncRate(); got < reconciler.StartupGuardRate {
		t.Fatalf("SyncRate %v during the retry, want at least %v", got, reconciler.StartupGuardRate)
	}
	close(release)
	waitFor(t, "retry stored", func() bool { return len(svc.delivered(ids[0])) == 1 })
	waitFor(t, "retry ended", func() bool { return h.r.SyncRate() < reconciler.StartupGuardRate })
}

// TestStripeWithheldUndeliveredOnly: a stripe is withheld only when its peer
// did not deliver IDs requested from it. An ID another fetch's claim held,
// which that fetch then stored, and an ID rejected within RejectTTL are
// listed but not requested from the peer, and do not make the stripe fail
// (T3.2 withholding false positives); rejected IDs count as done.
func TestStripeWithheldUndeliveredOnly(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	ids := []storer.ChunkID{testChunkID(1), testChunkID(2), testChunkID(3), testChunkID(4)}
	transfer := func(t *testing.T, withheld ...storer.ChunkID) reconciler.StripeResult {
		t.Helper()
		svc := newWantService(status(1))
		svc.lists(higher, ids...)
		svc.withholds(higher, withheld...)
		if r := svc.ws.Claim(fpID(ids[0]), lower); r != wantset.Granted { // a round fetch holds it
			t.Fatal(r)
		}
		svc.onFirstFetch(higher, func() { svc.storedBy(ids[0]) }) // and stores it
		svc.ws.Reject(fpID(ids[1]))
		r := newStripeReconciler(t, svc)
		res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
		if got := r.StripesTotal("reassigned"); got != 0 {
			t.Fatalf("stripes reassigned %v", got)
		}
		return res
	}

	res := transfer(t)
	if res.Err != nil {
		t.Fatalf("stripe failed: %v", res.Err)
	}
	if res.Listed != 4 || res.Stored != 2 || res.Undelivered != 0 || res.Deferred != 0 {
		t.Fatalf("listed %d, stored %d, undelivered %d, deferred %d, want 4, 2, 0, 0", res.Listed, res.Stored, res.Undelivered, res.Deferred)
	}

	res = transfer(t, ids[3])
	if !errors.Is(res.Err, reconciler.ErrStripeWithheld) {
		t.Fatalf("got %v, want %v", res.Err, reconciler.ErrStripeWithheld)
	}
	if res.Stored != 1 || res.Undelivered != 1 {
		t.Fatalf("stored %d, undelivered %d, want 1, 1", res.Stored, res.Undelivered)
	}
}

// newStripeReconciler returns a reconciler over svc that is not started, for
// stripe transfers, with a short stripe retry interval.
func newStripeReconciler(t *testing.T, svc *wantService) *reconciler.Reconciler {
	t.Helper()
	r := reconciler.New(self, nil, svc, log.Noop, reconciler.Options{})
	r.SetStripeRetryInterval(time.Millisecond)
	t.Cleanup(func() {
		_ = r.Close()
		_ = svc.ws.Close()
	})
	return r
}

// TestStripeDeferred: a stripe whose IDs the want set does not grant (held
// by another fetch's claim that does not end, the peer's share full, the set
// full) does not complete: it ends with ErrStripeDeferred after it asked
// for them again, is restarted with the same peer within maxStripeRestarts,
// and blames nobody. Before T4.4 fix round 1 such a stripe was done.
func TestStripeDeferred(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	x, y := testChunkID(1), testChunkID(2)
	fill := func(n int, peer func(i int) swarm.Address) func(t *testing.T, ws *wantset.Set) {
		return func(t *testing.T, ws *wantset.Set) {
			t.Helper()
			for i := range n {
				id := testChunkID(0)
				id.StampHash[1], id.StampHash[2], id.StampHash[3] = 0xff, byte(i>>8), byte(i)
				if r := ws.Claim(fpID(id), peer(i)); r != wantset.Granted {
					t.Fatal(r)
				}
			}
		}
	}
	for _, tc := range []struct {
		name string
		fill func(t *testing.T, ws *wantset.Set)
	}{
		{name: "held", fill: func(t *testing.T, ws *wantset.Set) {
			t.Helper()
			if r := ws.Claim(fpID(x), lower); r != wantset.Granted {
				t.Fatal(r)
			}
		}},
		{name: "peer full", fill: fill(wantset.MaxClaimsPerPeer, func(int) swarm.Address { return higher })},
		{name: "full", fill: fill(wantset.MaxClaims, func(i int) swarm.Address { return addr(0x30, byte(i/wantset.MaxClaimsPerPeer)) })},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := newWantService(status(1))
			svc.lists(higher, x)
			if tc.name == "held" {
				svc.lists(higher, x, y) // y is delivered, x is held
			}
			tc.fill(t, svc.ws)
			r := newStripeReconciler(t, svc)
			res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
			if !errors.Is(res.Err, reconciler.ErrStripeDeferred) || errors.Is(res.Err, reconciler.ErrStripeWithheld) {
				t.Fatalf("got %v, want %v", res.Err, reconciler.ErrStripeDeferred)
			}
			if res.Deferred != 1 || res.Undelivered != 0 {
				t.Fatalf("deferred %d, undelivered %d, want 1, 0", res.Deferred, res.Undelivered)
			}
			if n := len(svc.sessionOpens(higher)); n != 1+reconciler.MaxStripeRestarts {
				t.Fatalf("%d sessions with the peer, want %d (restarted with it)", n, 1+reconciler.MaxStripeRestarts)
			}
			if got := r.Score(higher); got != 0 {
				t.Fatalf("peer score %d, want 0", got)
			}
			if got := r.StripesTotal("done"); got != 0 {
				t.Fatalf("stripes done %v", got)
			}
			if v := r.VerifyStripes(context.Background(), []reconciler.StripeResult{res})[0]; !errors.Is(v.Err, reconciler.ErrStripeNotDone) {
				t.Fatalf("verification of a deferred stripe: %v", v.Err)
			}
		})
	}
}

// TestStripeDeferredResolved: an ID held by another fetch's claim that ends
// without the chunk is asked for again and fetched from the stripe peer; a
// stripe peer that holds the claim itself (its own fetch of IDs this node
// listed to it) and then withholds the ID is found withholding.
func TestStripeDeferredResolved(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	x := testChunkID(1)
	for _, tc := range []struct {
		name   string
		holder swarm.Address
		hold   bool // the stripe peer withholds x
	}{
		{name: "foreign claim released", holder: lower},
		{name: "own claim, withheld", holder: higher, hold: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := newWantService(status(1))
			svc.lists(higher, x)
			if tc.hold {
				svc.withholds(higher, x)
			}
			if r := svc.ws.Claim(fpID(x), tc.holder); r != wantset.Granted {
				t.Fatal(r)
			}
			svc.onFirstFetch(higher, func() { svc.ws.Release(fpID(x), tc.holder) })
			r := newStripeReconciler(t, svc)
			res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
			if n := len(svc.sessionOpens(higher)); n != 1 {
				t.Fatalf("%d sessions, want 1 (resolved within the attempt)", n)
			}
			if tc.hold {
				if !errors.Is(res.Err, reconciler.ErrStripeWithheld) || res.Undelivered != 1 {
					t.Fatalf("err %v, undelivered %d, want %v, 1", res.Err, res.Undelivered, reconciler.ErrStripeWithheld)
				}
				return
			}
			if res.Err != nil || res.Stored != 1 || res.Deferred != 0 {
				t.Fatalf("err %v, stored %d, deferred %d", res.Err, res.Stored, res.Deferred)
			}
			assertDelivered(t, svc, x, higher)
		})
	}
}

// TestStripeReaskReleasesPeer: while a stripe waits to ask its peer again
// for an ID the want set held, it holds neither the peer's Striped state nor
// its stripe lock, so a round session and another stripe's session with the
// peer run meanwhile; it takes both again for the next ask.
func TestStripeReaskReleasesPeer(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	x := testChunkID(1)
	other := addr(0xf1, 0) // not followed: its stripe fails, then moves to higher
	const retry = 7 * time.Second
	svc := newWantService(status(1))
	svc.remote[higher.ByteString()] = status(1) // equal digests: no round session yet
	svc.lists(higher, x)
	if r := svc.ws.Claim(fpID(x), lower); r != wantset.Granted { // another fetch holds x
		t.Fatal(r)
	}
	otherGate := make(chan struct{})
	svc.peerGate[other.ByteString()] = otherGate
	svc.reconErr[other.ByteString()] = errors.New("stream reset")
	clock := &fakeClock{now: time.Unix(1_000_000, 0)}
	w := newStripeWaits(clock, retry, true) // the stripe's waits to ask again
	r := reconciler.New(self, &fakeTopology{peers: []swarm.Address{higher}}, svc, log.Noop, reconciler.Options{Now: clock.Now, After: w.after})
	r.SetStripeRetryInterval(retry)
	t.Cleanup(func() {
		w.close()
		_ = r.Close()
		_ = svc.ws.Close()
	})
	r.Round() // higher is followed: Idle
	waitState(t, r, higher, reconciler.StateIdle)

	done := make(chan []reconciler.StripeResult, 1)
	go func() {
		done <- r.TransferStripes(context.Background(), []reconciler.Stripe{
			{Range: root.Child(1, 0), Peers: []swarm.Address{higher}},
			{Range: root.Child(1, 1), Peers: []swarm.Address{other, higher}},
		})
	}()
	wait := w.recv(t) // x was held: the first stripe waits to ask again
	if got := peerState(r, higher); got != reconciler.StateIdle {
		t.Fatalf("peer %v while the stripe waits to ask again, want %v", got, reconciler.StateIdle)
	}

	// a round session with the peer
	svc.set(func() { svc.remote[higher.ByteString()] = status(2) })
	r.Round()
	waitFor(t, "round session", func() bool { return len(svc.sessionOpens(higher)) == 2 })
	waitState(t, r, higher, reconciler.StateIdle)

	// the other stripe, moved to the peer, takes its stripe lock and stores x
	svc.ws.Release(fpID(x), lower)
	close(otherGate)
	waitFor(t, "other stripe's session with the peer", func() bool { return len(svc.sessionOpens(higher)) == 3 })
	waitDelivered(t, svc, x)

	wait <- clock.Now() // the next ask finds x stored
	for i, res := range <-done {
		if res.Err != nil || !res.Peer.Equal(higher) {
			t.Fatalf("stripe %d from %s: %v", i, res.Peer, res.Err)
		}
	}
	assertDelivered(t, svc, x, higher)
}

// stripeWaits hands a test the stripe's waits of exactly retry: each
// arrives on waits, and the test ends it by sending on it (recv). Once close
// is called they end at once. Other waits (a stripe clock's expiry) follow
// the clock when fire is set, and never end otherwise: then only the
// attempt's own checks (expireIfDue) end it at its timeout.
type stripeWaits struct {
	clock *fakeClock
	retry time.Duration
	fire  bool
	waits chan chan time.Time
	stop  chan struct{}
	once  sync.Once
}

func newStripeWaits(clock *fakeClock, retry time.Duration, fire bool) *stripeWaits {
	return &stripeWaits{clock: clock, retry: retry, fire: fire, waits: make(chan chan time.Time), stop: make(chan struct{})}
}

// after is the Options.After of the reconciler under test.
func (w *stripeWaits) after(d time.Duration) <-chan time.Time {
	if d != w.retry {
		if w.fire {
			return w.clock.After(d)
		}
		return make(chan time.Time)
	}
	c := make(chan time.Time, 1)
	select {
	case w.waits <- c:
	case <-w.stop:
		c <- w.clock.Now()
	}
	return c
}

// recv returns the next wait of retry.
func (w *stripeWaits) recv(t *testing.T) chan time.Time {
	t.Helper()
	select {
	case c := <-w.waits:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the stripe to wait")
		return nil
	}
}

// close ends every later wait of retry at once.
func (w *stripeWaits) close() { w.once.Do(func() { close(w.stop) }) }

// TestStripeReaskTimeoutBoundsPeerWait: the stripe timeout runs while a
// stripe waits to take its peer back for the next ask, so a round session
// holding the peer does not keep the stripe past it.
func TestStripeReaskTimeoutBoundsPeerWait(t *testing.T) {
	t.Parallel()

	x := testChunkID(1)
	const (
		retry   = 7 * time.Second
		timeout = time.Hour
	)
	svc := newWantService(status(1))
	svc.remote[higher.ByteString()] = status(1)
	svc.lists(higher, x)
	if r := svc.ws.Claim(fpID(x), lower); r != wantset.Granted {
		t.Fatal(r)
	}
	clock := &fakeClock{now: time.Unix(1_000_000, 0)}
	w := newStripeWaits(clock, retry, true)
	r := reconciler.New(self, &fakeTopology{peers: []swarm.Address{higher}}, svc, log.Noop, reconciler.Options{Now: clock.Now, After: w.after, StripeTimeout: timeout})
	r.SetStripeRetryInterval(retry)
	gate := make(chan struct{})
	t.Cleanup(func() {
		close(gate)
		w.close()
		_ = r.Close()
		_ = svc.ws.Close()
	})
	r.Round()
	waitState(t, r, higher, reconciler.StateIdle)

	done := make(chan reconciler.StripeResult, 1)
	go func() {
		done <- r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: fingerprint.NewRange(swarm.ZeroAddress, 0), Peers: []swarm.Address{higher}}})[0]
	}()
	wait := w.recv(t)
	svc.set(func() {
		svc.peerGate[higher.ByteString()] = gate // the round session holds the peer
		svc.remote[higher.ByteString()] = status(2)
	})
	r.Round()
	waitFor(t, "round session", func() bool { return len(svc.sessionOpens(higher)) == 2 })
	wait <- clock.Now()
	waitFor(t, "stripe waiting for the peer", func() bool { return r.StripeWaiters(higher) == 1 })

	clock.Add(timeout)
	select {
	case res := <-done:
		if !errors.Is(res.Err, reconciler.ErrStripeTimeout) || res.Deferred != 1 {
			t.Fatalf("stripe: %v, deferred %d; want a timeout with x deferred", res.Err, res.Deferred)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stripe waited for its peer past its timeout")
	}
}

// TestStripeReaskClockCheckedAfterPeerWait: a stripe whose timeout ran out
// while it waited to take its peer back for the next ask does not ask, even
// when the clock's goroutine has not seen the expiry (here it never does).
func TestStripeReaskClockCheckedAfterPeerWait(t *testing.T) {
	t.Parallel()

	x := testChunkID(1)
	const (
		retry   = 7 * time.Second
		timeout = time.Hour
	)
	svc := newWantService(status(1))
	svc.remote[higher.ByteString()] = status(1)
	svc.lists(higher, x)
	if r := svc.ws.Claim(fpID(x), lower); r != wantset.Granted {
		t.Fatal(r)
	}
	clock := &fakeClock{now: time.Unix(1_000_000, 0)}
	w := newStripeWaits(clock, retry, false)
	r := reconciler.New(self, &fakeTopology{peers: []swarm.Address{higher}}, svc, log.Noop, reconciler.Options{Now: clock.Now, After: w.after, StripeTimeout: timeout})
	r.SetStripeRetryInterval(retry)
	gate := make(chan struct{})
	t.Cleanup(func() {
		w.close()
		_ = r.Close()
		_ = svc.ws.Close()
	})
	r.Round()
	waitState(t, r, higher, reconciler.StateIdle)

	done := make(chan reconciler.StripeResult, 1)
	go func() {
		done <- r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: fingerprint.NewRange(swarm.ZeroAddress, 0), Peers: []swarm.Address{higher}}})[0]
	}()
	wait := w.recv(t)
	svc.set(func() {
		svc.peerGate[higher.ByteString()] = gate // the round session holds the peer
		svc.remote[higher.ByteString()] = status(2)
		svc.reconErr[higher.ByteString()] = errors.New("stream reset") // and ends without a fetch
	})
	r.Round()
	waitFor(t, "round session", func() bool { return len(svc.sessionOpens(higher)) == 2 })
	wait <- clock.Now()
	waitFor(t, "stripe waiting for the peer", func() bool { return r.StripeWaiters(higher) == 1 })

	clock.Add(timeout)
	close(gate)
	select {
	case res := <-done:
		if !errors.Is(res.Err, reconciler.ErrStripeTimeout) || res.Deferred != 1 {
			t.Fatalf("stripe: %v, deferred %d; want a timeout with x deferred", res.Err, res.Deferred)
		}
	case c := <-w.waits:
		c <- clock.Now()
		t.Fatal("the stripe asked again past its timeout")
	case <-time.After(5 * time.Second):
		t.Fatal("stripe did not end")
	}
	if n := len(svc.fetchesFrom(higher)); n != 1 {
		t.Fatalf("%d fetches, want 1: the stripe asked again past its timeout", n)
	}
}

// TestVerifyStripesUndeliveredOnly: a verifier is scored for withholding only
// for IDs requested from it and not delivered, not for IDs rejected within
// RejectTTL. A verification whose IDs the want set does not grant is
// incomplete (ErrStripeDeferred) and blames nobody.
func TestVerifyStripesUndeliveredOnly(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	verify := func(t *testing.T, withheld int, held bool) (*harness, reconciler.VerifyResult) {
		t.Helper()
		other := addr(0xf1, 0)
		svc := newWantService(status(1))
		h := newWantHarness(t, svc, higher, other)
		h.r.SetStripeRetryInterval(time.Millisecond)
		// equal digests: no round sessions, only the verification's
		svc.remote[higher.ByteString()] = status(1)
		svc.remote[other.ByteString()] = status(1)
		ids := make([]storer.ChunkID, 3*reconciler.VerifyMinDiff+1)
		for i := range ids {
			ids[i] = testChunkID(byte(i))
		}
		svc.lists(other, ids...)
		// 2 × VerifyMinDiff are rejected, more than the tolerance; withheld
		// of the rest are not delivered
		for _, id := range ids[:2*reconciler.VerifyMinDiff] {
			svc.ws.Reject(fpID(id))
		}
		if held {
			if r := svc.ws.Claim(fpID(ids[len(ids)-1]), lower); r != wantset.Granted {
				t.Fatal(r)
			}
		}
		svc.withholds(other, ids[len(ids)-withheld:]...)
		h.r.Start(context.Background())
		waitAdmitted(t, h.r, higher, other)

		res := reconciler.StripeResult{Range: root, Peer: higher, PeerStored: 1000, Tried: []swarm.Address{higher}}
		v := h.r.VerifyStripes(context.Background(), []reconciler.StripeResult{res})[0]
		if !v.Verifier.Equal(other) {
			t.Fatalf("verifier %s", v.Verifier)
		}
		if !held && v.Err != nil {
			t.Fatalf("verification: %v", v.Err)
		}
		return h, v
	}

	h, v := verify(t, 0, false)
	if v.Diff != 3*reconciler.VerifyMinDiff+1 || v.Stored != reconciler.VerifyMinDiff+1 || v.Undelivered != 0 {
		t.Fatalf("diff %d, stored %d, undelivered %d", v.Diff, v.Stored, v.Undelivered)
	}
	if got := h.r.Score(v.Verifier); got != 0 {
		t.Fatalf("verifier score %d, want 0", got)
	}
	if got := h.r.Score(higher); got != 0 {
		t.Fatalf("deliverer score %d, want 0", got)
	}

	h, v = verify(t, reconciler.VerifyMinDiff, false)
	if v.Undelivered != reconciler.VerifyMinDiff {
		t.Fatalf("undelivered %d, want %d", v.Undelivered, reconciler.VerifyMinDiff)
	}
	if got := h.r.Score(v.Verifier); got != 0 {
		t.Fatalf("verifier score %d at the tolerance, want 0", got)
	}
	h, v = verify(t, reconciler.VerifyMinDiff+1, false)
	if v.Undelivered != reconciler.VerifyMinDiff+1 {
		t.Fatalf("undelivered %d, want %d", v.Undelivered, reconciler.VerifyMinDiff+1)
	}
	if got := h.r.Score(v.Verifier); got != 1 {
		t.Fatalf("verifier score %d above the tolerance, want 1", got)
	}

	h, v = verify(t, 0, true)
	if !errors.Is(v.Err, reconciler.ErrStripeDeferred) {
		t.Fatalf("got %v, want %v", v.Err, reconciler.ErrStripeDeferred)
	}
	if h.r.Score(v.Verifier) != 0 || h.r.Score(higher) != 0 {
		t.Fatalf("scores verifier %d, deliverer %d, want 0", h.r.Score(v.Verifier), h.r.Score(higher))
	}
}

// TestStripeUnverified (T4.4 fix round 2): an ID delivered with a stamp this
// node cannot check (a batch store error) makes the stripe fail with
// ErrStripeDeferred, blaming nobody, but is not asked for again within the
// attempt: only IDs the want set did not grant are, so a peer cannot make
// the stripe ask by how it delivers. The restarts ask once each.
func TestStripeUnverified(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	x, y := testChunkID(1), testChunkID(2)
	svc := newWantService(status(1))
	svc.lists(higher, x, y)
	svc.unverified[fpID(x)] = true
	r := newStripeReconciler(t, svc)
	res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
	if !errors.Is(res.Err, reconciler.ErrStripeDeferred) || errors.Is(res.Err, reconciler.ErrStripeWithheld) {
		t.Fatalf("got %v, want %v", res.Err, reconciler.ErrStripeDeferred)
	}
	if res.Deferred != 1 || res.Undelivered != 0 || res.Stored != 1 {
		t.Fatalf("deferred %d, undelivered %d, stored %d, want 1, 0, 1", res.Deferred, res.Undelivered, res.Stored)
	}
	attempts := 1 + reconciler.MaxStripeRestarts
	if n := len(svc.sessionOpens(higher)); n != attempts {
		t.Fatalf("%d sessions, want %d", n, attempts)
	}
	if n := len(svc.fetchesFrom(higher)); n != attempts {
		t.Fatalf("%d fetches, want %d (one per attempt, none asking again)", n, attempts)
	}
	if got := r.Score(higher); got != 0 {
		t.Fatalf("peer score %d", got)
	}
}
