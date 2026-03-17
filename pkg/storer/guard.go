// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"bytes"
	"slices"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// samplingGuard tracks chunk addresses that have been freed (evicted) during
// an active ReserveSample run. When the direct sharky read path is used,
// there is a race window between iterating ChunkBinItems and reading the
// sharky location: the chunk may be evicted and its slot reused in the
// meantime. By recording evicted addresses and checking them before reading,
// we avoid returning wrong data from a reused sharky slot.
type samplingGuard struct {
	mu     sync.RWMutex
	addrs  [][32]byte // sorted for binary search
	active bool
}

// Activate clears the tracker and starts recording evicted addresses.
// Called at the beginning of ReserveSample.
func (g *samplingGuard) Activate() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.addrs = g.addrs[:0]
	g.active = true
}

// Deactivate stops recording and clears the tracked addresses.
// Called when ReserveSample completes.
func (g *samplingGuard) Deactivate() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.active = false
	g.addrs = g.addrs[:0]
}

// Add records a chunk address as freed. Maintains sorted order for
// binary search. No-op if the guard is not active.
func (g *samplingGuard) Add(addr swarm.Address) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.active {
		return
	}

	var key [32]byte
	copy(key[:], addr.Bytes())

	idx, found := slices.BinarySearchFunc(g.addrs, key, func(a, b [32]byte) int {
		return bytes.Compare(a[:], b[:])
	})
	if !found {
		g.addrs = slices.Insert(g.addrs, idx, key)
	}
}

// IsFreed returns true if the given address was evicted while sampling
// is active. Uses binary search for O(log n) lookup.
func (g *samplingGuard) IsFreed(addr swarm.Address) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if !g.active {
		return false
	}

	var key [32]byte
	copy(key[:], addr.Bytes())

	_, found := slices.BinarySearchFunc(g.addrs, key, func(a, b [32]byte) int {
		return bytes.Compare(a[:], b[:])
	})
	return found
}
