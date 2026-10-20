// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

// addr returns an overlay whose first byte is b.
func addr(b byte, n byte) swarm.Address {
	a := make([]byte, swarm.HashSize)
	a[0], a[1] = b, n
	return swarm.NewAddress(a)
}

var (
	self   = addr(0x80, 0)
	lower  = addr(0x10, 0) // initiates against self
	higher = addr(0xf0, 0) // self initiates
)

// status returns a Status with root bits 0 and count elements.
func status(count uint64) rangesync.Status {
	return rangesync.Status{
		Root:     fingerprint.NewRange(swarm.ZeroAddress, 0),
		Summary:  fingerprint.Summary{Count: count, Digest: [32]byte{byte(count)}},
		LeafBits: 8,
		Mapping:  fingerprint.DefaultMapperID,
	}
}

// fakeService is a rangesync service whose peers' statuses, wants and
// errors are set by the test.
type fakeService struct {
	mu          sync.Mutex
	local       rangesync.Status
	localErr    error
	remote      map[string]rangesync.Status // returned by LastStatus once sent
	unsupported map[string]bool
	wants       map[string]int // number of wants Reconcile returns
	reconErr    map[string]error
	sent        map[string]int
	reconciled  map[string]int
	fetched     map[string]int
	// reconcileGate and fetchGate, if not nil, block Reconcile and Fetch
	// until closed.
	reconcileGate chan struct{}
	fetchGate     chan struct{}
	// offer, by peer, is the range a session with no Open offers to its
	// Striper; opens records the Open of each session by peer.
	offer map[string]fingerprint.Range
	opens map[string][][]fingerprint.Range
	// fetchErr, if not nil, is returned by Fetch.
	fetchErr error
	// fetchDelay delays every Fetch; wantBatches, if above 1, splits the
	// wants ReconcileWith passes to opts.Wants into that many batches.
	fetchDelay  time.Duration
	wantBatches int
	// sessionErrAfter, by peer, ends one ReconcileWith with Wants with the
	// error after its wants were passed; withhold, by peer, makes Fetch
	// store nothing.
	sessionErrAfter map[string]error
	withhold        map[string]bool
	// sessionErrSticky keeps sessionErrAfter for every session; withholdN,
	// by peer, is the number of IDs of each Fetch not stored.
	sessionErrSticky bool
	withholdN        map[string]int
	// sendErr, by peer, is returned by SendStatusWith; rootCh is the
	// channel of SubscribeRootChange (rootChange signals it).
	sendErr map[string]error
	rootCh  chan struct{}
	// statuses, by peer, are the Statuses SendStatusWith sent, in order.
	// sendCalls, if not nil, receives a channel from each SendStatusWith
	// call, which then waits until that channel is closed.
	statuses  map[string][]rangesync.Status
	sendCalls chan chan struct{}
	// reconcileDelay delays every ReconcileWith; inSession and maxInSession
	// record, by peer, the ReconcileWith calls running and the most that ran
	// at once.
	reconcileDelay time.Duration
	inSession      map[string]int
	maxInSession   map[string]int
	// fetchCalls, if not nil, receives a channel from each Fetch call,
	// which then waits until that channel is closed.
	fetchCalls chan chan struct{}
	// inAll and maxInAll are the ReconcileWith calls running with any peer
	// and the most that ran at once; statusCh is the channel of
	// SubscribeStatus (statusChange signals it).
	inAll    int
	maxInAll int
	statusCh chan struct{}
	// peerGate, by peer, blocks that peer's ReconcileWith calls (counted as
	// running) until closed; busyN, by peer, is the number of its next
	// ReconcileWith calls answered rangesync.ErrPeerBusy (after
	// reconcileDelay), localBusyN the number that fail with this node's
	// fingerprint.ErrBusy.
	peerGate   map[string]chan struct{}
	busyN      map[string]int
	localBusyN map[string]int
	// wantIDs, by peer, are the wants Reconcile returns, instead of wants
	// zero IDs.
	wantIDs map[string][]storer.ChunkID
	// busyErr, by peer, is the error of its busyN answers instead of
	// rangesync.ErrPeerBusy; sessionHook, if not nil, is called at the start
	// of each ReconcileWith with the peer and the number of its sessions so
	// far, this one included.
	busyErr     map[string]error
	sessionHook func(peer swarm.Address, n int)
	// sendBlock, by peer, makes SendStatusWith wait until its context ends
	// (a peer that never answers).
	sendBlock map[string]bool
}

