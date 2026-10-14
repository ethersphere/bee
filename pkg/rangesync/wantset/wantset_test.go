// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wantset_test

import (
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const testDeadline = 30 * time.Second

// clock is a manually advanced clock, safe for concurrent use.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock {
	return &clock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func testID(i int) fingerprint.ID {
	var id fingerprint.ID
	id[0] = byte(i >> 24)
	id[1] = byte(i >> 16)
	id[2] = byte(i >> 8)
	id[3] = byte(i)
	id[fingerprint.IDSize-1] = 0xff
	return id
}

func testPeer(i int) swarm.Address {
	b := make([]byte, swarm.HashSize)
	b[0] = byte(i >> 8)
	b[1] = byte(i)
	return swarm.NewAddress(b)
}

func newTestSet(t *testing.T, maxClaims, maxPerPeer int) (*wantset.Set, *clock) {
	t.Helper()
	c := newClock()
	s := wantset.NewWithClock(testDeadline, maxClaims, maxPerPeer, c.Now)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	return s, c
}

func checkLen(t *testing.T, s *wantset.Set, want int) {
	t.Helper()
	if got := s.Len(); got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
}

func checkClaim(t *testing.T, s *wantset.Set, id, peer int, want wantset.Result) {
	t.Helper()
	if got := s.Claim(testID(id), testPeer(peer)); got != want {
		t.Fatalf("Claim(id %d, peer %d) = %v, want %v", id, peer, got, want)
	}
}

// TestResultString checks the metric labels of the claim results.
func TestResultString(t *testing.T) {
	t.Parallel()

	for r, want := range map[wantset.Result]string{
		wantset.Granted:  "granted",
		wantset.Held:     "held",
		wantset.Full:     "full",
		wantset.PeerFull: "peer_full",
		wantset.Closed:   "closed",
		0:                "Result(0)",
	} {
		if got := r.String(); got != want {
			t.Errorf("Result(%d).String() = %q, want %q", int(r), got, want)
		}
	}
}

// TestClaim checks that an ID can be claimed once and is held against every
// peer, including the holder, while the claim is active.
func TestClaim(t *testing.T) {
	t.Parallel()

	s, _ := newTestSet(t, wantset.MaxClaims, wantset.MaxClaimsPerPeer)

	checkLen(t, s, 0)
	checkClaim(t, s, 1, 1, wantset.Granted)
	checkClaim(t, s, 1, 2, wantset.Held)
	checkClaim(t, s, 1, 1, wantset.Held)
	checkClaim(t, s, 2, 2, wantset.Granted)
	checkLen(t, s, 2)
}

// TestDone checks that Done ends a claim whichever peer holds it, so the ID
// can be claimed again, and ignores unknown IDs.
func TestDone(t *testing.T) {
	t.Parallel()

	s, _ := newTestSet(t, wantset.MaxClaims, wantset.MaxClaimsPerPeer)
	checkClaim(t, s, 1, 1, wantset.Granted)
	checkClaim(t, s, 2, 1, wantset.Granted)

	s.Done(testID(1))
	checkLen(t, s, 1)
	checkClaim(t, s, 1, 2, wantset.Granted)
	checkClaim(t, s, 2, 2, wantset.Held)

	s.Done(testID(3)) // never claimed
	s.Done(testID(1))
	s.Done(testID(1)) // already ended
	checkLen(t, s, 1)
}

// TestRelease checks that Release ends a claim only when called with the peer
// that holds it, and ignores unknown IDs.
func TestRelease(t *testing.T) {
	t.Parallel()

	s, _ := newTestSet(t, wantset.MaxClaims, wantset.MaxClaimsPerPeer)
	checkClaim(t, s, 1, 1, wantset.Granted)
	checkClaim(t, s, 2, 1, wantset.Granted)

	s.Release(testID(1), testPeer(2)) // not the holder
	checkClaim(t, s, 1, 3, wantset.Held)
	checkLen(t, s, 2)

	s.Release(testID(1), testPeer(1))
	checkLen(t, s, 1)
	checkClaim(t, s, 1, 2, wantset.Granted)
	checkClaim(t, s, 2, 2, wantset.Held)

	s.Release(testID(3), testPeer(1)) // never claimed
	s.Release(testID(1), testPeer(2))
	s.Release(testID(1), testPeer(2)) // already ended
	checkLen(t, s, 1)
}

// TestRelease_AfterReclaim checks that a Release from a fetch that outlived
// its claim does not end the claim the ID got through another peer after it
// expired.
func TestRelease_AfterReclaim(t *testing.T) {
	t.Parallel()

	s, c := newTestSet(t, wantset.MaxClaims, wantset.MaxClaimsPerPeer)
	checkClaim(t, s, 1, 1, wantset.Granted)
	c.Advance(testDeadline)
	checkClaim(t, s, 1, 2, wantset.Granted)

	s.Release(testID(1), testPeer(1)) // late, from the expired claim
	checkClaim(t, s, 1, 3, wantset.Held)
	checkLen(t, s, 1)

	s.Release(testID(1), testPeer(2))
	checkClaim(t, s, 1, 3, wantset.Granted)
}

// TestExpiry checks that a claim expires exactly at the deadline after it was
// made, that the ID can then be claimed through another peer, and that the new
// claim gets a full deadline of its own.
func TestExpiry(t *testing.T) {
	t.Parallel()

	s, c := newTestSet(t, wantset.MaxClaims, wantset.MaxClaimsPerPeer)

	checkClaim(t, s, 1, 1, wantset.Granted)
	c.Advance(testDeadline - time.Nanosecond)
	checkClaim(t, s, 1, 2, wantset.Held)
	checkLen(t, s, 1)

	c.Advance(time.Nanosecond)
	checkLen(t, s, 0)
	checkClaim(t, s, 1, 2, wantset.Granted)
	checkLen(t, s, 1)

	// The reclaim starts a new deadline, counted from the reclaim.
	c.Advance(testDeadline - time.Nanosecond)
	checkClaim(t, s, 1, 3, wantset.Held)
	c.Advance(time.Nanosecond)
	checkClaim(t, s, 1, 3, wantset.Granted)
}

// TestExpiryOrder checks that claims expire one by one in the order they were
// made, also when claims in between ended early.
func TestExpiryOrder(t *testing.T) {
	t.Parallel()

	s, c := newTestSet(t, wantset.MaxClaims, wantset.MaxClaimsPerPeer)
	const n = 10
	step := testDeadline / n
	for i := range n {
		checkClaim(t, s, i, 0, wantset.Granted)
		c.Advance(step)
	}
	// The clock is now at the deadline of claim 0. Claims 3 and 6 end early.
	s.Done(testID(3))
	s.Release(testID(6), testPeer(0))

	for i := range n {
		// Claims 0..i have expired.
		want := 0
		for j := i + 1; j < n; j++ {
			if j != 3 && j != 6 {
				want++
			}
		}
		checkLen(t, s, want)
		if j := i + 1; j < n && j != 3 && j != 6 {
			checkClaim(t, s, j, 1, wantset.Held)
		}
		c.Advance(step)
	}
}

// TestClockBackwards pins the behavior when the clock steps back: a claim
// made after the step expires on time for Claim on its own ID, but counts in
// Len and against the cap until the claims made before it have expired.
func TestClockBackwards(t *testing.T) {
	t.Parallel()

	const back = testDeadline / 3
	s, c := newTestSet(t, 2, 2)

	checkClaim(t, s, 1, 1, wantset.Granted) // expires at T+D
	c.Advance(-back)
	checkClaim(t, s, 2, 1, wantset.Granted) // expires at T-back+D
	c.Advance(testDeadline)

	// At T-back+D, claim 2 has expired but sits behind claim 1.
	checkLen(t, s, 2)
	checkClaim(t, s, 3, 2, wantset.Full)
	checkClaim(t, s, 1, 2, wantset.Held)
	checkClaim(t, s, 2, 2, wantset.Granted)
	checkLen(t, s, 2)

	c.Advance(back) // T+D: claim 1 expires.
	checkLen(t, s, 1)
	checkClaim(t, s, 3, 2, wantset.Granted)
	// The set is full again until a claim ends.
	checkClaim(t, s, 4, 1, wantset.Full)
	s.Done(testID(3))
	checkClaim(t, s, 4, 1, wantset.Granted)
}

// TestBounded checks that the set holds at most maxClaims IDs: further claims
// are Full until a claim ends or expires.
func TestBounded(t *testing.T) {
	t.Parallel()

	const maxClaims = 16
	s, c := newTestSet(t, maxClaims, maxClaims)

	for i := range maxClaims {
		checkClaim(t, s, i, i, wantset.Granted)
	}
	checkClaim(t, s, maxClaims, 0, wantset.Full)
	checkLen(t, s, maxClaims)

	s.Done(testID(0))
	checkClaim(t, s, maxClaims, 0, wantset.Granted)
	checkClaim(t, s, maxClaims+1, 0, wantset.Full)

	c.Advance(testDeadline)
	checkLen(t, s, 0)
	for i := range maxClaims {
		checkClaim(t, s, maxClaims+1+i, i, wantset.Granted)
	}
	checkLen(t, s, maxClaims)
}

// TestBounded_Default checks the default caps of New.
func TestBounded_Default(t *testing.T) {
	t.Parallel()

	s := wantset.New(testDeadline)
	t.Cleanup(func() { _ = s.Close() })

	peers := wantset.MaxClaims / wantset.MaxClaimsPerPeer
	for p := range peers {
		for i := range wantset.MaxClaimsPerPeer {
			checkClaim(t, s, p*wantset.MaxClaimsPerPeer+i, p, wantset.Granted)
		}
		if p < peers-1 {
			checkClaim(t, s, wantset.MaxClaims, p, wantset.PeerFull)
		}
	}
	checkClaim(t, s, wantset.MaxClaims, peers, wantset.Full)
	checkLen(t, s, wantset.MaxClaims)
}

// TestPeerCap checks that one peer holds at most maxPerPeer claims, that other
// peers can still claim, that ended and expired claims free the peer's slots,
// and that Full is reported before PeerFull.
func TestPeerCap(t *testing.T) {
	t.Parallel()

	const maxPerPeer = 4
	s, c := newTestSet(t, 3*maxPerPeer, maxPerPeer)

	for i := range maxPerPeer {
		checkClaim(t, s, i, 1, wantset.Granted)
	}
	checkClaim(t, s, 100, 1, wantset.PeerFull)
	checkClaim(t, s, 100, 2, wantset.Granted) // same ID, another peer

	s.Done(testID(0))
	checkClaim(t, s, 101, 1, wantset.Granted)
	checkClaim(t, s, 102, 1, wantset.PeerFull)
	s.Release(testID(1), testPeer(1))
	checkClaim(t, s, 102, 1, wantset.Granted)
	s.Release(testID(2), testPeer(2)) // not the holder: no slot freed
	checkClaim(t, s, 103, 1, wantset.PeerFull)

	c.Advance(testDeadline)
	checkLen(t, s, 0)
	for i := range maxPerPeer {
		checkClaim(t, s, 200+i, 1, wantset.Granted)
	}

	// Fill the set: peers 2 and 3 take the remaining slots.
	for i := range maxPerPeer {
		checkClaim(t, s, 300+i, 2, wantset.Granted)
		checkClaim(t, s, 400+i, 3, wantset.Granted)
	}
	checkClaim(t, s, 500, 1, wantset.Full) // peer 1 is full too
	checkClaim(t, s, 500, 4, wantset.Full)
}

// TestClose checks that a closed set reports Closed, is empty, and that Close
// is idempotent.
func TestClose(t *testing.T) {
	t.Parallel()

	s := wantset.New(testDeadline)
	checkClaim(t, s, 1, 1, wantset.Granted)
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	checkLen(t, s, 0)
	checkClaim(t, s, 2, 1, wantset.Closed)
	s.Done(testID(1))
	s.Release(testID(1), testPeer(1))
	if err := s.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

// TestNew_InvalidDeadline checks that New rejects a deadline that is not
// positive.
func TestNew_InvalidDeadline(t *testing.T) {
	t.Parallel()

	for _, d := range []time.Duration{0, -time.Second} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("New(%v) did not panic", d)
				}
			}()
			_ = wantset.New(d)
		}()
	}
}

