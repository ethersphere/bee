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
//   - Triggers. A round runs on the round timer and on a trigger (a topology
//     change, a changed Status of a peer, a root change of this node, a moved
//     root delivered to a peer), which is deferred to the minimum round
//     interval after the last round, so that rounds never run sessions with
//     a peer faster than its session rate (rangesync.PeerSessionInterval)
//     and a peer's first Busy is retried no sooner than that. A topology
//     change also follows the topology at once (follow) and sends the
//     Statuses then due off the round loop, so that a new peer gets its
//     first Status without waiting and a peer that does not answer holds up
//     no round; its first session waits for the round. A round that is due
//     runs before the next trigger is handled.
//   - Cap. The sessions this node opens, round sessions, takeovers, stripe
//     transfers and verifications alike, take one of the per-node cap's
//     slots first and wait for one, first come, first served. Each round
//     sets the cap from its eligible peers (InitiatedSessionCap): room for
//     every higher-overlay peer's session and the stripe sessions within the
//     tokens the lower-overlay peers' sessions leave, one slot less per peer
//     past the node's session tokens, and at least MinInitiatedSessions.
//     Stripe transfers and verifications first take one of
//     MaxStripeSessions stripe slots, so that round sessions always have the
//     other slots, and free both while they wait to retry a Busy answer.
const (
	// MinRoundInterval is the shortest round interval, used while rounds
	// keep finding differences.
	MinRoundInterval = rangesync.MinRoundInterval
	// MaxRoundInterval is the longest round interval, used while every
	// digest matches.
	MaxRoundInterval = rangesync.MaxRoundInterval

	// MinInitiatedSessions is the least the per-node cap on the sessions
	// this node opens at once allows (InitiatedSessionCap), and the cap
	// until the first round has counted the peers: half of
	// rangesync.MaxConcurrentSessions (the constant cap before G6), twice
	// MaxStripeSessions.
	MinInitiatedSessions = rangesync.MaxConcurrentSessions / 2
	// MaxStripeSessions bounds the stripe transfers and verifications that
	// run at once. They hold their slot for a whole stripe session, which
	// the fetch queue backpressures (up to rangesync's session timeout), so
	// they take at most half of MinInitiatedSessions and round sessions keep
	// the rest. It is a constant of its own: it does not grow with the
	// per-node cap.
	MaxStripeSessions = 4

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

// InitiatedSessionCap returns the per-node cap on the sessions a node opens
// at once (round sessions, takeovers, stripe transfers and verifications
// together), from its eligible peers at a round: higher with a higher
// overlay, which it initiates with, and lower with a lower overlay, which
// initiate with it (§8.2). The node's rangesync.MaxConcurrentSessions tokens
// are shared by the sessions it opens and those it serves, and a peer runs
// one session with it at a time, either way.
//
// While its sessions fit its tokens (higher + lower ≤ MaxConcurrentSessions,
// a neighborhood of up to 17 nodes), the cap has room for a session with
// every higher-overlay peer and for MaxStripeSessions stripe sessions, within
// the tokens the lower-overlay peers' sessions leave, and at least
// MinInitiatedSessions: a round reaches every higher-overlay peer in one
// wave, and no session finds the node's tokens taken. With more peers the
// neighborhood's sessions do not fit the tokens of its higher-overlay nodes,
// which answer the excess Busy, and more sessions at once from the lower
// overlays only raise those answers (measured at N = 25 and 33, T4.7 part 2).
// There each peer past the tokens (excess) takes one more slot off the room,
// so that the cap slides down to MinInitiatedSessions, which leaves the other
// half of the tokens to the sessions the node serves, and is that at every
// rank from N = 25 on. One peer more or fewer moves the cap by at most 2,
// with no step at N = 17/18 (T4.7 part 2 fix round 1, measured at N = 18
// and 21: the Busy answers stay below N = 25's).
//
//	excess = max(0, higher + lower − MaxConcurrentSessions)
//	cap    = max(MinInitiatedSessions, min(higher + MaxStripeSessions, MaxConcurrentSessions − lower − excess))
func InitiatedSessionCap(higher, lower int) int {
	excess := max(0, higher+lower-rangesync.MaxConcurrentSessions)
	return max(MinInitiatedSessions, min(higher+MaxStripeSessions, rangesync.MaxConcurrentSessions-lower-excess))
}

// acquireSlot takes one of the slots of the per-node cap for a session this
// node opens, waiting for one until ctx ends. release frees it.
func (r *Reconciler) acquireSlot(ctx context.Context) (release func(), err error) {
	return r.slots.acquire(ctx)
}

// resizeSlotsLocked sets the per-node cap from the round's eligible peers
// with a higher overlay (higher) and a lower one (lower), unless
// Options.InitiatedSessions fixed it. r.mu must be held.
func (r *Reconciler) resizeSlotsLocked(higher, lower int) {
	if r.fixedSlots > 0 {
		return
	}
	r.slots.resize(InitiatedSessionCap(higher, lower))
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

// loop runs a round at start, on the round timer and on a trigger (a
// topology change on changes, wakeRound, a changed peer Status on statuses),
// until Close. A trigger waits until the minimum interval has passed since
// the last round. A due round runs before the next trigger is handled, so
// that a stream of triggers does not delay it. A topology change first
// follows the topology at once and sends the new peers' first contacts off
// the loop (followTopology). Triggers follow Options.Now and Options.After,
// and so does the round timer unless Options.RoundInterval fixes the
// interval (roundAfter).
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
		case <-timer: // due
		case <-deferred: // due
		default:
			select {
			case <-r.ctx.Done():
				return
			case <-timer:
			case <-deferred:
			case <-changes:
				r.followTopology()
				trigger()
				continue
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
		}
		last, deferred = r.now(), nil
		r.round()
		timer = r.roundAfter(r.currentInterval())
	}
}

