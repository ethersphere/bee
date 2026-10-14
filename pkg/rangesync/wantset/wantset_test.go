// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wantset_test

import (
	"math/rand/v2"
	"sync"
	"sync/atomic"
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

func newTestSet(t *testing.T, maxClaims int) (*wantset.Set, *clock) {
	t.Helper()
	c := newClock()
	s := wantset.NewWithClock(testDeadline, maxClaims, c.Now)
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

// TestClaim checks that an ID can be claimed once and is denied to every peer,
// including the holder, while the claim is active.
func TestClaim(t *testing.T) {
	t.Parallel()

	s, _ := newTestSet(t, wantset.MaxClaims)
	x, y := testID(1), testID(2)
	p1, p2 := testPeer(1), testPeer(2)

	checkLen(t, s, 0)
	if !s.Claim(x, p1) {
		t.Fatal("first claim of x denied")
	}
	if s.Claim(x, p2) {
		t.Fatal("claim of x through another peer granted while held")
	}
	if s.Claim(x, p1) {
		t.Fatal("second claim of x through the holder granted while held")
	}
	if !s.Claim(y, p2) {
		t.Fatal("claim of y denied")
	}
	checkLen(t, s, 2)
}

// TestDoneRelease checks that Done and Release end a claim so that the ID can
// be claimed again through any peer, and that both ignore unknown IDs.
func TestDoneRelease(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		end  func(*wantset.Set, fingerprint.ID)
	}{
		{"done", (*wantset.Set).Done},
		{"release", (*wantset.Set).Release},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s, _ := newTestSet(t, wantset.MaxClaims)
			x, y := testID(1), testID(2)

			if !s.Claim(x, testPeer(1)) || !s.Claim(y, testPeer(1)) {
				t.Fatal("claim denied")
			}
			tc.end(s, x)
			checkLen(t, s, 1)
			if !s.Claim(x, testPeer(2)) {
				t.Fatal("claim of x after it ended denied")
			}
			if s.Claim(y, testPeer(2)) {
				t.Fatal("claim of y granted, but only x ended")
			}

			tc.end(s, testID(3)) // never claimed
			tc.end(s, x)
			tc.end(s, x) // already ended
			checkLen(t, s, 1)
		})
	}
}

// TestExpiry checks that a claim expires exactly ClaimDeadline after it was
// made, that the ID can then be claimed through another peer, and that the new
// claim gets a full deadline of its own.
func TestExpiry(t *testing.T) {
	t.Parallel()

	s, c := newTestSet(t, wantset.MaxClaims)
	x := testID(1)
	p1, p2, p3 := testPeer(1), testPeer(2), testPeer(3)

	if !s.Claim(x, p1) {
		t.Fatal("claim denied")
	}
	c.Advance(testDeadline - time.Nanosecond)
	if s.Claim(x, p2) {
		t.Fatal("claim granted before the deadline")
	}
	checkLen(t, s, 1)

	c.Advance(time.Nanosecond)
	checkLen(t, s, 0)
	if !s.Claim(x, p2) {
		t.Fatal("reclaim after the deadline denied")
	}
	checkLen(t, s, 1)

	// The reclaim starts a new deadline, counted from the reclaim.
	c.Advance(testDeadline - time.Nanosecond)
	if s.Claim(x, p3) {
		t.Fatal("claim granted before the reclaim's deadline")
	}
	c.Advance(time.Nanosecond)
	if !s.Claim(x, p3) {
		t.Fatal("claim after the reclaim's deadline denied")
	}
}

// TestExpiryOrder checks that claims expire one by one in the order they were
// made, also when claims in between ended early.
func TestExpiryOrder(t *testing.T) {
	t.Parallel()

	s, c := newTestSet(t, wantset.MaxClaims)
	const n = 10
	step := testDeadline / n
	for i := range n {
		if !s.Claim(testID(i), testPeer(0)) {
			t.Fatalf("claim %d denied", i)
		}
		c.Advance(step)
	}
	// The clock is now at the deadline of claim 0. Claims 3 and 6 end early.
	s.Done(testID(3))
	s.Release(testID(6))

	for i := range n {
		// Claims 0..i have expired.
		want := 0
		for j := i + 1; j < n; j++ {
			if j != 3 && j != 6 {
				want++
			}
		}
		checkLen(t, s, want)
		if i+1 < n && i+1 != 3 && i+1 != 6 && s.Claim(testID(i+1), testPeer(1)) {
			t.Fatalf("claim %d granted before its deadline", i+1)
		}
		c.Advance(step)
	}
}

// TestBounded checks that the set holds at most maxClaims IDs: further claims
// are denied until a claim ends or expires.
func TestBounded(t *testing.T) {
	t.Parallel()

	const maxClaims = 16
	s, c := newTestSet(t, maxClaims)

	for i := range maxClaims {
		if !s.Claim(testID(i), testPeer(i)) {
			t.Fatalf("claim %d denied below the cap", i)
		}
	}
	if s.Claim(testID(maxClaims), testPeer(0)) {
		t.Fatal("claim granted over the cap")
	}
	checkLen(t, s, maxClaims)

	s.Done(testID(0))
	if !s.Claim(testID(maxClaims), testPeer(0)) {
		t.Fatal("claim denied after Done freed a slot")
	}
	if s.Claim(testID(maxClaims+1), testPeer(0)) {
		t.Fatal("claim granted over the cap")
	}

	c.Advance(testDeadline)
	checkLen(t, s, 0)
	for i := range maxClaims {
		if !s.Claim(testID(maxClaims+1+i), testPeer(i)) {
			t.Fatalf("claim %d denied after every claim expired", i)
		}
	}
	checkLen(t, s, maxClaims)
}

