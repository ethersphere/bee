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
// whatever the number of peers and bins. Work in flight adds one goroutine
// per session this node opens (at most one per peer, which also fetches the
// session's wants), per fetch of the wants a peer listed in a session this
// node served, and per retry source, three per stripe peer (its transfers,
// their fetch and the stripe clock while it runs), and at most
// maxConcurrentProbes for Status sends. They end
// with their work. So the count is O(neighbors), where the puller runs
// workers per peer and bin.
package reconciler
