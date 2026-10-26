// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"context"
	"errors"
	"slices"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// GuardReleased reports whether the start-up guard of SyncRate is released.
func (r *Reconciler) GuardReleased() bool {
	return r.guardReleased.Load()
}

// KnownPeers returns the number of peers the reconciler keeps state for.
func (r *Reconciler) KnownPeers() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.peers)
}

// OrderCandidates orders peers as a round runs their sessions.
func (r *Reconciler) OrderCandidates(peers []swarm.Address) []swarm.Address {
	return r.orderCandidates(peers)
}

// StripesTotal returns the stripes_total counter of result.
func (r *Reconciler) StripesTotal(result string) float64 {
	m := new(dto.Metric)
	if err := r.metrics.Stripes.WithLabelValues(result).Write(m); err != nil {
		panic(err)
	}
	return m.GetCounter().GetValue()
}

// MaxStripeReassign is maxStripeReassign.
const (
	MaxStripeReassign = maxStripeReassign
	MaxStripeRestarts = maxStripeRestarts
	MaxStripeRegistry = maxStripeRegistry
	MaxClaimOvertakes = maxClaimOvertakes
)

// EnqueueStripes registers ranges for the stripe worker, as claims of no
// peer: neither maxSessionStripes nor maxPeerStripes bounds them.
func (r *Reconciler) EnqueueStripes(ranges []fingerprint.Range) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := true
	for _, rg := range ranges {
		if !r.registerStripeLocked(&stripeJob{rg: rg}) {
			all = false
		}
	}
	return all
}

// EnqueuePeerStripes registers the ranges one session with peer took over
// (enqueueStripes).
func (r *Reconciler) EnqueuePeerStripes(peer swarm.Address, ranges []fingerprint.Range) bool {
	return r.enqueueStripes(peer, ranges)
}

// HoldStripe registers rg as an own job (a bootstrap or radius half) in
// flight without queuing it for the worker.
func (r *Reconciler) HoldStripe(rg fingerprint.Range) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stripeRegistry = append(r.stripeRegistry, &stripeJob{rg: rg, own: true})
	r.ownJobsChangedLocked()
}

// HoldClaims registers ranges as claims of no peer without queuing them for
// the worker, as claims it has not reached yet.
func (r *Reconciler) HoldClaims(ranges []fingerprint.Range) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rg := range ranges {
		r.stripeRegistry = append(r.stripeRegistry, &stripeJob{rg: rg})
	}
}

// HoldPeerStripe registers rg as peer's claim in flight without queuing it
// for the worker, as the worker runs a claim (its cancel, its context and an
// idle clock that is never started), and returns the context an own job's
// preemption, or EndIdleClaim, cancels.
func (r *Reconciler) HoldPeerStripe(rg fingerprint.Range, peer swarm.Address) context.Context {
	ctx, cancel := context.WithCancelCause(context.Background())
	job := &stripeJob{rg: rg, peer: peer, cancel: func() { cancel(nil) }, run: ctx}
	job.idle = newStripeClock(r.now, r.after, claimIdleTimeout, claimIdleTimeout, func() { r.endIdleClaim(job, cancel) })
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stripeRegistry = append(r.stripeRegistry, job)
	r.stripeRunning = job
	return ctx
}

// EndIdleClaim ends the claim in flight that HoldPeerStripe registered as its
// idle clock does when it expires (endIdleClaim); the claim stays in the
// registry and in flight, as until the worker sees its transfer end.
func (r *Reconciler) EndIdleClaim() {
	r.mu.Lock()
	job := r.stripeRunning
	r.mu.Unlock()
	job.idle.expire()
}

// IsClaimIdleCause reports whether err is the cause of a claim ended for
// storing nothing (endIdleClaim).
func IsClaimIdleCause(err error) bool { return errors.Is(err, errClaimIdle) }

// ReleaseStripe removes rg from the registry (a HoldStripe ended).
func (r *Reconciler) ReleaseStripe(rg fingerprint.Range) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stripeRegistry = slices.DeleteFunc(r.stripeRegistry, func(o *stripeJob) bool { return o.rg.Equal(rg) })
	if r.stripeRunning != nil && r.stripeRunning.rg.Equal(rg) {
		r.stripeRunning = nil
	}
	r.ownJobsChangedLocked()
}

