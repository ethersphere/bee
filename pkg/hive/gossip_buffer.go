// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	defaultGossipCoalesceInterval = time.Second
	// coalesceThreshold: gossips with fewer peers are buffered; larger
	// (already-batched) messages are dispatched immediately.
	coalesceThreshold = 2
)

// gossipBuffer accumulates single-peer outbound gossip per addressee so it can be
// flushed as one batched message.
type gossipBuffer struct {
	mu       sync.Mutex
	pending  map[string]map[string]swarm.Address // addressee key -> peer key -> peer
	interval time.Duration
	maxBatch int
}

type gossipBatch struct {
	addressee swarm.Address
	peers     []swarm.Address
}

func newGossipBuffer(interval time.Duration, maxBatch int) *gossipBuffer {
	if interval == 0 {
		interval = defaultGossipCoalesceInterval
	}
	return &gossipBuffer{
		pending:  make(map[string]map[string]swarm.Address),
		interval: interval,
		maxBatch: maxBatch,
	}
}

// stagePeers buffers peers for the addressee. If the buffer reaches maxBatch it is
// removed and returned so the caller can flush it immediately.
func (b *gossipBuffer) stagePeers(addressee swarm.Address, peers ...swarm.Address) (flushPeers []swarm.Address, flush bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := addressee.ByteString()
	peerSet, ok := b.pending[key]
	if !ok {
		peerSet = make(map[string]swarm.Address)
		b.pending[key] = peerSet
	}
	for _, p := range peers {
		peerSet[p.ByteString()] = p
	}

	if len(peerSet) >= b.maxBatch {
		delete(b.pending, key)
		return slices.Collect(maps.Values(peerSet)), true
	}
	return nil, false
}

// takeAll removes and returns all buffered entries.
func (b *gossipBuffer) takeAll() []gossipBatch {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.pending) == 0 {
		return nil
	}

	out := make([]gossipBatch, 0, len(b.pending))
	for key, peerSet := range b.pending {
		out = append(out, gossipBatch{
			addressee: swarm.NewAddress([]byte(key)),
			peers:     slices.Collect(maps.Values(peerSet)),
		})
	}
	b.pending = make(map[string]map[string]swarm.Address)
	return out
}

func (b *gossipBuffer) clearAddressee(addressee swarm.Address) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.pending, addressee.ByteString())
}

func (b *gossipBuffer) pendingAddressees() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending)
}
