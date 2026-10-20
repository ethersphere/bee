// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"context"
	"errors"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

// The round scheduler (§8.2).
//
//   - Initiator. Of two peers whose digests differ, the one with the lower
//     overlay starts the session. The other side takes over when the
//     mismatch persists for takeoverRounds round intervals plus a jitter of
//     up to one round interval with no session of the initiator with this
//     node (takeoverDueLocked; the maximum round interval for a peer of
//     another root); a session the initiator runs with this node, or a
//     takeover that finds the initiator alive (its session holding the
//     peer's session slot, or a Busy answer), restarts the wait.
//   - Rounds. A round follows the topology into the per-peer state machines,
//     sends the Statuses that are due, admits the peers whose Statuses are
//     exchanged, registers a bootstrap or radius-decrease transfer for the
//     stripe worker, then starts a session with every eligible peer whose
//     digest differs and that this node initiates with (or takes over), at
//     most one per peer (the peer's state machine). The round decides on one
//     snapshot of this node's Status (LocalStatus); each session reads the
//     index itself (see the T4.3 decisions log for why a round-wide index
//     snapshot does not exist).
//   - Interval. After each round the interval halves, down to
//     MinRoundInterval, when differences were found, and doubles, up to
//     MaxRoundInterval, otherwise (adaptInterval). Differences are a
//     differing digest of a peer with the same root, or a session since the
//     last round that found some (wants, ranges for stripes, or wants the
//     peer listed in a session this node served). Digests of peers with
//     different roots summarize different ranges and are not compared.
//   - Recheck. A session that waited for a slot starts only if the round's
//     decision still holds (recheckSession); otherwise it ends without a
//     backoff.
//   - Collisions. A session that finds the peer's session slot on this node
//     taken (the peer runs its own session with this node) ends without a
//     backoff, and a takeover answered Busy waits for the initiator again:
//     the initiator is alive.
//   - Triggers. A round runs on the round timer, on every topology change, and
//     on a trigger (a changed Status of a peer, a root change of this node, a
//     moved root delivered to a peer), which is deferred to the minimum round
//     interval after the last round, so that triggers do not run sessions
//     with a peer faster than its session rate (rangesync.PeerSessionInterval).
//     A topology change runs a round at once, so that a new peer gets its
//     first Status, and its first session, without waiting.
//   - Cap. The sessions this node opens, round sessions, takeovers, stripe
//     transfers and verifications alike, take one of MaxInitiatedSessions
//     slots first and wait for one, first come, first served. Stripe
//     transfers and verifications first take one of MaxStripeSessions
//     stripe slots, so that round sessions always have the other slots, and
//     free both while they wait to retry a Busy answer.
const (
	// MinRoundInterval is the shortest round interval, used while rounds
	// keep finding differences.
	MinRoundInterval = rangesync.MinRoundInterval
	// MaxRoundInterval is the longest round interval, used while every
	// digest matches.
	MaxRoundInterval = rangesync.MaxRoundInterval

	// MaxInitiatedSessions bounds the sessions this node opens at once:
	// round sessions, takeovers, stripe transfers and verifications
	// together. It is half of rangesync.MaxConcurrentSessions, whose tokens
	// the sessions peers open with this node share, so that the other half
	// stays for those.
	MaxInitiatedSessions = rangesync.MaxConcurrentSessions / 2
	// MaxStripeSessions bounds the stripe transfers and verifications that
	// run at once. They hold their slot for a whole stripe session, which
	// the fetch queue backpressures (up to rangesync's session timeout), so
	// they take at most half of MaxInitiatedSessions and round sessions keep
	// the rest.
	MaxStripeSessions = MaxInitiatedSessions / 2

	// takeoverRounds is the number of round intervals, plus a jitter of up
	// to one, that the peer with the higher overlay waits for the initiator
	// before it starts the session itself.
	takeoverRounds = 2
)

// statusSubscriber is implemented by a Service that signals changed peer
// Statuses (*rangesync.Service). The reconciler then wakes its gossip and
// triggers a round.
type statusSubscriber interface {
	SubscribeStatus() (c <-chan struct{}, unsubscribe func())
}

var _ statusSubscriber = (*rangesync.Service)(nil)

// currentInterval returns the current round interval.
func (r *Reconciler) currentInterval() time.Duration {
	return time.Duration(r.interval.Load())
}

// adaptInterval moves the round interval after a round: halved, down to the
// minimum, when a digest differed; doubled, up to the maximum, when every
// digest matched.
func (r *Reconciler) adaptInterval(differs bool) {
	cur := r.currentInterval()
	if differs {
		cur = max(r.minInterval, cur/2)
	} else {
		cur = min(r.maxInterval, cur*2)
	}
	r.interval.Store(int64(cur))
}

