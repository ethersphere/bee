// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package reconciler schedules rangesync sessions: it follows the topology,
// keeps per-peer state, runs reconciliation rounds and striped transfers, and
// reports the catch-up sync rate to the node.
package reconciler
