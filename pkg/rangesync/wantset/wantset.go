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
	// MaxRejected is the largest number of IDs the rejected cache of a Set
	// created by New holds at once; a rejection beyond it drops the oldest.
	// It is ten seconds of the node-wide fetch rate (rangesync
	// DefaultFetchRate): a larger set of rejected IDs takes longer to fetch
	// than that, so the fetch rate bounds its refetches.
	MaxRejected = 10000
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
	// Rejected means the ID was rejected (Reject) within the reject TTL; the
	// caller skips it, through every peer.
	Rejected
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
	case Rejected:
		return "rejected"
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
// A Set also keeps a bounded cache of rejected IDs (§8.2): IDs whose delivery
// failed stamp validation, or whose stamp is older than the one the reserve
// holds, are not claimed again until the reject TTL has passed.
//
// A Set is safe for concurrent use. It starts no goroutines: expired claims
// and rejections are dropped by the calls that observe them.
type Set struct {
	deadline    time.Duration
	rejectTTL   time.Duration
	maxClaims   int
	maxPerPeer  int
	maxRejected int
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
	// rejected holds the rejected IDs, rejectList the same in the order
	// they expire (values are *rejection), which is the order of their last
	// rejection, as for expiry.
	rejected   map[fingerprint.ID]*list.Element
	rejectList *list.List
	closed     bool
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

// rejection is one rejected ID.
type rejection struct {
	id      fingerprint.ID
	expires time.Time
}

// New returns an empty Set whose claims expire after deadline (the protocol
// uses rangesync.ClaimDeadline, 30 s), whose rejections expire after
// rejectTTL (rangesync.RejectTTL, 10 min), and which holds at most MaxClaims
// IDs, at most MaxClaimsPerPeer for one peer, and at most MaxRejected
// rejected IDs. It panics if deadline or rejectTTL is not positive.
func New(deadline, rejectTTL time.Duration) *Set {
	return newSet(deadline, rejectTTL, MaxClaims, MaxClaimsPerPeer, MaxRejected, time.Now)
}

func newSet(deadline, rejectTTL time.Duration, maxClaims, maxPerPeer, maxRejected int, now func() time.Time) *Set {
	if deadline <= 0 {
		panic("wantset: claim deadline must be positive")
	}
	if rejectTTL <= 0 {
		panic("wantset: reject TTL must be positive")
	}
	return &Set{
		deadline:    deadline,
		rejectTTL:   rejectTTL,
		maxClaims:   maxClaims,
		maxPerPeer:  maxPerPeer,
		maxRejected: maxRejected,
		now:         now,
		claims:      make(map[fingerprint.ID]*list.Element),
		peers:       make(map[string]*peerClaims),
		expiry:      list.New(),
		rejected:    make(map[fingerprint.ID]*list.Element),
		rejectList:  list.New(),
	}
}

// Claim records that id is about to be requested from peer unless something
// stops it, and reports the outcome. Only Granted records a claim. The checks
// are made in the order Closed, Rejected, Held, Full, PeerFull.
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
	if s.isRejected(id, now) {
		return Rejected
	}
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

// Reject ends the claim on id, whichever peer holds it, and records id as
// rejected for the reject TTL: Claim returns Rejected for it through every
// peer. The ID's delivery failed stamp validation, or the reserve holds a
// newer stamp of the chunk (storage.ErrOverwriteNewerChunk); either holds
// whichever peer delivers it, since the ID includes the stamp. Rejecting an
// ID again restarts its TTL. When the cache holds its maximum number of IDs,
// the oldest rejection is dropped. It is a no-op on a closed set.
func (s *Set) Reject(id fingerprint.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	if e, ok := s.claims[id]; ok {
		s.drop(e)
	}
	now := s.now()
	s.expireRejected(now)
	if e, ok := s.rejected[id]; ok {
		e.Value.(*rejection).expires = now.Add(s.rejectTTL)
		s.rejectList.MoveToBack(e)
		return
	}
	if s.maxRejected <= 0 {
		return
	}
	for len(s.rejected) >= s.maxRejected {
		s.dropRejected(s.rejectList.Front())
	}
	s.rejected[id] = s.rejectList.PushBack(&rejection{id: id, expires: now.Add(s.rejectTTL)})
}

// IsRejected reports whether id was rejected within the reject TTL.
func (s *Set) IsRejected(id fingerprint.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false
	}
	now := s.now()
	s.expireRejected(now)
	return s.isRejected(id, now)
}

// RejectedLen drops the expired rejections and returns the number of
// rejected IDs.
func (s *Set) RejectedLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0
	}
	s.expireRejected(s.now())
	return len(s.rejected)
}

// HeldBy reports whether peer holds an active claim on id. A fetch asks it
// when a delivery arrives or its claim ends, to tell whether its claim
// expired meanwhile (and the ID may have been claimed through another peer).
func (s *Set) HeldBy(id fingerprint.ID, peer swarm.Address) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.claims[id]
	if !ok {
		return false
	}
	c := e.Value.(*claim)
	return c.peer.key == string(peer.Bytes()) && s.now().Before(c.expires)
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
	s.rejected = nil
	s.rejectList.Init()
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

// isRejected reports whether id has a rejection that has not expired at now.
// s.mu must be held.
func (s *Set) isRejected(id fingerprint.ID, now time.Time) bool {
	e, ok := s.rejected[id]
	if !ok {
		return false
	}
	if now.Before(e.Value.(*rejection).expires) {
		return true
	}
	// Expired, but behind a rejection that has not: the clock went back.
	s.dropRejected(e)
	return false
}

// expireRejected drops the rejections at the front of s.rejectList that
// expired at or before now. s.mu must be held.
func (s *Set) expireRejected(now time.Time) {
	for e := s.rejectList.Front(); e != nil; e = s.rejectList.Front() {
		if now.Before(e.Value.(*rejection).expires) {
			return
		}
		s.dropRejected(e)
	}
}

// dropRejected removes the rejection held in e. s.mu must be held.
func (s *Set) dropRejected(e *list.Element) {
	s.rejectList.Remove(e)
	delete(s.rejected, e.Value.(*rejection).id)
}
