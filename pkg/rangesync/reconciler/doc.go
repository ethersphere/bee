// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package reconciler schedules rangesync sessions: it follows the topology,
// keeps per-peer state, runs reconciliation rounds and striped transfers, and
// reports the catch-up sync rate to the node.
//
// Two parts drive it. The per-peer state machine (§8.1, peerfsm.go, fired in
// peers.go) owns what the reconciler knows about each peer, and the round
// scheduler (§8.2, scheduler.go) decides when sessions start. The Status
// gossip (gossip.go) and the stripe worker (stripes_transfer.go) serve them.
//
// Goroutines (§2.6, P6): a started reconciler runs three long-lived
// goroutines, the round loop, the Status gossip loop and the stripe worker,
// whatever the number of peers and bins. Work in flight adds goroutines that
// end with their work: one per session this node opens (at most one per
// peer, which also fetches the session's wants), three per stripe peer (its
// transfers, their fetch and the stripe clock while it runs), at most
// maxConcurrentProbes for Status sends, one per fetch of the wants a peer
// listed in a session this node served, and one per batch of retries. The
// last two are not bounded per peer: a peer serves at most one session at a
// time, at its session rate (rangesync.PeerSessionBurst, then one per
// rangesync.PeerSessionInterval), and its retries run one fetch at a time,
// so they queue up while the node-wide fetch rate drains less than the peers
// list. What they open is bounded per peer all the same: every fetch waits
// for one of the fetch streams rangesync opens to the peer (one fewer than
// the peer's own limit) before it claims any ID. So streams are
// O(neighbors), and so are goroutines while the fetches keep up, where the
// puller runs workers per peer and bin.
package reconciler
