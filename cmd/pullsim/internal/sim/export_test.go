// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"math/rand"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// BuildAdjacency exposes the topology builder to the external test package.
var BuildAdjacency = buildAdjacency

// ChunkAt exposes the chunk miner to the external test package.
var ChunkAt = chunkAt

// PresenceKey exposes the (address, batchID, stampHash) key to the external
// test package.
var PresenceKey = presenceKey

// MinSurvivors is the floor Churn refuses to go below.
const MinSurvivors = minSurvivors

// Adjacency returns a copy of the current adjacency list.
func (n *Network) Adjacency() [][]int {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([][]int, len(n.adj))
	for i, peers := range n.adj {
		out[i] = append([]int(nil), peers...)
	}
	return out
}

// DeficitSets exposes the per-node deficit key sets.
func (n *Network) DeficitSets() map[int]map[string]struct{} { return n.deficitSets() }

// HasHandler reports whether the transport can dial dest.
func (t *Transport) HasHandler(dest swarm.Address) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.handlers[dest.String()]
	return ok
}

// HealTracker is the heal-episode tracker, exported for tests.
type HealTracker = healTracker

// NewHealTracker builds a heal tracker with an injectable clock.
func NewHealTracker(settleAfter time.Duration, now func() time.Time) *HealTracker {
	return newHealTracker(settleAfter, now)
}

// Open registers a heal episode.
func (t *HealTracker) Open(from, to uint8, deficits map[int]map[string]struct{}) int {
	return t.open(from, to, deficits)
}

// ObservePut folds one reserve put into the open episodes.
func (t *HealTracker) ObservePut(node int, key string, source PutSource) {
	t.observePut(node, key, source)
}

// Sweep settles quiescent episodes and returns those newly settled.
func (t *HealTracker) Sweep() []Heal { return t.sweep() }

// Get returns one episode by ID.
func (t *HealTracker) Get(id int) (Heal, bool) { return t.get(id) }

// List returns the retained episodes, oldest first.
func (t *HealTracker) List() []Heal { return t.list() }

// Rand builds a deterministic rng, so tests can reproduce a rewire.
func Rand(seed int64) *rand.Rand { return rand.New(rand.NewSource(seed)) }
