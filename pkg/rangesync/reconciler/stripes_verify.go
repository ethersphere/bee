// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// verifyMinDiff is the largest verification difference that never
	// counts against the deliverer, whatever the stripe's size: chunks
	// uploaded while the stripe was transferred.
	verifyMinDiff = 4
	// verifyDiffRatio sets the tolerance of larger stripes: a difference
	// above 1/verifyDiffRatio of the chunks the deliverer itself stored
	// (StripeResult.PeerStored, and above verifyMinDiff) is reported as a
	// failure of the deliverer. The difference counted is what the
	// verifier delivered (stored, so validated), never what it only listed.
	verifyDiffRatio = 100
)

var (
	// ErrNoVerifier is the error of a stripe that has no peer other than
	// its deliverer to verify it with; it is not verified.
	ErrNoVerifier = errors.New("reconciler: no peer to verify stripe")
	// ErrStripeNotDone is the error of a stripe whose transfer did not
	// complete; it is not verified.
	ErrStripeNotDone = errors.New("reconciler: stripe not done")
	// ErrVerifierWithheld is the reason reported to the PeerScorer for a
	// verifier that listed IDs its verification session did not deliver.
	ErrVerifierWithheld = errors.New("reconciler: verifier withheld listed IDs")
	// ErrStripeVerification is the reason reported to the PeerScorer for a
	// deliverer whose stripe the verification found incomplete.
	ErrStripeVerification = errors.New("reconciler: stripe incomplete at verification")
)

// VerifyResult is the outcome of the verification of one stripe.
type VerifyResult struct {
	// Range is the stripe's range.
	Range fingerprint.Range
	// Deliverer is the peer that transferred the stripe; Verifier the peer
	// it was verified with (zero when not verified).
	Deliverer, Verifier swarm.Address
	// Diff is the number of IDs the verification session listed; Stored
	// the number of chunks fetched (and validated) for them, the
	// difference the deliverer is judged on; Undelivered the number of IDs
	// requested from the verifier that it did not deliver, its shortfall.
	Diff, Stored, Undelivered int
	// Err is ErrStripeNotDone or ErrNoVerifier for a stripe not verified,
	// or the error of the verification session or its fetch.
	Err error
}

// VerifyStripes runs the verification round of a striped transfer (§7.4,
// SREP iteration 2): every stripe that completed is reconciled again, in a
// normal session opened with the stripe, with a peer with a Status that
// covers it other than its deliverer, and the IDs found are fetched from
// that peer. A difference above the tolerance (verifyMinDiff, or
// 1/verifyDiffRatio of what the deliverer stored) is reported to the
// PeerScorer as a failure of the deliverer (§9 stripe withholding); only
// chunks the verifier delivered count, so a verifier cannot get a
// deliverer scored by listing IDs it does not have. A verifier whose
// shortfall (IDs requested from it and not delivered; not the IDs the want
// set did not grant or whose stamp was rejected) is above the same
// tolerance (of what it listed) is scored itself (withholding). A stripe
// with no peer other than those that ran a session of it (StripeResult.Tried)
// is not verified (ErrNoVerifier): verifying with the deliverer could not
// find what it withheld. A peer that only answered Busy to the stripe may
// verify it. The verifications run
// like a transfer: one session per peer at a time, peers in parallel; they
// count as catch-up (SyncRate). The results are in the order of results.
func (r *Reconciler) VerifyStripes(ctx context.Context, results []StripeResult) []VerifyResult {
	r.mu.Lock()
	r.catchUpFetches++
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.catchUpFetches--
		r.mu.Unlock()
	}()

	out := make([]VerifyResult, len(results))
	locks := newPeerLocks()
	statuses, ready := r.stripePeers()
	var wg sync.WaitGroup
	for i, res := range results {
		out[i] = VerifyResult{Range: res.Range, Deliverer: res.Peer}
		if res.Err != nil || res.Peer.IsZero() {
			out[i].Err = ErrStripeNotDone
			continue
		}
		verifier, ok := r.verifier(res, statuses, ready)
		if !ok {
			out[i].Err = ErrNoVerifier
			r.logger.Debug("stripe not verified: no other peer", "range", res.Range, "peer_address", res.Peer)
			continue
		}
		out[i].Verifier = verifier
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = r.verifyStripe(ctx, res, verifier, locks)
		}()
	}
	wg.Wait()
	return out
}

// verifier returns the peer to verify res with: a peer of statuses whose
// root covers the stripe, other than the deliverer and every peer that ran a
// session of the stripe (res.Tried), of those in ready (not backing off) if
// any, lowest failure score first.
func (r *Reconciler) verifier(res StripeResult, statuses, ready []PeerStatus) (swarm.Address, bool) {
	for _, peers := range [][]PeerStatus{ready, statuses} {
		var cands []swarm.Address
		for _, ps := range peers {
			if !ps.Peer.Equal(res.Peer) && !slices.ContainsFunc(res.Tried, ps.Peer.Equal) && ps.Status.Root.ContainsRange(res.Range) {
				cands = append(cands, ps.Peer)
			}
		}
		if len(cands) > 0 {
			return r.orderCandidates(cands)[0], true
		}
	}
	return swarm.ZeroAddress, false
}

// verifyStripe verifies res with verifier and scores the deliverer.
func (r *Reconciler) verifyStripe(ctx context.Context, res StripeResult, verifier swarm.Address, locks *peerLocks) VerifyResult {
	v := VerifyResult{Range: res.Range, Deliverer: res.Peer, Verifier: verifier}
	t := r.transferStripe(ctx, res.Range, verifier, locks, func() {})
	v.Diff, v.Stored, v.Undelivered, v.Err = t.Listed, t.Stored, t.Undelivered, t.Err
	if v.Err != nil {
		// an incomplete verification says nothing about the deliverer
		return v
	}
	if v.Undelivered > max(verifyMinDiff, v.Diff/verifyDiffRatio) {
		r.Fail(verifier, fmt.Errorf("stripe %s: verifier stored %d of %d, %d undelivered: %w", res.Range, v.Stored, v.Diff, v.Undelivered, ErrVerifierWithheld))
	}
	r.metrics.VerificationDiff.Observe(float64(v.Stored))
	if v.Stored > max(verifyMinDiff, res.PeerStored/verifyDiffRatio) {
		r.Fail(res.Peer, fmt.Errorf("stripe %s: %d missing after verification with %s: %w", res.Range, v.Stored, verifier, ErrStripeVerification))
	}
	return v
}
