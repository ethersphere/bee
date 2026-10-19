// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// StripeTimeout bounds the transfer of one stripe: its session and the
	// fetch of the IDs the session listed.
	StripeTimeout = 15 * time.Minute

	// stripeQueue is the number of want batches of a stripe queued for
	// fetch. A batch holds the wants of one session Message (at most
	// MaxMessageBytes of IDs); a full queue holds the stripe's session back.
	stripeQueue = 4
	// stripeRetryInterval is the wait before a stripe session is opened
	// again after rangesync.ErrSessionLimit (the peer's session slot is
	// taken, for example by a round's session).
	stripeRetryInterval = time.Second
)

// Stripe transfer results, the values of the stripes_total result label.
const (
	stripeDone    = "done"
	stripeTimeout = "timeout"
)

var (
	// ErrStripeTimeout is the error of a stripe that did not complete
	// within the stripe timeout.
	ErrStripeTimeout = errors.New("reconciler: stripe timeout")
	// ErrNoStripePeer is the error of a stripe with no assigned peer.
	ErrNoStripePeer = errors.New("reconciler: no peer for stripe")
)

// StripeResult is the outcome of the transfer of one stripe.
type StripeResult struct {
	// Range is the stripe's range.
	Range fingerprint.Range
	// Peer is the peer the stripe was transferred from; zero when the
	// stripe has no peer.
	Peer swarm.Address
	// Listed is the number of IDs the peer listed that this node did not
	// hold; Stored the number of chunks stored.
	Listed, Stored int
	// Err is nil when the stripe is done: the session completed and every
	// listed ID went through Fetch. It wraps ErrStripeTimeout on timeout
	// and is ErrNoStripePeer for a stripe with no peer. A failed stripe can
	// be transferred again: its session lists only the IDs still missing.
	Err error
}

// TransferStripes transfers stripes (§7.4): for each, it opens a session
// with the first of its Peers whose opening is the stripe's Fingerprint
// (count 0 on an empty node, so the peer pages its IDs) and fetches the IDs
// from the same peer as the session's Messages arrive, through the want set
// and the node-wide fetch limiter of the Service. The stripes of one peer
// run one after another (one session per peer), those of different peers in
// parallel. Each stripe is bounded by the stripe timeout. The results are in
// the order of stripes. The stored chunks count towards SyncRate.
func (r *Reconciler) TransferStripes(ctx context.Context, stripes []Stripe) []StripeResult {
	r.mu.Lock()
	r.catchUpFetches++
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.catchUpFetches--
		r.mu.Unlock()
	}()

	results := make([]StripeResult, len(stripes))
	byPeer := make(map[string][]int)
	var order []string
	for i, st := range stripes {
		if len(st.Peers) == 0 {
			results[i] = StripeResult{Range: st.Range, Err: ErrNoStripePeer}
			continue
		}
		k := st.Peers[0].ByteString()
		if _, ok := byPeer[k]; !ok {
			order = append(order, k)
		}
		byPeer[k] = append(byPeer[k], i)
	}
	var wg sync.WaitGroup
	for _, k := range order {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, i := range byPeer[k] {
				results[i] = r.transferStripe(ctx, stripes[i].Range, stripes[i].Peers[0])
			}
		}()
	}
	wg.Wait()
	return results
}

// transferStripe transfers stripe sr from peer: a session opened with sr
// whose wants are queued (stripeQueue) for a fetch that runs alongside.
func (r *Reconciler) transferStripe(ctx context.Context, sr fingerprint.Range, peer swarm.Address) StripeResult {
	res := StripeResult{Range: sr, Peer: peer}
	ctx, cancel := context.WithTimeoutCause(ctx, r.stripeTimeout, ErrStripeTimeout)
	defer cancel()

	queue := make(chan []storer.ChunkID, stripeQueue)
	var fetchErr error
	fetched := make(chan struct{})
	go func() {
		defer close(fetched)
		for ids := range queue {
			if fetchErr != nil {
				continue // drain
			}
			n, err := r.svc.Fetch(ctx, peer, ids)
			res.Stored += n
			if n > 0 {
				r.rate.Add(n)
			}
			if err != nil {
				fetchErr = fmt.Errorf("fetch: %w", err)
				cancel() // end the session
			}
		}
	}()

	opts := rangesync.ReconcileOptions{
		Open: []fingerprint.Range{sr},
		Wants: func(ids []storer.ChunkID) {
			res.Listed += len(ids)
			select {
			case queue <- ids:
			case <-ctx.Done():
			}
		},
	}
	var err error
	for {
		_, err = r.svc.ReconcileWith(ctx, peer, opts)
		if !errors.Is(err, rangesync.ErrSessionLimit) {
			break
		}
		t := time.NewTimer(stripeRetryInterval)
		select {
		case <-ctx.Done():
		case <-t.C:
		}
		t.Stop()
		if ctx.Err() != nil {
			break
		}
	}
	close(queue)
	<-fetched

	switch {
	case errors.Is(context.Cause(ctx), ErrStripeTimeout):
		res.Err = fmt.Errorf("stripe %s from %s: %w", sr, peer, ErrStripeTimeout)
		r.metrics.Stripes.WithLabelValues(stripeTimeout).Inc()
	case fetchErr != nil:
		res.Err = fetchErr
	case err != nil:
		res.Err = fmt.Errorf("reconcile: %w", err)
	case ctx.Err() != nil:
		res.Err = ctx.Err()
	default:
		r.metrics.Stripes.WithLabelValues(stripeDone).Inc()
	}
	return res
}

// stripe plans and transfers the ranges a session's Striper took over,
// across the peers with a Status.
func (r *Reconciler) stripe(ranges []fingerprint.Range) {
	peers := r.peerStatuses()
	for _, rg := range ranges {
		for _, res := range r.TransferStripes(r.ctx, PlanStripes(rg, peers)) {
			if res.Err != nil {
				r.logger.Debug("stripe", "range", res.Range, "peer_address", res.Peer, "error", res.Err)
			}
		}
	}
}

// peerStatuses returns the known peers with their last Status.
func (r *Reconciler) peerStatuses() []PeerStatus {
	r.mu.Lock()
	var addrs []swarm.Address
	for k, st := range r.peers {
		if !st.unsupported {
			addrs = append(addrs, swarm.NewAddress([]byte(k)))
		}
	}
	r.mu.Unlock()
	var out []PeerStatus
	for _, p := range addrs {
		if st, ok := r.svc.LastStatus(p); ok {
			out = append(out, PeerStatus{Peer: p, Status: st})
		}
	}
	return out
}
