// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/prometheus/client_golang/prometheus"
)

// loggerName is the name of the reconciler's logger.
const loggerName = "rangesync-reconciler"

const (
	// CatchUpMinDiff is the smallest resolved difference (number of wants) of
	// a session for its fetched chunks to count towards SyncRate (§8.4).
	CatchUpMinDiff = rangesync.CatchUpMinDiff
	// SyncRateWindow is the window over which SyncRate is measured.
	SyncRateWindow = rangesync.SyncRateWindow
	// StartupGuardTimeout bounds the start-up guard of SyncRate.
	StartupGuardTimeout = rangesync.StartupGuardTimeout
	// StartupGuardRate is the sentinel SyncRate reports while the start-up
	// guard holds, and while a catch-up fetch is in flight but no chunk was
	// counted in the window yet.
	StartupGuardRate = 1.0

	// ScoreThreshold is the failure score above which a peer is
	// deprioritized. In Phase 1 deprioritized means "run last": a round runs
	// the sessions of peers above the threshold after those of every other
	// peer. Phase 4 may tighten this (skip or delay such peers).
	ScoreThreshold = 3
	// ScoreDecayInterval is how often a peer's failure score drops by one;
	// a success does not reset it.
	ScoreDecayInterval = 10 * time.Minute

	// maxConcurrentProbes bounds the concurrent SendStatus calls of a round
	// or a gossip pass.
	maxConcurrentProbes = 16
)

