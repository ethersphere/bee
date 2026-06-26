// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"math/rand/v2"
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
	pending  map[string]*pendingGossip // addressee bytestring -> buffered peers
	interval time.Duration
	jitter   time.Duration
	maxBatch int
}

type pendingGossip struct {
	addressee swarm.Address
	peers     map[string]swarm.Address // peer bytestring -> address (set semantics)
	deadline  time.Time
}

func newGossipBuffer(interval time.Duration, maxBatch int) *gossipBuffer {
	if interval == 0 {
		interval = defaultGossipCoalesceInterval
	}
	return &gossipBuffer{
		pending:  make(map[string]*pendingGossip),
		interval: interval,
		jitter:   interval / 4,
		maxBatch: maxBatch,
	}
}

// add buffers peers for the addressee. If the buffer reaches maxBatch it is
// removed and returned so the caller can flush it immediately; otherwise nil.
func (b *gossipBuffer) add(now time.Time, addressee swarm.Address, peers ...swarm.Address) *pendingGossip {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := addressee.ByteString()
	e, ok := b.pending[key]
	if !ok {
		e = &pendingGossip{
			addressee: addressee,
			peers:     make(map[string]swarm.Address),
			deadline:  now.Add(b.interval + time.Duration(rand.Int64N(int64(b.jitter)))),
		}
		b.pending[key] = e
	}
	for _, p := range peers {
		e.peers[p.ByteString()] = p
	}

	if len(e.peers) >= b.maxBatch {
		delete(b.pending, key)
		return e
	}
	return nil
}

// takeDue removes and returns all entries whose deadline has passed.
func (b *gossipBuffer) takeDue(now time.Time) []*pendingGossip {
	return b.take(func(e *pendingGossip) bool { return !e.deadline.After(now) })
}

// takeAll removes and returns everything (used for the shutdown drain).
func (b *gossipBuffer) takeAll() []*pendingGossip {
	return b.take(func(*pendingGossip) bool { return true })
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

func (b *gossipBuffer) take(match func(*pendingGossip) bool) []*pendingGossip {
	b.mu.Lock()
	defer b.mu.Unlock()

	var out []*pendingGossip
	for key, e := range b.pending {
		if match(e) {
			out = append(out, e)
			delete(b.pending, key)
		}
	}
	return out
}

func (e *pendingGossip) addresses() []swarm.Address {
	out := make([]swarm.Address, 0, len(e.peers))
	for _, p := range e.peers {
		out = append(out, p)
	}
	return out
}