// followTopology is what a topology change runs at once: it follows the
// root and the topology into the per-peer machines and picks the Statuses
// that are due (follow), new peers' first contacts included, and sends them
// in a goroutine of its own, so that a peer that does not answer (up to the
// send's timeout) holds up no round. A send that delivers a root the peer
// lacked triggers a round, as a gossip pass does: a round that ran while the
// send was in flight skipped the peer's session.
func (r *Reconciler) followTopology() {
	local, _, _, due, ok := r.follow()
	if !ok {
		return
	}
	r.wakeGossip()
	if len(due) == 0 {
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if r.probe(due, local) {
			r.wakeRound()
		}
	}()
}

// round is one round of the scheduler. It follows the topology into the
// per-peer machines and sends the Statuses that are due (track), admits the
// peers whose Statuses are now exchanged, registers the striped transfer that
// a bootstrap or a radius decrease calls for (planCatchUp) and starts the
// sessions (scheduleLocked).
func (r *Reconciler) round() {
	local, connected, radiusPending, ok := r.track()
	if !ok || r.ctx.Err() != nil {
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

// track is the first step of a round: it follows the root and the topology
// and picks the Statuses that are due (follow), then sends them (probe).
// First contacts, and a new root (it makes the peers' session scopes with
// this node stale), go out now unless the sender-side rate defers them, so
// that the round's evaluation sees the outcome; the gossip loop sends the
// rest when due. It returns follow's results.
func (r *Reconciler) track() (local rangesync.Status, connected []swarm.Address, radiusPending, ok bool) {
	local, connected, radiusPending, due, ok := r.follow()
	if !ok {
		return local, nil, false, false
	}
	r.probe(due, local)
	r.wakeGossip()
	return local, connected, radiusPending, true
}

// follow follows this node's root (trackRootLocked) and the topology's peers
// (trackPeersLocked) into the per-peer machines and picks the Statuses that
// are due (pickDueLocked), in the critical section that adds new peers, so
// that their first contacts are its caller's. It returns the Status the
// picks were taken with, the connected peers, whether a radius decrease's new
// half is pending and the picks; ok is false without a Status (the index is
// not built yet).
func (r *Reconciler) follow() (local rangesync.Status, connected []swarm.Address, radiusPending bool, due []statusPick, ok bool) {
	_ = r.topo.EachConnectedPeer(func(addr swarm.Address, _ uint8) (bool, bool, error) {
		connected = append(connected, addr)
		return false, false, nil
	}, topology.Select{})

	r.pickMu.Lock()
	local, err := r.svc.LocalStatus()
	if err != nil {
		r.pickMu.Unlock()
		r.logger.Debug("no local status", "error", err)
		return local, nil, false, nil, false
	}
	r.mu.Lock()
	r.trackRootLocked(local.Root)
	radiusPending = r.radius.pending
	r.trackPeersLocked(connected)
	due = r.pickDueLocked(local, r.now())
	r.mu.Unlock()
	r.pickMu.Unlock()
	return local, connected, radiusPending, due, true
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
	higher, lower := 0, 0 // the eligible peers by overlay (InitiatedSessionCap)
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
		if r.initiates(p) {
			higher++
		} else {
			lower++
		}
		known, equal := r.compareDigests(local, remote)
		if known && equal {
			st.takeoverSince = time.Time{}
			r.fireLocked(st, eventEqualDigest)
			continue
		}
		// Digests that are not comparable over the session root summarize
		// different ranges: such a peer's sessions report whether they found
		// differences (foundDiff).
		if known {
			differs = true
		}
		if !r.initiates(p) {
			if !st.served {
				r.awaiting[p.ByteString()] = true
			}
			// While the start-up guard holds, a peer that has not run a
			// session with this node and whose digest is not comparable is
			// taken over at once: its initiator side may compare the digests
			// over this node's root, find them equal and never start, and
			// the guard waits for the peer (awaiting).
			urgent := !known && !st.served && r.guardHoldsLocked(now)
			if !urgent && !r.takeoverDueLocked(st, now, local.Root.Equal(remote.Root)) {
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
		// Far behind the peer in the session root, this node would open
		// at a count far below the peer's, and the peer would list to it
		// what it holds: the session root is striped instead, as the
		// peer's claim (constraint (d) of AC-B2), and the sessions with
		// this peer wait while the check holds. The opening of a radius
		// decrease is the old half, which this node holds.
		if len(open) == 0 {
			if sr, lacks := r.lacksSessionRootLocked(local, remote); lacks {
				r.registerStripeLocked(&stripeJob{rg: sr, peer: p})
				differs, unresolved = true, true
				continue
			}
		}
		candidates = append(candidates, p)
		opens[p.ByteString()] = open
	}
	r.resizeSlotsLocked(higher, lower)
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

// rangeSummarizer is implemented by a Service that summarizes a range of this
// node's index (*rangesync.Service). With it, the round compares a peer's
// digest over the session root when the peer's root lies inside this node's
// (compareDigests).
type rangeSummarizer interface {
	RangeSummary(r fingerprint.Range) (fingerprint.Summary, error)
}

var _ rangeSummarizer = (*rangesync.Service)(nil)

// compareDigests compares the digests of this node's Status local and a
// peer's Status remote over their session root (rangesync.SessionRoot), and
// reports whether they are comparable (known) and, if so, equal. A Status
// summarizes its sender's root. With the same root the two summaries are
// compared. When the peer's root lies inside this node's, the session root
// is the peer's root, which the peer's summary covers: with a
// rangeSummarizer, this node's summary of that range is compared with it.
// Otherwise the digests are not comparable: this node's root lies inside the
// peer's (the peer's summary covers more than the session root), or the
// index cannot summarize the range (it is rebuilding, its root moved, or
// the range lies below its leaves). It may be called with r.mu held:
// RangeSummary reads the index under the index's own locks, briefly, and
// calls nothing back.
func (r *Reconciler) compareDigests(local, remote rangesync.Status) (known, equal bool) {
	if local.Root.Equal(remote.Root) {
		return true, local.Summary == remote.Summary
	}
	rs, ok := r.svc.(rangeSummarizer)
	if !ok || !local.Root.ContainsRange(remote.Root) {
		return false, false
	}
	sum, err := rs.RangeSummary(remote.Root)
	if err != nil {
		return false, false
	}
	return true, sum == remote.Summary
}

// lacksSessionRootLocked is LacksSessionRoot for this node's Status local and
// a peer's Status remote, with this node's summary of the session root from
// a rangeSummarizer, and the threshold Options.LargeDiff, or 0 while this
// node's own transfers cover the session root (transferringLocked): the
// transfer lists what this node lacks there. A claim registered on a peer's
// count does not lower it (T4.7 part 1 fix round 1). r.mu must be held;
// RangeSummary is called with it, as in compareDigests.
func (r *Reconciler) lacksSessionRootLocked(local, remote rangesync.Status) (fingerprint.Range, bool) {
	summary := func(rg fingerprint.Range) (uint64, bool) {
		rs, ok := r.svc.(rangeSummarizer)
		if !ok {
			return 0, false
		}
		sum, err := rs.RangeSummary(rg)
		return sum.Count, err == nil
	}
	threshold := r.largeDiff
	if sr, ok := rangesync.SessionRoot(local.Root, remote.Root); ok && r.transferringLocked(sr) {
		threshold = 0
	}
	return LacksSessionRoot(local, remote, summary, threshold)
}

// sameDigest reports whether the two statuses summarize the same elements
// over their session root (compareDigests). Digests that are not comparable
// count as different: a session finds out.
func (r *Reconciler) sameDigest(local, remote rangesync.Status) bool {
	known, equal := r.compareDigests(local, remote)
	return known && equal
}

// initiates reports whether this node starts sessions with peer: the peer
// with the lower overlay initiates (§8.2).
func (r *Reconciler) initiates(peer swarm.Address) bool {
	return r.overlay.Compare(peer) < 0
}

// session runs Reconcile then Fetch with peer, whose entry st is in
// Reconciling with machine generation gen, then transfers the ranges the
// session handed to its Striper (ranges this node lacks by more than
// LargeDiff, or at all inside this node's own running transfer) as stripes,
// at most maxSessionStripes of them, as the peer's claims. It waits for a
// session slot first (InitiatedSessionCap), then rechecks the round's
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
	striper := &StripeCollector{transferring: r.Transferring}
	release, err := r.acquireSlot(r.ctx)
	var wants []storer.ChunkID
	if err == nil {
		if err = r.recheckSession(peer, st, gen, open, root, decided); err == nil {
			wants, err = r.svc.ReconcileWith(r.ctx, peer, rangesync.ReconcileOptions{Open: open, Striper: striper})
		}
		release()
	}
	takeover := !r.initiates(peer)

	r.mu.Lock()
	r.pendingReconcile--
	catchUp := err == nil && len(wants) >= CatchUpMinDiff
	if catchUp {
		r.addCatchUpLocked(1)
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
		r.enqueueStripes(peer, ranges)
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
// the peer's Status is known and its digest still differs, for a session
// over the session root (open empty) this node is not far behind the peer
// there (lacksSessionRootLocked, with the transfers registered since), and,
// for a takeover, the initiator ran no session with this node since. The
// session may have waited minutes for its slot. A session the session-root
// check makes stale leaves the registering to the next round.
func (r *Reconciler) recheckSession(peer swarm.Address, st *peerState, gen uint64, open []fingerprint.Range, root fingerprint.Range, decided time.Time) error {
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
	case r.sameDigest(local, remote):
	case len(open) == 0 && r.lacksLocked(local, remote):
	case !r.initiates(peer) && st.takeoverSince.After(decided):
	default:
		return nil
	}
	return errStaleSession
}

// lacksLocked reports whether lacksSessionRootLocked holds. r.mu must be
// held.
func (r *Reconciler) lacksLocked(local, remote rangesync.Status) bool {
	_, lacks := r.lacksSessionRootLocked(local, remote)
	return lacks
}
