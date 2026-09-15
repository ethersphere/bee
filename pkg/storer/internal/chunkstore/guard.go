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