// PeerStripes returns the number of peer's claims in the registry.
func (r *Reconciler) PeerStripes(peer swarm.Address) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.peerStripesLocked(peer)
}

// QueuedStripes returns the ranges queued for the stripe worker, in order.
func (r *Reconciler) QueuedStripes() []fingerprint.Range {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]fingerprint.Range, len(r.stripeQueue))
	for i, j := range r.stripeQueue {
		out[i] = j.rg
	}
	return out
}

// MaxPeerStripes is maxPeerStripes; MaxSessionStripes is maxSessionStripes;
// StripePeerBudget is stripePeerBudget.
const (
	MaxPeerStripes    = maxPeerStripes
	MaxSessionStripes = maxSessionStripes
	StripePeerBudget  = stripePeerBudget
)

// VerifyMinDiff is verifyMinDiff; VerifyStampSlack is verifyStampSlack.
const (
	VerifyMinDiff    = verifyMinDiff
	VerifyStampSlack = verifyStampSlack
)

// VerificationDiffCount returns the number of verification_diff
// observations.
func (r *Reconciler) VerificationDiffCount() uint64 {
	m := new(dto.Metric)
	if err := r.metrics.VerificationDiff.Write(m); err != nil {
		panic(err)
	}
	return m.GetHistogram().GetSampleCount()
}

// CatchUpFetches returns the catch-up fetches in flight.
func (r *Reconciler) CatchUpFetches() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.catchUpFetches
}

// MaxServedPending is maxServedPending; CatchUpGrace is catchUpGrace;
// OwnJobGrace is ownJobGrace; ClaimIdleTimeout is claimIdleTimeout.
const (
	MaxServedPending = maxServedPending
	CatchUpGrace     = catchUpGrace
	OwnJobGrace      = ownJobGrace
	ClaimIdleTimeout = claimIdleTimeout
)

// RunningStripe returns the range of the job in flight in the stripe worker,
// and whether it is this node's own (stripeJob.own); ok is false when none
// is.
func (r *Reconciler) RunningStripe() (rg fingerprint.Range, own, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stripeRunning == nil {
		return rg, false, false
	}
	return r.stripeRunning.rg, r.stripeRunning.own, true
}

// FailBootstrap ends the bootstrap transfer, queued or in flight, as the
// stripe worker does after one that did not complete.
func (r *Reconciler) FailBootstrap() {
	r.mu.Lock()
	defer r.mu.Unlock()
	drop := func(j *stripeJob) bool { return j.bootstrap }
	r.stripeRegistry = slices.DeleteFunc(r.stripeRegistry, drop)
	r.stripeQueue = slices.DeleteFunc(r.stripeQueue, drop)
	r.bootstrapping, r.bootstrapFailed = false, true
	r.ownJobsChangedLocked()
}

// Bootstrapping reports whether a bootstrap transfer is queued or in flight.
func (r *Reconciler) Bootstrapping() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bootstrapping
}

// StripeRanges returns the registered stripe ranges, in order.
func (r *Reconciler) StripeRanges() []fingerprint.Range {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stripeRangesLocked()
}

// StripeRegistry returns the number of registered stripe ranges.
func (r *Reconciler) StripeRegistry() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.stripeRegistry)
}

// RadiusStriping reports whether a new half is registered for the stripe
// worker.
func (r *Reconciler) RadiusStriping() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.radius.striping
}

// RadiusPending reports whether a radius decrease's new half is pending.
func (r *Reconciler) RadiusPending() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.radius.pending
}

// RadiusHold is radiusHold; MaxRadiusAttempts is maxRadiusAttempts.
const (
	RadiusHold        = radiusHold
	MaxRadiusAttempts = maxRadiusAttempts
)

// RadiusAttempts returns the number of ended transfers of the pending half.
func (r *Reconciler) RadiusAttempts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.radius.attempts
}

// Round runs one round.
func (r *Reconciler) Round() { r.round() }