// TestBounded_Default checks the default cap of New.
func TestBounded_Default(t *testing.T) {
	t.Parallel()

	s := wantset.New(testDeadline)
	t.Cleanup(func() { _ = s.Close() })

	for i := range wantset.MaxClaims {
		if !s.Claim(testID(i), testPeer(0)) {
			t.Fatalf("claim %d denied below the cap", i)
		}
	}
	if s.Claim(testID(wantset.MaxClaims), testPeer(0)) {
		t.Fatal("claim granted over the cap")
	}
	checkLen(t, s, wantset.MaxClaims)
}

// TestClose checks that a closed set denies every claim, is empty, and that
// Close is idempotent.
func TestClose(t *testing.T) {
	t.Parallel()

	s := wantset.New(testDeadline)
	if !s.Claim(testID(1), testPeer(1)) {
		t.Fatal("claim denied")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	checkLen(t, s, 0)
	if s.Claim(testID(2), testPeer(1)) {
		t.Fatal("claim granted after Close")
	}
	s.Done(testID(1))
	s.Release(testID(1))
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
		maxClaims = 8
		ids       = 24
		ops       = 20000
	)
	s, c := newTestSet(t, maxClaims)
	rnd := rand.New(rand.NewPCG(1, 2))

	model := make(map[int]time.Time) // id -> expiry
	prune := func(now time.Time) {
		for i, exp := range model {
			if !now.Before(exp) {
				delete(model, i)
			}
		}
	}

	for op := range ops {
		i := rnd.IntN(ids)
		switch k := rnd.IntN(10); {
		case k < 5:
			now := c.Now()
			prune(now)
			_, held := model[i]
			want := !held && len(model) < maxClaims
			if want {
				model[i] = now.Add(testDeadline)
			}
			if got := s.Claim(testID(i), testPeer(rnd.IntN(4))); got != want {
				t.Fatalf("op %d: Claim(%d) = %v, want %v", op, i, got, want)
			}
		case k < 6:
			delete(model, i)
			s.Done(testID(i))
		case k < 7:
			delete(model, i)
			s.Release(testID(i))
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
		ids     = 512
		claimer = 16
	)
	s, _ := newTestSet(t, wantset.MaxClaims)
	var wins [ids]atomic.Int32

	var wg sync.WaitGroup
	for g := range claimer {
		wg.Go(func() {
			for k := range ids {
				i := (k + g*ids/claimer) % ids // start at different IDs
				if s.Claim(testID(i), testPeer(g)) {
					wins[i].Add(1)
				}
				_ = s.Len()
			}
		})
	}
	wg.Wait()

	for i := range ids {
		if n := wins[i].Load(); n != 1 {
			t.Fatalf("id %d granted %d times, want 1", i, n)
		}
	}
	checkLen(t, s, ids)

	for g := range claimer {
		wg.Go(func() {
			for i := g; i < ids; i += claimer {
				if i%2 == 0 {
					s.Done(testID(i))
				} else {
					s.Release(testID(i))
				}
			}
		})
	}
	wg.Wait()
	checkLen(t, s, 0)
}

// TestConcurrentChurn checks that, with goroutines claiming and ending claims
// on a small set of IDs at once, no ID is ever held by two claimers and the set
// never exceeds its cap.
func TestConcurrentChurn(t *testing.T) {
	t.Parallel()

	const (
		maxClaims = 4
		ids       = 8
		workers   = 8
		rounds    = 2000
	)
	s, _ := newTestSet(t, maxClaims)
	var holders [ids]atomic.Int32

	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			rnd := rand.New(rand.NewPCG(uint64(w), 7))
			for range rounds {
				i := rnd.IntN(ids)
				if !s.Claim(testID(i), testPeer(w)) {
					if n := s.Len(); n > maxClaims {
						t.Errorf("Len() = %d over the cap %d", n, maxClaims)
						return
					}
					continue
				}
				if n := holders[i].Add(1); n != 1 {
					t.Errorf("id %d held by %d claimers", i, n)
					return
				}
				holders[i].Add(-1)
				if rnd.IntN(2) == 0 {
					s.Done(testID(i))
				} else {
					s.Release(testID(i))
				}
			}
		})
	}
	wg.Wait()
	checkLen(t, s, 0)
}

// TestConcurrentExpiry checks that claims, ends and expiries running at once
// keep the set within its cap and leave it empty once every claim expired.
func TestConcurrentExpiry(t *testing.T) {
	t.Parallel()

	const (
		maxClaims = 32
		ids       = 64
		workers   = 8
		rounds    = 2000
	)
	s, c := newTestSet(t, maxClaims)

	var (
		wg   sync.WaitGroup
		stop = make(chan struct{})
	)
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				c.Advance(testDeadline / 100)
			}
		}
	})

	var workersWG sync.WaitGroup
	for w := range workers {
		workersWG.Go(func() {
			rnd := rand.New(rand.NewPCG(uint64(w), 11))
			for range rounds {
				i := rnd.IntN(ids)
				if s.Claim(testID(i), testPeer(w)) && rnd.IntN(4) == 0 {
					s.Release(testID(i))
				}
				if n := s.Len(); n > maxClaims {
					t.Errorf("Len() = %d over the cap %d", n, maxClaims)
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