// restartTakeoverLocked starts the wait of st for its initiator at now, with
// a new jitter. r.mu must be held.
func (r *Reconciler) restartTakeoverLocked(st *peerState, now time.Time) {
	st.takeoverSince, st.takeoverJitter = now, r.jitter()
}

// takeoverDueLocked reports whether this node takes over the session with
// the peer of st, which initiates, at now. The wait starts at the first round
// that sees the mismatch, and again after every session the peer ran with
// this node (markServedLocked). It is takeoverRounds round intervals plus the
// peer's jitter (a fraction of one). With a peer of the same root (sameRoot)
// both sides see the same digests and adapt alike, so the interval is the
// current one and the wait shrinks with it while the mismatch persists. With
// a peer of another root it is the maximum: digests do not show whether the
// two are in sync, so the initiator's interval may have grown to the
// maximum while this node's stays short for its other peers. r.mu must be
// held.
func (r *Reconciler) takeoverDueLocked(st *peerState, now time.Time, sameRoot bool) bool {
	if st.takeoverSince.IsZero() {
		r.restartTakeoverLocked(st, now)
		return false
	}
	ri := r.currentInterval()
	if !sameRoot {
		ri = r.maxInterval
	}
	wait := takeoverRounds*ri + time.Duration(st.takeoverJitter*float64(ri))
	return !now.Before(st.takeoverSince.Add(wait))
}

