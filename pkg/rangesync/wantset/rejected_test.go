// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wantset_test

import (
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
)

func newRejectSet(t *testing.T, maxRejected int) (*wantset.Set, *clock) {
	t.Helper()
	c := newClock()
	s := wantset.NewWithClock(testDeadline, testRejectTTL, wantset.MaxClaims, wantset.MaxClaimsPerPeer, maxRejected, c.Now)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	return s, c
}

func checkRejected(t *testing.T, s *wantset.Set, id int, want bool) {
	t.Helper()
	if got := s.IsRejected(testID(id)); got != want {
		t.Fatalf("IsRejected(id %d) = %v, want %v", id, got, want)
	}
}

func checkRejectedLen(t *testing.T, s *wantset.Set, want int) {
	t.Helper()
	if got := s.RejectedLen(); got != want {
		t.Fatalf("RejectedLen() = %d, want %d", got, want)
	}
}

// TestReject checks that a rejected ID is not granted through any peer for
// the reject TTL, exactly, and can be claimed again afterwards.
func TestReject(t *testing.T) {
	t.Parallel()

	s, c := newRejectSet(t, wantset.MaxRejected)
	checkRejected(t, s, 1, false)
	checkRejectedLen(t, s, 0)

	s.Reject(testID(1))
	checkRejected(t, s, 1, true)
	checkRejected(t, s, 2, false)
	checkRejectedLen(t, s, 1)
	checkClaim(t, s, 1, 1, wantset.Rejected)
	checkClaim(t, s, 1, 2, wantset.Rejected)
	checkClaim(t, s, 2, 1, wantset.Granted)
	checkLen(t, s, 1)

	c.Advance(testRejectTTL - time.Nanosecond)
	checkClaim(t, s, 1, 1, wantset.Rejected)
	c.Advance(time.Nanosecond)
	checkRejected(t, s, 1, false)
	checkRejectedLen(t, s, 0)
	checkClaim(t, s, 1, 1, wantset.Granted)
}

// TestReject_EndsClaim checks that Reject ends the claim on the ID whichever
// peer holds it, freeing the holder's slot, and that a later Release or Done
// of the ended claim leaves the rejection in place.
func TestReject_EndsClaim(t *testing.T) {
	t.Parallel()

	c := newClock()
	s := wantset.NewWithClock(testDeadline, testRejectTTL, 4, 1, wantset.MaxRejected, c.Now)
	t.Cleanup(func() { _ = s.Close() })

	checkClaim(t, s, 1, 1, wantset.Granted)
	checkClaim(t, s, 2, 1, wantset.PeerFull)
	s.Reject(testID(1))
	checkLen(t, s, 0)
	checkClaim(t, s, 2, 1, wantset.Granted) // the slot of peer 1 is free

	s.Release(testID(1), testPeer(1))
	s.Done(testID(1))
	checkRejected(t, s, 1, true)
	checkClaim(t, s, 1, 3, wantset.Rejected)
}

// TestReject_Refresh checks that rejecting an ID again restarts its TTL.
func TestReject_Refresh(t *testing.T) {
	t.Parallel()

	s, c := newRejectSet(t, wantset.MaxRejected)
	s.Reject(testID(1))
	s.Reject(testID(2))
	c.Advance(testRejectTTL / 2)
	s.Reject(testID(1))
	checkRejectedLen(t, s, 2)

	c.Advance(testRejectTTL / 2) // the first rejection of 1 and that of 2 expire
	checkRejected(t, s, 1, true)
	checkRejected(t, s, 2, false)
	checkRejectedLen(t, s, 1)

	c.Advance(testRejectTTL/2 - time.Nanosecond)
	checkRejected(t, s, 1, true)
	c.Advance(time.Nanosecond)
	checkRejected(t, s, 1, false)
	checkRejectedLen(t, s, 0)
}

