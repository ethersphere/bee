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
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// StripeTimeout is the inactivity timeout of one attempt of the
	// transfer of a stripe, on the stripe clock (stripeClock): the attempt,
	// its session and the fetch of the IDs the session listed, ends once
	// its fetches stored no chunk for StripeTimeout. An attempt that keeps
	// storing runs on, up to what is left of its peer's budget of
	// stripePeerBudget StripeTimeouts on the clock, so that a slow transfer
	// is not restarted (a restart lists again what the attempt had listed
	// and not yet fetched) and a peer that delivers a chunk now and then
	// holds a stripe no longer than before (T4.7 part 1 fix round 1). See
	// runStripe for the bound on a whole stripe.
	StripeTimeout = 15 * time.Minute

	// stripeQueue is the number of want batches of a stripe queued for
	// fetch. A batch holds the wants of one session Message (at most
	// MaxMessageBytes of IDs); a full queue holds the stripe's session back.
	stripeQueue = 4
	// stripeRetryInterval is the wait before a stripe session is opened
	// again after a Busy answer (isBusy): rangesync.ErrSessionLimit (the
	// peer's session slot is taken by a session the peer started, or this
	// node runs MaxConcurrentSessions) or an index rebuilding on either side
	// (after a radius change, §7.5). A round session of this node never
	// takes the slot: the peer's state machine keeps them apart.
	stripeRetryInterval = time.Second
	// maxStripeBusyRetries bounds the retries of one stripe attempt after
	// the peer's Busy answers (about 30 s at stripeRetryInterval, longer
	// than an index rebuild); past it the attempt fails and runStripe
	// reassigns the stripe. Busy causes on this node (localBusy) are not
	// counted.
	maxStripeBusyRetries = 30
	// maxStripeReassign bounds the reassignments of one stripe to another
	// peer after a failure, timeout or withholding without progress.
	maxStripeReassign = 3
	// maxStripeRestarts bounds the restarts of one stripe with the same
	// peer after attempts that failed with progress or were deferred; past
	// it the stripe is reassigned as if the attempt made no progress. It
	// also bounds the restarts after attempts held up only by this node
	// (localBusy) over the whole stripe: reassigning does not reset that
	// budget.
	maxStripeRestarts = 8
	// stripePeerBudget is the budget, in StripeTimeouts on the stripe
	// clock, of the attempts of one stripe with one peer, all together:
	// what 1 + maxStripeRestarts attempts of at most StripeTimeout each
	// took before StripeTimeout became an inactivity timeout. An attempt
	// that runs out of it is not restarted with the same peer: the stripe
	// is reassigned.
	stripePeerBudget = 1 + maxStripeRestarts
	// maxStripeRegistry bounds the ranges queued for or in a striped
	// transfer; a range past it is dropped, and so are the parts of a
	// trimmed range past it.
	maxStripeRegistry = 1024
	// maxPeerStripes bounds the ranges registered on one peer's claim (a
	// session's Striper, the rounds' session-root check) that are queued
	// or in flight at once, trimmed parts counted one by one; a range or
	// part past it is dropped. maxSessionStripes bounds the ranges one
	// session registers; the session takes the others over and drops them,
	// and a later session offers them again. A peer that claims far larger
	// counts than it holds thus gets at most maxPeerStripes transfers
	// queued, not one per range of its openings (T4.7 part 1 fix round 1).
	maxPeerStripes    = 2
	maxSessionStripes = 1
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
	// ErrStripeDeferred is the error of a stripe whose session completed
	// with IDs the want set did not grant (another fetch held their claim,
	// or it was full), still neither stored nor rejected after
	// maxPendingPasses, or with IDs whose stamp check failed for a cause on
	// this node (FetchResult.Unverified): nobody's fault. The stripe is
	// restarted with the same peer.
	ErrStripeDeferred = errors.New("reconciler: stripe deferred by the want set")
)

// maxPendingPasses is how many times, stripeRetryInterval apart, a stripe
// requests the IDs the want set did not grant from its peer again before it
// fails with ErrStripeDeferred: a claim deadline, by when the claims of
// other fetches on them have ended or expired.
const maxPendingPasses = int(rangesync.ClaimDeadline / stripeRetryInterval)

// StripeResult is the outcome of the transfer of one stripe.
type StripeResult struct {
	// Range is the stripe's range.
	Range fingerprint.Range
	// Peer is the peer the stripe was transferred from; zero when the
	// stripe has no peer.
	Peer swarm.Address
	// Listed is the number of IDs the peer listed that this node did not
	// hold; Stored the number of chunks stored; Undelivered the number of
	// IDs requested from the peer that it did not deliver. IDs the want set
	// did not grant (another fetch held them, or they were rejected), IDs
	// whose stamp was rejected and IDs stored meanwhile are listed but
	// neither stored nor undelivered.
	Listed, Stored, Undelivered int
	// Deferred is the number of IDs the want set did not grant, still
	// pending when the last attempt ended, and of IDs whose stamp check
	// failed for a cause on this node in it (ErrStripeDeferred). Rejected
	// IDs (an invalid or older stamp, or a signature that does not recover)
	// are deliberately not fetched and count as done: the rounds fetch them
	// after RejectTTL if they are still listed.
	Deferred int
	// PeerStored is the number of chunks Peer itself stored (over its
	// restarts); Stored also counts the peers the stripe was reassigned
	// from. Tried is every peer that ran a session of the stripe, Peer
	// included once it did: a peer that only answered Busy, or whose
	// attempts ended before a session, is left out, so that it may still
	// verify the stripe.
	PeerStored int
	Tried      []swarm.Address
	// Err is nil when the stripe is done: the session completed, every
	// listed ID went through Fetch, the peer delivered every ID requested
	// and none is deferred. It wraps ErrStripeTimeout on timeout and is
	// ErrNoStripePeer for a stripe with no peer. A failed stripe can be
	// transferred again: its session lists only the IDs still missing.
	Err error

	// ran reports that an attempt ran a session with Peer: it was answered
	// other than Busy, or listed IDs. outsideRoot is how many of
	// Undelivered were not requested because they lie outside Peer's root.
	ran         bool
	outsideRoot int
	// clock is how long the attempt's stripe clock ran, and capped
	// reports that it timed out at its cap, the rest of its peer's budget
	// (stripePeerBudget), not for inactivity.
	clock  time.Duration
	capped bool
}