// Service is the part of *rangesync.Service the reconciler uses, narrow so
// that tests can fake it.
type Service interface {
	// LocalStatus returns this node's current Status.
	LocalStatus() (rangesync.Status, error)
	// SendStatusWith sends st, which LocalStatus returned, to peer. An error
	// wrapping rangesync.ErrProtocolUnsupported means the peer does not
	// speak rangesync; the reconciler does not probe it again while
	// connected.
	SendStatusWith(ctx context.Context, peer swarm.Address, st rangesync.Status) error
	LastStatus(peer swarm.Address) (rangesync.Status, bool)
	// SubscribeRootChange signals that this node's root summary may have
	// changed (debounced); the reconciler then pushes its Status.
	SubscribeRootChange() (c <-chan struct{}, unsubscribe func())
	ReconcileWith(ctx context.Context, peer swarm.Address, opts rangesync.ReconcileOptions) ([]storer.ChunkID, error)
	Fetch(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (int, error)
}

var _ Service = (*rangesync.Service)(nil)

// Topology is the part of topology.Driver the reconciler uses.
type Topology interface {
	SubscribeTopologyChange() (c <-chan struct{}, unsubscribe func())
	EachConnectedPeer(topology.EachPeerFunc, topology.Select) error
}

// Options are the optional parameters of New. Zero values select the
// defaults.
type Options struct {
	// Now returns the current time; time.Now if nil. Tests set a fake clock.
	// It drives the start-up guard, the score decay, the SyncRate window and
	// the Status gossip schedule.
	Now func() time.Time
	// After returns a channel that receives once d has passed; time.After if
	// nil. Tests set one that follows the fake clock of Now. It wakes the
	// Status gossip when the next Status is due, and runs a triggered round
	// once the minimum round interval has passed (scheduler.go).
	After func(d time.Duration) <-chan time.Time
	// RoundInterval, if set, fixes the round interval: it is both the
	// minimum and the maximum of the adaptive interval (MinRoundInterval,
	// MaxRoundInterval). Tests set it.
	RoundInterval time.Duration
	// StartupGuardTimeout overrides StartupGuardTimeout.
	StartupGuardTimeout time.Duration
	// StripeTimeout overrides StripeTimeout.
	StripeTimeout time.Duration
}

// peerState is what the reconciler keeps for a connected peer.
type peerState struct {
	fsm      peerFSM // the peer's state machine (§8.1, peerfsm.go)
	sent     bool    // a Status of ours reached the peer
	served   bool    // the peer started a session with this node (Succeed or Wants)
	fetching bool    // the wants of the peer's last session are being fetched
	// stripeWaiters counts the stripe sessions waiting for the peer to leave
	// Reconciling or Striped (acquireStripe); rounds start no session with
	// it meanwhile. idle, if not nil, is closed when it does, or leaves.
	stripeWaiters int
	idle          chan struct{}
	// verdict is the peer's Status, if any (verdictHave), when it was found
	// Unsupported; a Status not seen then shows a reconnect.
	verdict     rangesync.Status
	verdictHave bool

	// Status gossip (gossip.go): lastSent is when our Status last, last,
	// was picked for the peer (zero: never), sending holds while that send
	// runs, failed records that it did not reach the peer (statusFails
	// times in a row), resend asks for it again although unchanged, and tat
	// is the theoretical arrival time of the sender-side rate (statusBurst).
	// reached is the root of the last Status of ours that reached the peer
	// (valid while sent); rounds start no session with a peer that lacks
	// this node's current root.
	lastSent    time.Time
	last        rangesync.Status
	sending     bool
	failed      bool
	statusFails int
	resend      bool
	tat         time.Time
	reached     fingerprint.Range

	// takeoverSince, if not zero, is when this node started to wait for the
	// peer, which initiates, to run a session while their digests differ;
	// takeoverJitter is the jitter of the wait, a fraction of one round
	// interval (takeoverDueLocked).
	takeoverSince  time.Time
	takeoverJitter float64
}

// score is a peer's failure score at time at.
type score struct {
	value int
	at    time.Time
}

// Reconciler follows the topology, exchanges Status with the connected peers
// and runs Reconcile then Fetch in rounds (scheduler.go) with every eligible
// peer whose digest differs and whose overlay is higher than this node's, or
// lower and silent for too long. It keeps a state machine per peer (§8.1,
// peerfsm.go): one session per peer, round or stripe, and a backoff for peers
// that answer Busy or fail. It owns the per-peer failure score
// (rangesync.PeerScorer) and reports the catch-up rate (storer.Syncer).
type Reconciler struct {
	overlay       swarm.Address
	topo          Topology
	svc           Service
	logger        log.Logger
	now           func() time.Time
	after         func(time.Duration) <-chan time.Time
	guardTimeout  time.Duration
	stripeTimeout time.Duration
	rate          *rateWindow
	metrics       metrics

	// minInterval and maxInterval bound the adaptive round interval, whose
	// current value (nanoseconds) is interval. slots holds a token per
	// running session this node opened (MaxInitiatedSessions). roundWake
	// triggers a round (wakeRound).
	minInterval, maxInterval time.Duration
	interval                 atomic.Int64
	slots                    chan struct{}
	roundWake                chan struct{}

	// pickMu orders the Status snapshot (LocalStatus) of a round or a
	// gossip pass together with its pick (pickDueLocked), so that a pick
	// never records an older Status than an earlier one. It is taken before
	// mu and never while holding it.
	pickMu sync.Mutex

	mu         sync.Mutex
	peers      map[string]*peerState
	scores     map[string]score
	guardArmed bool // a round completed the exchange with every neighbor
	// awaiting holds the eligible peers with a differing digest that
	// initiate (lower overlay) and have not started a session with this
	// node yet; the start-up guard holds while it is not empty.
	awaiting         map[string]bool
	pendingReconcile int // sessions whose Reconcile has not returned
	catchUpFetches   int // catch-up fetches in flight
	startedAt        time.Time
	// stripeRegistry holds the ranges queued for or in a striped transfer;
	// stripeQueue the queued ones, in order, for the stripe worker that
	// stripeWake wakes.
	stripeRegistry []fingerprint.Range
	stripeQueue    []stripeJob
	stripeWake     chan struct{}
	// gossipWake makes the gossip loop recompute when the next Status is
	// due.
	gossipWake chan struct{}
	// bootstrapping holds while a bootstrap transfer of the root is queued
	// or in flight; bootstrapFailed is set when one did not complete.
	bootstrapping, bootstrapFailed bool
	// radius follows the local root for radius changes (§7.5).
	radius radiusState

	started       atomic.Bool
	guardReleased atomic.Bool
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	closeOnce     sync.Once
}

var (
	_ rangesync.PeerScorer = (*Reconciler)(nil)
	_ storer.Syncer        = (*Reconciler)(nil)
)

// New returns a Reconciler for the node with the given overlay. It does no
// work until Start. svc may be set after New with SetService, because the
// rangesync.Service takes the Reconciler as its PeerScorer.
func New(overlay swarm.Address, topo Topology, svc Service, logger log.Logger, o Options) *Reconciler {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.After == nil {
		o.After = time.After
	}
	minInterval, maxInterval := MinRoundInterval, MaxRoundInterval
	if o.RoundInterval > 0 {
		minInterval, maxInterval = o.RoundInterval, o.RoundInterval
	}
	if o.StartupGuardTimeout <= 0 {
		o.StartupGuardTimeout = StartupGuardTimeout
	}
	if o.StripeTimeout <= 0 {
		o.StripeTimeout = StripeTimeout
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Reconciler{
		overlay:       overlay,
		topo:          topo,
		svc:           svc,
		logger:        logger.WithName(loggerName).Register(),
		now:           o.Now,
		after:         o.After,
		guardTimeout:  o.StartupGuardTimeout,
		stripeTimeout: o.StripeTimeout,
		rate:          newRateWindow(SyncRateWindow, o.Now),
		minInterval:   minInterval,
		maxInterval:   maxInterval,
		slots:         make(chan struct{}, MaxInitiatedSessions),
		roundWake:     make(chan struct{}, 1),
		peers:         make(map[string]*peerState),
		scores:        make(map[string]score),
		awaiting:      make(map[string]bool),
		stripeWake:    make(chan struct{}, 1),
		gossipWake:    make(chan struct{}, 1),
		ctx:           ctx,
		cancel:        cancel,
	}
	r.interval.Store(int64(minInterval))
	r.metrics = newMetrics(r.SyncRate, func() float64 { return r.currentInterval().Seconds() })
	return r
}

// SetService sets the Service when it was nil at New. It must be called
// before Start and is not safe for concurrent use with any other method:
// node wiring calls it once, between New and Start.
func (r *Reconciler) SetService(svc Service) {
	r.svc = svc
}

// Start subscribes to the topology, to the index's root changes and to the
// peers' Status changes (when the Service signals them, statusSubscriber),
// and starts the round and Status gossip loops. It returns immediately and is
// safe to call once, from the storer's reserve-worker start-up; later calls
// are no-ops. It panics when the Service is nil (a wiring bug). ctx is not
// retained: Close stops the loops.
func (r *Reconciler) Start(_ context.Context) {
	if r.svc == nil {
		panic("reconciler: Start without a Service")
	}
	if !r.started.CompareAndSwap(false, true) {
		return
	}
	r.mu.Lock()
	r.startedAt = r.now()
	r.mu.Unlock()
	if r.ctx.Err() != nil { // closed before start
		return
	}
	changes, unsubscribe := r.topo.SubscribeTopologyChange()
	rootChanges, unsubscribeRoot := r.svc.SubscribeRootChange()
	var (
		statuses          <-chan struct{}
		unsubscribeStatus = func() {}
	)
	if s, ok := r.svc.(statusSubscriber); ok {
		statuses, unsubscribeStatus = s.SubscribeStatus()
	}
	r.wg.Add(3)
	go func() {
		defer r.wg.Done()
		r.stripeWorker()
	}()
	go func() {
		defer r.wg.Done()
		defer unsubscribe()
		defer unsubscribeStatus()
		r.loop(changes, statuses)
	}()
	go func() {
		defer r.wg.Done()
		defer unsubscribeRoot()
		r.gossipLoop(rootChanges)
	}()
}

// round refreshes the peer set from the topology, sends the Statuses that
// are due (first contacts, a moved root; pickDueLocked), starts a session
// with every peer this node initiates with or takes over (scheduler.go) and
// adapts the round interval.
func (r *Reconciler) round() {
	var connected []swarm.Address
	_ = r.topo.EachConnectedPeer(func(addr swarm.Address, _ uint8) (bool, bool, error) {
		connected = append(connected, addr)
		return false, false, nil
	}, topology.Select{})

	r.pickMu.Lock()
	local, err := r.svc.LocalStatus()
	if err != nil {
		r.pickMu.Unlock()
		r.logger.Debug("round skipped", "error", err)
		return
	}
	r.mu.Lock()
	r.trackRootLocked(local.Root)
	radiusPending := r.radius.pending
	present := make(map[string]bool, len(connected))
	for _, p := range connected {
		k := p.ByteString()
		present[k] = true
		st, ok := r.peers[k]
		if !ok {
			st = &peerState{fsm: newPeerFSM()}
			r.peers[k] = st
		}
		// Without the peer's Status the peer may lack this node's too: both
		// are forgotten on a disconnect, which a reconnect between two
		// rounds hides from the peer set. Ask for it to be sent again.
		remote, have := r.svc.LastStatus(p)
		if !have && !st.sending {
			st.resend = true
		}
		// A Status from a peer that did not speak rangesync, one it had not
		// sent when it was found so (statuses are forgotten on a
		// disconnect), shows such a reconnect: follow the new connection
		// from a first contact.
		if have && st.fsm.state == stateUnsupported && !st.sending && (!st.verdictHave || !equalStatus(remote, st.verdict)) {
			st.fsm = newPeerFSM()
			st.sent, st.lastSent = false, time.Time{}
			r.logger.Debug("peer speaks rangesync after a reconnect", "peer_address", p)
		}
	}
	for k, st := range r.peers {
		if !present[k] {
			r.fireLocked(st, eventDisconnected)
			st.wakeLocked()
			delete(r.peers, k)
		}
	}
	// First contacts, and a new root (it makes the peers' session scopes
	// with this node stale), go out now unless the sender-side rate defers
	// them, so that the evaluation below sees the outcome. The gossip loop
	// sends the rest when due.
	due := r.pickDueLocked(local, r.now())
	r.mu.Unlock()
	r.pickMu.Unlock()

	r.probe(due, local)
	r.wakeGossip()
	if r.ctx.Err() != nil {
		return
	}
	// The peers whose Statuses are now exchanged leave Probing before any
	// stripe is planned below, so that the stripe sessions find them Idle.
	r.mu.Lock()
	r.admitLocked(local, connected)
	r.mu.Unlock()

	// An empty or far-behind node transfers its root striped (§7.4) instead
	// of one full listing per peer; its per-peer sessions wait while that
	// transfer is in flight. After a bootstrap transfer that did not
	// complete, one round runs the per-peer sessions before the next try.
	// After a radius decrease only the new half is striped (§7.5), and the
	// node is not bootstrapping however far its count is behind.
	bootstrap := false
	if radiusPending {
		r.enqueueRadiusHalf()
	} else if IsBootstrap(local, r.peerStatuses()) {
		r.mu.Lock()
		switch {
		case r.bootstrapping:
			bootstrap = true
		case r.bootstrapFailed:
			r.bootstrapFailed = false
		default:
			started := func() {
				r.mu.Lock()
				r.pendingReconcile--
				r.checkGuardLocked()
				r.mu.Unlock()
			}
			if r.registerStripeLocked(stripeJob{rg: local.Root, started: started, bootstrap: true}) {
				r.bootstrapping, bootstrap = true, true
				r.pendingReconcile++
			}
		}
		r.mu.Unlock()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// A peer that accepts our Status but never sends its own keeps
	// exchanged false; the guard then releases only at StartupGuardTimeout
	// (the fallback path). Its eligibility is unknown without its Status.
	// A differing peer this node initiates with that backs off, or that
	// lacks this node's current root, keeps its difference unresolved, and
	// so does the guard.
	exchanged, eligible, unresolved, differs := true, 0, false, false
	clear(r.awaiting)
	var candidates []swarm.Address
	opens := make(map[string][]fingerprint.Range)
	now := r.now()
	for _, p := range connected {
		st, ok := r.peers[p.ByteString()]
		if !ok {
			continue
		}
		// admitted from the Status evaluated below, so that an exchanged,
		// eligible peer is never left out of Idle by a Status that arrived
		// since admitLocked
		remote, have := r.svc.LastStatus(p)
		r.admitPeerLocked(p, st, local, remote, have)
		if st.fsm.state == stateUnsupported {
			continue
		}
		if !st.sent || !have {
			exchanged = false
			st.takeoverSince = time.Time{}
			continue
		}
		if !isEligible(local, remote) {
			st.takeoverSince = time.Time{}
			continue
		}
		eligible++
		if sameDigest(local, remote) {
			st.takeoverSince = time.Time{}
			r.fireLocked(st, eventEqualDigest)
			continue
		}
		differs = true
		if !r.initiates(p) {
			if !st.served {
				r.awaiting[p.ByteString()] = true
			}
			if !r.takeoverDueLocked(st, now) {
				continue
			}
		}
		if st.fsm.state == stateIdle && !st.fsm.canReconcile(now) {
			unresolved = true // backing off
			continue
		}
		// Idle and past its backoff, with no stripe session waiting for it
		if bootstrap || st.stripeWaiters > 0 || !st.fsm.canReconcile(now) {
			continue
		}
		// The peer must have this node's current root: one moved since the
		// last Status that reached the peer may still be in flight, or
		// deferred by the sender-side rate, and the peer would refuse the
		// session. The gossip pass that delivers it triggers a round.
		if !st.reached.Equal(local.Root) {
			unresolved = true
			continue
		}
		open, skip := r.radiusOpenLocked(local, remote)
		if skip {
			continue
		}
		candidates = append(candidates, p)
		opens[p.ByteString()] = open
	}
	for _, p := range r.orderCandidatesLocked(candidates) {
		st := r.peers[p.ByteString()]
		r.fireLocked(st, eventReconcile)
		r.pendingReconcile++
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.session(p, st, opens[p.ByteString()])
		}()
	}
	r.guardArmed = exchanged && eligible > 0 && !unresolved
	r.checkGuardLocked()
	r.adaptInterval(differs)
}

// probe sends local to the picked peers, at most maxConcurrentProbes at
// once, and records the outcome in the entry each was picked with. It
// reports whether a send delivered a root the peer did not have from this
// node before (a first contact or a moved root).
func (r *Reconciler) probe(picks []statusPick, local rangesync.Status) (newRoot bool) {
	sem := make(chan struct{}, maxConcurrentProbes)
	var wg sync.WaitGroup
	for _, pk := range picks {
		select {
		case sem <- struct{}{}:
		case <-r.ctx.Done():
			wg.Wait()
			return newRoot
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			err := r.svc.SendStatusWith(r.ctx, pk.peer, local)
			r.mu.Lock()
			defer r.mu.Unlock()
			st := pk.st
			if r.peers[pk.peer.ByteString()] != st {
				return // left meanwhile, maybe with a new entry since
			}
			st.sending = false
			switch {
			case err == nil:
				if !st.sent || !st.reached.Equal(local.Root) {
					newRoot = true
				}
				st.sent, st.failed, st.statusFails = true, false, 0
				st.reached = local.Root
			case errors.Is(err, rangesync.ErrProtocolUnsupported):
				if st.fsm.state != stateProbing {
					// a reconnect the peer set did not show: a new peer
					st.fsm = newPeerFSM()
					st.wakeLocked()
				}
				r.fireLocked(st, eventUnsupported)
				st.verdict, st.verdictHave = r.svc.LastStatus(pk.peer)
				r.logger.Debug("peer does not support rangesync", "peer_address", pk.peer)
			default:
				// sent stays: an earlier Status of ours reached the peer
				st.failed = true
				st.statusFails++
				r.logger.Debug("send status", "peer_address", pk.peer, "failures", st.statusFails, "error", err)
			}
		}()
	}
	wg.Wait()
	return newRoot
}

// isEligible reports whether this node and a peer with status remote run
// sessions: same Mapping and a valid session scope (rangesync.NewScope,
// which applies rangesync.SessionRoot).
func isEligible(local, remote rangesync.Status) bool {
	if local.Mapping != remote.Mapping {
		return false
	}
	_, err := rangesync.NewScope(local, remote)
	return err == nil
}

// sameDigest reports whether the two statuses summarize the same elements.
// Statuses with different roots are compared as different: their summaries
// cover different ranges (Phase 4 compares summaries of the session root).
func sameDigest(local, remote rangesync.Status) bool {
	return local.Root.Equal(remote.Root) && local.Summary == remote.Summary
}

// initiates reports whether this node starts sessions with peer: the peer
// with the lower overlay initiates (§8.2).
func (r *Reconciler) initiates(peer swarm.Address) bool {
	return r.overlay.Compare(peer) < 0
}

// session runs Reconcile then Fetch with peer, whose entry st is in
// Reconciling, then transfers the ranges the session handed to its Striper
// (d̂ > LargeDiff) as stripes. It waits for a session slot first
// (MaxInitiatedSessions). open, if not empty, is the session's opening (the
// old half after a radius decrease). The outcome moves st on: the peer's Busy
// or an error back to Idle with a backoff, a cause on this node (localCause)
// back to Idle without one, a completed session to Fetching and, once its
// wants are fetched, to Idle. A takeover (the peer initiates) that finds the
// peer's session slot taken on this node found the initiator running a
// session with this node: it goes back to Idle without a backoff and waits
// for the initiator again.
func (r *Reconciler) session(peer swarm.Address, st *peerState, open []fingerprint.Range) {
	striper := new(StripeCollector)
	release, err := r.acquireSlot(r.ctx)
	var wants []storer.ChunkID
	if err == nil {
		wants, err = r.svc.ReconcileWith(r.ctx, peer, rangesync.ReconcileOptions{Open: open, Striper: striper})
		release()
	}
	takeover := !r.initiates(peer)

	r.mu.Lock()
	r.pendingReconcile--
	catchUp := err == nil && len(wants) >= CatchUpMinDiff
	if catchUp {
		r.catchUpFetches++
	}
	if r.peers[peer.ByteString()] == st { // not left meanwhile
		switch {
		case err == nil:
			if r.fireLocked(st, eventSessionDone) {
				st.fetching = true
			}
		case takeover && errors.Is(err, rangesync.ErrPeerSessionLimit):
			r.fireLocked(st, eventAborted)
			st.restartTakeover(r.now())
		case localCause(err):
			r.fireLocked(st, eventAborted)
		case isBusy(err):
			r.fireLocked(st, eventBusy)
		default:
			r.fireLocked(st, eventError)
		}
		if err != nil {
			// the difference is unresolved: the next round arms the guard
			// again, or keeps it while the peer backs off
			r.guardArmed = false
		}
	}
	r.checkGuardLocked()
	r.mu.Unlock()

	switch {
	case errors.Is(err, rangesync.ErrSessionLimit):
		r.metrics.Sessions.WithLabelValues("session_limit").Inc()
		return // retried after the backoff, not scored
	case isBusy(err):
		r.metrics.Sessions.WithLabelValues("busy").Inc()
		return // an index is rebuilding: retried after the backoff, not scored
	case err != nil:
		r.metrics.Sessions.WithLabelValues("error").Inc()
		r.logger.Debug("reconcile", "peer_address", peer, "error", err)
		return
	}
	r.metrics.Sessions.WithLabelValues("ok").Inc()
	r.fetch(peer, wants, catchUp)
	r.mu.Lock()
	if r.peers[peer.ByteString()] == st {
		st.fetching = false
		r.fireLocked(st, eventFetched) // a stripe session in flight ends Fetching instead
	}
	r.mu.Unlock()
	if ranges := striper.Ranges(); len(ranges) > 0 {
		r.enqueueStripes(ranges)
	}
}

// fetch fetches ids from peer and counts the stored chunks towards SyncRate
// when catchUp. The caller incremented catchUpFetches for a catch-up fetch.
func (r *Reconciler) fetch(peer swarm.Address, ids []storer.ChunkID, catchUp bool) {
	if catchUp {
		defer func() {
			r.mu.Lock()
			r.catchUpFetches--
			r.mu.Unlock()
		}()
	}
	if len(ids) == 0 {
		return
	}
	stored, err := r.svc.Fetch(r.ctx, peer, ids)
	if catchUp && stored > 0 {
		r.rate.Add(stored)
	}
	if err != nil {
		r.logger.Debug("fetch", "peer_address", peer, "error", err)
	}
}

// Wants fetches the IDs a peer listed in a session this node served and this
// node does not hold (rangesync.Options.Wants). The fetch counts towards
// SyncRate like an initiated session of the same size. It returns at once;
// it is a no-op before Start and after Close.
func (r *Reconciler) Wants(peer swarm.Address, ids []storer.ChunkID) {
	if !r.started.Load() || len(ids) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx.Err() != nil {
		return
	}
	r.markServedLocked(peer)
	catchUp := len(ids) >= CatchUpMinDiff
	if catchUp {
		r.catchUpFetches++
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.fetch(peer, ids, catchUp)
	}()
}

// checkGuardLocked releases the start-up guard once a round completed the
// exchange with every neighbor and every session it started has resolved its
// difference (any catch-up it triggered is fetching).
func (r *Reconciler) checkGuardLocked() {
	if r.guardArmed && r.pendingReconcile == 0 && len(r.awaiting) == 0 {
		r.guardReleased.Store(true)
	}
}

// SyncRate returns the rate, in chunks per second over SyncRateWindow, of
// chunks fetched by sessions that resolved at least CatchUpMinDiff wants.
// Until the first Status exchange with every eligible connected peer has
// completed and any catch-up it triggered has started (sessions this node
// initiates have resolved their difference, and every eligible peer with a
// differing digest that initiates has run a session with this node), or until
// StartupGuardTimeout after Start, it returns StartupGuardRate. With no
// eligible neighbor the guard holds until the timeout. While a catch-up
// fetch is in flight, or a range is registered for a striped transfer
// (queued, transferring or being verified), or a radius decrease's new half
// waits for its transfer (at most radiusHold), it is at least
// StartupGuardRate.
func (r *Reconciler) SyncRate() float64 {
	if !r.started.Load() {
		return StartupGuardRate
	}
	r.mu.Lock()
	inFlight := r.catchUpFetches > 0 || len(r.stripeRegistry) > 0 || r.radiusHoldLocked()
	guard := !r.guardReleased.Load() && r.now().Sub(r.startedAt) < r.guardTimeout
	r.mu.Unlock()
	rt := r.rate.Rate()
	if (guard || inFlight) && rt < StartupGuardRate {
		return StartupGuardRate
	}
	return rt
}

// Fail implements rangesync.PeerScorer: it raises the failure score of peer
// by one.
func (r *Reconciler) Fail(peer swarm.Address, reason error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.scores[peer.ByteString()] = score{value: r.scoreLocked(peer, now) + 1, at: now}
	r.metrics.Failures.Inc()
	r.logger.Debug("peer failed", "peer_address", peer, "reason", reason)
}

// Succeed implements rangesync.PeerScorer. A success does not lower the
// score; only time does. It marks that a session with peer completed, which
// the start-up guard waits for when the peer is the initiator.
func (r *Reconciler) Succeed(peer swarm.Address) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.markServedLocked(peer)
}

// markServedLocked records that a session with peer completed (or the peer
// listed wants in one this node served) and re-checks the start-up guard. A
// pending takeover of the peer waits again from now: the initiator is
// active, or this node's own takeover completed.
func (r *Reconciler) markServedLocked(peer swarm.Address) {
	if st, ok := r.peers[peer.ByteString()]; ok {
		st.served = true
		if !st.takeoverSince.IsZero() {
			st.restartTakeover(r.now())
		}
	}
	delete(r.awaiting, peer.ByteString())
	r.checkGuardLocked()
}

// Score returns the failure score of peer at Now(), after decay: the score
// drops by one every ScoreDecayInterval since the last Fail, down to zero.
func (r *Reconciler) Score(peer swarm.Address) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.scoreLocked(peer, r.now())
}

