// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// hopShallowPause is the time a first hop is left out for a neighborhood
	// after one shallow receipt. It doubles with each shallow receipt in a row.
	hopShallowPause = time.Minute
	// hopShallowMaxPause is the longest pause of a first hop.
	hopShallowMaxPause = 30 * time.Minute
	// hopProbeInterval is the shortest time between two probes of one neighborhood.
	hopProbeInterval = 30 * time.Second
	// hopValidIdle is the time after which a valid record without a new result is removed.
	hopValidIdle = 30 * time.Minute
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

type hopHood struct {
	peers     map[string]*hopRecord
	lastProbe time.Time
}

// hopMemory remembers, per neighborhood, which first hops gave a valid and
// which gave a shallow receipt, so that an origin does not pay again for a
// result it has already seen. A neighborhood is the set of chunk addresses
// that share the first radius bits.
type hopMemory struct {
	mtx    sync.Mutex
	now    func() time.Time
	radius uint8
	hoods  map[string]*hopHood
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

// hood returns the neighborhood of the chunk. Records made for another
// radius describe other neighborhoods, so they are dropped first.
// Must be called with the lock held.
func (m *hopMemory) hood(rad uint8, chunk swarm.Address, create bool) *hopHood {
	if rad != m.radius {
		m.radius = rad
		clear(m.hoods)
	}
	key := hoodKey(rad, chunk)
	h := m.hoods[key]
	if h == nil && create {
		h = &hopHood{peers: make(map[string]*hopRecord)}
		m.hoods[key] = h
	}
	return h
}

// recordValid notes that the peer gave a valid receipt for the neighborhood of the chunk.
func (m *hopMemory) recordValid(rad uint8, chunk, peer swarm.Address) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.hood(rad, chunk, true).peers[peer.ByteString()] = &hopRecord{peer: peer, valid: true, at: m.now()}
}

// recordShallow notes that the peer gave a shallow receipt for the neighborhood of the chunk.
// A shallow receipt that comes while the pause of the peer runs belongs to a
// push that was sent before the pause began, so it does not make the pause longer.
func (m *hopMemory) recordShallow(rad uint8, chunk, peer swarm.Address) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

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
	r.valid = false
	r.shallow++
	r.at = now
}

// forget removes the record of the peer for the neighborhood of the chunk.
// A shallow record whose pause still runs stays: the error belongs to a push
// that was sent before the pause began.
func (m *hopMemory) forget(rad uint8, chunk, peer swarm.Address) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	h := m.hood(rad, chunk, false)
	if h == nil {
		return
	}
	if r := h.peers[peer.ByteString()]; r != nil && !r.valid && m.now().Before(r.blockedUntil()) {
		return
	}
	delete(h.peers, peer.ByteString())
	if len(h.peers) == 0 {
		delete(m.hoods, hoodKey(rad, chunk))
	}
}

// view returns, for the neighborhood of the chunk, the peers with a usable
// valid record, the peers whose shallow pause still runs, and whether a probe
// of the neighborhood is due.
func (m *hopMemory) view(rad uint8, chunk swarm.Address) (valid, blocked []swarm.Address, probeDue bool) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	h := m.hood(rad, chunk, false)
	if h == nil {
		return nil, nil, true
	}

	now := m.now()
	for key, r := range h.peers {
		switch {
		case r.valid && now.Sub(r.at) >= hopValidIdle:
			delete(h.peers, key)
		case r.valid:
			valid = append(valid, r.peer)
		case now.Before(r.blockedUntil()):
			blocked = append(blocked, r.peer)
		}
	}
	if len(h.peers) == 0 {
		delete(m.hoods, hoodKey(rad, chunk))
	}

	return valid, blocked, h.lastProbe.IsZero() || now.Sub(h.lastProbe) >= hopProbeInterval
}

// markProbe notes that a probe of the neighborhood of the chunk was sent now.
func (m *hopMemory) markProbe(rad uint8, chunk swarm.Address) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.hood(rad, chunk, true).lastProbe = m.now()
}

// len returns the number of records.
func (m *hopMemory) len() int {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	var n int
	for _, h := range m.hoods {
		n += len(h.peers)
	}
	return n
}
