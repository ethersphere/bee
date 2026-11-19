// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// testClock is a clock that a test moves forward by hand.
type testClock struct {
	mtx sync.Mutex
	t   time.Time
}

func newTestClock() *testClock {
	return &testClock{t: time.Unix(1_700_000_000, 0)}
}

func (c *testClock) Now() time.Time {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.t
}

func (c *testClock) Add(d time.Duration) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.t = c.t.Add(d)
}

// hopAddr returns an address that starts with the given bytes.
func hopAddr(prefix ...byte) swarm.Address {
	b := make([]byte, swarm.HashSize)
	copy(b, prefix)
	return swarm.NewAddress(b)
}

func hasAddr(addrs []swarm.Address, a swarm.Address) bool {
	return swarm.ContainsAddress(addrs, a)
}

func TestHopMemoryRecord(t *testing.T) {
	t.Parallel()

	var (
		clock = newTestClock()
		m     = pushsync.NewHopMemory(clock.Now)
		chunk = hopAddr(0x6b, 0x00)
		peer  = hopAddr(0x10)
	)

	m.RecordValid(10, chunk, peer)
	valid, blocked, _ := m.View(10, chunk)
	if !hasAddr(valid, peer) || len(blocked) != 0 {
		t.Fatalf("after a valid receipt: valid %v blocked %v", valid, blocked)
	}

	m.RecordShallow(10, chunk, peer)
	valid, blocked, _ = m.View(10, chunk)
	if len(valid) != 0 || !hasAddr(blocked, peer) {
		t.Fatalf("after a shallow receipt: valid %v blocked %v", valid, blocked)
	}

	m.RecordValid(10, chunk, peer)
	m.Forget(10, chunk, peer)
	valid, blocked, _ = m.View(10, chunk)
	if len(valid) != 0 || len(blocked) != 0 {
		t.Fatalf("after forget: valid %v blocked %v", valid, blocked)
	}
	if got := m.Len(); got != 0 {
		t.Fatalf("got %d records, want 0", got)
	}
}

// TestHopMemoryShallowDuringPause checks that a shallow receipt during a
// pause does not make the pause longer.
func TestHopMemoryShallowDuringPause(t *testing.T) {
	t.Parallel()

	var (
		clock = newTestClock()
		m     = pushsync.NewHopMemory(clock.Now)
		chunk = hopAddr(0x6b, 0x00)
		peer  = hopAddr(0x10)
	)

	// results of pushes that ran in parallel
	m.RecordShallow(10, chunk, peer)
	m.RecordShallow(10, chunk, peer)
	clock.Add(30 * time.Second)
	m.RecordShallow(10, chunk, peer)

	clock.Add(30 * time.Second)
	if _, blocked, _ := m.View(10, chunk); len(blocked) != 0 {
		t.Fatal("peer is blocked 60s after the first shallow receipt")
	}
}

// TestHopMemoryForgetKeepsPause checks that forget does not end a pause.
func TestHopMemoryForgetKeepsPause(t *testing.T) {
	t.Parallel()

	var (
		clock = newTestClock()
		m     = pushsync.NewHopMemory(clock.Now)
		chunk = hopAddr(0x6b, 0x00)
		peer  = hopAddr(0x10)
	)

	m.RecordShallow(10, chunk, peer)
	m.Forget(10, chunk, peer)
	if _, blocked, _ := m.View(10, chunk); !hasAddr(blocked, peer) {
		t.Fatal("forget ended a pause that still runs")
	}

	clock.Add(time.Minute)
	m.Forget(10, chunk, peer)
	if got := m.Len(); got != 0 {
		t.Fatalf("got %d records after the pause, want 0", got)
	}
}