// TestReject_Bounded checks that the rejected cache holds at most maxRejected
// IDs: a new rejection drops the oldest one, a refreshed one counts as new.
func TestReject_Bounded(t *testing.T) {
	t.Parallel()

	const maxRejected = 4
	s, c := newRejectSet(t, maxRejected)
	for i := range maxRejected {
		s.Reject(testID(i))
		c.Advance(time.Second)
	}
	checkRejectedLen(t, s, maxRejected)

	s.Reject(testID(0)) // refreshed: 1 is now the oldest
	s.Reject(testID(maxRejected))
	checkRejectedLen(t, s, maxRejected)
	checkRejected(t, s, 1, false)
	checkClaim(t, s, 1, 1, wantset.Granted)
	for _, i := range []int{0, 2, 3, maxRejected} {
		checkRejected(t, s, i, true)
	}
}

// TestReject_DefaultBound checks the bound of New.
func TestReject_DefaultBound(t *testing.T) {
	t.Parallel()

	s := wantset.New(testDeadline, testRejectTTL)
	t.Cleanup(func() { _ = s.Close() })
	for i := range wantset.MaxRejected + 1 {
		s.Reject(testID(i))
	}
	checkRejectedLen(t, s, wantset.MaxRejected)
	checkRejected(t, s, 0, false)
	checkRejected(t, s, wantset.MaxRejected, true)
}

// TestHeldBy checks that HeldBy reports only the active claim of the given
// peer: not another peer's, not an ended or expired one.
func TestHeldBy(t *testing.T) {
	t.Parallel()

	s, c := newTestSet(t, wantset.MaxClaims, wantset.MaxClaimsPerPeer)
	held := func(id, peer int, want bool) {
		t.Helper()
		if got := s.HeldBy(testID(id), testPeer(peer)); got != want {
			t.Fatalf("HeldBy(id %d, peer %d) = %v, want %v", id, peer, got, want)
		}
	}

	held(1, 1, false)
	checkClaim(t, s, 1, 1, wantset.Granted)
	held(1, 1, true)
	held(1, 2, false)

	c.Advance(testDeadline - time.Nanosecond)
	held(1, 1, true)
	c.Advance(time.Nanosecond)
	held(1, 1, false)

	checkClaim(t, s, 1, 2, wantset.Granted) // claimed through another peer
	held(1, 1, false)
	held(1, 2, true)
	s.Done(testID(1))
	held(1, 2, false)

	checkClaim(t, s, 2, 1, wantset.Granted)
	s.Release(testID(2), testPeer(1))
	held(2, 1, false)
	checkClaim(t, s, 3, 1, wantset.Granted)
	s.Reject(testID(3))
	held(3, 1, false)
}

// TestPeek checks that Peek reports what Claim would return without
// recording a claim.
func TestPeek(t *testing.T) {
	t.Parallel()

	c := newClock()
	s := wantset.NewWithClock(testDeadline, testRejectTTL, 3, 2, wantset.MaxRejected, c.Now)
	peek := func(id, peer int, want wantset.Result) {
		t.Helper()
		if got := s.Peek(testID(id), testPeer(peer)); got != want {
			t.Fatalf("Peek(id %d, peer %d) = %v, want %v", id, peer, got, want)
		}
	}

	peek(1, 1, wantset.Granted)
	checkLen(t, s, 0)
	checkClaim(t, s, 1, 1, wantset.Granted)
	peek(1, 2, wantset.Held)
	checkClaim(t, s, 2, 1, wantset.Granted)
	peek(3, 1, wantset.PeerFull)
	peek(3, 2, wantset.Granted)
	checkClaim(t, s, 3, 2, wantset.Granted)
	peek(4, 2, wantset.Full)
	s.Reject(testID(5))
	peek(5, 3, wantset.Rejected)
	checkLen(t, s, 3)
	c.Advance(testDeadline)
	peek(1, 2, wantset.Granted)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	peek(1, 2, wantset.Closed)
}