// TransferStripes transfers stripes (§7.4): for each, it opens a session
// with the first of its Peers whose opening is the stripe's Fingerprint
// (count 0 on an empty node, so the peer pages its IDs) and fetches the IDs
// from the same peer as the session's Messages arrive, through the want set
// and the node-wide fetch limiter of the Service. The stripes of one peer
// run one after another (one session per peer), those of different peers in
// parallel. Each stripe is bounded by the stripe timeout. The results are in
// the order of stripes. The stored chunks count towards SyncRate as they are
// stored, and the whole transfer counts as a catch-up fetch in flight,
// including its session phases: a stripe exists only for a large difference,
// so the node is catching up from the first page on. A transfer the stripe
// worker runs holds SyncRate at least at StartupGuardRate through its
// registered range; one that is not registered only within the catch-up
// grace or while the rate is above 0 (SyncRate).
//
// Backpressure: the wants of a stripe are queued in at most stripeQueue
// Message batches. While the queue is full the session's goroutine blocks in
// ReconcileOptions.Wants, so the peer waits for the next Message. The peer's
// session is bounded only by its own session timeout (rangesync's
// sessionTimeout); a responder that enforced a shorter read deadline would
// see a slow fetch as an idle initiator and end the session, which fails the
// stripe (a restart lists only what is still missing).
func (r *Reconciler) TransferStripes(ctx context.Context, stripes []Stripe) []StripeResult {
	return r.transferStripes(ctx, stripes, nil, false)
}

// transferStripes is TransferStripes; started, if not nil, is called once,
// when the first wants of any stripe arrive or when the transfer ends.
// bootstrap marks the transfer of a bootstrap: its stripes keep their peer's
// stripe lock while they wait to ask again (peerLocks.keep).
func (r *Reconciler) transferStripes(ctx context.Context, stripes []Stripe, started func(), bootstrap bool) []StripeResult {
	r.mu.Lock()
	r.addCatchUpLocked(1)
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.addCatchUpLocked(-1)
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
	locks.keep = bootstrap
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
// peer), and the fetches that follow them, when a reassigned stripe moves to
// a peer that has stripes of its own, or verifications share a verifier. A
// stripe frees its peer's lock while it waits to ask again for IDs the want
// set held (resolvePending), unless keep is set: a verification or a
// bootstrap's stripe keeps it, so that the stripes queued on the peer do not
// run their whole attempts while its own clock runs. Verifications do not
// restart, so a timed-out one would leave its deliverer unverified, and
// during a bootstrap rounds start no session, so the release buys them
// nothing.
type peerLocks struct {
	mu    sync.Mutex
	locks map[string]*peerLock
	keep  bool
}

// peerLock is the lock of one peer; refs counts its holder and waiters, and
// the lock is forgotten when it drops to zero.
type peerLock struct {
	c    chan struct{}
	refs int
}

func newPeerLocks() *peerLocks {
	return &peerLocks{locks: make(map[string]*peerLock)}
}

// acquire takes peer's lock; the caller holds at most one at a time.
func (l *peerLocks) acquire(ctx context.Context, peer swarm.Address) (func(), error) {
	k := peer.ByteString()
	l.mu.Lock()
	pl, ok := l.locks[k]
	if !ok {
		pl = &peerLock{c: make(chan struct{}, 1)}
		l.locks[k] = pl
	}
	pl.refs++
	l.mu.Unlock()
	unref := func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if pl.refs--; pl.refs == 0 {
			delete(l.locks, k)
		}
	}
	select {
	case pl.c <- struct{}{}:
		return func() {
			<-pl.c
			unref()
		}, nil
	case <-ctx.Done():
		unref()
		return nil, context.Cause(ctx)
	}
}