func TestHopMemoryPauseDoubles(t *testing.T) {
	t.Parallel()

	var (
		clock = newTestClock()
		m     = pushsync.NewHopMemory(clock.Now)
		chunk = hopAddr(0x6b, 0x00)
		peer  = hopAddr(0x10)
	)

	isBlocked := func() bool {
		_, blocked, _ := m.View(10, chunk)
		return hasAddr(blocked, peer)
	}

	// each step adds one shallow receipt in a row
	for i, pause := range []time.Duration{
		time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		16 * time.Minute,
		30 * time.Minute,
		30 * time.Minute,
	} {
		m.RecordShallow(10, chunk, peer)
		clock.Add(pause - time.Second)
		if !isBlocked() {
			t.Fatalf("shallow receipt %d: peer is free one second before the end of a %s pause", i+1, pause)
		}
		clock.Add(time.Second)
		if isBlocked() {
			t.Fatalf("shallow receipt %d: peer is blocked after a %s pause", i+1, pause)
		}
	}

	// a valid receipt starts the count again
	m.RecordValid(10, chunk, peer)
	m.RecordShallow(10, chunk, peer)
	clock.Add(time.Minute)
	if isBlocked() {
		t.Fatal("peer is blocked 60s after the first shallow receipt that follows a valid one")
	}
}

// TestHopMemoryShallowIdle checks that a shallow record is removed one hour
// after its last shallow receipt.
func TestHopMemoryShallowIdle(t *testing.T) {
	t.Parallel()

	var (
		clock = newTestClock()
		m     = pushsync.NewHopMemory(clock.Now)
		chunk = hopAddr(0x6b, 0x00)
	)

	for i := range 50 {
		m.RecordShallow(10, chunk, hopAddr(0x10, byte(i)))
	}

	clock.Add(time.Hour - time.Second)
	m.View(10, chunk)
	if got := m.Len(); got != 50 {
		t.Fatalf("got %d records one second before the end of the idle time, want 50", got)
	}

	clock.Add(time.Second)
	m.View(10, chunk)
	if got := m.Len(); got != 0 {
		t.Fatalf("got %d records after the idle time, want 0", got)
	}
}

// TestHopMemoryShallowCountRestarts checks that the count starts again after
// one hour without a shallow receipt, also when view was not called.
func TestHopMemoryShallowCountRestarts(t *testing.T) {
	t.Parallel()

	var (
		clock = newTestClock()
		m     = pushsync.NewHopMemory(clock.Now)
		chunk = hopAddr(0x6b, 0x00)
		peer  = hopAddr(0x10)
	)

	isBlocked := func() bool {
		_, blocked, _ := m.View(10, chunk)
		return hasAddr(blocked, peer)
	}

	m.RecordShallow(10, chunk, peer)
	clock.Add(time.Hour)
	m.RecordShallow(10, chunk, peer)

	clock.Add(time.Minute - time.Second)
	if !isBlocked() {
		t.Fatal("peer is free one second before the end of a 1m0s pause")
	}
	clock.Add(time.Second)
	if isBlocked() {
		t.Fatal("peer is blocked after a 1m0s pause: the count did not start again")
	}
}

func TestHopMemoryValidIdle(t *testing.T) {
	t.Parallel()

	var (
		clock = newTestClock()
		m     = pushsync.NewHopMemory(clock.Now)
		chunk = hopAddr(0x6b, 0x00)
		peer  = hopAddr(0x10)
	)

	m.RecordValid(10, chunk, peer)

	clock.Add(29 * time.Minute)
	if valid, _, _ := m.View(10, chunk); !hasAddr(valid, peer) {
		t.Fatal("valid record is gone after 29 minutes")
	}

	clock.Add(time.Minute)
	if valid, _, _ := m.View(10, chunk); len(valid) != 0 {
		t.Fatal("valid record is still used after 30 minutes without a result")
	}
	if got := m.Len(); got != 0 {
		t.Fatalf("got %d records, want 0", got)
	}
}

func TestHopMemoryProbe(t *testing.T) {
	t.Parallel()

	var (
		clock = newTestClock()
		m     = pushsync.NewHopMemory(clock.Now)
		chunk = hopAddr(0x6b, 0x00)
		other = hopAddr(0x1c, 0x00)
		peer  = hopAddr(0x10)
	)

	m.RecordValid(10, chunk, peer)
	m.RecordValid(10, other, peer)

	if _, _, due := m.View(10, chunk); !due {
		t.Fatal("probe is not due for a neighborhood that was never probed")
	}

	if !m.ClaimProbe(10, chunk) {
		t.Fatal("probe that is due was not given")
	}
	if m.ClaimProbe(10, chunk) {
		t.Fatal("second probe was given right after a probe")
	}
	if _, _, due := m.View(10, chunk); due {
		t.Fatal("probe is due right after a probe")
	}
	if _, _, due := m.View(10, other); !due {
		t.Fatal("probe of one neighborhood changed another neighborhood")
	}

	clock.Add(29 * time.Second)
	if _, _, due := m.View(10, chunk); due {
		t.Fatal("probe is due 29s after a probe")
	}
	clock.Add(time.Second)
	if _, _, due := m.View(10, chunk); !due {
		t.Fatal("probe is not due 30s after a probe")
	}
}

