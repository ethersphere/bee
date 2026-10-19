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
	"github.com/ethersphere/bee/v2/pkg/rate"
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
	// RoundInterval is the interval of the timer-driven rounds. Every round
	// also exchanges Status with every connected peer.
	RoundInterval = rangesync.StatusMaxInterval
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

	// maxConcurrentProbes bounds the concurrent SendStatus calls of a round.
	maxConcurrentProbes = 16
)

// Service is the part of *rangesync.Service the reconciler uses, narrow so
// that tests can fake it.
type Service interface {
	// LocalStatus returns this node's current Status.
	LocalStatus() (rangesync.Status, error)
	// SendStatus sends this node's Status to peer. An error wrapping
	// rangesync.ErrProtocolUnsupported means the peer does not speak
	// rangesync; the reconciler does not probe it again while connected.
	SendStatus(ctx context.Context, peer swarm.Address) error
	LastStatus(peer swarm.Address) (rangesync.Status, bool)
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
	// It drives the start-up guard and the score decay; the rate window
	// (pkg/rate) uses the real clock.
	Now func() time.Time
	// RoundInterval overrides RoundInterval.
	RoundInterval time.Duration
	// StartupGuardTimeout overrides StartupGuardTimeout.
	StartupGuardTimeout time.Duration
	// StripeTimeout overrides StripeTimeout.
	StripeTimeout time.Duration
}

// peerState is what the reconciler keeps for a connected peer.
type peerState struct {
	unsupported bool // SendStatus reported ErrProtocolUnsupported
	sent        bool // our Status reached the peer
	running     bool // a session with the peer is running
	served      bool // the peer started a session with this node (Succeed or Wants)
}

// score is a peer's failure score at time at.
type score struct {
	value int
	at    time.Time
}

// Reconciler follows the topology, exchanges Status with the connected peers
// and runs Reconcile then Fetch with every eligible peer whose digest differs
// and whose overlay is higher than this node's. It owns the per-peer failure
// score (rangesync.PeerScorer) and reports the catch-up rate (storer.Syncer).
type Reconciler struct {
	overlay       swarm.Address
	topo          Topology
	svc           Service
	logger        log.Logger
	now           func() time.Time
	roundInterval time.Duration
	guardTimeout  time.Duration
	stripeTimeout time.Duration
	rate          *rate.Rate
	metrics       metrics

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
	if o.RoundInterval <= 0 {
		o.RoundInterval = RoundInterval
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
		roundInterval: o.RoundInterval,
		guardTimeout:  o.StartupGuardTimeout,
		stripeTimeout: o.StripeTimeout,
		rate:          rate.New(SyncRateWindow),
		peers:         make(map[string]*peerState),
		scores:        make(map[string]score),
		awaiting:      make(map[string]bool),
		stripeWake:    make(chan struct{}, 1),
		ctx:           ctx,
		cancel:        cancel,
	}
	r.metrics = newMetrics(r.SyncRate)
	return r
}

// SetService sets the Service when it was nil at New. It must be called
// before Start and is not safe for concurrent use with any other method:
// node wiring calls it once, between New and Start.
func (r *Reconciler) SetService(svc Service) {
	r.svc = svc
}

// Start subscribes to the topology and starts the round loop. It returns
// immediately and is safe to call once, from the storer's reserve-worker
// start-up; later calls are no-ops. It panics when the Service is nil (a
// wiring bug). ctx is not retained: Close stops the loop.
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
	r.wg.Add(2)
	go func() {
		defer r.wg.Done()
		r.stripeWorker()
	}()
	go func() {
		defer r.wg.Done()
		defer unsubscribe()
		r.loop(changes)
	}()
}

// loop runs a full round at start and every round interval, and a probing
// round on every topology change, until Close.
func (r *Reconciler) loop(changes <-chan struct{}) {
	ticker := time.NewTicker(r.roundInterval)
	defer ticker.Stop()
	r.round(true)
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.round(true)
		case <-changes:
			r.round(false)
		}
	}
}