// runStripe transfers st: from its first peer, restarted with the same peer
// while an attempt that failed stored chunks (progress, for example a stripe
// longer than the session timeout) or ended with IDs still deferred
// (ErrStripeDeferred, blaming nobody), at most maxStripeRestarts times per
// peer, else reassigned to the next of its Peers or, when they are
// exhausted, to the next eligible peer with a Status, at most
// maxStripeReassign times. An attempt held up only by a Busy cause on this
// node (localBusy) is restarted too, but those restarts draw on one budget
// of maxStripeRestarts per stripe that reassigning does not reset: a cause
// on this node is the same with every peer. A restart re-runs FP(stripe), so
// it lists only the IDs still missing. A session that completed with IDs
// requested from the peer and not delivered is withholding
// (ErrStripeWithheld) and is reassigned; IDs the want set did not grant and
// rejected stamps are not. An attempt whose session ran and whose fetch
// found the peer's fetch stream opens backing off (rangesync.ErrOpenBackoff)
// is restarted with the same peer too (maxStripeRestarts): the next attempt
// waits for the backoff as for a Busy answer (transferStripe), and is
// reassigned once those waits run out.
//
// The attempts with one peer share one budget on the stripe clock,
// stripePeerBudget × StripeTimeout: each attempt's clock is capped at what
// is left of it (transferStripe), and an attempt that runs out of it is
// reassigned whatever it stored. An attempt ends earlier when its fetches
// store nothing for StripeTimeout (the inactivity timeout), so an honest
// slow transfer runs as one attempt, while a peer that delivers a chunk now
// and then is reassigned after the budget, as when 1 + maxStripeRestarts
// attempts of StripeTimeout each bounded it (T4.7 part 1 fix round 1).
//
// Bound: a stripe makes at most (1 + maxStripeReassign) × (1 +
// maxStripeRestarts) = 36 attempts, and its attempts with one peer run at
// most stripePeerBudget × StripeTimeout on the stripe clock together, so at
// most (1 + maxStripeReassign) × stripePeerBudget × StripeTimeout in all (9
// h at the defaults); at most 1 + maxStripeRestarts + maxStripeReassign = 12
// attempts (3 h of inactivity) when only this node holds them up. Not on
// the stripe clock, per attempt: the wait for the peer's stripe lock and for
// the slots before the clock starts, and the waits to retry the peer's Busy,
// at most maxStripeBusyRetries of stripeRetryInterval plus the wait for the
// slots again.
func (r *Reconciler) runStripe(ctx context.Context, st Stripe, locks *peerLocks, started func()) StripeResult {
	total := StripeResult{Range: st.Range}
	tried := make(map[string]bool)
	next := st.Peers
	budget := stripePeerBudget * r.stripeTimeout
	var used time.Duration // of budget, by the attempts with peer
	peer, ok := r.nextStripePeer(st.Range, &next, tried)
	for reassigns, restarts, localRestarts := 0, 0, 0; ok; {
		if !tried[peer.ByteString()] {
			tried[peer.ByteString()] = true
			total.PeerStored, used = 0, 0
		}
		res := r.transferStripe(ctx, st.Range, peer, locks, started, budget-used)
		used += res.clock
		if res.ran && !slices.ContainsFunc(total.Tried, peer.Equal) {
			total.Tried = append(total.Tried, peer)
		}
		total.Peer = peer
		total.Listed += res.Listed
		total.Stored += res.Stored
		total.Undelivered += res.Undelivered
		total.Deferred = res.Deferred
		total.PeerStored += res.Stored
		total.Err = res.Err
		if res.Undelivered > 0 && (res.Err == nil || errors.Is(res.Err, ErrStripeDeferred)) {
			total.Err = fmt.Errorf("stripe %s from %s: stored %d of %d, %d undelivered: %w", st.Range, peer, res.Stored, res.Listed, res.Undelivered, ErrStripeWithheld)
		}
		progress := res.Stored > 0 || errors.Is(res.Err, ErrStripeDeferred)
		local := !progress && localBusy(res.Err) // held up by this node only
		// the session ran and its fetch found the peer's opens backing off:
		// as for a Busy answer, ask the same peer again (the next attempt
		// waits for the backoff as for a Busy answer)
		backoff := !progress && res.ran && errors.Is(res.Err, rangesync.ErrOpenBackoff)
		// the peer's budget on the stripe clock is spent: reassign
		spent := res.capped || used >= budget
		switch {
		case total.Err == nil:
			r.metrics.Stripes.WithLabelValues(stripeDone).Inc()
			return total
		case ctx.Err() != nil:
			return total
		case res.Err != nil && !errors.Is(total.Err, ErrStripeWithheld) && restarts < maxStripeRestarts && !spent &&
			(progress || backoff || local && localRestarts < maxStripeRestarts):
			restarts++
			if local {
				localRestarts++
			}
			continue // progress, the peer's open backoff or held up by this node: restart with the same peer
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
// covers sr (§7.6), a peer that does not back off first (stripePeers).
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
	all, ready := r.stripePeers()
	for _, peers := range [][]PeerStatus{ready, all} {
		for _, ps := range peers {
			if !tried[ps.Peer.ByteString()] && ps.Status.Root.ContainsRange(sr) {
				return ps.Peer, true
			}
		}
	}
	return swarm.ZeroAddress, false
}

// planStripes plans rg (PlanStripes) across the peers that do not back off
// (stripePeers), and across every peer offered for stripes when those leave
// a stripe without a peer.
func (r *Reconciler) planStripes(rg fingerprint.Range) []Stripe {
	all, ready := r.stripePeers()
	if len(ready) < len(all) {
		if plan := PlanStripes(rg, ready); len(Unassigned(plan)) == 0 {
			return plan
		}
	}
	return PlanStripes(rg, all)
}

// stripePeers returns the peers offered for stripes (peerStatuses), and
// those of them that do not back off: neither after Busy or failed round
// sessions (peerFSM.retryAt) nor in their fetch stream opens (opensBackOff),
// where a stripe would only wait.
func (r *Reconciler) stripePeers() (all, ready []PeerStatus) {
	all = r.peerStatuses()
	r.mu.Lock()
	now := r.now()
	var idle []PeerStatus
	for _, ps := range all {
		if st, ok := r.peers[ps.Peer.ByteString()]; ok && !now.Before(st.fsm.retryAt) {
			idle = append(idle, ps)
		}
	}
	r.mu.Unlock()
	for _, ps := range idle {
		if !r.opensBackOff(ps.Peer) {
			ready = append(ready, ps)
		}
	}
	return all, ready
}

// transferStripe transfers stripe sr from peer: a session opened with sr
// whose wants are queued (stripeQueue) for a fetch that runs alongside.
// It is one attempt of runStripe; started is called with the first wants.
// It takes the peer's lock in locks first (one stripe session per peer), and
// holds it, and the peer in Striped (acquireStripe), until it ends, except
// while it waits to ask again for IDs the want set held (resolvePending):
// then it frees the Striped state, and the lock unless locks.keep.
// It holds a stripe slot (MaxStripeSessions) and a session slot
// (MaxInitiatedSessions) while its session runs, and frees both while it
// waits to retry a Busy answer: at most maxStripeBusyRetries times for the
// peer's Busy, and without a count for a cause on this node (localBusy). A
// peer whose fetch stream opens back off (opensBackOff) is not asked for a
// session: that counts as the peer's Busy (stripeBusy). The stripe clock
// (stripeClock) starts once it holds them and then runs until it ends, the
// fetch after the session included, except while it waits to retry the
// peer's Busy with no fetch of the attempt running. The attempt times out
// (ErrStripeTimeout) once the clock ran StripeTimeout since the attempt's
// fetches last stored a chunk (or since it started), or once it ran
// capacity in all (res.capped; runStripe passes what is left of the peer's
// budget). The waits for a cause on this node count, so they end at the
// timeout, and runStripe restarts such an attempt with the same peer
// (maxStripeRestarts) instead of reassigning it. The wait for the peer's
// round session to end (acquireStripe) counts towards the stripe timeout.
func (r *Reconciler) transferStripe(ctx context.Context, sr fingerprint.Range, peer swarm.Address, locks *peerLocks, started func(), capacity time.Duration) (res StripeResult) {
	res = StripeResult{Range: sr, Peer: peer}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	unlock, err := locks.acquire(ctx, peer)
	if err != nil {
		res.Err = err
		return res
	}
	hold := &peerHold{unlock: unlock}
	defer hold.release()
	clock := newStripeClock(r.now, r.after, r.stripeTimeout, capacity, func() { cancel(ErrStripeTimeout) })
	defer func() {
		clock.pause()
		res.clock, res.capped = clock.ran(), clock.isCapped()
	}()
	releaseSlots, err := r.acquireStripeSlots(ctx)
	if err != nil {
		res.Err = err
		return res
	}
	held := true
	defer func() {
		if held {
			releaseSlots()
		}
	}()
	clock.resume()
	hold.unstripe, err = r.acquireStripe(ctx, peer)
	if err == nil {
		clock.expireIfDue() // the wait for the peer may have outlasted the timeout
		err = context.Cause(ctx)
	}
	if err != nil {
		res.Err = err
		if errors.Is(err, ErrStripeTimeout) {
			res.Err = fmt.Errorf("stripe %s from %s: waiting for the peer: %w", sr, peer, ErrStripeTimeout)
		}
		return res
	}

	queue := make(chan []storer.ChunkID, stripeQueue)
	var (
		fetchErr   error
		pending    []storer.ChunkID // not granted by the want set (fetchOutcome)
		unverified int              // stamp check failed on this node
		queued     atomic.Int64     // want batches queued and not yet fetched
	)
	fetched := make(chan struct{})
	go func() {
		defer close(fetched)
		for ids := range queue {
			clock.expireIfDue() // the last fetch may have outlasted the timeout
			if fetchErr != nil || ctx.Err() != nil {
				queued.Add(-1)
				continue // drain
			}
			out, err := r.fetchFrom(ctx, peer, ids, true, clock.progress)
			queued.Add(-1)
			res.Stored += out.stored
			res.Undelivered += out.undelivered
			res.outsideRoot += out.outsideRoot
			pending = append(pending, out.pending...)
			unverified += out.unverified
			if err != nil {
				fetchErr = fmt.Errorf("fetch: %w", err)
				cancel(nil) // end the session
			}
		}
	}()

	opts := rangesync.ReconcileOptions{
		Open: []fingerprint.Range{sr},
		Wants: func(ids []storer.ChunkID) {
			started()
			queued.Add(1)
			select {
			case queue <- ids:
				res.Listed += len(ids)
			case <-ctx.Done(): // dropped: the stripe fails
				queued.Add(-1)
			}
		},
	}
	var lastLocal error // the last answer, if Busy for a cause on this node
	for busy := 0; ; {
		if r.opensBackOff(peer) {
			// its fetches would fail at once: wait as for its Busy
			err = rangesync.ErrOpenBackoff
		} else {
			_, err = r.svc.ReconcileWith(ctx, peer, opts)
		}
		clock.expireIfDue()
		lastLocal = nil
		if !stripeBusy(err) {
			res.ran = true
			break
		}
		if localBusy(err) {
			lastLocal = err
		} else {
			if busy == maxStripeBusyRetries {
				break
			}
			busy++
		}
		// wait for the retry without the slots, and off the stripe clock
		// unless this node is the cause or the fetch of IDs an earlier
		// session of the attempt listed is still running (the clock bounds
		// that fetch)
		if lastLocal == nil && queued.Load() == 0 {
			clock.pause()
		}
		releaseSlots()
		held = false
		select {
		case <-ctx.Done():
		case <-r.after(r.stripeRetry):
		}
		if clock.expireIfDue(); ctx.Err() != nil {
			break
		}
		if releaseSlots, err = r.acquireStripeSlots(ctx); err != nil {
			break
		}
		held = true
		clock.resume() // a no-op when it ran through the wait
		// the wait for the slots may have outlasted the timeout
		if clock.expireIfDue(); ctx.Err() != nil {
			break
		}
	}
	if held { // the session ended; its fetch does not use the slots (the clock runs on)
		releaseSlots()
		held = false
	}
	close(queue)
	<-fetched
	res.ran = res.ran || res.Listed > 0 // a session that listed IDs ran, even if it then ended Busy
	if fetchErr == nil && err == nil && ctx.Err() == nil && len(pending) > 0 {
		pending, unverified, fetchErr = r.resolvePending(ctx, clock, peer, locks, hold, pending, unverified, &res)
	}
	res.Deferred = len(pending) + unverified

	switch {
	case errors.Is(context.Cause(ctx), ErrStripeTimeout) && lastLocal != nil:
		res.Err = fmt.Errorf("stripe %s from %s: %w: %w", sr, peer, ErrStripeTimeout, lastLocal)
	case errors.Is(context.Cause(ctx), ErrStripeTimeout):
		res.Err = fmt.Errorf("stripe %s from %s: %w", sr, peer, ErrStripeTimeout)
	case fetchErr != nil:
		res.Err = fetchErr
	case err != nil:
		res.Err = fmt.Errorf("reconcile: %w", err)
	case ctx.Err() != nil:
		res.Err = ctx.Err()
	case res.Deferred > 0:
		res.Err = fmt.Errorf("stripe %s from %s: %d IDs deferred, %d unverified: %w", sr, peer, len(pending), unverified, ErrStripeDeferred)
	}
	return res
}

// resolvePending requests pending, IDs of a stripe the want set did not
// grant, from peer again, stripeRetry apart, at most maxPendingPasses
// times, until none is left: an ID another fetch stored meanwhile is
// skipped, one whose claim ended without the chunk is claimed for the stripe
// peer, which then delivers it, withholds it (Undelivered) or delivers it
// with a stamp this node rejects or cannot check (unverified). Only IDs the
// want set did not grant are asked for again: a peer cannot make the stripe
// ask for IDs by how it delivers them. A pass waits for the fetch rate only
// for the IDs the want set would grant (wantset.Set.Peek). It adds the
// outcome to res and returns the IDs still pending, the unverified count
// and the error of a failed fetch.
//
// While it waits between passes it releases the peer's Striped state in
// hold, and the peer's lock in locks unless locks.keep, so that round
// sessions and other stripes' sessions with the peer run meanwhile, and
// takes them again for the next pass. It holds no slot. The stripe clock
// runs throughout, so the timeout bounds the waits and the passes alike; a
// pass that stores chunks restarts its inactivity timeout.
func (r *Reconciler) resolvePending(ctx context.Context, clock *stripeClock, peer swarm.Address, locks *peerLocks, hold *peerHold, pending []storer.ChunkID, unverified int, res *StripeResult) ([]storer.ChunkID, int, error) {
	for pass := 0; len(pending) > 0 && pass < maxPendingPasses; pass++ {
		hold.releaseStripe()
		if !locks.keep {
			hold.releaseLock()
		}
		select {
		case <-ctx.Done():
		case <-r.after(r.stripeRetry):
		}
		clock.expireIfDue()
		select {
		case <-ctx.Done(): // the caller reports the cause
			return pending, unverified, nil
		default:
		}
		if err := r.holdPeer(ctx, locks, peer, hold); err != nil {
			return pending, unverified, fmt.Errorf("waiting for the peer: %w", err)
		}
		clock.expireIfDue() // taking the peer back may have outlasted the timeout
		select {
		case <-ctx.Done():
			return pending, unverified, nil
		default:
		}
		out, err := r.fetchFrom(ctx, peer, pending, true, clock.progress)
		res.Stored += out.stored
		res.Undelivered += out.undelivered
		res.outsideRoot += out.outsideRoot
		unverified += out.unverified
		if err != nil {
			return out.pending, unverified, fmt.Errorf("fetch: %w", err)
		}
		pending = out.pending
	}
	return pending, unverified, nil
}

// peerHold is what a stripe attempt holds on its peer: the peer's stripe
// lock (peerLocks) and its Striped state (acquireStripe). Either is nil when
// not held.
type peerHold struct {
	unlock, unstripe func()
}

// release frees what h holds: the Striped state, then the lock.
func (h *peerHold) release() {
	h.releaseStripe()
	h.releaseLock()
}

// releaseStripe frees the Striped state, if h holds it.
func (h *peerHold) releaseStripe() {
	if h.unstripe != nil {
		h.unstripe()
		h.unstripe = nil
	}
}

// releaseLock frees the lock, if h holds it.
func (h *peerHold) releaseLock() {
	if h.unlock != nil {
		h.unlock()
		h.unlock = nil
	}
}

// holdPeer takes peer's lock in locks, unless h holds it, then moves the
// peer to Striped, into h, waiting for each until ctx ends; on an error h
// holds nothing.
func (r *Reconciler) holdPeer(ctx context.Context, locks *peerLocks, peer swarm.Address, h *peerHold) (err error) {
	if h.unlock == nil {
		if h.unlock, err = locks.acquire(ctx, peer); err != nil {
			return err
		}
	}
	if h.unstripe, err = r.acquireStripe(ctx, peer); err != nil {
		h.release()
		return err
	}
	return nil
}

// openBackoffReporter is implemented by a Service that backs off a peer's
// fetch stream opens after a failed one (*rangesync.Service). Stripes treat
// a peer whose opens back off as Busy, and are planned and verified on other
// peers when they can.
type openBackoffReporter interface {
	FetchOpensBackOff(peer swarm.Address) bool
}

var _ openBackoffReporter = (*rangesync.Service)(nil)

// opensBackOff reports whether peer's fetch stream opens back off
// (openBackoffReporter), so that a fetch from it now fails at once; false
// without an openBackoffReporter.
func (r *Reconciler) opensBackOff(peer swarm.Address) bool {
	b, ok := r.svc.(openBackoffReporter)
	return ok && b.FetchOpensBackOff(peer)
}

// stripeBusy reports whether err makes a stripe wait and ask its peer again,
// as Busy (maxStripeBusyRetries): a Busy answer (isBusy), or the peer's
// fetch stream opens backing off (rangesync.ErrOpenBackoff), which says
// nothing about the peer either.
func stripeBusy(err error) bool {
	return isBusy(err) || errors.Is(err, rangesync.ErrOpenBackoff)
}

// localBusy reports whether err is a Busy for a cause on this node: its index
// is being rebuilt (fingerprint.ErrBusy), or it runs MaxConcurrentSessions
// (rangesync.ErrNodeSessionLimit). The peer's session slot on this node
// taken (rangesync.ErrPeerSessionLimit) is the peer's Busy: only the peer's
// own session with this node holds it, so the peer can keep it taken.
func localBusy(err error) bool {
	return errors.Is(err, fingerprint.ErrBusy) || errors.Is(err, rangesync.ErrNodeSessionLimit)
}

// acquireStripeSlots takes a stripe slot, then a session slot, waiting for
// each until ctx ends. release frees both.
func (r *Reconciler) acquireStripeSlots(ctx context.Context) (release func(), err error) {
	releaseStripe, err := r.acquireStripeSlot(ctx)
	if err != nil {
		return nil, err
	}
	releaseSlot, err := r.acquireSlot(ctx)
	if err != nil {
		releaseStripe()
		return nil, err
	}
	return func() {
		releaseSlot()
		releaseStripe()
	}, nil
}

// stripeClock is the stripe timeout of one attempt, on the reconciler's
// clock (Options.Now and Options.After). It runs while resumed, and expire is
// called once, when the clock has run idle since the attempt last made
// progress (a chunk its fetches stored: progress) or since it started, or
// when it has run capacity in all, whichever comes first; capped then
// reports the latter. While it runs, a goroutine waits for the expiry, and,
// when a progress moved it, for the new expiry; pause ends it, and the
// attempt pauses the clock when it ends. The attempt's goroutine drives
// resume and pause; expireIfDue and progress are also called on the
// goroutines of its fetches.
//
// The inactivity timeout supersedes the attempt timeout of T4.3 (an attempt
// ended StripeTimeout after its clock started, however much it stored),
// which restarted every stripe that outlasted it and so listed again what
// the attempt had listed and not yet fetched (T4.7 part 1 fix round 1).
type stripeClock struct {
	now      func() time.Time
	after    func(time.Duration) <-chan time.Time
	expire   func()
	idle     time.Duration // the inactivity timeout
	capacity time.Duration // the most the clock runs

	mu sync.Mutex
	// idleLeft and capLeft are what is left of idle and capacity at since
	// while the clock runs, and now while it is paused.
	idleLeft, capLeft time.Duration
	since             time.Time
	stop              chan struct{} // closed by pause; nil while paused
	expired, capped   bool
}

// newStripeClock returns a paused stripeClock.
func newStripeClock(now func() time.Time, after func(time.Duration) <-chan time.Time, idle, capacity time.Duration, expire func()) *stripeClock {
	return &stripeClock{now: now, after: after, expire: expire, idle: idle, capacity: capacity, idleLeft: idle, capLeft: capacity}
}

// resume starts the clock; it is a no-op when the clock is running.
func (c *stripeClock) resume() {
	c.mu.Lock()
	if c.stop != nil {
		c.mu.Unlock()
		return
	}
	c.since = c.now()
	stop := make(chan struct{})
	c.stop = stop
	left := c.leftLocked()
	c.mu.Unlock()
	go c.wait(stop, c.after(max(left, 0)))
}

// wait is the goroutine of a resumed stretch of the clock: it expires the
// clock at timer, or waits again when a progress moved the expiry, until
// stop is closed.
func (c *stripeClock) wait(stop chan struct{}, timer <-chan time.Time) {
	for {
		select {
		case <-timer:
		case <-stop:
			return
		}
		c.mu.Lock()
		if c.stop != stop { // paused before this goroutine saw the expiry
			c.mu.Unlock()
			return
		}
		left := c.leftLocked()
		first := left <= 0 && c.expireLocked()
		c.mu.Unlock()
		if left <= 0 {
			if first {
				c.expire()
			}
			return
		}
		timer = c.after(left)
	}
}

// leftLocked returns the time left until the running clock expires. c.mu
// must be held.
func (c *stripeClock) leftLocked() time.Duration {
	return min(c.idleLeft, c.capLeft) - c.now().Sub(c.since)
}

// expireLocked records the expiry of the running clock, and whether it ran
// out of capacity, and reports whether it had not expired before. c.mu must
// be held.
func (c *stripeClock) expireLocked() bool {
	if c.expired {
		return false
	}
	c.expired = true
	c.capped = c.capLeft-c.now().Sub(c.since) <= 0
	return true
}

// pause stops the clock; it is a no-op when the clock is not running.
func (c *stripeClock) pause() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stop == nil {
		return
	}
	close(c.stop)
	c.stop = nil
	ran := c.now().Sub(c.since)
	c.idleLeft -= ran
	c.capLeft -= ran
}

// progress restarts the inactivity timeout: a fetch of the attempt stored a
// chunk. The capacity keeps running.
func (c *stripeClock) progress() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stop != nil {
		now := c.now()
		c.capLeft -= now.Sub(c.since)
		c.since = now
	}
	c.idleLeft = c.idle
}