func TestHopMemoryNeighborhoodKey(t *testing.T) {
	t.Parallel()

	peer := hopAddr(0x10)

	t.Run("radius 10", func(t *testing.T) {
		t.Parallel()

		m := pushsync.NewHopMemory(newTestClock().Now)
		m.RecordValid(10, hopAddr(0x6b, 0x00), peer)

		// 0x6b3f shares the first 10 bits with 0x6b00, 0x6b40 does not
		if valid, _, _ := m.View(10, hopAddr(0x6b, 0x3f, 0xff)); !hasAddr(valid, peer) {
			t.Fatal("chunk of the same neighborhood does not see the record")
		}
		if valid, _, _ := m.View(10, hopAddr(0x6b, 0x40)); len(valid) != 0 {
			t.Fatal("chunk of another neighborhood sees the record")
		}
		if valid, _, _ := m.View(10, hopAddr(0x6a, 0x00)); len(valid) != 0 {
			t.Fatal("chunk of another neighborhood sees the record")
		}
	})

	t.Run("radius 8", func(t *testing.T) {
		t.Parallel()

		m := pushsync.NewHopMemory(newTestClock().Now)
		m.RecordValid(8, hopAddr(0x6b, 0x00), peer)

		if valid, _, _ := m.View(8, hopAddr(0x6b, 0xff)); !hasAddr(valid, peer) {
			t.Fatal("chunk of the same neighborhood does not see the record")
		}
		if valid, _, _ := m.View(8, hopAddr(0x6a, 0x00)); len(valid) != 0 {
			t.Fatal("chunk of another neighborhood sees the record")
		}
	})

	t.Run("radius 0", func(t *testing.T) {
		t.Parallel()

		m := pushsync.NewHopMemory(newTestClock().Now)
		m.RecordValid(0, hopAddr(0x6b), peer)

		if valid, _, _ := m.View(0, hopAddr(0xff, 0xff)); !hasAddr(valid, peer) {
			t.Fatal("at radius 0 every chunk is in one neighborhood")
		}
	})
}

func TestHopMemoryRadiusChange(t *testing.T) {
	t.Parallel()

	var (
		m     = pushsync.NewHopMemory(newTestClock().Now)
		chunk = hopAddr(0x6b, 0x00)
	)

	m.RecordValid(9, chunk, hopAddr(0x10))
	m.RecordShallow(9, chunk, hopAddr(0x20))

	valid, blocked, _ := m.View(10, chunk)
	if len(valid) != 0 || len(blocked) != 0 {
		t.Fatalf("records of radius 9 are used at radius 10: valid %v blocked %v", valid, blocked)
	}
	if got := m.Len(); got != 0 {
		t.Fatalf("got %d records, want 0", got)
	}
}

// TestHopMemoryLateResult checks that a result of a push that began at
// another radius does not remove the records of the current radius.
func TestHopMemoryLateResult(t *testing.T) {
	t.Parallel()

	var (
		m     = pushsync.NewHopMemory(newTestClock().Now)
		chunk = hopAddr(0x6b, 0x00)
		peer  = hopAddr(0x10)
		late  = hopAddr(0x20)
	)

	m.View(10, chunk)
	m.RecordValid(10, chunk, peer)

	m.RecordValid(9, chunk, late)
	m.RecordShallow(9, chunk, late)
	m.Forget(9, chunk, peer)
	if m.ClaimProbe(9, chunk) {
		t.Fatal("probe was given for another radius")
	}

	valid, blocked, _ := m.View(10, chunk)
	if len(valid) != 1 || !hasAddr(valid, peer) || len(blocked) != 0 {
		t.Fatalf("got valid %v blocked %v, want only the record of radius 10", valid, blocked)
	}
}

