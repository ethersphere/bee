// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
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
//   - as a heartbeat: to the same peers, at least every StatusMaxInterval;
//   - after a failed send, or when a round asks for it again because this
//     node lacks the peer's Status: after StatusMinInterval.
//
// A round and the gossip loop both pick the peers through pickDueLocked,
// which marks a peer's Status as sent before it is sent, so the two never
// send the same change twice. A round picks in the critical section that
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
)

// statusDue returns when this node's Status local is next due to a peer in
// state st whose last Status this node received is remote (have reports
// whether there is one), and false when none is scheduled. A time at or
// before now is due now.
func statusDue(st *peerState, local, remote rangesync.Status, have bool) (time.Time, bool) {
	switch {
	case st.unsupported, st.sending:
		return time.Time{}, false
	case st.lastSent.IsZero(), !st.last.Root.Equal(local.Root):
		return st.lastSent, true
	case !st.sent, st.resend:
		return st.lastSent.Add(StatusMinInterval), true
	case have && !isEligible(local, remote):
		return time.Time{}, false
	case !equalStatus(st.last, local):
		return st.lastSent.Add(StatusMinInterval), true
	default:
		return st.lastSent.Add(StatusMaxInterval), true
	}
}

// equalStatus reports whether a and b are the same Status.
func equalStatus(a, b rangesync.Status) bool {
	return a.Root.Equal(b.Root) && a.Summary == b.Summary && a.LeafBits == b.LeafBits && a.Mapping == b.Mapping
}

// pickDueLocked returns the known peers local is due to at now and marks
// them as sending it, so that no other caller picks them until probe has
// recorded the outcome. r.mu must be held.
func (r *Reconciler) pickDueLocked(local rangesync.Status, now time.Time) []swarm.Address {
	var due []swarm.Address
	for k, st := range r.peers {
		p := swarm.NewAddress([]byte(k))
		remote, have := r.svc.LastStatus(p)
		if at, ok := statusDue(st, local, remote, have); ok && !at.After(now) {
			st.sending, st.resend = true, false
			st.lastSent, st.last = now, local
			due = append(due, p)
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
// signals a root change.
func (r *Reconciler) gossip() time.Duration {
	local, err := r.svc.LocalStatus()
	if err != nil {
		return StatusMaxInterval
	}
	r.mu.Lock()
	due := r.pickDueLocked(local, r.now())
	r.mu.Unlock()
	r.probe(due)
	return r.nextStatusIn(local)
}

// gossipLoop runs a gossip pass on every root change of the index, when a
// round wakes it and when the next Status is due, until Close.
func (r *Reconciler) gossipLoop(rootChanges <-chan struct{}) {
	for {
		wait := r.gossip()
		select {
		case <-r.ctx.Done():
			return
		case _, ok := <-rootChanges:
			if !ok {
				rootChanges = nil // unsubscribed elsewhere: rely on the timer
			}
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