// acquireSlot takes one of the MaxInitiatedSessions slots for a session
// this node opens, waiting for one until ctx ends. release frees it.
func (r *Reconciler) acquireSlot(ctx context.Context) (release func(), err error) {
	select {
	case r.slots <- struct{}{}:
		return func() { <-r.slots }, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

// acquireStripeSlot takes one of the MaxStripeSessions stripe slots,
// waiting for one until ctx ends. release frees it.
func (r *Reconciler) acquireStripeSlot(ctx context.Context) (release func(), err error) {
	select {
	case r.stripeSlots <- struct{}{}:
		return func() { <-r.stripeSlots }, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

// wakeRound triggers a round, deferred to MinRoundInterval after the last.
func (r *Reconciler) wakeRound() {
	select {
	case r.roundWake <- struct{}{}:
	default:
	}
}

// loop runs a round at start, on the round timer, on every topology change
// and on a trigger (wakeRound, a changed peer Status on statuses), until
// Close. A trigger waits until the minimum interval has passed since the last
// round. Triggers follow Options.Now and Options.After, and so does the round
// timer unless Options.RoundInterval fixes the interval (roundAfter).
func (r *Reconciler) loop(changes, statuses <-chan struct{}) {
	last := r.now()
	r.round()
	timer := r.roundAfter(r.currentInterval())
	var deferred <-chan time.Time // a trigger waiting for the minimum interval
	trigger := func() {
		if deferred == nil {
			deferred = r.after(r.minInterval - r.now().Sub(last))
		}
	}
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-timer:
		case <-changes:
		case <-deferred:
		case <-r.roundWake:
			trigger()
			continue
		case _, ok := <-statuses:
			if !ok {
				statuses = nil // unsubscribed elsewhere
				continue
			}
			r.wakeGossip()
			trigger()
			continue
		}
		last, deferred = r.now(), nil
		r.round()
		timer = r.roundAfter(r.currentInterval())
	}
}

// round is one round of the scheduler. It follows the topology into the
// per-peer machines (trackPeersLocked), sends the Statuses that are due
// (first contacts, a moved root; pickDueLocked), admits the peers whose
// Statuses are now exchanged, registers the striped transfer that a
// bootstrap or a radius decrease calls for (planCatchUp) and starts the
// sessions (scheduleLocked).
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
	r.trackPeersLocked(connected)
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
	// stripe is planned, so that the stripe sessions find them Idle.
	r.mu.Lock()
	r.admitLocked(local, connected)
	r.mu.Unlock()

	bootstrap := r.planCatchUp(local, radiusPending)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.scheduleLocked(local, connected, bootstrap)
}

// scheduleLocked starts a session with every connected peer that is
// eligible, has a differing digest, is Idle past its backoff, has no stripe
// session waiting for it, holds this node's current root, and that this node
// initiates with or takes over, unless a bootstrap transfer is in flight
// (bootstrap). local is the round's Status. It then arms the start-up guard
// when every neighbor has exchanged Statuses and no difference is left
// unresolved, and adapts the round interval. r.mu must be held.
func (r *Reconciler) scheduleLocked(local rangesync.Status, connected []swarm.Address, bootstrap bool) {
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
		// Digests of different roots summarize different ranges: such a
		// peer's sessions report whether they found differences (foundDiff).
		if local.Root.Equal(remote.Root) {
			differs = true
		}
		if !r.initiates(p) {
			if !st.served {
				r.awaiting[p.ByteString()] = true
			}
			if !r.takeoverDueLocked(st, now, local.Root.Equal(remote.Root)) {
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
		gen := st.gen
		go func() {
			defer r.wg.Done()
			r.session(p, st, gen, opens[p.ByteString()], local.Root, now)
		}()
	}
	r.guardArmed = exchanged && eligible > 0 && !unresolved
	r.checkGuardLocked()
	r.adaptInterval(differs || r.foundDiff)
	r.foundDiff = false
}

// sameDigest reports whether the two statuses summarize the same elements.
// Statuses with different roots are compared as different: their summaries
// cover different ranges (comparing summaries of the session root is a
// follow-up, see the phase-4 plan).
func sameDigest(local, remote rangesync.Status) bool {
	return local.Root.Equal(remote.Root) && local.Summary == remote.Summary
}

// initiates reports whether this node starts sessions with peer: the peer
// with the lower overlay initiates (§8.2).
func (r *Reconciler) initiates(peer swarm.Address) bool {
	return r.overlay.Compare(peer) < 0
}

// session runs Reconcile then Fetch with peer, whose entry st is in
// Reconciling with machine generation gen, then transfers the ranges the
// session handed to its Striper (d̂ > LargeDiff) as stripes. It waits for a
// session slot first (MaxInitiatedSessions), then rechecks the round's
// decision, made at decided with this node's root root (recheckSession).
// Once the peer has another entry or machine (currentLocked) neither the
// session nor its fetch moves the peer's machine. open, if not empty,
// is the session's opening (the old half after a radius decrease). The
// outcome moves st on: the peer's Busy or an error back to Idle with a
// backoff, a cause on this node (localCause) or a stale decision back to Idle
// without one, a completed session to Fetching and, once its wants are
// fetched, to Idle. A session that finds the peer's session slot taken on
// this node found the peer running its own session with this node: it goes
// back to Idle without a backoff. A takeover (the peer initiates) that finds
// so, or is answered Busy, found the initiator alive: it also waits for the
// initiator again.
func (r *Reconciler) session(peer swarm.Address, st *peerState, gen uint64, open []fingerprint.Range, root fingerprint.Range, decided time.Time) {
	striper := new(StripeCollector)
	release, err := r.acquireSlot(r.ctx)
	var wants []storer.ChunkID
	if err == nil {
		if err = r.recheckSession(peer, st, gen, root, decided); err == nil {
			wants, err = r.svc.ReconcileWith(r.ctx, peer, rangesync.ReconcileOptions{Open: open, Striper: striper})
		}
		release()
	}
	takeover := !r.initiates(peer)

	r.mu.Lock()
	r.pendingReconcile--
	catchUp := err == nil && len(wants) >= CatchUpMinDiff
	if catchUp {
		r.catchUpFetches++
	}
	if err == nil && (len(wants) > 0 || len(striper.Ranges()) > 0) {
		r.foundDiff = true
	}
	if r.currentLocked(peer, st, gen) { // neither left nor reconnected meanwhile
		switch {
		case err == nil:
			r.fireLocked(st, eventSessionDone)
		case errors.Is(err, rangesync.ErrPeerSessionLimit), takeover && errors.Is(err, rangesync.ErrPeerBusy):
			r.fireLocked(st, eventAborted)
			if takeover {
				r.restartTakeoverLocked(st, r.now())
			}
		case errors.Is(err, errStaleSession), localCause(err):
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
	case errors.Is(err, errStaleSession):
		r.metrics.Sessions.WithLabelValues("stale").Inc()
		return // the next round decides again
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
	if r.currentLocked(peer, st, gen) {
		r.fireLocked(st, eventFetched) // during a stripe session the peer stays Striped
	}
	r.mu.Unlock()
	if ranges := striper.Ranges(); len(ranges) > 0 {
		r.enqueueStripes(ranges)
	}
}

// errStaleSession ends a session whose round decision no longer holds when it
// gets its slot (recheckSession).
var errStaleSession = errors.New("reconciler: session decision stale")

// recheckSession reports errStaleSession unless the decision of the round that
// started the session with peer, made at decided with this node's root root,
// still holds once the session has its slot: st is the peer's current entry,
// with the machine of generation gen, in Reconciling, this node's root is
// still root and reached the peer,
// the peer's Status is known and its digest still differs, and, for a
// takeover, the initiator ran no session with this node since. The session
// may have waited minutes for its slot.
func (r *Reconciler) recheckSession(peer swarm.Address, st *peerState, gen uint64, root fingerprint.Range, decided time.Time) error {
	local, err := r.svc.LocalStatus()
	if err != nil {
		return err
	}
	remote, have := r.svc.LastStatus(peer)
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case !r.currentLocked(peer, st, gen), st.fsm.state != stateReconciling:
	case !have, !local.Root.Equal(root), !st.reached.Equal(local.Root):
	case sameDigest(local, remote):
	case !r.initiates(peer) && st.takeoverSince.After(decided):
	default:
		return nil
	}
	return errStaleSession
}
