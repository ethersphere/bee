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
	// maxStripeReassign bounds the reassignments of one stripe to another
	// peer after a failure, timeout or withholding without progress.
	maxStripeReassign = 3
	// maxStripeRestarts bounds the restarts of one stripe with the same
	// peer after attempts that failed with progress; past it the stripe is
	// reassigned as if the attempt made no progress.
	maxStripeRestarts = 8
	// maxStripeRegistry bounds the ranges queued for or in a striped
	// transfer; a range past it is dropped.
	maxStripeRegistry = 1024
)

// Stripe transfer results, the values of the stripes_total result label.
const (
	stripeDone       = "done"
	stripeReassigned = "reassigned"
	stripeDropped    = "dropped"
	stripeTimeout    = "timeout"
)

var (
	// ErrStripeTimeout is the error of a stripe that did not complete
	// within the stripe timeout.
	ErrStripeTimeout = errors.New("reconciler: stripe timeout")
	// ErrNoStripePeer is the error of a stripe with no assigned peer.
	ErrNoStripePeer = errors.New("reconciler: no peer for stripe")
	// ErrStripeWithheld is the error of a stripe whose session completed
	// but whose peer did not deliver every listed ID.
	ErrStripeWithheld = errors.New("reconciler: stripe withheld")
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
// the order of stripes. The stored chunks count towards SyncRate, and the
// whole transfer counts as a catch-up in flight (SyncRate reports at least
// StartupGuardRate), including its session phases: a stripe exists only for
// a large difference, so the node is catching up from the first page on.
//
// Backpressure: the wants of a stripe are queued in at most stripeQueue
// Message batches. While the queue is full the session's goroutine blocks in
// ReconcileOptions.Wants, so the peer waits for the next Message. The peer's
// session is bounded only by its own session timeout (rangesync's
// sessionTimeout); a responder that enforced a shorter read deadline would
// see a slow fetch as an idle initiator and end the session, which fails the
// stripe (a restart lists only what is still missing).
func (r *Reconciler) TransferStripes(ctx context.Context, stripes []Stripe) []StripeResult {
	return r.transferStripes(ctx, stripes, nil)
}

// transferStripes is TransferStripes; started, if not nil, is called once,
// when the first wants of any stripe arrive or when the transfer ends.
func (r *Reconciler) transferStripes(ctx context.Context, stripes []Stripe, started func()) []StripeResult {
	r.mu.Lock()
	r.catchUpFetches++
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.catchUpFetches--
		r.mu.Unlock()
	}()
	var startOnce sync.Once
	start := func() {
		if started != nil {
			startOnce.Do(started)
		}
	}
	defer start()

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
	locks := newPeerLocks()
	var wg sync.WaitGroup
	for _, k := range order {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, i := range byPeer[k] {
				results[i] = r.runStripe(ctx, stripes[i], locks, start)
			}
		}()
	}
	wg.Wait()
	return results
}

// peerLocks serializes the stripe sessions with each peer (one session per
// peer) when a reassigned stripe moves to a peer that has stripes of its
// own.
type peerLocks struct {
	mu    sync.Mutex
	locks map[string]chan struct{}
}

func newPeerLocks() *peerLocks {
	return &peerLocks{locks: make(map[string]chan struct{})}
}