// TestModel drives the set and a reference model with the same random
// operations and checks that they agree.
func TestModel(t *testing.T) {
	t.Parallel()

	const (
		maxClaims  = 8
		maxPerPeer = 3
		ids        = 24
		peers      = 4
		ops        = 20000
	)
	s, c := newTestSet(t, maxClaims, maxPerPeer)
	rnd := rand.New(rand.NewPCG(1, 2))

	type entry struct {
		peer    int
		expires time.Time
	}
	model := make(map[int]entry)
	prune := func(now time.Time) {
		for i, e := range model {
			if !now.Before(e.expires) {
				delete(model, i)
			}
		}
	}
	held := func(p int) int {
		n := 0
		for _, e := range model {
			if e.peer == p {
				n++
			}
		}
		return n
	}

	for op := range ops {
		i, p := rnd.IntN(ids), rnd.IntN(peers)
		switch k := rnd.IntN(10); {
		case k < 5:
			now := c.Now()
			prune(now)
			var want wantset.Result
			if _, ok := model[i]; ok {
				want = wantset.Held
			} else if len(model) >= maxClaims {
				want = wantset.Full
			} else if held(p) >= maxPerPeer {
				want = wantset.PeerFull
			} else {
				want = wantset.Granted
				model[i] = entry{peer: p, expires: now.Add(testDeadline)}
			}
			if got := s.Claim(testID(i), testPeer(p)); got != want {
				t.Fatalf("op %d: Claim(id %d, peer %d) = %v, want %v", op, i, p, got, want)
			}
		case k < 6:
			delete(model, i)
			s.Done(testID(i))
		case k < 7:
			if e, ok := model[i]; ok && e.peer == p {
				delete(model, i)
			}
			s.Release(testID(i), testPeer(p))
		case k < 9:
			c.Advance(time.Duration(rnd.Int64N(int64(testDeadline / 4))))
		default:
			prune(c.Now())
			checkLen(t, s, len(model))
		}
	}
}

