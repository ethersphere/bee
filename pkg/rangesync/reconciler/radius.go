// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"errors"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
)

// radiusState follows the local root across rounds to handle a radius
// decrease (§7.5). After the root widens, oldHalf is the child of the new
// root that holds the previous root and newHalf its sibling; while pending,
// newHalf awaits its striped transfer and the sessions whose root is the new
// root open only oldHalf. A radius increase only narrows the root: nothing is
// transferred.
type radiusState struct {
	root, oldHalf, newHalf fingerprint.Range
	known                  bool // root is set
	pending                bool // newHalf awaits or is in its striped transfer
	striping               bool // newHalf is registered for the stripe worker
}

// isBusy reports whether err is a Busy answer: the peer's (ErrPeerBusy) or
// this node's (fingerprint.ErrBusy) index is being rebuilt, or the peer's
// session slots are taken. Busy is retried after a back-off, never scored.
func isBusy(err error) bool {
	return errors.Is(err, rangesync.ErrPeerBusy) || errors.Is(err, fingerprint.ErrBusy) || errors.Is(err, rangesync.ErrSessionLimit)
}

// trackRootLocked records the local root of a round and reports whether it
// changed (the peers then need this node's new Status). A root that widens to
// contain the previous one is a radius decrease: the halves of the new root
// are recorded and the new half is pending. A pending half the root no longer
// contains (a radius increase) is dropped.
func (r *Reconciler) trackRootLocked(root fingerprint.Range) (changed bool) {
	s := &r.radius
	prev, known := s.root, s.known
	s.root, s.known = root, true
	if !known || root.Equal(prev) {
		return false
	}
	if root.Bits < prev.Bits && root.ContainsRange(prev) {
		i := root.ChildIndex(prev.Prefix, 1)
		s.oldHalf, s.newHalf = root.Child(1, i), root.Child(1, 1-i)
		s.pending = true
		r.logger.Debug("radius decreased", "root", root, "new_half", s.newHalf)
		return true
	}
	if s.pending && !root.ContainsRange(s.newHalf) {
		s.pending = false
	}
	return true
}

// enqueueRadiusHalf registers the pending new half for a striped transfer
// once a peer whose root covers it has a Status (PlanStripes on the half
// only, §7.5).
func (r *Reconciler) enqueueRadiusHalf() {
	r.mu.Lock()
	s := r.radius
	r.mu.Unlock()
	if !s.pending || s.striping {
		return
	}
	covered := false
	for _, ps := range r.peerStatuses() {
		if ps.Status.Root.ContainsRange(s.newHalf) {
			covered = true
			break
		}
	}
	if !covered {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.radius.pending && !r.radius.striping && r.radius.newHalf.Equal(s.newHalf) &&
		r.registerStripeLocked(stripeJob{rg: s.newHalf, radius: true}) {
		r.radius.striping = true
	}
}

// radiusOpenLocked returns the Open of a session with a peer whose root is
// remote while a radius decrease is pending: only the old half when the
// session root is the new root, and skip for a session root inside the new
// half (the striped transfer covers it).
func (r *Reconciler) radiusOpenLocked(local, remote rangesync.Status) (open []fingerprint.Range, skip bool) {
	s := r.radius
	if !s.pending || !s.root.Equal(local.Root) {
		return nil, false
	}
	sr, ok := rangesync.SessionRoot(local.Root, remote.Root)
	switch {
	case !ok:
		return nil, false
	case sr.Equal(local.Root):
		return []fingerprint.Range{s.oldHalf}, false
	case s.newHalf.ContainsRange(sr):
		return nil, true
	}
	return nil, false
}

// radiusDoneLocked ends the striped transfer of the new half rg. The half is
// transferred once: after a failed transfer the sessions run over the whole
// root again and what is still missing goes through their Striper.
func (r *Reconciler) radiusDoneLocked(rg fingerprint.Range) {
	r.radius.striping = false
	if r.radius.newHalf.Equal(rg) {
		r.radius.pending = false
	}
}