func newFakeService(local rangesync.Status) *fakeService {
	return &fakeService{
		local:       local,
		remote:      make(map[string]rangesync.Status),
		unsupported: make(map[string]bool),
		wants:       make(map[string]int),
		reconErr:    make(map[string]error),
		sent:        make(map[string]int),
		reconciled:  make(map[string]int),
		fetched:     make(map[string]int),
		offer:       make(map[string]fingerprint.Range),
		opens:       make(map[string][][]fingerprint.Range),

		sessionErrAfter: make(map[string]error),
		withhold:        make(map[string]bool),
		withholdN:       make(map[string]int),
		sendErr:         make(map[string]error),
		rootCh:          make(chan struct{}, 1),
		statuses:        make(map[string][]rangesync.Status),
		inSession:       make(map[string]int),
		maxInSession:    make(map[string]int),
		statusCh:        make(chan struct{}, 1),
		peerGate:        make(map[string]chan struct{}),
		busyN:           make(map[string]int),
		localBusyN:      make(map[string]int),
		wantIDs:         make(map[string][]storer.ChunkID),
		busyErr:         make(map[string]error),
		sendBlock:       make(map[string]bool),
	}
}

// running returns the ReconcileWith calls running with any peer.
func (s *fakeService) running() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inAll
}

// SubscribeStatus returns statusCh; unsubscribing does nothing.
func (s *fakeService) SubscribeStatus() (<-chan struct{}, func()) {
	return s.statusCh, func() {}
}

// statusChange signals that a peer's Status changed, coalescing like the
// Service.
func (s *fakeService) statusChange() {
	select {
	case s.statusCh <- struct{}{}:
	default:
	}
}

// maxConcurrent returns the most sessions with peer that ran at once.
func (s *fakeService) maxConcurrent(peer swarm.Address) int {
	return s.count(s.maxInSession, peer)
}

// sentStatuses returns the Statuses sent to peer, in order.
func (s *fakeService) sentStatuses(peer swarm.Address) []rangesync.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.statuses[peer.ByteString()])
}

// SubscribeRootChange returns rootCh; unsubscribing does nothing.
func (s *fakeService) SubscribeRootChange() (<-chan struct{}, func()) {
	return s.rootCh, func() {}
}

// rootChange signals a root change, coalescing like the Index.
func (s *fakeService) rootChange() {
	select {
	case s.rootCh <- struct{}{}:
	default:
	}
}