// TestConcurrentClaims checks that, with many goroutines claiming the same IDs
// at once, every ID is granted exactly once.
func TestConcurrentClaims(t *testing.T) {
	t.Parallel()

	const (
		ids      = 512
		claimers = 16
	)
	s, _ := newTestSet(t, wantset.MaxClaims, wantset.MaxClaimsPerPeer)
	var (
		mu   sync.Mutex
		wins [ids]int
	)

	var wg sync.WaitGroup
	for g := range claimers {
		wg.Go(func() {
			for k := range ids {
				i := (k + g*ids/claimers) % ids // start at different IDs
				switch r := s.Claim(testID(i), testPeer(g)); r {
				case wantset.Granted:
					mu.Lock()
					wins[i]++
					mu.Unlock()
				case wantset.Held:
				default:
					t.Errorf("Claim(id %d) = %v", i, r)
					return
				}
				_ = s.Len()
			}
		})
	}
	wg.Wait()

	for i := range ids {
		if wins[i] != 1 {
			t.Fatalf("id %d granted %d times, want 1", i, wins[i])
		}
	}
	checkLen(t, s, ids)

	for g := range claimers {
		wg.Go(func() {
			for i := g; i < ids; i += claimers {
				s.Done(testID(i))
			}
		})
	}
	wg.Wait()
	checkLen(t, s, 0)
}