// expireIfDue calls expire when the running clock has run out, whether or
// not its goroutine has seen the expiry yet. The attempt checks it after each
// session answer, after each wait the clock runs through (for its peer, for
// its slots, to retry, to take its peer back for an ask) and before each
// fetch of a listed batch, so that it starts no session, no ask and no fetch
// past its timeout. A fetch already running ends once the clock's goroutine
// cancels it.
func (c *stripeClock) expireIfDue() {
	c.mu.Lock()
	first := c.stop != nil && c.leftLocked() <= 0 && c.expireLocked()
	c.mu.Unlock()
	if first {
		c.expire()
	}
}

// ran returns how long the clock has run.
func (c *stripeClock) ran() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.capacity - c.capLeft
	if c.stop != nil {
		d += c.now().Sub(c.since)
	}
	return d
}

// isCapped reports whether the clock expired at its capacity.
func (c *stripeClock) isCapped() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.capped
}

// stripeJob is a range queued for or in a striped transfer (a registry
// entry); started, if not nil, is called once when its transfer has its
// first wants or ends; bootstrap marks the bootstrap transfer of the root.
type stripeJob struct {
	rg        fingerprint.Range
	started   func()
	bootstrap bool
	// radius marks the transfer of the new half after a radius decrease;
	// ctx, if not nil, bounds the job (canceled on a radius increase).
	radius bool
	ctx    context.Context
	// own marks a job this node decided on from its own state: the
	// bootstrap of its root (IsBootstrap, against the median of its peers'
	// counts) and a radius decrease's new half. Every other job is a
	// peer's claim: a range a session's Striper took over because the
	// peer's Fingerprint counted far more than this node holds, or a
	// session root the rounds' check found far behind the peer's Status
	// (LacksSessionRoot); peer is that peer (zero only in tests). Only own
	// jobs lower the sessions' threshold to 0 (Transferring); an own job
	// is queued before the claims and makes way for itself by dropping
	// them (preemptLocked); the claims are capped per peer
	// (maxPeerStripes). So a peer that claims a far larger count than it
	// holds neither stops this node's round sessions nor keeps its own
	// transfers from running (T4.7 part 1 fix round 1).
	own  bool
	peer swarm.Address
	// cancel ends the transfer of a claim in flight; the stripe worker
	// sets it.
	cancel context.CancelFunc
}