// TestHopMemoryBlockedOrder checks that the peers in a pause are returned in
// the order in which their pauses end.
func TestHopMemoryBlockedOrder(t *testing.T) {
	t.Parallel()

	var (
		clock = newTestClock()
		m     = pushsync.NewHopMemory(clock.Now)
		chunk = hopAddr(0x6b, 0x00)
		long  = hopAddr(0x6a)
		first = hopAddr(0x10)
		last  = hopAddr(0x20)
	)

	// two shallow receipts in a row: the pause of this peer ends at 3m
	m.RecordShallow(10, chunk, long)
	clock.Add(time.Minute)
	m.RecordShallow(10, chunk, long)
	// these pauses end at 2m10s and 2m20s
	clock.Add(10 * time.Second)
	m.RecordShallow(10, chunk, first)
	clock.Add(10 * time.Second)
	m.RecordShallow(10, chunk, last)

	_, blocked, _ := m.View(10, chunk)
	want := []swarm.Address{first, last, long}
	if len(blocked) != len(want) {
		t.Fatalf("got %d peers in a pause, want %d", len(blocked), len(want))
	}
	for i := range want {
		if !blocked[i].Equal(want[i]) {
			t.Fatalf("position %d: got %s, want %s", i, blocked[i], want[i])
		}
	}
}

// TestHopMemoryClaimProbeOnce checks that only one of the callers that run
// at the same time gets the probe of a neighborhood.
func TestHopMemoryClaimProbeOnce(t *testing.T) {
	t.Parallel()

	var (
		clock = newTestClock()
		m     = pushsync.NewHopMemory(clock.Now)
		chunk = hopAddr(0x6b, 0x00)
	)
	m.RecordValid(10, chunk, hopAddr(0x10))

	for round := range 3 {
		var (
			wg    sync.WaitGroup
			given atomic.Int32
		)
		for range 64 {
			wg.Go(func() {
				if m.ClaimProbe(10, chunk) {
					given.Add(1)
				}
			})
		}
		wg.Wait()
		if got := given.Load(); got != 1 {
			t.Fatalf("round %d: got %d probes, want 1", round, got)
		}
		clock.Add(30 * time.Second)
	}
}

// TestHopMemoryConcurrent runs all operations at the same time and checks
// that the memory stays consistent.
func TestHopMemoryConcurrent(t *testing.T) {
	t.Parallel()

	var (
		clock = newTestClock()
		m     = pushsync.NewHopMemory(clock.Now)
		wg    sync.WaitGroup
	)

	for i := range 64 {
		wg.Go(func() {
			r := rand.New(rand.NewSource(int64(i)))
			for range 200 {
				chunk := hopAddr(byte(r.Intn(4)), byte(r.Intn(256)))
				peer := hopAddr(byte(r.Intn(8)))
				switch r.Intn(6) {
				case 0:
					m.RecordValid(10, chunk, peer)
				case 1:
					m.RecordShallow(10, chunk, peer)
				case 2:
					m.Forget(10, chunk, peer)
				case 3:
					m.ClaimProbe(10, chunk)
				case 4:
					clock.Add(time.Second)
				default:
					m.View(10, chunk)
				}
			}
		})
	}
	wg.Wait()

	// 4 values of the first byte and 4 of the next two bits give at most
	// 16 neighborhoods at radius 10, each with at most 8 peers
	if got := m.Len(); got > 16*8 {
		t.Fatalf("got %d records, want at most %d", got, 16*8)
	}
	var seen int
	for a := range 4 {
		for b := range 4 {
			valid, blocked, _ := m.View(10, hopAddr(byte(a), byte(b<<6)))
			for _, p := range valid {
				if hasAddr(blocked, p) {
					t.Fatalf("peer %s is valid and in a pause", p)
				}
			}
			seen += len(valid) + len(blocked)
		}
	}
	if got := m.Len(); seen > got {
		t.Fatalf("views returned %d peers, the memory has %d records", seen, got)
	}
}
