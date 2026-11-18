// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package legacy is a frozen copy of the pullsync 1.4.0 protocol as released
// before SWIP-101, offering chunks by address, batch ID and stamp hash only.
// It is imported by tests alone, to check that the current protocol syncs with
// peers that predate SWIP-101 in both directions.
//
// Do not change its behavior. The only edits to the released code are the
// package names (the protobuf package is renamed to pullsync_legacy so both
// copies can be registered in one test binary; the wire encoding does not
// depend on it) and the reserve lookup, which calls ReserveHasStamp, the
// current name of the released ReserveHas(address, batchID, stampHash).
//
// Remove it together with the BatchID and StampHash offer fields once no peer
// depends on them.
package legacy
