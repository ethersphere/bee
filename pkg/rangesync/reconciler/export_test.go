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