// Gossip runs one pass of the Status gossip at Now and returns how long
// until a Status is next due.
func (r *Reconciler) Gossip() time.Duration { return r.gossip() }

// StatusSending reports whether a Status send to peer is in flight.
func (r *Reconciler) StatusSending(peer swarm.Address) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.peers[peer.ByteString()]
	return ok && st.sending
}

// The per-peer state machine (peerfsm.go).
type (
	PeerFSM   = peerFSM
	PeerState = fsmState
	PeerEvent = fsmEvent
)

const (
	StateProbing     = stateProbing
	StateUnsupported = stateUnsupported
	StateIdle        = stateIdle
	StateReconciling = stateReconciling
	StateFetching    = stateFetching
	StateStriped     = stateStriped
	StateRemoved     = stateRemoved

	EventUnsupported  = eventUnsupported
	EventExchanged    = eventExchanged
	EventEqualDigest  = eventEqualDigest
	EventReconcile    = eventReconcile
	EventSessionDone  = eventSessionDone
	EventLargeDiff    = eventLargeDiff
	EventStripe       = eventStripe
	EventStripeDone   = eventStripeDone
	EventFetched      = eventFetched
	EventBusy         = eventBusy
	EventError        = eventError
	EventAborted      = eventAborted
	EventDisjoint     = eventDisjoint
	EventDisconnected = eventDisconnected

	MaxSessionBackoff = maxSessionBackoff
)

// PeerStates and PeerEvents are every state and event of the machine.
var (
	PeerStates = []PeerState{stateProbing, stateUnsupported, stateIdle, stateReconciling, stateFetching, stateStriped, stateRemoved}
	PeerEvents = []PeerEvent{
		eventUnsupported, eventExchanged, eventEqualDigest, eventReconcile, eventSessionDone, eventLargeDiff, eventStripe,
		eventStripeDone, eventFetched, eventBusy, eventError, eventAborted, eventDisjoint, eventDisconnected,
	}
)

// NewPeerFSM returns the machine of a peer that connected.
func NewPeerFSM() *PeerFSM {
	m := newPeerFSM()
	return &m
}

// PeerFSMIn returns a machine in state s. In Fetching and Striped the wants
// of a round session are being fetched, as when the machine got there
// through Fetching.
func PeerFSMIn(s PeerState) *PeerFSM {
	return &PeerFSM{state: s, fetching: s == stateFetching || s == stateStriped}
}

// Fire applies e at now.
func (m *peerFSM) Fire(e PeerEvent, now time.Time) bool { return m.fire(e, now) }

// State returns the machine's state.
func (m *peerFSM) State() PeerState { return m.state }

// CanReconcile reports whether a session may start at now.
func (m *peerFSM) CanReconcile(now time.Time) bool { return m.canReconcile(now) }

// RetryAt returns when the next session may start.
func (m *peerFSM) RetryAt() time.Time { return m.retryAt }

// Transition is transition.
func Transition(s PeerState, e PeerEvent) (PeerState, bool) { return transition(s, e) }

// StatusRetry is statusRetry.
func StatusRetry(n int) time.Duration { return statusRetry(n) }

// PeerState returns the state of peer's machine, and false when r keeps no
// entry for peer.
func (r *Reconciler) PeerState(peer swarm.Address) (PeerState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.peers[peer.ByteString()]
	if !ok {
		return 0, false
	}
	return st.fsm.state, true
}

// KnowsPeer reports whether r keeps an entry for peer.
func (r *Reconciler) KnowsPeer(peer swarm.Address) bool {
	_, ok := r.PeerState(peer)
	return ok
}

// PeerStatuses returns the peers offered for stripes, with their Status.
func (r *Reconciler) PeerStatuses() []PeerStatus { return r.peerStatuses() }

// PeerFetching reports whether the wants of peer's last round session are
// being fetched.
func (r *Reconciler) PeerFetching(peer swarm.Address) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.peers[peer.ByteString()]
	return ok && st.fsm.fetching
}

// StripeWaiters returns the number of stripe sessions waiting for peer.
func (r *Reconciler) StripeWaiters(peer swarm.Address) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.peers[peer.ByteString()]; ok {
		return st.stripeWaiters
	}
	return 0
}