func (s *fakeService) ReconcileWith(ctx context.Context, peer swarm.Address, opts rangesync.ReconcileOptions) ([]storer.ChunkID, error) {
	s.mu.Lock()
	k := peer.ByteString()
	s.opens[k] = append(s.opens[k], opts.Open)
	offer, ok := s.offer[k]
	s.inSession[k]++
	s.maxInSession[k] = max(s.maxInSession[k], s.inSession[k])
	s.inAll++
	s.maxInAll = max(s.maxInAll, s.inAll)
	delay, gate, hook, calls := s.reconcileDelay, s.peerGate[k], s.sessionHook, len(s.opens[k])
	s.mu.Unlock()
	defer s.set(func() { s.inSession[k]--; s.inAll-- })
	if hook != nil {
		hook(peer, calls)
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	s.mu.Lock()
	busy, localBusy := s.busyN[k] > 0, s.localBusyN[k] > 0
	switch {
	case busy:
		s.busyN[k]--
	case localBusy:
		s.localBusyN[k]--
	}
	busyErr := s.busyErr[k]
	s.mu.Unlock()
	switch {
	case busy && busyErr != nil:
		return nil, busyErr
	case busy:
		return nil, rangesync.ErrPeerBusy
	case localBusy:
		return nil, fmt.Errorf("index: %w", fingerprint.ErrBusy)
	}
	if ok && len(opts.Open) == 0 && opts.Striper != nil {
		opts.Striper.Stripe(offer, 1, rangesync.LargeDiff+2)
	}
	wants, err := s.Reconcile(ctx, peer)
	if err != nil || opts.Wants == nil {
		return wants, err
	}
	n := max(s.wantBatches, 1)
	for i := range n {
		if b := wants[i*len(wants)/n : (i+1)*len(wants)/n]; len(b) > 0 {
			opts.Wants(b)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.sessionErrAfter[peer.ByteString()]; err != nil {
		if !s.sessionErrSticky {
			delete(s.sessionErrAfter, peer.ByteString()) // once
		}
		return nil, err
	}
	return nil, ctx.Err()
}

// sessionOpens returns the Open of each session with peer.
func (s *fakeService) sessionOpens(peer swarm.Address) [][]fingerprint.Range {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.opens[peer.ByteString()])
}

func (s *fakeService) LocalStatus() (rangesync.Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.local, s.localErr
}

func (s *fakeService) SendStatusWith(ctx context.Context, peer swarm.Address, st rangesync.Status) error {
	s.mu.Lock()
	s.sent[peer.ByteString()]++
	s.statuses[peer.ByteString()] = append(s.statuses[peer.ByteString()], st)
	calls, unsupported, err := s.sendCalls, s.unsupported[peer.ByteString()], s.sendErr[peer.ByteString()]
	block := s.sendBlock[peer.ByteString()]
	s.mu.Unlock()
	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	if calls != nil {
		release := make(chan struct{})
		select {
		case calls <- release:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if unsupported {
		return fmt.Errorf("%w: test", rangesync.ErrProtocolUnsupported)
	}
	return err
}

func (s *fakeService) LastStatus(peer swarm.Address) (rangesync.Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.remote[peer.ByteString()]
	return st, ok
}

func (s *fakeService) Reconcile(ctx context.Context, peer swarm.Address) ([]storer.ChunkID, error) {
	s.mu.Lock()
	gate := s.reconcileGate
	s.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconciled[peer.ByteString()]++
	if err := s.reconErr[peer.ByteString()]; err != nil {
		delete(s.reconErr, peer.ByteString()) // once
		return nil, err
	}
	if ids, ok := s.wantIDs[peer.ByteString()]; ok {
		return slices.Clone(ids), nil
	}
	return make([]storer.ChunkID, s.wants[peer.ByteString()]), nil
}

func (s *fakeService) Fetch(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (int, error) {
	s.mu.Lock()
	gate, delay, calls := s.fetchGate, s.fetchDelay, s.fetchCalls
	s.mu.Unlock()
	if calls != nil {
		release := make(chan struct{})
		select {
		case calls <- release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
		select {
		case <-release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fetchErr != nil {
		return 0, s.fetchErr
	}
	if s.withhold[peer.ByteString()] {
		return 0, nil
	}
	n := max(len(ids)-s.withholdN[peer.ByteString()], 0)
	s.fetched[peer.ByteString()] += n
	return n, nil
}

func (s *fakeService) count(m map[string]int, peer swarm.Address) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return m[peer.ByteString()]
}

func (s *fakeService) set(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f()
}

// fakeTopology is a topology whose connected peers are set by the test.
// onEach, if not nil, is called at the start of each EachConnectedPeer.
type fakeTopology struct {
	mu     sync.Mutex
	peers  []swarm.Address
	subs   []chan struct{}
	onEach func()
}

func (f *fakeTopology) SubscribeTopologyChange() (<-chan struct{}, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := make(chan struct{}, 1)
	f.subs = append(f.subs, c)
	return c, func() {}
}

func (f *fakeTopology) EachConnectedPeer(fn topology.EachPeerFunc, _ topology.Select) error {
	f.mu.Lock()
	peers := append([]swarm.Address(nil), f.peers...)
	hook := f.onEach
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	for _, p := range peers {
		if stop, _, err := fn(p, 0); err != nil || stop {
			return err
		}
	}
	return nil
}

// setPeersQuiet sets the connected peers without signaling a topology
// change: a round that runs next finds them.
func (f *fakeTopology) setPeersQuiet(peers ...swarm.Address) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.peers = peers
}

// setOnEach sets onEach.
func (f *fakeTopology) setOnEach(hook func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onEach = hook
}

func (f *fakeTopology) setPeers(peers ...swarm.Address) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.peers = peers
	for _, c := range f.subs {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

// fakeClock is a settable clock. The channels of After receive when Add
// moves the clock to or past their time.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []fakeTimer
}

type fakeTimer struct {
	at time.Time
	c  chan time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	c.timers = slices.DeleteFunc(c.timers, func(t fakeTimer) bool {
		if t.at.After(c.now) {
			return false
		}
		t.c <- c.now
		return true
	})
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.now
		return ch
	}
	c.timers = append(c.timers, fakeTimer{at: c.now.Add(d), c: ch})
	return ch
}

// advancingAfter returns an After that, for a wait of exactly d (a test's
// stripe retry interval), moves c on by d and fires at once, as if the caller
// waited d and nothing else happened meanwhile; other waits (the stripe
// timeout) follow c. Tests pick d unlike any time left on a stripe clock.
func advancingAfter(c *fakeClock, d time.Duration) func(time.Duration) <-chan time.Time {
	return func(w time.Duration) <-chan time.Time {
		if w != d {
			return c.After(w)
		}
		c.Add(d)
		ch := make(chan time.Time, 1)
		ch <- c.Now()
		return ch
	}
}

type harness struct {
	r     *reconciler.Reconciler
	svc   *fakeService
	topo  *fakeTopology
	clock *fakeClock
}

// newHarness returns a started reconciler with overlay self and fast rounds.
func newHarness(t *testing.T, svc *fakeService, peers ...swarm.Address) *harness {
	t.Helper()
	h := &harness{
		svc:   svc,
		topo:  &fakeTopology{peers: peers},
		clock: &fakeClock{now: time.Unix(1_000_000, 0)},
	}
	h.r = reconciler.New(self, h.topo, svc, log.Noop, reconciler.Options{
		Now:           h.clock.Now,
		After:         h.clock.After,
		RoundInterval: 10 * time.Millisecond,
	})
	t.Cleanup(func() {
		if err := h.r.Close(); err != nil {
			t.Error(err)
		}
	})
	h.r.Start(context.Background())
	return h
}

// waitFor polls cond until it holds or a deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// settle lets several rounds run.
func settle() { time.Sleep(100 * time.Millisecond) }

func TestPeerJoinLeave(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	h := newHarness(t, svc)
	settle()
	if n := svc.count(svc.reconciled, higher); n != 0 {
		t.Fatalf("reconciled %d times before join", n)
	}

	h.topo.setPeers(higher)
	waitFor(t, "status sent", func() bool { return svc.count(svc.sent, higher) > 0 })
	waitFor(t, "session", func() bool { return svc.count(svc.reconciled, higher) > 0 })
	waitFor(t, "peer known", func() bool { return h.r.KnownPeers() == 1 })

	h.topo.setPeers()
	waitFor(t, "peer forgotten", func() bool { return h.r.KnownPeers() == 0 })
	settle()
	n := svc.count(svc.reconciled, higher)
	settle()
	if m := svc.count(svc.reconciled, higher); m != n {
		t.Fatalf("sessions with a peer that left: %d -> %d", n, m)
	}
}

func TestOnlyLowerOverlayInitiates(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.remote[lower.ByteString()] = status(2)
	newHarness(t, svc, lower, higher)

	waitFor(t, "session with higher", func() bool { return svc.count(svc.reconciled, higher) > 0 })
	settle()
	if n := svc.count(svc.reconciled, lower); n != 0 {
		t.Fatalf("initiated %d sessions with a lower peer", n)
	}
	if n := svc.count(svc.sent, lower); n == 0 {
		t.Fatal("no status sent to the lower peer")
	}
}

func TestResponderWantsFetched(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[lower.ByteString()] = status(2)
	h := newHarness(t, svc, lower)
	h.r.Wants(lower, make([]storer.ChunkID, 3))
	waitFor(t, "fetch", func() bool { return svc.count(svc.fetched, lower) == 3 })
}

func TestUnsupportedPeerSkipped(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.unsupported[higher.ByteString()] = true
	svc.remote[higher.ByteString()] = status(2)
	svc.remote[lower.ByteString()] = status(1)
	h := newHarness(t, svc, higher, lower)
	waitFor(t, "probe", func() bool { return svc.count(svc.sent, higher) == 1 })
	h.topo.setPeers(higher, lower) // topology change
	settle()
	if n := svc.count(svc.sent, higher); n != 1 {
		t.Fatalf("unsupported peer probed %d times", n)
	}
	if n := svc.count(svc.reconciled, higher); n != 0 {
		t.Fatalf("reconciled an unsupported peer %d times", n)
	}
	// an unsupported peer does not hold the start-up guard
	waitFor(t, "guard released", h.r.GuardReleased)
}

func TestNotEligible(t *testing.T) {
	t.Parallel()

	equal := status(1)
	disjoint := status(2)
	disjoint.Root = fingerprint.NewRange(addr(0x00, 0), 1)
	ownRoot := status(1)
	ownRoot.Root = fingerprint.NewRange(addr(0x80, 0), 1)
	mapping := status(2)
	mapping.Mapping++

	for _, tc := range []struct {
		name          string
		local, remote rangesync.Status
	}{
		{"equal digest", status(1), equal},
		{"disjoint roots", ownRoot, disjoint},
		{"mapping mismatch", status(1), mapping},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newFakeService(tc.local)
			svc.remote[higher.ByteString()] = tc.remote
			newHarness(t, svc, higher)
			waitFor(t, "probe", func() bool { return svc.count(svc.sent, higher) > 0 })
			settle()
			if n := svc.count(svc.reconciled, higher); n != 0 {
				t.Fatalf("reconciled %d times", n)
			}
		})
	}
}

func TestTimerTriggersRounds(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	newHarness(t, svc, higher)
	waitFor(t, "several sessions", func() bool { return svc.count(svc.reconciled, higher) >= 3 })
}

func TestSyncRateLargeCatchUp(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.wants[higher.ByteString()] = reconciler.CatchUpMinDiff
	svc.fetchGate = make(chan struct{})
	h := newHarness(t, svc, higher)

	waitFor(t, "guard released", h.r.GuardReleased)
	// a catch-up fetch is in flight and nothing was counted yet
	if got := h.r.SyncRate(); got <= 0 {
		t.Fatalf("sync rate %v during catch-up, want > 0", got)
	}
	close(svc.fetchGate)
	waitFor(t, "fetch", func() bool { return svc.count(svc.fetched, higher) > 0 })
	waitFor(t, "rate", func() bool { return h.r.SyncRate() > 0 })
}

func TestSyncRateSmallSessionsZero(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.wants[higher.ByteString()] = reconciler.CatchUpMinDiff - 1
	h := newHarness(t, svc, higher)

	waitFor(t, "several sessions", func() bool { return svc.count(svc.fetched, higher) >= 3*(reconciler.CatchUpMinDiff-1) })
	waitFor(t, "guard released", h.r.GuardReleased)
	if got := h.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v with small sessions only", got)
	}
	// past the guard timeout nothing changes
	h.clock.Add(reconciler.StartupGuardTimeout + time.Second)
	if got := h.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v", got)
	}
}

func TestFailRaisesScore(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	h := newHarness(t, svc)
	p := higher
	if s := h.r.Score(p); s != 0 {
		t.Fatalf("initial score %d", s)
	}
	h.r.Fail(p, rangesync.ErrInvalidMessage)
	h.r.Fail(p, rangesync.ErrInvalidMessage)
	if s := h.r.Score(p); s != 2 {
		t.Fatalf("score %d, want 2", s)
	}
	h.r.Succeed(p)
	if s := h.r.Score(p); s != 2 {
		t.Fatalf("score after success %d, want 2", s)
	}
	h.clock.Add(reconciler.ScoreDecayInterval)
	if s := h.r.Score(p); s != 1 {
		t.Fatalf("score after one decay interval %d, want 1", s)
	}
	h.clock.Add(5 * reconciler.ScoreDecayInterval)
	if s := h.r.Score(p); s != 0 {
		t.Fatalf("score after decay %d, want 0", s)
	}
}

func TestHighScorePeerDeprioritized(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	h := newHarness(t, svc)
	bad, good := addr(0xf0, 1), addr(0xf0, 2)
	for range reconciler.ScoreThreshold + 1 {
		h.r.Fail(bad, rangesync.ErrInvalidMessage)
	}
	h.r.Fail(good, rangesync.ErrInvalidMessage) // at or below the threshold
	got := h.r.OrderCandidates([]swarm.Address{bad, good})
	if len(got) != 2 || !got[0].Equal(good) || !got[1].Equal(bad) {
		t.Fatalf("order %v", got)
	}
}

func TestSessionLimitNotScored(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.reconErr[higher.ByteString()] = fmt.Errorf("x: %w", rangesync.ErrSessionLimit)
	h := newHarness(t, svc, higher)
	waitFor(t, "retry", func() bool { return svc.count(svc.reconciled, higher) >= 2 })
	if s := h.r.Score(higher); s != 0 {
		t.Fatalf("score %d", s)
	}
}

func TestStartupGuardEmptyReserve(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(0)) // empty reserve
	svc.reconcileGate = make(chan struct{})
	svc.fetchGate = make(chan struct{})
	svc.wants[higher.ByteString()] = 2 * reconciler.CatchUpMinDiff
	h := newHarness(t, svc, higher)

	// no Status from the neighbor yet
	waitFor(t, "probe", func() bool { return svc.count(svc.sent, higher) > 0 })
	settle()
	if got := h.r.SyncRate(); got != reconciler.StartupGuardRate || h.r.GuardReleased() {
		t.Fatalf("sync rate %v before the status exchange", got)
	}
	// the exchange completes; the catch-up session is still reconciling
	svc.set(func() { svc.remote[higher.ByteString()] = status(5) })
	settle()
	if got := h.r.SyncRate(); got != reconciler.StartupGuardRate || h.r.GuardReleased() {
		t.Fatalf("sync rate %v before the catch-up started", got)
	}
	// the catch-up starts (fetch in flight): the guard is released and the
	// rate stays above zero
	close(svc.reconcileGate)
	waitFor(t, "guard released", h.r.GuardReleased)
	if got := h.r.SyncRate(); got <= 0 {
		t.Fatalf("sync rate %v during catch-up", got)
	}
	close(svc.fetchGate)
}

