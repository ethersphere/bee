// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher

import (
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// parkDelay is how long a deferred chunk that failed its first round
	// stays untouched in the upload queue. Pushsync skips the peers that
	// refused the chunk for as long (skiplistDur), so an earlier round would
	// end the same way.
	parkDelay = 5 * time.Minute

	// parkTTL is how long after its delay a parked entry is kept for a
	// chunk that never came back, e.g. because its upload was removed.
	parkTTL = time.Hour
)

// parked tracks deferred chunks that failed their first round and wait in the
// upload queue for their second and last one.
type parked struct {
	mu        sync.Mutex
	entries   map[[64]byte]time.Time // identity address + batch ID -> until
	delay     time.Duration
	ttl       time.Duration
	lastPrune time.Time
}

func newParked(delay, ttl time.Duration) *parked {
	return &parked{
		entries: make(map[[64]byte]time.Time),
		delay:   delay,
		ttl:     ttl,
	}
}

func parkedKey(idAddress swarm.Address, batchID []byte) [64]byte {
	var key [64]byte
	copy(key[:32], idAddress.Bytes())
	copy(key[32:], batchID)
	return key
}

// park parks the chunk for its second round. It reports false when the chunk
// was parked before; the caller then gives up on it.
func (p *parked) park(idAddress swarm.Address, batchID []byte, now time.Time) bool {
	key := parkedKey(idAddress, batchID)

	p.mu.Lock()
	defer p.mu.Unlock()

	// walk the map at most once per delay: chunksWorker waits on the same
	// lock, and a burst of failures parks many chunks at once
	if now.Sub(p.lastPrune) >= p.delay {
		for k, until := range p.entries {
			if now.After(until.Add(p.ttl)) {
				delete(p.entries, k)
			}
		}
		p.lastPrune = now
	}

	if _, ok := p.entries[key]; ok {
		return false
	}
	p.entries[key] = now.Add(p.delay)
	return true
}

// waiting reports whether the chunk must still be left alone. The entry stays
// after its delay, so that park recognises the second round.
func (p *parked) waiting(idAddress swarm.Address, batchID []byte, now time.Time) bool {
	key := parkedKey(idAddress, batchID)

	p.mu.Lock()
	defer p.mu.Unlock()

	until, ok := p.entries[key]
	return ok && now.Before(until)
}

// forget removes the chunk once it is synced, stored or given up on.
func (p *parked) forget(idAddress swarm.Address, batchID []byte) {
	key := parkedKey(idAddress, batchID)

	p.mu.Lock()
	delete(p.entries, key)
	p.mu.Unlock()
}
