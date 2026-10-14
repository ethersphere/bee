// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package wantset tracks the chunk IDs a node has requested through rangesync
// and not yet received, so the same chunk is not fetched from several peers at
// once, and the IDs it recently rejected.
package wantset
