// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wantset

import (
	"container/list"
	"strconv"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// MaxClaims is the largest number of IDs a Set created by New holds at
	// once (the in-flight want set budget of proposal §5.3).
	MaxClaims = 10000
	// MaxClaimsPerPeer is the largest number of IDs a Set created by New
	// holds at once for one peer: four Want messages of 250 IDs (§6.2). It
	// keeps a peer that wins claims and stalls from denying every fetch of
	// the node until its claims expire.
	MaxClaimsPerPeer = 1000
)

// Result is the outcome of Claim.
type Result int

const (
	// Granted means the ID was claimed: the caller requests it from the peer
	// and ends the claim with Done or Release.
	Granted Result = iota + 1
	// Held means the ID has an active claim (through any peer, the given one
	// included); the caller skips it.
	Held
	// Full means the set holds its maximum number of IDs; trying another
	// peer does not help until claims end or expire.
	Full
	// PeerFull means the set holds the maximum number of IDs for this peer;
	// the ID can still be claimed through another peer.
	PeerFull
	// Closed means the set is closed.
	Closed
)

// String returns the name of r, usable as a metric label.
func (r Result) String() string {
	switch r {
	case Granted:
		return "granted"
	case Held:
		return "held"
	case Full:
		return "full"
	case PeerFull:
		return "peer_full"
	case Closed:
		return "closed"
	default:
		return "Result(" + strconv.Itoa(int(r)) + ")"
	}
}

// Set is the node-wide set of IDs requested through rangesync and not yet
// received (proposal §7.8, §8.2). Before an ID is requested from a peer it is
// claimed; while the claim is active no other fetch requests it. A claim ends
// with Done or Release, or expires after the claim deadline, after which the
// ID can be claimed again, typically through another peer.
//
// A Set is safe for concurrent use. It starts no goroutines: expired claims
// are dropped by the calls that observe them.
type Set struct {
	deadline   time.Duration
	maxClaims  int
	maxPerPeer int
	// now must not go backwards. time.Now carries a monotonic clock reading,
	// so wall-clock steps do not affect it. If now does step back (an
	// injected clock), a claim made after the step still expires on time for
	// Claim on its own ID, but it keeps counting in Len and against the caps
	// until every claim made before it has expired: by at most the step.
	now func() time.Time

	mu     sync.Mutex
	claims map[fingerprint.ID]*list.Element // values are *claim
	peers  map[string]*peerClaims           // by peer address bytes
	// expiry holds the active claims in the order they expire. The deadline
	// is the same for every claim and the time is read under mu, so this is
	// the order in which they were made.
	expiry *list.List
	closed bool
}

// peerClaims counts the active claims of one peer.
type peerClaims struct {
	key string
	n   int
}

// claim is one active claim.
type claim struct {
	id      fingerprint.ID
	peer    *peerClaims
	expires time.Time
}

// New returns an empty Set whose claims expire after deadline (the protocol
// uses rangesync.ClaimDeadline, 30 s) and which holds at most MaxClaims IDs,
// and at most MaxClaimsPerPeer for one peer. It panics if deadline is not
// positive.
func New(deadline time.Duration) *Set {
	return newSet(deadline, MaxClaims, MaxClaimsPerPeer, time.Now)
}

func newSet(deadline time.Duration, maxClaims, maxPerPeer int, now func() time.Time) *Set {
	if deadline <= 0 {
		panic("wantset: claim deadline must be positive")
	}
	return &Set{
		deadline:   deadline,
		maxClaims:  maxClaims,
		maxPerPeer: maxPerPeer,
		now:        now,
		claims:     make(map[fingerprint.ID]*list.Element),
		peers:      make(map[string]*peerClaims),
		expiry:     list.New(),
	}
}

// Claim records that id is about to be requested from peer unless something
// stops it, and reports the outcome. Only Granted records a claim. The checks
// are made in the order Closed, Held, Full, PeerFull.
//
// Callers claim right before each Want, only the IDs that Want carries.
func (s *Set) Claim(id fingerprint.ID, peer swarm.Address) Result {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return Closed
	}
	now := s.now()
	s.expire(now)
	if e, ok := s.claims[id]; ok {
		if now.Before(e.Value.(*claim).expires) {
			return Held
		}
		// Expired, but behind a claim that has not: the clock went back.
		s.drop(e)
	}
	if len(s.claims) >= s.maxClaims {
		return Full
	}
	p := s.peers[string(peer.Bytes())]
	if p != nil && p.n >= s.maxPerPeer {
		return PeerFull
	}
	if p == nil {
		p = &peerClaims{key: string(peer.Bytes())}
		s.peers[p.key] = p
	}
	p.n++
	s.claims[id] = s.expiry.PushBack(&claim{
		id:      id,
		peer:    p,
		expires: now.Add(s.deadline),
	})
	return Granted
}

// Done ends the claim on id because the chunk was stored (or Put returned
// ErrOverwriteNewerChunk, so a newer stamp is already held). Once the chunk is
// stored any claim on it is moot, so Done ends the claim whichever peer holds
// it. It is a no-op if id has no active claim.
func (s *Set) Done(id fingerprint.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.claims[id]; ok {
		s.drop(e)
	}
}

// Release ends the claim on id because the chunk was not delivered or was
// invalid, so that it can be claimed again through another peer. It ends the
// claim only if peer holds it, so a fetch that outlived its claim does not end
// a later claim on the same ID through another peer. Otherwise it is a no-op.
func (s *Set) Release(id fingerprint.ID, peer swarm.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.claims[id]; ok && e.Value.(*claim).peer.key == string(peer.Bytes()) {
		s.drop(e)
	}
}

// Len drops the expired claims and returns the number of active claims.
func (s *Set) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0
	}
	s.expire(s.now())
	return len(s.claims)
}

// Close drops every claim. Afterwards Claim always returns Closed and Len
// returns 0. Close is idempotent and always returns nil.
func (s *Set) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	s.claims = nil
	s.peers = nil
	s.expiry.Init()
	return nil
}

// expire drops the claims at the front of s.expiry that expired at or before
// now. s.mu must be held.
func (s *Set) expire(now time.Time) {
	for e := s.expiry.Front(); e != nil; e = s.expiry.Front() {
		if now.Before(e.Value.(*claim).expires) {
			return
		}
		s.drop(e)
	}
}

// drop removes the claim held in e. s.mu must be held.
func (s *Set) drop(e *list.Element) {
	c := e.Value.(*claim)
	s.expiry.Remove(e)
	delete(s.claims, c.id)
	c.peer.n--
	if c.peer.n == 0 {
		delete(s.peers, c.peer.key)
	}
}
