// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	"slices"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// hopShallowPause is the pause of a first hop after a shallow receipt.
	// It doubles with each shallow receipt in a row, up to hopShallowMaxPause.
	hopShallowPause    = time.Minute
	hopShallowMaxPause = 30 * time.Minute
	// hopProbeInterval is the shortest time between two probes of a neighborhood.
	hopProbeInterval = 30 * time.Second
	// hopValidIdle and hopShallowIdle are the times after which a record
	// without a new result is removed.
	hopValidIdle   = 30 * time.Minute
	hopShallowIdle = 2 * hopShallowMaxPause
	// hopSweepInterval is the shortest time between two removals of idle
	// records in all neighborhoods.
	hopSweepInterval = hopValidIdle
)

// hopRecord is the last result of one first hop for one neighborhood.
type hopRecord struct {
	peer    swarm.Address
	valid   bool
	at      time.Time
	shallow int // shallow receipts in a row
}

// blockedUntil returns the end of the pause of a shallow record.
func (r *hopRecord) blockedUntil() time.Time {
	pause := hopShallowPause
	for i := 1; i < r.shallow && pause < hopShallowMaxPause; i++ {
		pause *= 2
	}
	return r.at.Add(min(pause, hopShallowMaxPause))
}

// idle reports whether the record had no new result for long enough to be removed.
func (r *hopRecord) idle(now time.Time) bool {
	if r.valid {
		return now.Sub(r.at) >= hopValidIdle
	}
	return now.Sub(r.at) >= hopShallowIdle
}

type hopHood struct {
	peers     map[string]*hopRecord
	lastProbe time.Time
}

// hopMemory keeps the last receipt result of each first hop per neighborhood
// (the chunk addresses that share the first radius bits).
type hopMemory struct {
	mtx       sync.Mutex
	now       func() time.Time
	radius    uint8
	radiusSet bool // false until the first use of the memory
	hoods     map[string]*hopHood
	lastSweep time.Time
}

func newHopMemory(now func() time.Time) *hopMemory {
	return &hopMemory{
		now:   now,
		hoods: make(map[string]*hopHood),
	}
}

// hoodKey returns the first rad bits of the chunk address.
func hoodKey(rad uint8, chunk swarm.Address) string {
	b := chunk.Bytes()
	if int(rad) >= len(b)*8 {
		return string(b)
	}
	n, rem := int(rad/8), rad%8
	if rem == 0 {
		return string(b[:n])
	}
	key := make([]byte, n+1)
	copy(key, b[:n])
	key[n] = b[n] & (0xff << (8 - rem))
	return string(key)
}

// useRadius reports whether the memory holds records of the radius. Only a
// selection (change is true) drops the records of another radius, so that a
// late result of a push that began at the old radius is ignored, also when
// the memory has no records.
// Must be called with the lock held.
func (m *hopMemory) useRadius(rad uint8, change bool) bool {
	if m.radiusSet && rad == m.radius {
		return true
	}
	if m.radiusSet && !change {
		return false
	}
	m.radius, m.radiusSet = rad, true
	clear(m.hoods)
	return true
}

// hood returns the neighborhood of the chunk.
// Must be called with the lock held.
func (m *hopMemory) hood(rad uint8, chunk swarm.Address, create bool) *hopHood {
	key := hoodKey(rad, chunk)
	h := m.hoods[key]
	if h == nil && create {
		h = &hopHood{peers: make(map[string]*hopRecord)}
		m.hoods[key] = h
	}
	return h
}

// dropEmpty removes the neighborhood when it has no records and its last
// probe no longer limits the next one.
// Must be called with the lock held.
func (m *hopMemory) dropEmpty(key string, h *hopHood, now time.Time) {
	if len(h.peers) == 0 && now.Sub(h.lastProbe) >= hopProbeInterval {
		delete(m.hoods, key)
	}
}