// TestConcurrentPeerCap checks that, with several goroutines per peer claiming
// distinct IDs at once, every peer gets exactly maxPerPeer claims.
func TestConcurrentPeerCap(t *testing.T) {
	t.Parallel()

	const (
		maxPerPeer     = 50
		peers          = 4
		workersPerPeer = 4
		idsPerWorker   = 100
	)
	s, _ := newTestSet(t, wantset.MaxClaims, maxPerPeer)
	var (
		mu      sync.Mutex
		granted [peers]int
	)

	var wg sync.WaitGroup
	for p := range peers {
		for w := range workersPerPeer {
			wg.Go(func() {
				base := (p*workersPerPeer + w) * idsPerWorker
				for i := range idsPerWorker {
					switch r := s.Claim(testID(base+i), testPeer(p)); r {
					case wantset.Granted:
						mu.Lock()
						granted[p]++
						mu.Unlock()
					case wantset.PeerFull:
					default:
						t.Errorf("Claim(id %d, peer %d) = %v", base+i, p, r)
						return
					}
				}
			})
		}
	}
	wg.Wait()

	for p := range peers {
		if granted[p] != maxPerPeer {
			t.Errorf("peer %d granted %d claims, want %d", p, granted[p], maxPerPeer)
		}
	}
	checkLen(t, s, peers*maxPerPeer)
}