// TestPeekPicked checks that PeekPicked counts the IDs picked for the peer
// since its last Claim towards both caps, as if they were claimed, and
// records nothing.
func TestPeekPicked(t *testing.T) {
	t.Parallel()

	c := newClock()
	s := wantset.NewWithClock(testDeadline, testRejectTTL, 4, 3, wantset.MaxRejected, c.Now)
	peek := func(id, peer, picked int, want wantset.Result) {
		t.Helper()
		if got := s.PeekPicked(testID(id), testPeer(peer), picked); got != want {
			t.Fatalf("PeekPicked(id %d, peer %d, picked %d) = %v, want %v", id, peer, picked, got, want)
		}
	}

	checkClaim(t, s, 1, 1, wantset.Granted)
	peek(2, 1, 1, wantset.Granted)
	peek(2, 1, 2, wantset.PeerFull) // 1 claimed and 2 picked: the peer's share
	peek(2, 2, 2, wantset.Granted)
	peek(2, 2, 3, wantset.Full) // 1 claimed and 3 picked: the set's cap
	peek(1, 2, 0, wantset.Held) // a claim is checked before the caps
	if got := s.Peek(testID(2), testPeer(1)); got != wantset.Granted {
		t.Fatalf("Peek = %v, want %v (PeekPicked with none picked)", got, wantset.Granted)
	}
	checkLen(t, s, 1)
}

// TestReject_Closed checks that a closed set rejects nothing and reports no
// rejected IDs and no claims.
func TestReject_Closed(t *testing.T) {
	t.Parallel()

	s := wantset.New(testDeadline, testRejectTTL)
	s.Reject(testID(1))
	checkClaim(t, s, 2, 1, wantset.Granted)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s.Reject(testID(3))
	checkRejectedLen(t, s, 0)
	checkRejected(t, s, 1, false)
	checkRejected(t, s, 3, false)
	if s.HeldBy(testID(2), testPeer(1)) {
		t.Fatal("HeldBy after Close")
	}
	checkClaim(t, s, 1, 1, wantset.Closed)
}

// TestModelReject drives the set and a reference model of claims and
// rejections with the same random operations and checks that they agree.
func TestModelReject(t *testing.T) {
	t.Parallel()

	const (
		maxClaims  = 8
		maxPerPeer = 3
		ids        = 24
		peers      = 4
		ops        = 20000
	)
	c := newClock()
	s := wantset.NewWithClock(testDeadline, testRejectTTL, maxClaims, maxPerPeer, ids, c.Now)
	t.Cleanup(func() { _ = s.Close() })
	rnd := rand.New(rand.NewPCG(3, 4))

	type entry struct {
		peer    int
		expires time.Time
	}
	claims := make(map[int]entry)
	rejected := make(map[int]time.Time)
	prune := func(now time.Time) {
		for i, e := range claims {
			if !now.Before(e.expires) {
				delete(claims, i)
			}
		}
		for i, e := range rejected {
			if !now.Before(e) {
				delete(rejected, i)
			}
		}
	}
	held := func(p int) int {
		n := 0
		for _, e := range claims {
			if e.peer == p {
				n++
			}
		}
		return n
	}

	// expectPicked is what a claim of i by p returns with picked more IDs
	// counted as claimed by p (PeekPicked).
	expectPicked := func(i, p, picked int) wantset.Result {
		if _, ok := rejected[i]; ok {
			return wantset.Rejected
		}
		if _, ok := claims[i]; ok {
			return wantset.Held
		}
		if len(claims)+picked >= maxClaims {
			return wantset.Full
		}
		if held(p)+picked >= maxPerPeer {
			return wantset.PeerFull
		}
		return wantset.Granted
	}
	expect := func(i, p int) wantset.Result { return expectPicked(i, p, 0) }

	for op := range ops {
		i, p := rnd.IntN(ids), rnd.IntN(peers)
		now := c.Now()
		prune(now)
		switch k := rnd.IntN(13); {
		case k < 5:
			want := expect(i, p)
			if want == wantset.Granted {
				claims[i] = entry{peer: p, expires: now.Add(testDeadline)}
			}
			if got := s.Claim(testID(i), testPeer(p)); got != want {
				t.Fatalf("op %d: Claim(id %d, peer %d) = %v, want %v", op, i, p, got, want)
			}
		case k == 12:
			if got, want := s.Peek(testID(i), testPeer(p)), expect(i, p); got != want {
				t.Fatalf("op %d: Peek(id %d, peer %d) = %v, want %v", op, i, p, got, want)
			}
		case k == 11:
			picked := rnd.IntN(maxClaims)
			if got, want := s.PeekPicked(testID(i), testPeer(p), picked), expectPicked(i, p, picked); got != want {
				t.Fatalf("op %d: PeekPicked(id %d, peer %d, picked %d) = %v, want %v", op, i, p, picked, got, want)
			}
		case k < 6:
			delete(claims, i)
			rejected[i] = now.Add(testRejectTTL)
			s.Reject(testID(i))
		case k < 7:
			delete(claims, i)
			s.Done(testID(i))
		case k < 8:
			e, ok := claims[i]
			want := ok && e.peer == p
			if got := s.HeldBy(testID(i), testPeer(p)); got != want {
				t.Fatalf("op %d: HeldBy(id %d, peer %d) = %v, want %v", op, i, p, got, want)
			}
			if want {
				delete(claims, i)
			}
			s.Release(testID(i), testPeer(p))
		case k < 10:
			c.Advance(time.Duration(rnd.Int64N(int64(testDeadline / 4))))
		default:
			checkLen(t, s, len(claims))
			checkRejectedLen(t, s, len(rejected))
		}
	}
}

