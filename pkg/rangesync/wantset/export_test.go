// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wantset

import "time"

// NewWithClock returns a Set with the claim deadline deadline that holds at
// most maxClaims IDs, at most maxPerPeer of them for one peer, and reads the
// current time from now.
func NewWithClock(deadline time.Duration, maxClaims, maxPerPeer int, now func() time.Time) *Set {
	return newSet(deadline, maxClaims, maxPerPeer, now)
}
