// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"errors"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Status gossip (§6.1, §7.1). This node's Status goes to a peer:
//   - on first contact, and when its root moved (a radius change, which makes
//     the peer's session scope with this node stale): at once, to every peer
//     that speaks rangesync;
//   - when its root summary changed: to every eligible neighbor (or peer
//     whose Status this node lacks), at most every StatusMinInterval;
//   - as a heartbeat: to every peer that speaks rangesync, at least every
//     StatusMaxInterval, so that a Status a peer lost heals in that time;
//   - after a failed send: after StatusMinInterval, doubling with each
//     failed send in a row up to StatusMaxInterval (statusRetry);
//   - when a round asks for it again because this node lacks the peer's
//     Status: after StatusMinInterval;
//   - never faster than statusBurst back to back, then one per
//     PeerStatusInterval, so that it stays within the peer's inbound rate.
//
// A round and the gossip loop both take their snapshot (LocalStatus) and pick
// the peers (pickDueLocked) under pickMu, and send exactly the picked Status.
// Picking records the Status as the peer's last before it is sent, so the
// two never send the same change twice, and no pick records an older
// snapshot than an earlier one. A round picks in the critical section that
// adds new peers, so first contacts are always its own and the outcome is
// known to its evaluation. The Index's own debounce (also StatusMinInterval)
// only coalesces the loop's wake-ups; the per-peer schedule bounds the sends.
const (
	// StatusMinInterval is the shortest interval between two Statuses sent
	// to a peer for a change of the root summary.
	StatusMinInterval = rangesync.StatusMinInterval
	// StatusMaxInterval is the longest interval between two Statuses sent to
	// an eligible neighbor (the heartbeat).
	StatusMaxInterval = rangesync.StatusMaxInterval

	// statusBurst is the number of Statuses sent to a peer back to back
	// (first contact, moved roots); after it, one per PeerStatusInterval.
	// It is one below the peer's PeerStatusBurst, so that arrival jitter of
	// up to PeerStatusInterval cannot push them over the peer's rate.
	statusBurst = rangesync.PeerStatusBurst - 1
	// statusTolerance is how far ahead of a send its theoretical arrival
	// time (peerState.tat) may be (GCRA burst tolerance).
	statusTolerance = (statusBurst - 1) * rangesync.PeerStatusInterval
)

// statusPick is a peer picked for this node's Status, with the entry it was
// picked with.
type statusPick struct {
	peer swarm.Address
	st   *peerState
}

// statusDue returns when this node's Status local is next due to a peer in
// state st whose last Status this node received is remote (have reports
// whether there is one), and false when none is scheduled. A time at or
// before now is due now. The sender-side rate defers a send past the time
// the peer's rate allows it.
func statusDue(st *peerState, local, remote rangesync.Status, have bool) (time.Time, bool) {
	var at time.Time
	switch {
	case st.fsm.state == stateUnsupported, st.sending:
		return time.Time{}, false
	case st.lastSent.IsZero(), !st.last.Root.Equal(local.Root):
		at = st.lastSent
	case st.failed:
		at = st.lastSent.Add(statusRetry(st.statusFails))
	case st.resend:
		at = st.lastSent.Add(StatusMinInterval)
	case !equalStatus(st.last, local) && (!have || isEligible(local, remote)):
		at = st.lastSent.Add(StatusMinInterval)
	default:
		at = st.lastSent.Add(StatusMaxInterval)
	}
	if allowed := st.tat.Add(-statusTolerance); at.Before(allowed) {
		at = allowed
	}
	return at, true
}

// equalStatus reports whether a and b are the same Status.
func equalStatus(a, b rangesync.Status) bool {
	return a.Root.Equal(b.Root) && a.Summary == b.Summary && a.LeafBits == b.LeafBits && a.Mapping == b.Mapping
}

// pickDueLocked returns the known peers local is due to at now and marks
// them as sending it, so that no other caller picks them until probe has
// recorded the outcome. r.pickMu and r.mu must be held, and local must have
// been taken under r.pickMu.
func (r *Reconciler) pickDueLocked(local rangesync.Status, now time.Time) []statusPick {
	var due []statusPick
	for k, st := range r.peers {
		p := swarm.NewAddress([]byte(k))
		remote, have := r.svc.LastStatus(p)
		if at, ok := statusDue(st, local, remote, have); ok && !at.After(now) {
			st.sending, st.resend = true, false
			st.lastSent, st.last = now, local
			if st.tat.Before(now) {
				st.tat = now
			}
			st.tat = st.tat.Add(rangesync.PeerStatusInterval)
			due = append(due, statusPick{peer: p, st: st})
		}
	}
	return due
}