// enqueueStripes registers the ranges the Striper of a session this node
// opened with peer took over, as peer's claims (registerStripeLocked), at
// most maxSessionStripes of them, and reports whether every range was
// registered. The others are dropped: a later session offers them again.
func (r *Reconciler) enqueueStripes(peer swarm.Address, ranges []fingerprint.Range) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	all, n := true, 0
	for _, rg := range ranges {
		if n == maxSessionStripes {
			all = false
			break
		}
		before := len(r.stripeRegistry)
		if !r.registerStripeLocked(&stripeJob{rg: rg, peer: peer}) {
			all = false
		}
		if len(r.stripeRegistry) > before {
			n++
		}
	}
	return all
}

// servedStriper is the rangesync.TransferStriper of one session this node
// serves with peer (Reconciler.ServedStriper). It is used on the session's
// goroutine only.
type servedStriper struct {
	r    *Reconciler
	peer swarm.Address
	// registered counts the ranges the session registered
	// (maxSessionStripes).
	registered int
}

var _ rangesync.TransferStriper = (*servedStriper)(nil)

// ServedStriper returns the Striper of a session this node serves with peer
// (rangesync.Options.Striper). It takes over every range the session offers:
// the peer's Fingerprint of the range counts more than LargeDiff above this
// node's, or more at all inside this node's own transfer (Transferring)
// (candidate A of AC-B2, T4.7 part 1), so that the session answers nothing
// for the range and the peer lists none of its IDs to this node. It
// registers the range for a striped transfer as the peer's claim
// (registerStripeLocked; at most maxSessionStripes per session and
// maxPeerStripes per peer), unless the bootstrap plan holds it: while a
// bootstrap transfer is queued or in flight, and while this node is
// bootstrapping (IsBootstrap), since the next round registers the bootstrap
// of its root, which would drop the claim. A range that is not registered
// (covered by a registered one, past a bound, after Close) is offered again
// by a later session.
//
// The registry holds SyncRate for as long as a claim is registered, whatever
// its transfer stores: the stripe-registry SyncRate pin (T4.7 part 3), which
// a peer claiming a far larger count reaches through the sessions this node
// serves too, at most maxPeerStripes ranges at a time.
func (r *Reconciler) ServedStriper(peer swarm.Address) rangesync.Striper {
	return &servedStriper{r: r, peer: peer}
}

