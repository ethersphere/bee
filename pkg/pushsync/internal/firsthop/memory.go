// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package firsthop keeps the results of the first hops of an origin.
//
// An origin sends a chunk to one of its peers, the first hop, and gets back a
// receipt that is valid (the chunk was stored inside its neighborhood) or
// shallow (it was stored outside of it), or an error. The Memory keeps the last
// result of each first hop per neighborhood, so that the next chunk of the
// neighborhood goes to a first hop that gave a valid receipt and not to one
// that gave a shallow receipt or an error.
package firsthop

import (
	"encoding/binary"
	"slices"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// minPause is the pause of a first hop after a shallow receipt or an
	// error. It doubles with each pause in a row, up to maxPause.
	minPause = time.Minute
	maxPause = 30 * time.Minute
	// probeInterval is the shortest time between two probes of a neighborhood.
	probeInterval = 30 * time.Second
	// validIdle and pausedIdle are the times after which a record without a
	// new result is removed.
	validIdle  = 30 * time.Minute
	pausedIdle = 2 * maxPause
	// sweepInterval is the shortest time between two removals of idle records
	// in all neighborhoods.
	sweepInterval = validIdle
	// maxKeyBits is the largest number of address bits that tell two
	// neighborhoods apart. It limits the memory to 1<<maxKeyBits
	// neighborhoods: at a larger radius the neighborhoods that share these
	// bits share their records.
	maxKeyBits = 14
)

// record is the last result of one first hop for one neighborhood.
type record struct {
	peer   swarm.Address
	valid  bool
	at     time.Time
	pauses int // pauses in a row
}

// pausedUntil returns the end of the pause of a record that is not valid.
func (r *record) pausedUntil() time.Time {
	pause := minPause
	for i := 1; i < r.pauses && pause < maxPause; i++ {
		pause *= 2
	}
	return r.at.Add(min(pause, maxPause))
}

// paused reports whether the record is not valid and its pause still runs.
func (r *record) paused(now time.Time) bool {
	return !r.valid && now.Before(r.pausedUntil())
}

// idle reports whether the record had no new result for long enough to be removed.
func (r *record) idle(now time.Time) bool {
	if r.valid {
		return now.Sub(r.at) >= validIdle
	}
	return now.Sub(r.at) >= pausedIdle
}

// neighborhood holds the records of the chunk addresses that share the first
// radius bits, at most maxKeyBits.
type neighborhood struct {
	peers     map[string]*record
	lastProbe time.Time
}

// probeDue reports whether the last probe no longer limits the next one.
func (n *neighborhood) probeDue(now time.Time) bool {
	return n.lastProbe.IsZero() || now.Sub(n.lastProbe) >= probeInterval
}

// Memory keeps the last result of each first hop per neighborhood.
// It holds the records of one radius at a time. It is safe for concurrent use.
type Memory struct {
	now func() time.Time

	mtx           sync.Mutex // guards the fields below
	radius        uint8
	radiusSet     bool // false until the first use of the memory
	neighborhoods map[uint16]*neighborhood
	lastSweep     time.Time
}

// New returns an empty memory that reads the time from now.
func New(now func() time.Time) *Memory {
	return &Memory{
		now:           now,
		neighborhoods: make(map[uint16]*neighborhood),
	}
}

// View is what the memory holds about the first hops of one neighborhood.
type View struct {
	// Valid are the peers whose last receipt was valid.
	Valid []swarm.Address
	// Paused are the peers in a pause; the pause that ends first
	// comes first.
	Paused []swarm.Address
	// ProbeDue is true when the neighborhood had no probe for the probe interval.
	ProbeDue bool
}

// View returns the records of the neighborhood of the chunk. The radius of
// the call becomes the radius of the memory: the records of another radius
// are removed.
func (m *Memory) View(radius uint8, chunk swarm.Address) View {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	now := m.now()
	m.setRadius(radius)
	m.sweep(now)

	key := neighborhoodKey(radius, chunk)
	n := m.neighborhoods[key]
	if n == nil {
		return View{ProbeDue: true}
	}

	var (
		view   View
		paused []*record
	)
	for peer, r := range n.peers {
		switch {
		case r.idle(now):
			delete(n.peers, peer)
		case r.valid:
			view.Valid = append(view.Valid, r.peer)
		case r.paused(now):
			paused = append(paused, r)
		}
	}
	slices.SortFunc(paused, func(a, b *record) int {
		if c := a.pausedUntil().Compare(b.pausedUntil()); c != 0 {
			return c
		}
		c, _ := swarm.DistanceCmp(chunk, b.peer, a.peer)
		return c
	})
	for _, r := range paused {
		view.Paused = append(view.Paused, r.peer)
	}
	view.ProbeDue = n.probeDue(now)
	m.dropEmpty(key, n, now)

	return view
}

