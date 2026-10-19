// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wantset

import "time"

// NewWithClock returns a Set with the claim deadline deadline and the reject
// TTL rejectTTL that holds at most maxClaims IDs, at most maxPerPeer of them
// for one peer, at most maxRejected rejected IDs, and reads the current time
// from now.
func NewWithClock(deadline, rejectTTL time.Duration, maxClaims, maxPerPeer, maxRejected int, now func() time.Time) *Set {
	return newSet(deadline, rejectTTL, maxClaims, maxPerPeer, maxRejected, now)
}