func TestStartupGuardLowerPeerInitiates(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(0))
	svc.remote[lower.ByteString()] = status(5) // lower initiates
	h := newHarness(t, svc, lower)
	waitFor(t, "probe", func() bool { return svc.count(svc.sent, lower) > 0 })
	settle()
	if got := h.r.SyncRate(); got != reconciler.StartupGuardRate || h.r.GuardReleased() {
		t.Fatalf("sync rate %v before the lower peer's session", got)
	}
	// the lower peer's session lists what this node lacks
	h.r.Wants(lower, make([]storer.ChunkID, reconciler.CatchUpMinDiff))
	waitFor(t, "guard released", h.r.GuardReleased)

	// a completed session without wants releases it too
	svc2 := newFakeService(status(0))
	svc2.remote[lower.ByteString()] = status(5)
	h2 := newHarness(t, svc2, lower)
	waitFor(t, "probe", func() bool { return svc2.count(svc2.sent, lower) > 0 })
	settle()
	if h2.r.GuardReleased() {
		t.Fatal("guard released before the lower peer's session")
	}
	h2.r.Succeed(lower)
	waitFor(t, "guard released", h2.r.GuardReleased)
}

func TestStartupGuardNoPeers(t *testing.T) {
	t.Parallel()

	h := newHarness(t, newFakeService(status(0)))
	settle()
	if got := h.r.SyncRate(); got != reconciler.StartupGuardRate {
		t.Fatalf("sync rate %v with no neighbors", got)
	}
	h.clock.Add(reconciler.StartupGuardTimeout)
	if got := h.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v after the guard timeout", got)
	}
}