func (r *Reconciler) scoreLocked(peer swarm.Address, now time.Time) int {
	s, ok := r.scores[peer.ByteString()]
	if !ok {
		return 0
	}
	v := s.value - int(now.Sub(s.at)/ScoreDecayInterval)
	if v <= 0 {
		delete(r.scores, peer.ByteString())
		return 0
	}
	return v
}

// orderCandidates returns peers with those above ScoreThreshold moved last,
// keeping the order otherwise.
func (r *Reconciler) orderCandidates(peers []swarm.Address) []swarm.Address {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.orderCandidatesLocked(peers)
}

func (r *Reconciler) orderCandidatesLocked(peers []swarm.Address) []swarm.Address {
	now := r.now()
	out := slices.Clone(peers)
	slices.SortStableFunc(out, func(a, b swarm.Address) int {
		ah := r.scoreLocked(a, now) > ScoreThreshold
		bh := r.scoreLocked(b, now) > ScoreThreshold
		switch {
		case ah == bh:
			return 0
		case ah:
			return 1
		default:
			return -1
		}
	})
	return out
}

// Metrics returns the reconciler's collectors. Its sync_rate gauge is this
// reconciler's own SyncRate, not the node's combined rate.
func (r *Reconciler) Metrics() []prometheus.Collector {
	return r.metrics.collectors()
}

// Close unsubscribes from the topology and the index, stops the round and
// gossip loops, cancels running sessions and waits for them to end. It is
// safe to call more than once.
func (r *Reconciler) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.cancel()
		r.mu.Unlock()
		r.wg.Wait()
	})
	return nil
}
