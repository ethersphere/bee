// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wantset

import "time"

// NewWithClock returns a Set with the claim deadline deadline and the reject
// TTL rejectTTL that holds at most maxClaims IDs, at most maxPerPeer of them
// for one peer, at most maxRejected rejected IDs, and reads the current time
// from now.
// Each peer's share of the rejected IDs is the whole cache.
func NewWithClock(deadline, rejectTTL time.Duration, maxClaims, maxPerPeer, maxRejected int, now func() time.Time) *Set {
	return newSet(deadline, rejectTTL, maxClaims, maxPerPeer, maxRejected, maxRejected, now)
}

// NewWithRejectShare returns a Set with the default claim caps whose
// rejected cache holds at most maxRejected IDs, at most maxRejectedPerPeer
// of them rejected through one peer (RejectFrom), and reads the current time
// from now.
func NewWithRejectShare(deadline, rejectTTL time.Duration, maxRejected, maxRejectedPerPeer int, now func() time.Time) *Set {
	return newSet(deadline, rejectTTL, MaxClaims, MaxClaimsPerPeer, maxRejected, maxRejectedPerPeer, now)
}