// TestConcurrentReject checks that, with goroutines claiming, rejecting and
// ending claims at once, an ID is never granted after its rejection was
// recorded and before its TTL ends (the clock does not move).
func TestConcurrentReject(t *testing.T) {
	t.Parallel()

	const (
		ids     = 64
		workers = 8
		rounds  = 2000
	)
	s, _ := newRejectSet(t, ids)
	var (
		idMu     [ids]sync.Mutex
		rejected [ids]bool
		wg       sync.WaitGroup
	)
	for w := range workers {
		wg.Go(func() {
			rnd := rand.New(rand.NewPCG(uint64(w), 13))
			for range rounds {
				i := rnd.IntN(ids)
				idMu[i].Lock()
				switch rnd.IntN(4) {
				case 0:
					s.Reject(testID(i))
					rejected[i] = true
				case 1:
					s.Done(testID(i))
				default:
					r := s.Claim(testID(i), testPeer(w))
					switch {
					case rejected[i] && r != wantset.Rejected:
						t.Errorf("Claim(id %d) = %v after its rejection", i, r)
					case !rejected[i] && r == wantset.Rejected:
						t.Errorf("Claim(id %d) rejected without a rejection", i)
					case r == wantset.Granted:
						s.Release(testID(i), testPeer(w))
					}
				}
				idMu[i].Unlock()
				if t.Failed() {
					return
				}
			}
		})
	}
	wg.Wait()
	n := 0
	for _, r := range rejected {
		if r {
			n++
		}
	}
	checkRejectedLen(t, s, n)
}

// TestRejectFrom_PeerShare checks the per-peer share of the rejected cache: a
// peer that holds its share drops its own oldest rejection, so it cannot
// push out the rejections of other peers once there; rejecting an ID again
// moves it to the new peer's share; Reject counts against no share; and the
// cache's own bound still drops the oldest rejection.
func TestRejectFrom_PeerShare(t *testing.T) {
	t.Parallel()

	const (
		maxRejected = 6
		share       = 3
	)
	c := newClock()
	s := wantset.NewWithRejectShare(testDeadline, testRejectTTL, maxRejected, share, c.Now)
	t.Cleanup(func() { _ = s.Close() })

	s.RejectFrom(testID(100), testPeer(2))
	s.RejectFrom(testID(101), testPeer(2))
	for i := range 20 { // peer 1 floods its own IDs
		s.RejectFrom(testID(i), testPeer(1))
	}
	checkRejectedLen(t, s, 2+share)
	for i := range 20 {
		checkRejected(t, s, i, i >= 20-share)
	}
	checkRejected(t, s, 100, true)
	checkRejected(t, s, 101, true)
	checkRejectedBy(t, s, 1, share)
	checkRejectedBy(t, s, 2, 2)

	// 100 moves to peer 1's share, whose oldest (17) is dropped
	s.RejectFrom(testID(100), testPeer(1))
	checkRejected(t, s, 17, false)
	checkRejectedLen(t, s, 2+share-1)
	s.RejectFrom(testID(102), testPeer(2))
	s.RejectFrom(testID(103), testPeer(2))
	checkRejected(t, s, 101, true) // peer 2 holds 101, 102 and 103
	checkRejectedLen(t, s, maxRejected)

	// unattributed: the cache's bound drops the oldest (101)
	s.Reject(testID(30))
	checkRejectedLen(t, s, maxRejected)
	checkRejected(t, s, 101, false)
	checkRejected(t, s, 30, true)
	checkRejectedBy(t, s, 2, 2)

	// expired rejections leave the share
	c.Advance(testRejectTTL)
	checkRejectedBy(t, s, 1, 0)
	checkRejectedBy(t, s, 2, 0)
}