// CurrentRoundInterval returns the current round interval.
func (r *Reconciler) CurrentRoundInterval() time.Duration { return r.currentInterval() }

// RoundIntervalGauge returns the round_interval_seconds gauge.
func (r *Reconciler) RoundIntervalGauge() float64 {
	m := new(dto.Metric)
	if err := r.metrics.RoundInterval.Write(m); err != nil {
		panic(err)
	}
	return m.GetGauge().GetValue()
}

// InitiatedSessionCapGauge returns the initiated_session_cap gauge.
func (r *Reconciler) InitiatedSessionCapGauge() float64 {
	m := new(dto.Metric)
	if err := r.metrics.InitiatedSessionCap.Write(m); err != nil {
		panic(err)
	}
	return m.GetGauge().GetValue()
}

// PlanStripesFor returns the plan the stripe worker would transfer rg with.
func (r *Reconciler) PlanStripesFor(rg fingerprint.Range) []Stripe { return r.planStripes(rg) }

// NextStripePeerFallback returns the peer nextStripePeer falls back to for
// sr once a stripe's own peers are exhausted, with tried already tried.
func (r *Reconciler) NextStripePeerFallback(sr fingerprint.Range, tried ...swarm.Address) (swarm.Address, bool) {
	t := make(map[string]bool, len(tried))
	for _, p := range tried {
		t[p.ByteString()] = true
	}
	return r.nextStripePeer(sr, new([]swarm.Address), t)
}

// VerifierFor returns the peer VerifyStripes would verify res with.
func (r *Reconciler) VerifierFor(res StripeResult) (swarm.Address, bool) {
	all, ready := r.stripePeers()
	return r.verifier(res, all, ready)
}

// StripeSlotsTaken returns the number of stripe slots taken
// (MaxStripeSessions).
func (r *Reconciler) StripeSlotsTaken() int { return len(r.stripeSlots) }

// SessionSlots returns the per-node cap on the sessions this node opens at
// once (InitiatedSessionCap, set by the last round) and the slots taken.
func (r *Reconciler) SessionSlots() (size, used int) { return r.slots.state() }

// SlotPool is the semaphore of the per-node cap, for its tests.
type SlotPool struct{ p *slotPool }

func NewSlotPool(size int) SlotPool { return SlotPool{p: newSlotPool(size)} }

func (s SlotPool) Acquire(ctx context.Context) (release func(), err error) {
	return s.p.acquire(ctx)
}
func (s SlotPool) Resize(size int)         { s.p.resize(size) }
func (s SlotPool) State() (size, used int) { return s.p.state() }

// Waiting returns the number of callers waiting for a slot.
func (s SlotPool) Waiting() int {
	s.p.mu.Lock()
	defer s.p.mu.Unlock()
	return s.p.waiters.Len()
}

// MaxStripeBusyRetries is maxStripeBusyRetries.
const MaxStripeBusyRetries = maxStripeBusyRetries

// SetStripeRetryInterval sets the wait before a Busy stripe session is
// retried. It must be called before the reconciler runs stripes.
func (r *Reconciler) SetStripeRetryInterval(d time.Duration) { r.stripeRetry = d }

// SessionsTotal returns the sessions_total counter of result.
func (r *Reconciler) SessionsTotal(result string) float64 {
	m := new(dto.Metric)
	if err := r.metrics.Sessions.WithLabelValues(result).Write(m); err != nil {
		panic(err)
	}
	return m.GetCounter().GetValue()
}

// PeerFailures returns the Busy or failed sessions in a row of peer (its
// backoff streak).
func (r *Reconciler) PeerFailures(peer swarm.Address) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.peers[peer.ByteString()]; ok {
		return st.fsm.failures
	}
	return 0
}

// SourceIDs returns the number of IDs the id → peers map keeps at Now.
func (r *Reconciler) SourceIDs() int {
	w := r.sources
	w.mu.Lock()
	defer w.mu.Unlock()
	w.prune(r.now())
	return len(w.byID)
}

// MaxPendingPasses is maxPendingPasses.
const MaxPendingPasses = maxPendingPasses
