// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// fireLocked applies e to the machine of st at Now and reports whether it was
// a transition. A peer that leaves Reconciling or Striped wakes the stripe
// sessions waiting for it. r.mu must be held.
func (r *Reconciler) fireLocked(st *peerState, e fsmEvent) bool {
	if !st.fsm.fire(e, r.now()) {
		return false
	}
	if s := st.fsm.state; s != stateReconciling && s != stateStriped {
		st.wakeLocked()
	}
	return true
}

// wakeLocked wakes the stripe sessions waiting for the peer of st. r.mu must
// be held.
func (st *peerState) wakeLocked() {
	if st.idle != nil {
		close(st.idle)
		st.idle = nil
	}
}

// admitLocked moves the connected peers whose Statuses are exchanged (a
// Status of this node reached the peer and the peer's is known) out of
// Probing: to Idle, and on to Removed when they are not eligible (disjoint
// neighborhoods or another Mapping, isEligible), which drops them from
// sessions and stripes. A dropped peer gets a new machine once its relation to
// this node is unknown again (no Status) or it is eligible again (one of the
// roots moved). The Status gossip goes on with every peer that speaks
// rangesync. r.mu must be held.
func (r *Reconciler) admitLocked(local rangesync.Status, connected []swarm.Address) {
	for _, p := range connected {
		if st, ok := r.peers[p.ByteString()]; ok {
			remote, have := r.svc.LastStatus(p)
			r.admitPeerLocked(p, st, local, remote, have)
		}
	}
}

// admitPeerLocked is admitLocked for the peer p with entry st, whose last
// Status is remote (have reports whether there is one). r.mu must be held.
func (r *Reconciler) admitPeerLocked(p swarm.Address, st *peerState, local, remote rangesync.Status, have bool) {
	exchanged := st.sent && have
	if st.fsm.state == stateRemoved && (!exchanged || isEligible(local, remote)) {
		st.fsm = newPeerFSM()
	}
	if !exchanged {
		return
	}
	r.fireLocked(st, eventExchanged)
	if st.fsm.state == stateIdle && !isEligible(local, remote) {
		r.fireLocked(st, eventDisjoint)
		r.logger.Debug("peer dropped: no session scope", "peer_address", p, "root", local.Root, "peer_root", remote.Root)
	}
}

// localCause reports whether a session failed with err for a cause on this
// node, which says nothing about the peer: this node runs
// MaxConcurrentSessions sessions, its index is rebuilding, the two Statuses
// give no session scope (this node's root moved since the round picked the
// peer), or the peer's Status is gone or has another Mapping (it left or is
// about to be dropped). Such a session is retried by the next round without
// a backoff.
func localCause(err error) bool {
	for _, target := range []error{
		rangesync.ErrNodeSessionLimit,
		fingerprint.ErrBusy,
		rangesync.ErrDisjointRoots,
		rangesync.ErrInvalidScope,
		rangesync.ErrNoStatus,
		rangesync.ErrMappingMismatch,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// acquireStripe moves peer to Striped for a stripe session (a transfer or a
// verification), waiting while it is Reconciling or Striped, so that a round
// session and a stripe session never race for the peer's session slot
// (MaxConcurrentSessionsPerPeer). Rounds start no session with the peer while
// a stripe session waits. A peer the reconciler does not follow (unknown,
// Probing, Unsupported or Removed) is not tracked. It returns the cause of
// ctx when ctx ends first. release, called once the stripe's session and
// fetch have ended, moves the peer to Fetching, and to Idle unless the wants
// of its last round session are still being fetched.
func (r *Reconciler) acquireStripe(ctx context.Context, peer swarm.Address) (release func(), err error) {
	k := peer.ByteString()
	var waiting *peerState
	// unwait ends the wait on the entry it was registered with; r.mu must
	// be held.
	unwait := func() {
		if waiting != nil {
			waiting.stripeWaiters--
			waiting = nil
		}
	}
	for {
		r.mu.Lock()
		st, ok := r.peers[k]
		if !ok || waiting != st {
			unwait() // left, maybe with a new entry since
		}
		if !ok {
			r.mu.Unlock()
			return func() {}, nil
		}
		switch st.fsm.state {
		case stateIdle, stateFetching:
			unwait()
			r.fireLocked(st, eventStripe)
			r.mu.Unlock()
			return func() {
				r.mu.Lock()
				defer r.mu.Unlock()
				if r.peers[k] == st && r.fireLocked(st, eventStripeDone) && !st.fetching {
					r.fireLocked(st, eventFetched)
				}
			}, nil
		case stateReconciling, stateStriped:
			if waiting == nil {
				st.stripeWaiters++
				waiting = st
			}
			if st.idle == nil {
				st.idle = make(chan struct{})
			}
			idle := st.idle
			r.mu.Unlock()
			select {
			case <-idle:
			case <-ctx.Done():
				r.mu.Lock()
				unwait()
				r.mu.Unlock()
				return nil, context.Cause(ctx)
			}
		default:
			unwait()
			r.mu.Unlock()
			return func() {}, nil
		}
	}
}
