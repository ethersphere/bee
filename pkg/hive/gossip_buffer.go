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

const defaultGossipCoalesceInterval = time.Second

// gossipBuffer accumulates single-peer outbound gossip per addressee so it can be
// flushed as one batched message.
type gossipBuffer struct {
	mu       sync.Mutex
	pending  map[string]map[string]swarm.Address // addressee key -> peer key -> peer
	ready    []gossipBatch                       // full batches waiting for worker flush
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

// stagePeers buffers peers for the addressee. If the buffer reaches maxBatch the
// batch is moved to the ready queue and wakeup is requested for the worker.
func (b *gossipBuffer) stagePeers(addressee swarm.Address, peers ...swarm.Address) (wakeup bool) {
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
		b.ready = append(b.ready, gossipBatch{
			addressee: addressee,
			peers:     slices.Collect(maps.Values(peerSet)),
		})
		return true
	}
	return false
}

// takeReady removes and returns batches that filled to maxBatch.
func (b *gossipBuffer) takeReady() []gossipBatch {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := b.ready
	b.ready = nil
	return out
}

// takeAll removes and returns all buffered entries (ready and pending).
func (b *gossipBuffer) takeAll() []gossipBatch {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := b.ready
	b.ready = nil

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
	n := 0
	for _, batch := range b.ready {
		if batch.addressee.Equal(addressee) {
			continue
		}
		b.ready[n] = batch
		n++
	}
	b.ready = b.ready[:n]
}

func (b *gossipBuffer) pendingAddressees() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending) + len(b.ready)
}