// round refreshes the peer set from the topology, sends Status to the peers
// (all of them when full, else those without a completed exchange) and
// starts a session with every peer this node initiates with.
func (r *Reconciler) round(full bool) {
	local, err := r.svc.LocalStatus()
	if err != nil {
		r.logger.Debug("round skipped", "error", err)
		return
	}

	var connected []swarm.Address
	_ = r.topo.EachConnectedPeer(func(addr swarm.Address, _ uint8) (bool, bool, error) {
		connected = append(connected, addr)
		return false, false, nil
	}, topology.Select{})

	r.mu.Lock()
	present := make(map[string]bool, len(connected))
	var probe []swarm.Address
	for _, p := range connected {
		k := p.ByteString()
		present[k] = true
		st, ok := r.peers[k]
		if !ok {
			st = &peerState{}
			r.peers[k] = st
		}
		if st.unsupported {
			continue
		}
		if _, have := r.svc.LastStatus(p); full || !st.sent || !have {
			probe = append(probe, p)
		}
	}
	for k := range r.peers {
		if !present[k] {
			delete(r.peers, k)
		}
	}
	r.mu.Unlock()

	r.probe(probe)
	if r.ctx.Err() != nil {
		return
	}

	// An empty or far-behind node transfers its root striped (§7.4) instead
	// of one full listing per peer; its per-peer sessions wait until the
	// transfer is done.
	bootstrap := IsBootstrap(local, r.peerStatuses())
	if bootstrap {
		r.mu.Lock()
		r.pendingReconcile++
		r.mu.Unlock()
		started := func() {
			r.mu.Lock()
			r.pendingReconcile--
			r.checkGuardLocked()
			r.mu.Unlock()
		}
		if !r.enqueueStripes([]fingerprint.Range{local.Root}, started) {
			started() // in flight already
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// A peer that accepts our Status but never sends its own keeps
	// exchanged false; the guard then releases only at StartupGuardTimeout
	// (the fallback path). Its eligibility is unknown without its Status.
	exchanged, eligible := true, 0
	clear(r.awaiting)
	var candidates []swarm.Address
	for _, p := range connected {
		st, ok := r.peers[p.ByteString()]
		if !ok || st.unsupported {
			continue
		}
		remote, have := r.svc.LastStatus(p)
		if !st.sent || !have {
			exchanged = false
			continue
		}
		if !isEligible(local, remote) {
			continue
		}
		eligible++
		if sameDigest(local, remote) {
			continue
		}
		if !r.initiates(p) {
			if !st.served {
				r.awaiting[p.ByteString()] = true
			}
			continue
		}
		if st.running || bootstrap {
			continue
		}
		candidates = append(candidates, p)
	}
	for _, p := range r.orderCandidatesLocked(candidates) {
		r.peers[p.ByteString()].running = true
		r.pendingReconcile++
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.session(p)
		}()
	}
	r.guardArmed = exchanged && eligible > 0
	r.checkGuardLocked()
}

// probe sends Status to peers, at most maxConcurrentProbes at once, and
// records the outcome.
func (r *Reconciler) probe(peers []swarm.Address) {
	sem := make(chan struct{}, maxConcurrentProbes)
	var wg sync.WaitGroup
	for _, p := range peers {
		select {
		case sem <- struct{}{}:
		case <-r.ctx.Done():
			wg.Wait()
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			err := r.svc.SendStatus(r.ctx, p)
			r.mu.Lock()
			defer r.mu.Unlock()
			st, ok := r.peers[p.ByteString()]
			if !ok {
				return // left meanwhile
			}
			switch {
			case err == nil:
				st.sent = true
			case errors.Is(err, rangesync.ErrProtocolUnsupported):
				st.unsupported = true
			default:
				st.sent = false
				r.logger.Debug("send status", "peer_address", p, "error", err)
			}
		}()
	}
	wg.Wait()
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

// session runs Reconcile then Fetch with peer, then transfers the ranges
// the session handed to its Striper (d̂ > LargeDiff) as stripes.
func (r *Reconciler) session(peer swarm.Address) {
	striper := new(StripeCollector)
	wants, err := r.svc.ReconcileWith(r.ctx, peer, rangesync.ReconcileOptions{Striper: striper})

	r.mu.Lock()
	r.pendingReconcile--
	catchUp := err == nil && len(wants) >= CatchUpMinDiff
	if catchUp {
		r.catchUpFetches++
	}
	r.checkGuardLocked()
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		if st, ok := r.peers[peer.ByteString()]; ok {
			st.running = false
		}
		r.mu.Unlock()
	}()

	switch {
	case errors.Is(err, rangesync.ErrSessionLimit):
		r.metrics.Sessions.WithLabelValues("session_limit").Inc()
		return // retry next round, not scored
	case err != nil:
		r.metrics.Sessions.WithLabelValues("error").Inc()
		r.logger.Debug("reconcile", "peer_address", peer, "error", err)
		return
	}
	r.metrics.Sessions.WithLabelValues("ok").Inc()
	r.fetch(peer, wants, catchUp)
	if ranges := striper.Ranges(); len(ranges) > 0 {
		r.enqueueStripes(ranges, nil)
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
// fetch is in flight it is at least StartupGuardRate.
func (r *Reconciler) SyncRate() float64 {
	if !r.started.Load() {
		return StartupGuardRate
	}
	r.mu.Lock()
	inFlight := r.catchUpFetches > 0
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

// markServedLocked records that peer ran a session with this node and
// re-checks the start-up guard.
func (r *Reconciler) markServedLocked(peer swarm.Address) {
	if st, ok := r.peers[peer.ByteString()]; ok {
		st.served = true
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

// Close unsubscribes from the topology, stops the round loop, cancels
// running sessions and waits for them to end. It is safe to call more than
// once.
func (r *Reconciler) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.cancel()
		r.mu.Unlock()
		r.wg.Wait()
	})
	return nil
}