// TestConcurrentChurn checks that, with goroutines claiming and ending claims
// on a small set of IDs at once, no ID is ever granted while another claimer
// holds it, and the set never exceeds its caps. Each claim and end of an ID
// is made together with the test's record of its holder, under a per-ID lock,
// so the check does not depend on timing.
func TestConcurrentChurn(t *testing.T) {
	t.Parallel()

	const (
		maxClaims  = 4
		maxPerPeer = 2
		ids        = 8
		workers    = 8
		rounds     = 2000
	)
	s, _ := newTestSet(t, maxClaims, maxPerPeer)
	var (
		idMu   [ids]sync.Mutex
		holder [ids]int // worker+1, 0 if none
	)

	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			rnd := rand.New(rand.NewPCG(uint64(w), 7))
			var mine []int
			for range rounds {
				if len(mine) > 0 && rnd.IntN(2) == 0 {
					k := rnd.IntN(len(mine))
					i := mine[k]
					mine = append(mine[:k], mine[k+1:]...)
					idMu[i].Lock()
					if holder[i] != w+1 {
						t.Errorf("worker %d ends id %d held by %d", w, i, holder[i]-1)
					}
					holder[i] = 0
					if rnd.IntN(2) == 0 {
						s.Done(testID(i))
					} else {
						s.Release(testID(i), testPeer(w))
					}
					idMu[i].Unlock()
					continue
				}

				i := rnd.IntN(ids)
				idMu[i].Lock()
				r := s.Claim(testID(i), testPeer(w))
				switch {
				case r == wantset.Granted && holder[i] != 0:
					t.Errorf("id %d granted to worker %d while held by %d", i, w, holder[i]-1)
				case r == wantset.Granted:
					holder[i] = w + 1
					mine = append(mine, i)
				case r == wantset.Held && holder[i] == 0:
					t.Errorf("id %d reported held but nobody holds it", i)
				}
				idMu[i].Unlock()
				if n := s.Len(); n > maxClaims {
					t.Errorf("Len() = %d over the cap %d", n, maxClaims)
				}
				if len(mine) > maxPerPeer {
					t.Errorf("worker %d holds %d claims over the per-peer cap %d", w, len(mine), maxPerPeer)
				}
				if t.Failed() {
					return
				}
			}
			for _, i := range mine {
				idMu[i].Lock()
				holder[i] = 0
				s.Release(testID(i), testPeer(w))
				idMu[i].Unlock()
			}
		})
	}
	wg.Wait()
	checkLen(t, s, 0)
}

// TestConcurrentExpiry checks claims, ends and expiries running at once: an ID
// is granted only when it has no live claim, reported held only when it has
// one, and a Release by a peer whose claim expired leaves the new holder's
// claim in place. The clock advances concurrently, but not during an
// operation and its bookkeeping, so the test knows when each claim expires.
func TestConcurrentExpiry(t *testing.T) {
	t.Parallel()

	const (
		ids     = 16
		workers = 8
		rounds  = 2000
	)
	s, c := newTestSet(t, wantset.MaxClaims, wantset.MaxClaimsPerPeer)

	type live struct {
		holder  int // worker+1, 0 if none
		expires time.Time
	}
	var (
		gate  sync.RWMutex // write-held while the clock advances
		idMu  [ids]sync.Mutex
		state [ids]live
		wg    sync.WaitGroup
		stop  = make(chan struct{})
	)

	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				gate.Lock()
				c.Advance(testDeadline / 50)
				gate.Unlock()
			}
		}
	})

	var workersWG sync.WaitGroup
	for w := range workers {
		workersWG.Go(func() {
			rnd := rand.New(rand.NewPCG(uint64(w), 11))
			for range rounds {
				i := rnd.IntN(ids)
				gate.RLock()
				idMu[i].Lock()
				now := c.Now()
				st := &state[i]
				isLive := st.holder != 0 && now.Before(st.expires)
				switch op := rnd.IntN(4); {
				case op < 2:
					r := s.Claim(testID(i), testPeer(w))
					switch {
					case r == wantset.Granted && isLive:
						t.Errorf("id %d granted to worker %d while live for %d", i, w, st.holder-1)
					case r == wantset.Granted:
						*st = live{holder: w + 1, expires: now.Add(testDeadline)}
					case r == wantset.Held && !isLive:
						t.Errorf("id %d reported held without a live claim", i)
					case r != wantset.Held:
						t.Errorf("Claim(id %d) = %v", i, r)
					}
				case op < 3:
					// Possibly a late Release of an expired or foreign claim.
					s.Release(testID(i), testPeer(w))
					if st.holder == w+1 {
						st.holder = 0
					}
				default:
					s.Done(testID(i))
					st.holder = 0
				}
				idMu[i].Unlock()
				gate.RUnlock()
				if t.Failed() {
					return
				}
			}
		})
	}
	workersWG.Wait()
	close(stop)
	wg.Wait()

	c.Advance(testDeadline)
	checkLen(t, s, 0)
}
