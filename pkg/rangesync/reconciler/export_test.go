// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
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