// Valid stores a valid receipt of the peer.
func (m *Memory) Valid(radius uint8, chunk, peer swarm.Address) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if !m.holdsRadius(radius) {
		return
	}
	n := m.neighborhood(neighborhoodKey(radius, chunk))
	n.peers[peer.ByteString()] = &record{peer: peer, valid: true, at: m.now()}
}

// Pause starts a pause of the peer after a shallow receipt or an error. A
// result that comes during the pause does not make the pause longer.
func (m *Memory) Pause(radius uint8, chunk, peer swarm.Address) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if !m.holdsRadius(radius) {
		return
	}
	now := m.now()
	n := m.neighborhood(neighborhoodKey(radius, chunk))
	r := n.peers[peer.ByteString()]
	if r == nil {
		r = &record{peer: peer}
		n.peers[peer.ByteString()] = r
	}
	if r.paused(now) {
		return
	}
	// idle record that View did not remove yet
	if !r.valid && r.idle(now) {
		r.pauses = 0
	}
	r.valid = false
	r.pauses++
	r.at = now
}

// Forget removes the record of the peer, except a record whose pause still runs.
func (m *Memory) Forget(radius uint8, chunk, peer swarm.Address) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if !m.holdsRadius(radius) {
		return
	}
	key := neighborhoodKey(radius, chunk)
	n := m.neighborhoods[key]
	if n == nil {
		return
	}
	now := m.now()
	if r := n.peers[peer.ByteString()]; r != nil && r.paused(now) {
		return
	}
	delete(n.peers, peer.ByteString())
	m.dropEmpty(key, n, now)
}

// ClaimProbe reports whether a probe of the neighborhood of the chunk is due,
// and stores the time of the probe when it is. Of the callers that run at the
// same time, only one gets true.
func (m *Memory) ClaimProbe(radius uint8, chunk swarm.Address) bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if !m.holdsRadius(radius) {
		return false
	}
	now := m.now()
	n := m.neighborhood(neighborhoodKey(radius, chunk))
	if !n.probeDue(now) {
		return false
	}
	n.lastProbe = now
	return true
}

// Size returns the number of neighborhoods and the number of records.
func (m *Memory) Size() (neighborhoods, records int) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	for _, n := range m.neighborhoods {
		records += len(n.peers)
	}
	return len(m.neighborhoods), records
}

// setRadius makes radius the radius of the memory and removes the records of
// another radius.
// Must be called with the lock held.
func (m *Memory) setRadius(radius uint8) {
	if m.radiusSet && radius == m.radius {
		return
	}
	m.radius, m.radiusSet = radius, true
	clear(m.neighborhoods)
}

// holdsRadius reports whether the memory holds the records of the radius.
// A result does not change the radius of a memory that has one, so that a
// late result of a push that began at the old radius is ignored, also when
// the memory has no records.
// Must be called with the lock held.
func (m *Memory) holdsRadius(radius uint8) bool {
	if !m.radiusSet {
		m.setRadius(radius)
	}
	return radius == m.radius
}

// neighborhood returns the neighborhood of the key and makes it when the
// memory does not have it.
// Must be called with the lock held.
func (m *Memory) neighborhood(key uint16) *neighborhood {
	n := m.neighborhoods[key]
	if n == nil {
		n = &neighborhood{peers: make(map[string]*record)}
		m.neighborhoods[key] = n
	}
	return n
}

// dropEmpty removes the neighborhood when it has no records and its last
// probe no longer limits the next one.
// Must be called with the lock held.
func (m *Memory) dropEmpty(key uint16, n *neighborhood, now time.Time) {
	if len(n.peers) == 0 && n.probeDue(now) {
		delete(m.neighborhoods, key)
	}
}

// sweep removes the idle records of all neighborhoods, at most once per
// sweepInterval. Without it the records of a neighborhood that gets no more
// chunks would stay until the radius changes.
// Must be called with the lock held.
func (m *Memory) sweep(now time.Time) {
	if now.Sub(m.lastSweep) < sweepInterval {
		return
	}
	m.lastSweep = now
	for key, n := range m.neighborhoods {
		for peer, r := range n.peers {
			if r.idle(now) {
				delete(n.peers, peer)
			}
		}
		m.dropEmpty(key, n, now)
	}
}

// neighborhoodKey returns the first radius bits of the chunk address, at most
// maxKeyBits, as a number. A memory holds the records of one radius, so the
// number tells the neighborhoods apart.
func neighborhoodKey(radius uint8, chunk swarm.Address) uint16 {
	b := chunk.Bytes()
	if len(b) < 2 {
		return 0
	}
	return binary.BigEndian.Uint16(b) >> (16 - min(radius, maxKeyBits))
}