// Stripe takes rg over and registers it (ServedStriper).
func (s *servedStriper) Stripe(rg fingerprint.Range, _, _ uint64) bool {
	r := s.r
	if r.ctx.Err() != nil {
		return true
	}
	if local, err := r.svc.LocalStatus(); err == nil && IsBootstrap(local, r.peerStatuses()) {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.foundDiff = true
	if r.bootstrapping || s.registered == maxSessionStripes {
		return true
	}
	before := len(r.stripeRegistry)
	r.registerStripeLocked(&stripeJob{rg: rg, peer: s.peer})
	if len(r.stripeRegistry) > before {
		s.registered++
	}
	return true
}

// Transferring is Reconciler.Transferring.
func (s *servedStriper) Transferring(rg fingerprint.Range) bool {
	return s.r.Transferring(rg)
}

// Transferring reports whether this node's own striped transfers (queued
// or in flight, stripeJob.own: its bootstrap and a radius decrease's new
// half) cover rg, together: the Striper of the sessions this node serves
// and the StripeCollector of its round sessions ask it. In such a range the
// sessions decline whatever this node lacks, so that no peer lists what the
// transfer lists, also in its tail, below LargeDiff (T4.7 part 1). A peer's
// claim does not count (T4.7 part 1 fix round 1): a peer whose Fingerprint
// or Status claims a far larger count would otherwise stop this node's
// round sessions, and its reconciling of small differences, over the
// claimed range with every peer. A range that only partly lies in the own
// transfers is decided with the session's threshold: the side that splits
// it must send all its children (the frozen wire), and the holder decides
// on them, so declining the part would decline the rest too.
func (r *Reconciler) Transferring(rg fingerprint.Range) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.transferringLocked(rg)
}