// acquire takes peer's lock; the caller holds at most one at a time.
func (l *peerLocks) acquire(ctx context.Context, peer swarm.Address) (func(), error) {
	l.mu.Lock()
	c, ok := l.locks[peer.ByteString()]
	if !ok {
		c = make(chan struct{}, 1)
		l.locks[peer.ByteString()] = c
	}
	l.mu.Unlock()
	select {
	case c <- struct{}{}:
		return func() { <-c }, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

// runStripe transfers st: from its first peer, restarted with the same peer
// while an attempt that failed stored chunks (progress, for example a stripe
// longer than the session timeout), else reassigned to the next of its Peers
// or, when they are exhausted, to the next eligible peer with a Status, at
// most maxStripeReassign times. A restart re-runs FP(stripe), so it lists
// only the IDs still missing. A session that completed without every listed
// ID stored is withholding (ErrStripeWithheld) and is reassigned.
func (r *Reconciler) runStripe(ctx context.Context, st Stripe, locks *peerLocks, started func()) StripeResult {
	total := StripeResult{Range: st.Range}
	tried := make(map[string]bool)
	next := st.Peers
	peer, ok := r.nextStripePeer(st.Range, &next, tried)
	for reassigns, restarts := 0, 0; ok; {
		tried[peer.ByteString()] = true
		release, err := locks.acquire(ctx, peer)
		if err != nil {
			total.Err = err
			break
		}
		res := r.transferStripe(ctx, st.Range, peer, started)
		release()
		total.Peer = peer
		total.Listed += res.Listed
		total.Stored += res.Stored
		total.Err = res.Err
		if res.Err == nil && res.Stored < res.Listed {
			total.Err = fmt.Errorf("stripe %s from %s: stored %d of %d: %w", st.Range, peer, res.Stored, res.Listed, ErrStripeWithheld)
		}
		switch {
		case total.Err == nil:
			r.metrics.Stripes.WithLabelValues(stripeDone).Inc()
			return total
		case ctx.Err() != nil:
			return total
		case res.Err != nil && res.Stored > 0 && restarts < maxStripeRestarts:
			restarts++
			continue // progress: restart with the same peer
		case reassigns == maxStripeReassign:
			ok = false
			continue
		}
		r.logger.Debug("stripe reassigned", "range", st.Range, "peer_address", peer, "error", total.Err)
		if peer, ok = r.nextStripePeer(st.Range, &next, tried); ok {
			reassigns, restarts = reassigns+1, 0
			r.metrics.Stripes.WithLabelValues(stripeReassigned).Inc()
		}
	}
	if errors.Is(total.Err, ErrStripeTimeout) {
		r.metrics.Stripes.WithLabelValues(stripeTimeout).Inc()
	}
	return total
}

// nextStripePeer returns the next untried peer of *next, consuming it, or,
// when *next is exhausted, the first untried peer with a Status whose root
// covers sr (§7.6).
func (r *Reconciler) nextStripePeer(sr fingerprint.Range, next *[]swarm.Address, tried map[string]bool) (swarm.Address, bool) {
	for len(*next) > 0 {
		p := (*next)[0]
		*next = (*next)[1:]
		if !tried[p.ByteString()] {
			return p, true
		}
	}
	if len(tried) == 0 {
		return swarm.ZeroAddress, false
	}
	for _, ps := range r.peerStatuses() {
		if !tried[ps.Peer.ByteString()] && ps.Status.Root.ContainsRange(sr) {
			return ps.Peer, true
		}
	}
	return swarm.ZeroAddress, false
}

// transferStripe transfers stripe sr from peer: a session opened with sr
// whose wants are queued (stripeQueue) for a fetch that runs alongside.
// It is one attempt of runStripe; started is called with the first wants.
func (r *Reconciler) transferStripe(ctx context.Context, sr fingerprint.Range, peer swarm.Address, started func()) StripeResult {
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
			started()
			select {
			case queue <- ids:
				res.Listed += len(ids)
			case <-ctx.Done(): // dropped: the stripe fails
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
	case fetchErr != nil:
		res.Err = fetchErr
	case err != nil:
		res.Err = fmt.Errorf("reconcile: %w", err)
	case ctx.Err() != nil:
		res.Err = ctx.Err()
	}
	return res
}

// stripeJob is a range queued for a striped transfer; started, if not nil,
// is called once when its transfer has its first wants or ends; bootstrap
// marks the bootstrap transfer of the root.
type stripeJob struct {
	rg        fingerprint.Range
	started   func()
	bootstrap bool
}

// enqueueStripes registers ranges for a striped transfer by the stripe
// worker (registerStripeLocked) and reports whether every range was
// registered.
func (r *Reconciler) enqueueStripes(ranges []fingerprint.Range) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := true
	for _, rg := range ranges {
		if !r.registerStripeLocked(stripeJob{rg: rg}) {
			all = false
		}
	}
	return all
}

// registerStripeLocked queues job for the stripe worker. A range that
// overlaps a registered (queued or in-flight) range is not registered, so
// that it is not transferred twice; what it misses shows up again in a
// later round. Past maxStripeRegistry ranges it is dropped (counted as
// stripes_total{dropped}). It reports whether job was registered.
func (r *Reconciler) registerStripeLocked(job stripeJob) bool {
	rg := fingerprint.NewRange(job.rg.Prefix, job.rg.Bits)
	if slices.ContainsFunc(r.stripeRegistry, func(o fingerprint.Range) bool {
		return o.ContainsRange(rg) || rg.ContainsRange(o)
	}) {
		return false
	}
	if len(r.stripeRegistry) >= maxStripeRegistry {
		r.metrics.Stripes.WithLabelValues(stripeDropped).Inc()
		r.logger.Debug("stripe range dropped", "range", rg)
		return false
	}
	job.rg = rg
	r.stripeRegistry = append(r.stripeRegistry, rg)
	r.stripeQueue = append(r.stripeQueue, job)
	select {
	case r.stripeWake <- struct{}{}:
	default:
	}
	return true
}

// stripeWorker plans and transfers the queued ranges, one at a time, across
// the peers with a Status, until Close.
func (r *Reconciler) stripeWorker() {
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-r.stripeWake:
		}
		for {
			r.mu.Lock()
			if len(r.stripeQueue) == 0 || r.ctx.Err() != nil {
				r.mu.Unlock()
				break
			}
			job := r.stripeQueue[0]
			r.stripeQueue = r.stripeQueue[1:]
			r.mu.Unlock()

			failed := false
			for _, res := range r.transferStripes(r.ctx, PlanStripes(job.rg, r.peerStatuses()), job.started) {
				if res.Err != nil {
					failed = true
					r.logger.Debug("stripe", "range", res.Range, "peer_address", res.Peer, "error", res.Err)
				}
			}

			r.mu.Lock()
			r.stripeRegistry = slices.DeleteFunc(r.stripeRegistry, func(o fingerprint.Range) bool { return o.Equal(job.rg) })
			if job.bootstrap {
				r.bootstrapping, r.bootstrapFailed = false, failed
			}
			r.mu.Unlock()
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
