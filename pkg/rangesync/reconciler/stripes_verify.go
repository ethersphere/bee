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
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// verifyMinDiff is the largest verification difference that never
	// counts against the deliverer, whatever the stripe's size: older
	// chunks that reached the neighborhood late (pushed long after they
	// were stamped) and stamps skewed past verifyStampSlack.
	verifyMinDiff = 4
	// verifyDiffRatio sets the tolerance of larger stripes: a difference
	// above 1/verifyDiffRatio of the chunks the deliverer itself stored
	// (StripeResult.PeerStored, and above verifyMinDiff) is reported as a
	// failure of the deliverer. The difference counted is what the
	// verifier delivered (stored, so validated), never what it only listed,
	// and of that only the chunks stamped before the deliverer's listing
	// (verifyStampSlack).
	verifyDiffRatio = 100
	// verifyStampSlack is how much older than the deliverer's listing of
	// the stripe (StripeResult.ListedAt) the stamp of a chunk the
	// verification stored must be for the chunk to count against the
	// deliverer (T6.5 part 1). A chunk stamped later was uploaded after the
	// listing opened, or so shortly before that it may not have reached
	// the deliverer by then: the slack covers the spread of an upload
	// through the neighborhood (its rounds, up to (takeoverRounds + 1) ×
	// MaxRoundInterval for a peer that takes over), the latency of the
	// push and the skew between the uploader's clock and this node's. A
	// stamp more than the slack ahead of this node's clock when the chunk
	// was stored is no evidence of a late upload either: such a chunk
	// counts. The stamps are signed by their batch owners and bound to the
	// IDs listed (an ID holds its stamp's hash), so neither the deliverer
	// nor the verifier can move them. See the T6.5 part 1 decisions log for
	// what a deliverer can still withhold unscored.
	verifyStampSlack = 10 * time.Minute
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
	// the number of chunks fetched (and validated) for them; Newer the
	// number of those whose stamps are not older than the deliverer's
	// listing (StripeResult.ListedAt) by verifyStampSlack, nor ahead of
	// this node's clock by more: uploaded after it listed the stripe. The
	// deliverer is judged on Stored − Newer.
	// Undelivered is the number of IDs requested from the verifier that it
	// did not deliver, its shortfall (not those outside its root by its
	// last Status, which it was not asked for).
	Diff, Stored, Newer, Undelivered int
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
// deliverer scored by listing IDs it does not have, and of those only the
// ones whose stamps are older than the deliverer's listing of the stripe
// (StripeResult.ListedAt) by more than verifyStampSlack, or ahead of this
// node's clock by more, so that the chunks uploaded into the stripe since
// are not held against it (T6.5 part 1). A stored chunk whose stamp time the
// Service does not report counts. A verifier whose
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
	return r.verifyStripes(ctx, results, nil)
}

// verifyStripes is VerifyStripes; stored, if not nil, is called whenever a
// verification's fetch stores chunks (the stripe worker's job progress,
// jobStored).
func (r *Reconciler) verifyStripes(ctx context.Context, results []StripeResult, stored func()) []VerifyResult {
	r.mu.Lock()
	r.addCatchUpLocked(1)
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.addCatchUpLocked(-1)
		r.mu.Unlock()
	}()

	out := make([]VerifyResult, len(results))
	locks := newPeerLocks()
	locks.keep = true // a verification keeps its verifier's lock across asks
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
			out[i] = r.verifyStripe(ctx, res, verifier, locks, stored)
		}()
	}
	wg.Wait()
	return out
}

// verifier returns the peer to verify res with: a peer of statuses whose
// root covers the stripe, other than the deliverer and every peer that ran a
// session of the stripe (res.Tried), of those in ready (not backing off,
// stripePeers) if any, lowest failure score first.
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

// verifyStripe verifies res with verifier and scores the deliverer; stored,
// if not nil, is called whenever its fetch stores chunks.
func (r *Reconciler) verifyStripe(ctx context.Context, res StripeResult, verifier swarm.Address, locks *peerLocks, stored func()) VerifyResult {
	v := VerifyResult{Range: res.Range, Deliverer: res.Peer, Verifier: verifier}
	var since time.Time // the stamps from which on a chunk is not held against the deliverer
	if !res.ListedAt.IsZero() {
		since = res.ListedAt.Add(-verifyStampSlack)
	}
	// one attempt, with a peer's whole budget on the stripe clock
	t := r.transferStripe(ctx, res.Range, verifier, locks, func() {}, stored, stripePeerBudget*r.stripeTimeout, since)
	v.Diff, v.Stored, v.Newer, v.Undelivered, v.Err = t.Listed, t.Stored, t.newer, t.Undelivered-t.outsideRoot, t.Err
	if v.Err != nil {
		// an incomplete verification says nothing about the deliverer
		return v
	}
	if v.Undelivered > max(verifyMinDiff, v.Diff/verifyDiffRatio) {
		r.Fail(verifier, fmt.Errorf("stripe %s: verifier stored %d of %d, %d undelivered: %w", res.Range, v.Stored, v.Diff, v.Undelivered, ErrVerifierWithheld))
	}
	r.metrics.VerificationDiff.Observe(float64(v.Stored))
	if missing := v.Stored - v.Newer; missing > max(verifyMinDiff, res.PeerStored/verifyDiffRatio) {
		r.Fail(res.Peer, fmt.Errorf("stripe %s: %d missing after verification with %s (and %d uploaded since the listing): %w", res.Range, missing, verifier, v.Newer, ErrStripeVerification))
	}
	return v
}