// transferringLocked is Transferring; r.mu must be held.
func (r *Reconciler) transferringLocked(rg fingerprint.Range) bool {
	var own []fingerprint.Range
	for _, j := range r.stripeRegistry {
		if j.own {
			own = append(own, j.rg)
		}
	}
	switch {
	case slices.ContainsFunc(own, func(o fingerprint.Range) bool { return o.ContainsRange(rg) }):
		return true
	case !slices.ContainsFunc(own, rg.ContainsRange):
		return false
	}
	_, whole := trimStripe(rg, own, 0) // no part left uncovered
	return whole
}

// stripeRangesLocked returns the ranges of the registry. r.mu must be held.
func (r *Reconciler) stripeRangesLocked() []fingerprint.Range {
	ranges := make([]fingerprint.Range, len(r.stripeRegistry))
	for i, j := range r.stripeRegistry {
		ranges[i] = j.rg
	}
	return ranges
}

// peerStripesLocked returns the number of peer's claims in the registry.
// r.mu must be held.
func (r *Reconciler) peerStripesLocked(peer swarm.Address) int {
	n := 0
	for _, j := range r.stripeRegistry {
		if !j.own && j.peer.Equal(peer) {
			n++
		}
	}
	return n
}

// registerStripeLocked queues job for the stripe worker, so that no range is
// transferred twice. Prefix ranges overlap only by containment:
//   - a range inside a registered (queued or in-flight) range is not
//     registered: the registered one covers it;
//   - a session's range (a claim, with no bookkeeping of its own) that
//     contains registered ranges is trimmed to the parts they leave
//     uncovered (trimStripe), each queued as a job of its own;
//   - an own job (bootstrap or radius) is not registered while it overlaps
//     another own job: its bookkeeping (started, the radius half and its
//     cancel) is that of one job, and its round tries again. Otherwise the
//     claims make way for it (preemptLocked), and it is queued before them.
//
// What a range that is not registered misses shows up again in a later
// round. Past maxStripeRegistry ranges, or a claim's peer's maxPeerStripes,
// a range, or a trimmed part, is dropped (counted as
// stripes_total{dropped}). It reports whether all of job's range was
// registered.
func (r *Reconciler) registerStripeLocked(job *stripeJob) bool {
	job.rg = fingerprint.NewRange(job.rg.Prefix, job.rg.Bits)
	rg := job.rg
	if job.own {
		if slices.ContainsFunc(r.stripeRegistry, func(o *stripeJob) bool {
			return o.own && (o.rg.ContainsRange(rg) || rg.ContainsRange(o.rg))
		}) {
			return false
		}
		r.preemptLocked(rg)
	}
	ranges := r.stripeRangesLocked()
	if slices.ContainsFunc(ranges, func(o fingerprint.Range) bool { return o.ContainsRange(rg) }) {
		return false
	}
	room := maxStripeRegistry - len(ranges)
	if !job.own && !job.peer.IsZero() {
		room = min(room, maxPeerStripes-r.peerStripesLocked(job.peer))
	}
	if slices.ContainsFunc(ranges, rg.ContainsRange) {
		if job.own || job.started != nil || job.ctx != nil {
			return false
		}
		parts, whole := trimStripe(rg, ranges, max(room, 0))
		if !whole {
			r.metrics.Stripes.WithLabelValues(stripeDropped).Inc()
			r.logger.Debug("stripe range trimmed past a registry bound", "range", rg, "parts", len(parts), "peer_address", job.peer)
		}
		if len(parts) == 0 {
			return false
		}
		defer r.wakeStripes()
		for _, p := range parts {
			j := *job
			j.rg = p
			r.addStripeLocked(&j)
		}
		return whole
	}
	if room <= 0 {
		r.metrics.Stripes.WithLabelValues(stripeDropped).Inc()
		r.logger.Debug("stripe range dropped", "range", rg, "peer_address", job.peer)
		return false
	}
	r.addStripeLocked(job)
	r.wakeStripes()
	return true
}

// addStripeLocked adds job to the registry and the queue: an own job after
// the own jobs queued before it, ahead of the claims, and a claim last.
// r.mu must be held.
func (r *Reconciler) addStripeLocked(job *stripeJob) {
	r.stripeRegistry = append(r.stripeRegistry, job)
	i := len(r.stripeQueue)
	if job.own {
		i = 0
		for i < len(r.stripeQueue) && r.stripeQueue[i].own {
			i++
		}
	}
	r.stripeQueue = slices.Insert(r.stripeQueue, i, job)
}

