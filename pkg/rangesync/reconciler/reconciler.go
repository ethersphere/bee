// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"context"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
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
	// deprioritized: a round starts the sessions of peers above the
	// threshold after those of every other peer. Rounds do not skip or
	// delay such peers: a violation already disconnects the peer, and peers
	// that answer Busy or fail back off (peerfsm.go).
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
	// It drives the start-up guard, the score decay, the SyncRate window,
	// the Status gossip schedule and the stripe timeouts.
	Now func() time.Time
	// After returns a channel that receives once d has passed; time.After if
	// nil. Tests set one that follows the fake clock of Now. It wakes the
	// Status gossip when the next Status is due, runs a triggered round once
	// the minimum round interval has passed (scheduler.go), and times the
	// stripe timeouts and a stripe's waits to retry (stripes_transfer.go).
	After func(d time.Duration) <-chan time.Time
	// RoundInterval, if set, fixes the round interval: it is both the
	// minimum and the maximum of the adaptive interval (MinRoundInterval,
	// MaxRoundInterval), and the round timer runs on the real clock
	// (time.After), so that tests with a frozen fake clock still get timed
	// rounds. Without it the round timer runs on After. It is for tests:
	// the node leaves it zero.
	RoundInterval time.Duration
	// Jitter returns the jitter of a takeover wait, a fraction in [0, 1) of
	// the round interval; rand.Float64 if nil. Tests fix it.
	Jitter func() float64
	// StartupGuardTimeout overrides StartupGuardTimeout.
	StartupGuardTimeout time.Duration
	// StripeTimeout overrides StripeTimeout.
	StripeTimeout time.Duration
}

// peerState is what the reconciler keeps for a connected peer.
type peerState struct {
	fsm peerFSM // the peer's state machine (§8.1, peerfsm.go)
	// gen counts the machines the entry has had (resetLocked). A session,
	// fetch or stripe moves the machine only while gen is the one it started
	// with (currentLocked).
	gen    uint64
	sent   bool // a Status of ours reached the peer
	served bool // the peer started a session with this node (Succeed or Wants)
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
// (gossip.go) and runs Reconcile then Fetch in rounds (scheduler.go) with
// every eligible peer whose digest differs and whose overlay is higher than
// this node's, or lower and silent for too long. It keeps a state machine
// per peer (§8.1, peerfsm.go, peers.go): one session per peer, round or
// stripe, and a backoff for peers that answer Busy or fail. Large
// differences go to the stripe worker (stripes_transfer.go). It owns the
// per-peer failure score (rangesync.PeerScorer) and reports the catch-up
// rate (storer.Syncer).
type Reconciler struct {
	overlay       swarm.Address
	topo          Topology
	svc           Service
	logger        log.Logger
	now           func() time.Time
	after         func(time.Duration) <-chan time.Time
	guardTimeout  time.Duration
	stripeTimeout time.Duration
	stripeRetry   time.Duration // stripeRetryInterval; tests shorten it
	rate          *rateWindow
	metrics       metrics
	// sources is the id → peers map of the IDs the want set denied a fetch
	// (§7.8); retryLocks lets one retry fetch per peer run at a time
	// (wants.go).
	sources    *wantSources
	retryLocks *peerLocks

	// minInterval and maxInterval bound the adaptive round interval, whose
	// current value (nanoseconds) is interval; roundAfter runs the round
	// timer and jitter draws takeover jitters. slots holds a token per
	// running session this node opened (MaxInitiatedSessions), stripeSlots
	// one per stripe session (MaxStripeSessions). roundWake triggers a round
	// (wakeRound). foundDiff records that a session found differences since
	// the last round adapted the interval.
	minInterval, maxInterval time.Duration
	interval                 atomic.Int64
	roundAfter               func(time.Duration) <-chan time.Time
	jitter                   func() float64
	slots                    chan struct{}
	stripeSlots              chan struct{}
	roundWake                chan struct{}
	foundDiff                bool

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
	minInterval, maxInterval, roundAfter := MinRoundInterval, MaxRoundInterval, o.After
	if o.RoundInterval > 0 {
		minInterval, maxInterval, roundAfter = o.RoundInterval, o.RoundInterval, time.After
	}
	if o.Jitter == nil {
		o.Jitter = rand.Float64
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
		stripeRetry:   stripeRetryInterval,
		rate:          newRateWindow(SyncRateWindow, o.Now),
		sources:       newWantSources(),
		retryLocks:    newPeerLocks(),
		minInterval:   minInterval,
		maxInterval:   maxInterval,
		roundAfter:    roundAfter,
		jitter:        o.Jitter,
		slots:         make(chan struct{}, MaxInitiatedSessions),
		stripeSlots:   make(chan struct{}, MaxStripeSessions),
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
	r.metrics = newMetrics(r.SyncRate, func() float64 { return r.currentInterval().Seconds() }, r.peerStates)
	return r
}

// peerStates returns the number of followed peers in each state, for the
// peers{state} gauge.
func (r *Reconciler) peerStates() map[fsmState]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	counts := make(map[fsmState]int, len(stateNames))
	for _, st := range r.peers {
		counts[st.fsm.state]++
	}
	return counts
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

// fetch fetches ids from peer (fetchFrom: IDs the peer does not deliver are
// retried from other peers known to hold them) and counts the stored chunks
// towards SyncRate when catchUp. The caller incremented catchUpFetches for a
// catch-up fetch.
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
	out, err := r.fetchFrom(r.ctx, peer, ids, catchUp)
	if catchUp && out.stored > 0 {
		r.rate.Add(out.stored)
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
	r.foundDiff = true
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
			r.restartTakeoverLocked(st, r.now())
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
