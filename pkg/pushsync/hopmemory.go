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

// hopMemory keeps the last receipt result of each first hop per neighborhood
// (the chunk addresses that share the first radius bits).
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

// hood returns the neighborhood of the chunk. A radius change drops all
// records. Must be called with the lock held.
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

// recordValid stores a valid receipt of the peer.
func (m *hopMemory) recordValid(rad uint8, chunk, peer swarm.Address) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.hood(rad, chunk, true).peers[peer.ByteString()] = &hopRecord{peer: peer, valid: true, at: m.now()}
}

// recordShallow stores a shallow receipt of the peer. A receipt that comes
// during the pause of the peer does not make the pause longer.
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

// view returns the peers with a valid record, the peers in a shallow pause,
// and whether a probe of the neighborhood is due.
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
		case now.Sub(r.at) >= hopShallowIdle:
			delete(h.peers, key)
		}
	}
	if len(h.peers) == 0 {
		delete(m.hoods, hoodKey(rad, chunk))
	}

	return valid, blocked, h.lastProbe.IsZero() || now.Sub(h.lastProbe) >= hopProbeInterval
}

// markProbe stores the time of a probe of the neighborhood.
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
