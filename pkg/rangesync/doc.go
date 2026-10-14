// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package rangesync implements the rangesync protocol, which reconciles the
reserves of neighboring nodes by comparing fingerprints of address ranges
instead of exchanging per-bin cursors.

The protocol has three streams:

  - status: either side gossips the summary of its neighborhood (root range).
  - reconcile: the initiator runs one reconciliation session; the peers
    exchange Messages of range fingerprints, coded symbols and item lists until
    one side sends an empty Message.
  - fetch: the requester asks for chunks by ID and receives Deliveries.

The protocol is independent of pullsync and runs in parallel with it. The
fingerprint index lives in pkg/rangesync/fingerprint, the in-flight want set
in pkg/rangesync/wantset and the scheduling of sessions in
pkg/rangesync/reconciler.
*/
package rangesync
