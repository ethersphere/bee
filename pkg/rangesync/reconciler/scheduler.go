// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"context"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
)

// The round scheduler (§8.2).
//
//   - Initiator. Of two peers whose digests differ, the one with the lower
//     overlay starts the session. The other side takes over when the
//     mismatch persists for takeoverRounds round intervals plus a jitter of
//     up to one round interval with no session of the initiator with this
//     node (takeoverDueLocked); a session the initiator runs with this node,
//     or a takeover that finds the initiator's session holding the peer's
//     session slot, restarts the wait.
//   - Rounds. A round starts a session with every eligible peer whose digest
//     differs and that this node initiates with (or takes over), at most one
//     per peer (the peer's state machine). The round decides on one snapshot
//     of this node's Status (LocalStatus); each session reads the index
//     itself (see the T4.3 decisions log for why a round-wide index snapshot
//     does not exist).
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
//   - Cap. The sessions this node opens, round sessions, takeovers, stripe
//     transfers and verifications alike, take one of MaxInitiatedSessions
//     slots first and wait for one, first come, first served. Stripe
//     transfers and verifications first take one of MaxStripeSessions
//     stripe slots, so that round sessions always have the other slots.
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
// this node (markServedLocked). It is takeoverRounds current round intervals
// plus the peer's jitter (a fraction of one), so it shrinks with the
// interval while the mismatch persists. r.mu must be held.
func (r *Reconciler) takeoverDueLocked(st *peerState, now time.Time) bool {
	if st.takeoverSince.IsZero() {
		r.restartTakeoverLocked(st, now)
		return false
	}
	ri := r.currentInterval()
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