// preemptLocked makes way for an own job of rg: it drops from the registry
// and the queue the claims that overlap rg (contain it or lie inside it),
// and ends the claim in flight, whatever its range, since the one stripe
// worker would run it first for as long as its peers keep it going. A
// dropped claim's peers offer what it did not transfer again in later
// sessions. r.mu must be held.
func (r *Reconciler) preemptLocked(rg fingerprint.Range) {
	running := r.stripeRunning
	if running != nil && (running.own || running.cancel == nil) {
		running = nil
	}
	if running != nil {
		running.cancel()
	}
	drop := func(j *stripeJob) bool {
		return !j.own && (j == running || j.rg.ContainsRange(rg) || rg.ContainsRange(j.rg))
	}
	if n := len(r.stripeRegistry); slices.ContainsFunc(r.stripeRegistry, drop) {
		r.stripeRegistry = slices.DeleteFunc(r.stripeRegistry, drop)
		r.stripeQueue = slices.DeleteFunc(r.stripeQueue, drop)
		r.logger.Debug("stripe claims dropped for an own transfer", "range", rg, "dropped", n-len(r.stripeRegistry))
	}
}

// wakeStripes wakes the stripe worker.
func (r *Reconciler) wakeStripes() {
	select {
	case r.stripeWake <- struct{}{}:
	default:
	}
}

// trimStripe returns the parts of rg that no range of registry covers, as the
// fewest prefix ranges, in address order: rg is split into its two halves
// until each part is either covered by a registered range or holds none. It
// returns at most room parts, and whole reports whether they are all the
// parts; it stops looking once it has room parts, so its work is bounded by
// room and the size of registry, not by the depth of the registered ranges.
// No range of registry may cover rg.
func trimStripe(rg fingerprint.Range, registry []fingerprint.Range, room int) (parts []fingerprint.Range, whole bool) {
	whole = true
	var walk func(r fingerprint.Range)
	walk = func(r fingerprint.Range) {
		switch {
		case !whole:
		case slices.ContainsFunc(registry, func(o fingerprint.Range) bool { return o.ContainsRange(r) }):
			// covered by a registered range
		case !slices.ContainsFunc(registry, r.ContainsRange):
			if len(parts) == room {
				whole = false
				return
			}
			parts = append(parts, r)
		default:
			walk(r.Child(1, 0))
			walk(r.Child(1, 1))
		}
	}
	walk(rg)
	return parts, whole
}

// planCatchUp registers the striped transfer a round's Status local calls
// for, and reports whether a bootstrap transfer is queued or in flight, in
// which case the round starts no per-peer session. An empty or far-behind
// node transfers its root striped (§7.4, IsBootstrap) instead of one full
// listing per peer. After a bootstrap transfer that did not complete, one
// round runs the per-peer sessions before the next try. After a radius
// decrease (radiusPending) only the new half is striped (§7.5), and the node
// is not bootstrapping however far its count is behind. A queued bootstrap
// counts as a pending reconcile for the start-up guard until its first wants
// arrive or it ends.
func (r *Reconciler) planCatchUp(local rangesync.Status, radiusPending bool) (bootstrap bool) {
	if radiusPending {
		r.enqueueRadiusHalf()
		return false
	}
	if !IsBootstrap(local, r.peerStatuses()) {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case r.bootstrapping:
		return true
	case r.bootstrapFailed:
		r.bootstrapFailed = false
		return false
	}
	started := func() {
		r.mu.Lock()
		r.pendingReconcile--
		r.checkGuardLocked()
		r.mu.Unlock()
	}
	if !r.registerStripeLocked(&stripeJob{rg: local.Root, started: started, bootstrap: true, own: true}) {
		return false
	}
	r.bootstrapping = true
	r.pendingReconcile++
	return true
}

// stripeWorker plans and transfers the queued ranges, one at a time, across
// the peers with a Status, until Close. A claim runs under a context of its
// own (stripeJob.cancel), which an own job that preempts it ends. On Close it
// calls started of the jobs still queued, so that no pending reconcile is
// left counted.
func (r *Reconciler) stripeWorker() {
	defer func() {
		r.mu.Lock()
		queued := r.stripeQueue
		r.stripeQueue = nil
		r.stripeRegistry = nil // nothing is transferred after Close
		r.stripeRunning = nil
		r.mu.Unlock()
		for _, job := range queued {
			if job.started != nil {
				job.started()
			}
		}
	}()
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
			ctx, cancel := r.ctx, context.CancelFunc(func() {})
			if job.ctx != nil {
				ctx = job.ctx
			}
			if !job.own {
				ctx, cancel = context.WithCancel(ctx)
				job.cancel = cancel
			}
			r.stripeRunning = job
			r.mu.Unlock()

			results := r.transferStripes(ctx, r.planStripes(job.rg), job.started, job.bootstrap)
			// the verification round, before leaving bootstrap (§7.4)
			r.VerifyStripes(ctx, results)
			failed := len(results) == 0 // defensive: PlanStripes returns at least one stripe
			for _, res := range results {
				if res.Err != nil {
					failed = true
					r.logger.Debug("stripe", "range", res.Range, "peer_address", res.Peer, "error", res.Err)
				}
			}

			r.mu.Lock()
			cancel()
			r.stripeRunning = nil
			// a no-op for a claim an own job dropped (preemptLocked)
			r.stripeRegistry = slices.DeleteFunc(r.stripeRegistry, func(o *stripeJob) bool { return o == job })
			if job.bootstrap {
				r.bootstrapping, r.bootstrapFailed = false, failed
			}
			if job.radius {
				r.radiusDoneLocked(job.rg, failed)
			}
			r.mu.Unlock()
		}
	}
}

// peerStatuses returns the peers the reconciler follows (admitted: neither
// Probing, Unsupported nor Removed) with their last Status. A Probing peer
// is left out: a round could admit it during a stripe session that would
// then race the round's session for the peer's session slot.
func (r *Reconciler) peerStatuses() []PeerStatus {
	r.mu.Lock()
	var addrs []swarm.Address
	for k, st := range r.peers {
		switch st.fsm.state {
		case stateIdle, stateReconciling, stateFetching, stateStriped:
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