// sweep removes the idle records of all neighborhoods, at most once per
// hopSweepInterval. Without it the records of a neighborhood that gets no
// more chunks would stay until the radius changes.
// Must be called with the lock held.
func (m *hopMemory) sweep(now time.Time) {
	if now.Sub(m.lastSweep) < hopSweepInterval {
		return
	}
	m.lastSweep = now
	for key, h := range m.hoods {
		for peer, r := range h.peers {
			if r.idle(now) {
				delete(h.peers, peer)
			}
		}
		m.dropEmpty(key, h, now)
	}
}

// recordValid stores a valid receipt of the peer.
func (m *hopMemory) recordValid(rad uint8, chunk, peer swarm.Address) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if !m.useRadius(rad, false) {
		return
	}
	m.hood(rad, chunk, true).peers[peer.ByteString()] = &hopRecord{peer: peer, valid: true, at: m.now()}
}

// recordShallow stores a shallow receipt of the peer. A receipt that comes
// during the pause of the peer does not make the pause longer.
func (m *hopMemory) recordShallow(rad uint8, chunk, peer swarm.Address) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if !m.useRadius(rad, false) {
		return
	}
	now := m.now()
	h := m.hood(rad, chunk, true)
	r := h.peers[peer.ByteString()]
	if r == nil {
		r = &hopRecord{peer: peer}
		h.peers[peer.ByteString()] = r
	}
	if !r.valid && r.shallow > 0 && now.Before(r.blockedUntil()) {
		return
	}
	// idle record that view did not remove yet
	if !r.valid && now.Sub(r.at) >= hopShallowIdle {
		r.shallow = 0
	}
	r.valid = false
	r.shallow++
	r.at = now
}

// forget removes the record of the peer, except a shallow record whose
// pause still runs.
func (m *hopMemory) forget(rad uint8, chunk, peer swarm.Address) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if !m.useRadius(rad, false) {
		return
	}
	h := m.hood(rad, chunk, false)
	if h == nil {
		return
	}
	if r := h.peers[peer.ByteString()]; r != nil && !r.valid && m.now().Before(r.blockedUntil()) {
		return
	}
	delete(h.peers, peer.ByteString())
	m.dropEmpty(hoodKey(rad, chunk), h, m.now())
}

// view returns the peers with a valid record, the peers in a shallow pause
// (the pause that ends first comes first), and whether a probe of the
// neighborhood is due.
func (m *hopMemory) view(rad uint8, chunk swarm.Address) (valid, blocked []swarm.Address, probeDue bool) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.useRadius(rad, true)
	now := m.now()
	m.sweep(now)
	h := m.hood(rad, chunk, false)
	if h == nil {
		return nil, nil, true
	}

	var paused []*hopRecord
	for key, r := range h.peers {
		switch {
		case r.idle(now):
			delete(h.peers, key)
		case r.valid:
			valid = append(valid, r.peer)
		case now.Before(r.blockedUntil()):
			paused = append(paused, r)
		}
	}
	slices.SortFunc(paused, func(a, b *hopRecord) int {
		if c := a.blockedUntil().Compare(b.blockedUntil()); c != 0 {
			return c
		}
		c, _ := swarm.DistanceCmp(chunk, b.peer, a.peer)
		return c
	})
	for _, r := range paused {
		blocked = append(blocked, r.peer)
	}
	m.dropEmpty(hoodKey(rad, chunk), h, now)

	return valid, blocked, h.lastProbe.IsZero() || now.Sub(h.lastProbe) >= hopProbeInterval
}

// claimProbe reports whether a probe of the neighborhood is due, and stores
// the time of the probe when it is. Of the callers that run at the same time,
// only one gets true.
func (m *hopMemory) claimProbe(rad uint8, chunk swarm.Address) bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if !m.useRadius(rad, false) {
		return false
	}
	now := m.now()
	h := m.hood(rad, chunk, true)
	if !h.lastProbe.IsZero() && now.Sub(h.lastProbe) < hopProbeInterval {
		return false
	}
	h.lastProbe = now
	return true
}
