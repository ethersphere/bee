// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import "github.com/ethersphere/bee/v2/pkg/swarm"

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
