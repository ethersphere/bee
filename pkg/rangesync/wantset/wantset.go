// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wantset

import (
	"container/list"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// MaxClaims is the largest number of IDs a Set created by New holds at once
// (the in-flight want set budget of proposal §5.3). Claims over it are denied.
const MaxClaims = 10000

// Set is the node-wide set of IDs requested through rangesync and not yet
// received (proposal §7.8, §8.2). Before an ID is requested from a peer it is
// claimed; while the claim is active no other fetch requests it. A claim ends
// with Done or Release, or expires after the claim deadline, after which the
// ID can be claimed again, typically through another peer.
//
// A Set is safe for concurrent use. It starts no goroutines: expired claims
// are dropped by the calls that observe them.
type Set struct {
	deadline  time.Duration
	maxClaims int
	now       func() time.Time

	mu     sync.Mutex
	claims map[fingerprint.ID]*list.Element // values are *claim
	// expiry holds the active claims in the order they expire. The deadline
	// is the same for every claim and the time is read under mu, so this is
	// the order in which they were made.
	expiry *list.List
	closed bool
}

// claim is one active claim.
type claim struct {
	id fingerprint.ID
	// peer is the peer the ID is requested from. Nothing reads it yet; it
	// lets the claim be attributed to a peer when it expires.
	peer    swarm.Address
	expires time.Time
}

// New returns an empty Set whose claims expire after deadline (the protocol
// uses rangesync.ClaimDeadline, 30 s) and which holds at most MaxClaims IDs.
// It panics if deadline is not positive.
func New(deadline time.Duration) *Set {
	return newSet(deadline, MaxClaims, time.Now)
}

func newSet(deadline time.Duration, maxClaims int, now func() time.Time) *Set {
	if deadline <= 0 {
		panic("wantset: claim deadline must be positive")
	}
	return &Set{
		deadline:  deadline,
		maxClaims: maxClaims,
		now:       now,
		claims:    make(map[fingerprint.ID]*list.Element),
		expiry:    list.New(),
	}
}

// Claim records that id is about to be requested from peer and reports
// whether the caller may request it. It returns false, and records nothing,
// if id already has an active claim (through any peer, peer included), if the
// set holds its maximum number of IDs, or if the set is closed. A denied ID
// is skipped by the caller; it can be claimed again once the active claim
// ends or expires, or once a slot frees up.
func (s *Set) Claim(id fingerprint.ID, peer swarm.Address) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false
	}
	now := s.now()
	s.expire(now)
	if _, ok := s.claims[id]; ok {
		return false
	}
	if len(s.claims) >= s.maxClaims {
		return false
	}
	s.claims[id] = s.expiry.PushBack(&claim{
		id:      id,
		peer:    peer,
		expires: now.Add(s.deadline),
	})
	return true
}

// Done ends the claim on id because the chunk was stored (or Put returned
// ErrOverwriteNewerChunk, so a newer stamp is already held). The claim ends
// whichever peer holds it; Done is a no-op if id has no active claim.
func (s *Set) Done(id fingerprint.ID) {
	s.remove(id)
}

// Release ends the claim on id because the chunk was not delivered or was
// invalid, so that it can be claimed again through another peer. The claim
// ends whichever peer holds it; Release is a no-op if id has no active claim.
func (s *Set) Release(id fingerprint.ID) {
	s.remove(id)
}

// Len returns the number of active claims.
func (s *Set) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0
	}
	s.expire(s.now())
	return len(s.claims)
}

// Close drops every claim. Afterwards Claim always returns false and Len
// returns 0. Close is idempotent and always returns nil.
func (s *Set) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	s.claims = nil
	s.expiry.Init()
	return nil
}

func (s *Set) remove(id fingerprint.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.claims[id]; ok {
		s.expiry.Remove(e)
		delete(s.claims, id)
	}
}

// expire drops the claims that expired at or before now. s.mu must be held.
func (s *Set) expire(now time.Time) {
	for e := s.expiry.Front(); e != nil; e = s.expiry.Front() {
		c := e.Value.(*claim)
		if now.Before(c.expires) {
			return
		}
		s.expiry.Remove(e)
		delete(s.claims, c.id)
	}
}
