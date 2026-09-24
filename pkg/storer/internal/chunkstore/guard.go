// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"sync"

	"github.com/ethersphere/bee/v2/pkg/storage"
)

// LocationGuard tracks storage chunk locations that have been released (evicted or replaced)
// during an active sampling session. This protects against use-after-free and silent data
// corruption if Sharky reuses an empty slot while sampling is concurrently reading.
//
// Invariant Chain for Safe ChunkLocation Hint Retrieval:
// Direct chunk data retrieval via ChunkLocation hints (bypassing LevelDB index lookups)
// is safe against use-after-free races and Sharky slot reuse IF AND ONLY IF the following
// four invariants hold simultaneously:
//
//  1. Snapshot semantics of iterator (Phase 1):
//     When reserve sampling (or any other batch reader) iterates chunk items in Phase 1,
//     each ChunkBinItem's ChunkLocation was valid and committed at the moment of iteration.
//
//  2. Per-address mutual exclusion (c.lock(addr) in GetIntoLoc, Put, ReplaceLoc, Delete):
//     In chunkStoreTrx, all read and write operations for a given chunk address are serialized
//     using the global address locker (Multex). GetIntoLoc MUST acquire c.lock(addr) before
//     inspecting the guard or reading from Sharky.
//
//  3. MarkFreed under the same lock prior to slot release:
//     In Delete and ReplaceLoc, guard.MarkFreed(loc) is called while holding c.lock(addr)
//     BEFORE Sharky.Release(loc) is executed. Because the address lock is held throughout the
//     invalidation, any concurrent GetIntoLoc call for that address is blocked. Once unblocked,
//     GetIntoLoc will observe guard.IsFreed(loc) == true and will not read the released slot.
//
//  4. Conservative guard with fail-safe fallback:
//     If loc.IsZero(), guard == nil, !guard.SessionActive(), guard.IsFreed(loc), or if Sharky
//     direct read returns an error, GetIntoLoc immediately and transparently falls back to
//     the authoritative, index-backed GetInto(addr) retrieval.
type LocationGuard struct {
	mu     sync.RWMutex
	active int32
	freed  map[storage.ChunkLocation]struct{}
}

// NewLocationGuard returns an initialized LocationGuard.
func NewLocationGuard() *LocationGuard {
	return &LocationGuard{}
}

// StartSession marks a sampling session as active and returns a completion callback.
func (g *LocationGuard) StartSession() func() {
	if g == nil {
		return func() {}
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.active == 0 {
		g.freed = make(map[storage.ChunkLocation]struct{})
	}
	g.active++

	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.active--
			if g.active == 0 {
				g.freed = nil
			}
		})
	}
}

// MarkFreed records that a ChunkLocation was released. If no session is active, it is a no-op.
func (g *LocationGuard) MarkFreed(loc storage.ChunkLocation) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.active > 0 {
		g.freed[loc] = struct{}{}
	}
}

// IsFreed returns whether the given ChunkLocation was released during an active session.
func (g *LocationGuard) IsFreed(loc storage.ChunkLocation) bool {
	if g == nil {
		return false
	}
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.active == 0 {
		return false
	}
	_, found := g.freed[loc]
	return found
}

// SessionActive reports whether at least one sampling session is currently active.
func (g *LocationGuard) SessionActive() bool {
	if g == nil {
		return false
	}
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.active > 0
}
