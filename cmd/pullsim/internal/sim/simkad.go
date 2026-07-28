// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"sync"

	"github.com/ethersphere/bee/v2/pkg/topology"
	kadmock "github.com/ethersphere/bee/v2/pkg/topology/kademlia/mock"
)

var _ topology.Driver = (*simKad)(nil)

// simKad is the simulator's kademlia driver. It embeds the production mock for
// everything the puller touches except the peer set, which it owns itself.
//
// The mock can only append peers (AddRevPeers) or clear all of them
// (ResetPeers). Rewiring after churn needs a replace, and clear-then-append is
// not atomic: the puller's manage loop can observe the empty list in between and
// disconnect every peer for a tick, producing a spurious resync storm. Owning
// the slice here makes the swap atomic without touching a production package.
type simKad struct {
	*kadmock.Mock

	mu    sync.Mutex
	peers []kadmock.AddrTuple
}

// newSimKad builds a driver over the given peer tuples.
func newSimKad(peers []kadmock.AddrTuple) *simKad {
	k := &simKad{Mock: kadmock.NewMockKademlia()}
	k.peers = append(k.peers, peers...)
	return k
}

// SetPeers replaces the peer set in one step and then triggers, so subscribers
// never observe an intermediate empty set.
func (k *simKad) SetPeers(peers []kadmock.AddrTuple) {
	next := make([]kadmock.AddrTuple, len(peers))
	copy(next, peers)
	k.mu.Lock()
	k.peers = next
	k.mu.Unlock()
	k.Trigger()
}

// Peers returns a copy of the current peer set.
func (k *simKad) Peers() []kadmock.AddrTuple {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]kadmock.AddrTuple, len(k.peers))
	copy(out, k.peers)
	return out
}

// EachConnectedPeerRev overrides the embedded mock so the puller iterates this
// type's peer slice. The callback runs outside the lock: the puller takes its
// own mutex in there, and holding both would order two locks needlessly.
func (k *simKad) EachConnectedPeerRev(f topology.EachPeerFunc, _ topology.Select) error {
	for _, v := range k.Peers() {
		stop, _, err := f(v.Addr, v.PO)
		if stop {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}