// nextStatusIn returns how long until local is next due to a known peer, at
// most StatusMaxInterval.
func (r *Reconciler) nextStatusIn(local rangesync.Status) time.Duration {
	now := r.now()
	next := now.Add(StatusMaxInterval)
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, st := range r.peers {
		remote, have := r.svc.LastStatus(swarm.NewAddress([]byte(k)))
		if at, ok := statusDue(st, local, remote, have); ok && at.Before(next) {
			next = at
		}
	}
	return max(next.Sub(now), 0)
}

// gossip sends the Statuses that are due and returns how long until the next
// one is. Until the index is built there is nothing to send; its first build
// signals a root change. A send that delivered a root the peer lacked (a
// moved root, or a first contact that failed before) triggers a round, which
// skipped the peer's session meanwhile.
func (r *Reconciler) gossip() time.Duration {
	r.pickMu.Lock()
	local, err := r.svc.LocalStatus()
	if err != nil {
		r.pickMu.Unlock()
		return StatusMaxInterval
	}
	r.mu.Lock()
	due := r.pickDueLocked(local, r.now())
	r.mu.Unlock()
	r.pickMu.Unlock()
	if r.probe(due, local) {
		r.wakeRound()
	}
	return r.nextStatusIn(local)
}

// gossipLoop runs a gossip pass on every root change of the index, when a
// round or a changed peer Status wakes it and when the next Status is due,
// until Close. A root change also triggers a round: the digests it compares
// may differ now.
func (r *Reconciler) gossipLoop(rootChanges <-chan struct{}) {
	for {
		wait := r.gossip()
		select {
		case <-r.ctx.Done():
			return
		case _, ok := <-rootChanges:
			if !ok {
				rootChanges = nil // unsubscribed elsewhere: rely on the timer
				continue
			}
			r.wakeRound()
		case <-r.gossipWake:
		case <-r.after(wait):
		}
	}
}

// wakeGossip makes the gossip loop recompute when the next Status is due.
func (r *Reconciler) wakeGossip() {
	select {
	case r.gossipWake <- struct{}{}:
	default:
	}
}

// probe sends local to the picked peers, at most maxConcurrentProbes at
// once, and records the outcome in the entry each was picked with. It
// reports whether a send delivered a root the peer did not have from this
// node before (a first contact or a moved root).
func (r *Reconciler) probe(picks []statusPick, local rangesync.Status) (newRoot bool) {
	sem := make(chan struct{}, maxConcurrentProbes)
	var wg sync.WaitGroup
	for _, pk := range picks {
		select {
		case sem <- struct{}{}:
		case <-r.ctx.Done():
			wg.Wait()
			return newRoot
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			err := r.svc.SendStatusWith(r.ctx, pk.peer, local)
			r.mu.Lock()
			defer r.mu.Unlock()
			st := pk.st
			if r.peers[pk.peer.ByteString()] != st {
				return // left meanwhile, maybe with a new entry since
			}
			st.sending = false
			switch {
			case err == nil:
				if !st.sent || !st.reached.Equal(local.Root) {
					newRoot = true
				}
				st.sent, st.failed, st.statusFails = true, false, 0
				st.reached = local.Root
				if st.rootWake != nil { // stripe sessions wait for the root
					close(st.rootWake)
					st.rootWake = nil
				}
			case errors.Is(err, rangesync.ErrProtocolUnsupported):
				if st.fsm.state != stateProbing {
					// a reconnect the peer set did not show: a new peer
					st.resetLocked()
				}
				r.fireLocked(st, eventUnsupported)
				st.verdict, st.verdictHave = r.svc.LastStatus(pk.peer)
				r.logger.Debug("peer does not support rangesync", "peer_address", pk.peer)
			default:
				// sent stays: an earlier Status of ours reached the peer
				st.failed = true
				st.statusFails++
				r.logger.Debug("send status", "peer_address", pk.peer, "failures", st.statusFails, "error", err)
			}
		}()
	}
	wg.Wait()
	return newRoot
}