func checkRejectedBy(t *testing.T, s *wantset.Set, peer, want int) {
	t.Helper()
	if got := s.RejectedBy(testPeer(peer)); got != want {
		t.Fatalf("RejectedBy(peer %d) = %d, want %d", peer, got, want)
	}
}

// TestReject_DefaultShare checks the per-peer share of New.
func TestReject_DefaultShare(t *testing.T) {
	t.Parallel()

	s := wantset.New(testDeadline, testRejectTTL)
	t.Cleanup(func() { _ = s.Close() })
	s.RejectFrom(testID(-1), testPeer(2))
	for i := range wantset.MaxRejected {
		s.RejectFrom(testID(i), testPeer(1))
	}
	checkRejectedLen(t, s, 1+wantset.MaxRejectedPerPeer)
	checkRejected(t, s, -1, true)
	checkRejected(t, s, wantset.MaxRejected-wantset.MaxRejectedPerPeer-1, false)
	checkRejected(t, s, wantset.MaxRejected-wantset.MaxRejectedPerPeer, true)
}

// TestModelRejectShare drives the rejected cache and a reference model with
// the same random rejections, attributed (RejectFrom) or not (Reject), and
// expiries, and checks that they agree on every ID.
func TestModelRejectShare(t *testing.T) {
	t.Parallel()

	const (
		maxRejected = 10
		share       = 4
		ids         = 30
		peers       = 4
		ops         = 20000
	)
	c := newClock()
	s := wantset.NewWithRejectShare(testDeadline, testRejectTTL, maxRejected, share, c.Now)
	t.Cleanup(func() { _ = s.Close() })
	rnd := rand.New(rand.NewPCG(5, 6))

	type entry struct {
		id, peer int // peer -1: Reject
		expires  time.Time
	}
	var model []entry // oldest first
	drop := func(i int) { model = append(model[:i], model[i+1:]...) }
	find := func(match func(entry) bool) int {
		for i, e := range model {
			if match(e) {
				return i
			}
		}
		return -1
	}
	reject := func(id, peer int, now time.Time) {
		for len(model) > 0 && !now.Before(model[0].expires) {
			drop(0)
		}
		if i := find(func(e entry) bool { return e.id == id }); i >= 0 {
			drop(i)
		}
		if peer >= 0 {
			n := 0
			for _, e := range model {
				if e.peer == peer {
					n++
				}
			}
			if n >= share {
				drop(find(func(e entry) bool { return e.peer == peer }))
			}
		}
		if len(model) >= maxRejected {
			drop(0)
		}
		model = append(model, entry{id: id, peer: peer, expires: now.Add(testRejectTTL)})
	}

	for op := range ops {
		i, p := rnd.IntN(ids), rnd.IntN(peers+1)-1
		now := c.Now()
		switch k := rnd.IntN(10); {
		case k < 6:
			reject(i, p, now)
			if p < 0 {
				s.Reject(testID(i))
			} else {
				s.RejectFrom(testID(i), testPeer(p))
			}
		case k < 7:
			c.Advance(time.Duration(rnd.Int64N(int64(testRejectTTL / 20))))
		default:
			for len(model) > 0 && !now.Before(model[0].expires) {
				drop(0)
			}
			for id := range ids {
				want := find(func(e entry) bool { return e.id == id }) >= 0
				if got := s.IsRejected(testID(id)); got != want {
					t.Fatalf("op %d: IsRejected(id %d) = %v, want %v", op, id, got, want)
				}
			}
			checkRejectedLen(t, s, len(model))
		}
	}
}