func TestStartupGuardSilentNeighbor(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(0))
	h := newHarness(t, svc, higher) // never answers: no LastStatus
	waitFor(t, "probe", func() bool { return svc.count(svc.sent, higher) > 0 })
	h.clock.Add(reconciler.StartupGuardTimeout - time.Second)
	if got := h.r.SyncRate(); got != reconciler.StartupGuardRate {
		t.Fatalf("sync rate %v before the guard timeout", got)
	}
	h.clock.Add(time.Second)
	if got := h.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v after the guard timeout", got)
	}
}

func TestStartOnce(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	h := newHarness(t, svc)
	h.r.Start(context.Background()) // no-op
	if got := h.r.SyncRate(); got != reconciler.StartupGuardRate {
		t.Fatalf("sync rate %v", got)
	}
}

func TestStartNilServicePanics(t *testing.T) {
	t.Parallel()

	r := reconciler.New(self, &fakeTopology{}, nil, log.Noop, reconciler.Options{})
	defer func() {
		if recover() == nil {
			t.Fatal("no panic")
		}
	}()
	r.Start(context.Background())
}

func TestSetService(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	r := reconciler.New(self, &fakeTopology{peers: []swarm.Address{higher}}, nil, log.Noop, reconciler.Options{RoundInterval: 10 * time.Millisecond})
	r.SetService(svc)
	r.Start(context.Background())
	t.Cleanup(func() { _ = r.Close() })
	waitFor(t, "session", func() bool { return svc.count(svc.reconciled, higher) > 0 })
}

func TestClose(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.fetchGate = make(chan struct{}) // blocks until Close cancels it
	h := newHarness(t, svc, higher)
	waitFor(t, "session", func() bool { return svc.count(svc.reconciled, higher) > 0 })
	if err := h.r.Close(); err != nil {
		t.Fatal(err)
	}
	n := svc.count(svc.sent, higher)
	settle()
	if m := svc.count(svc.sent, higher); m != n {
		t.Fatalf("rounds after Close: %d -> %d", n, m)
	}
	h.r.Wants(higher, make([]storer.ChunkID, 1)) // ignored after Close
}

func TestMetrics(t *testing.T) {
	t.Parallel()

	h := newHarness(t, newFakeService(status(1)))
	if len(h.r.Metrics()) == 0 {
		t.Fatal("no collectors")
	}
}
