// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"slices"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	dto "github.com/prometheus/client_model/go"
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
)

// EnqueueStripes registers ranges for the stripe worker.
func (r *Reconciler) EnqueueStripes(ranges []fingerprint.Range) bool {
	return r.enqueueStripes(ranges)
}

// HoldStripe registers rg as in flight without queuing it for the worker.
func (r *Reconciler) HoldStripe(rg fingerprint.Range) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stripeRegistry = append(r.stripeRegistry, rg)
}

// ReleaseStripe removes rg from the registry (a HoldStripe ended).
func (r *Reconciler) ReleaseStripe(rg fingerprint.Range) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stripeRegistry = slices.DeleteFunc(r.stripeRegistry, func(o fingerprint.Range) bool { return o.Equal(rg) })
}

// VerifyMinDiff is verifyMinDiff.
const VerifyMinDiff = verifyMinDiff

// VerificationDiffCount returns the number of verification_diff
// observations.
func (r *Reconciler) VerificationDiffCount() uint64 {
	m := new(dto.Metric)
	if err := r.metrics.VerificationDiff.Write(m); err != nil {
		panic(err)
	}
	return m.GetHistogram().GetSampleCount()
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

// PeerFSMIn returns a machine in state s.
func PeerFSMIn(s PeerState) *PeerFSM {
	return &PeerFSM{state: s}
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
	return ok && st.fetching
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
