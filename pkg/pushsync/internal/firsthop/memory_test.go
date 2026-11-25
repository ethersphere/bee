// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package firsthop_test

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/pushsync/internal/firsthop"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const radius = 10

// testClock is a clock that a test moves forward by hand.
type testClock struct {
	mtx sync.Mutex
	t   time.Time
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

func newMemory() (*firsthop.Memory, *testClock) {
	clock := &testClock{t: time.Unix(1_700_000_000, 0)}
	return firsthop.New(clock.Now), clock
}

// addr returns an address that starts with the given bytes.
func addr(prefix ...byte) swarm.Address {
	b := make([]byte, swarm.HashSize)
	copy(b, prefix)
	return swarm.NewAddress(b)
}

func has(addrs []swarm.Address, a swarm.Address) bool {
	return swarm.ContainsAddress(addrs, a)
}

func records(m *firsthop.Memory) int {
	_, n := m.Size()
	return n
}

// TestRecord checks that the last result of a peer decides where View puts it.
func TestRecord(t *testing.T) {
	t.Parallel()

	var (
		m, clock = newMemory()
		chunk    = addr(0x6b, 0x00)
		peer     = addr(0x10)
	)

	m.Valid(radius, chunk, peer)
	if v := m.View(radius, chunk); !has(v.Valid, peer) || len(v.Paused) != 0 {
		t.Fatalf("after a valid receipt: valid %v paused %v", v.Valid, v.Paused)
	}

	m.Pause(radius, chunk, peer)
	if v := m.View(radius, chunk); len(v.Valid) != 0 || !has(v.Paused, peer) {
		t.Fatalf("after a shallow receipt: valid %v paused %v", v.Valid, v.Paused)
	}

	m.Forget(radius, chunk, peer)
	if v := m.View(radius, chunk); !has(v.Paused, peer) {
		t.Fatal("forget ended a pause that still runs")
	}

	clock.Add(time.Minute)
	m.Forget(radius, chunk, peer)
	if got := records(m); got != 0 {
		t.Fatalf("got %d records after forget that follows the pause, want 0", got)
	}

	m.Valid(radius, chunk, peer)
	m.Forget(radius, chunk, peer)
	if neighborhoods, records := m.Size(); neighborhoods != 0 || records != 0 {
		t.Fatalf("after forget of a valid record: got %d neighborhoods and %d records, want 0", neighborhoods, records)
	}
}

// TestPause checks the length of the pause.
func TestPause(t *testing.T) {
	t.Parallel()

	var (
		chunk = addr(0x6b, 0x00)
		peer  = addr(0x10)
	)

	// wantPause checks that the pause of the peer ends after d and not
	// one second before.
	wantPause := func(t *testing.T, m *firsthop.Memory, clock *testClock, d time.Duration) {
		t.Helper()

		clock.Add(d - time.Second)
		if !has(m.View(radius, chunk).Paused, peer) {
			t.Fatalf("peer is not paused one second before the end of a %s pause", d)
		}
		clock.Add(time.Second)
		if has(m.View(radius, chunk).Paused, peer) {
			t.Fatalf("peer is paused after a %s pause", d)
		}
	}

	for _, tc := range []struct {
		name string
		// before runs after the first shallow receipt of the peer
		before func(m *firsthop.Memory, clock *testClock)
		// pauses are the pauses of the shallow receipts that follow, in a row
		pauses []time.Duration
	}{
		{
			name:   "doubles up to the maximum",
			before: func(_ *firsthop.Memory, clock *testClock) { clock.Add(time.Minute) },
			pauses: []time.Duration{2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 30 * time.Minute, 30 * time.Minute},
		},
		{
			name: "valid receipt starts the count again",
			before: func(m *firsthop.Memory, clock *testClock) {
				clock.Add(time.Minute)
				m.Valid(radius, chunk, peer)
			},
			pauses: []time.Duration{time.Minute},
		},
		{
			// one hour without a shallow receipt, also when View was not called
			name:   "idle time starts the count again",
			before: func(_ *firsthop.Memory, clock *testClock) { clock.Add(time.Hour) },
			pauses: []time.Duration{time.Minute},
		},
		{
			// results of pushes that ran in parallel
			name: "receipt during a pause does not make it longer",
			before: func(m *firsthop.Memory, clock *testClock) {
				m.Pause(radius, chunk, peer)
				clock.Add(30 * time.Second)
			},
			pauses: []time.Duration{30 * time.Second},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, clock := newMemory()
			m.Pause(radius, chunk, peer)
			tc.before(m, clock)
			for _, pause := range tc.pauses {
				m.Pause(radius, chunk, peer)
				wantPause(t, m, clock, pause)
			}
		})
	}
}

// TestIdle checks that a record without a new result is removed: a valid
// record after 30 minutes, a paused record after one hour.
func TestIdle(t *testing.T) {
	t.Parallel()

	var (
		chunk = addr(0x6b, 0x00)
		peer  = addr(0x10)
	)

	for _, tc := range []struct {
		name   string
		record func(m *firsthop.Memory)
		idle   time.Duration
	}{
		{"valid", func(m *firsthop.Memory) { m.Valid(radius, chunk, peer) }, 30 * time.Minute},
		{"paused", func(m *firsthop.Memory) { m.Pause(radius, chunk, peer) }, time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, clock := newMemory()
			tc.record(m)

			clock.Add(tc.idle - time.Second)
			m.View(radius, chunk)
			if got := records(m); got != 1 {
				t.Fatalf("got %d records one second before the end of the idle time, want 1", got)
			}

			clock.Add(time.Second)
			if v := m.View(radius, chunk); len(v.Valid) != 0 || len(v.Paused) != 0 {
				t.Fatalf("idle record is used: valid %v paused %v", v.Valid, v.Paused)
			}
			if got := records(m); got != 0 {
				t.Fatalf("got %d records after the idle time, want 0", got)
			}
		})
	}
}

// TestSweep checks that the idle records of a neighborhood that gets no more
// chunks are removed when another neighborhood is used.
func TestSweep(t *testing.T) {
	t.Parallel()

	var (
		m, clock = newMemory()
		chunk    = addr(0x6b, 0x00)
		other    = addr(0x1c, 0x00)
	)

	m.View(radius, chunk) // the first sweep
	m.Valid(radius, chunk, addr(0x10))
	m.Pause(radius, chunk, addr(0x20))

	clock.Add(30 * time.Minute)
	m.View(radius, other)
	if got := records(m); got != 1 {
		t.Fatalf("got %d records after 30 minutes, want 1 (the paused record)", got)
	}

	clock.Add(30 * time.Minute)
	m.View(radius, other)
	if neighborhoods, records := m.Size(); neighborhoods != 0 || records != 0 {
		t.Fatalf("after 60 minutes: got %d neighborhoods and %d records, want 0", neighborhoods, records)
	}
}

// TestProbe checks that a neighborhood gets at most one probe per 30 seconds.
func TestProbe(t *testing.T) {
	t.Parallel()

	var (
		m, clock = newMemory()
		chunk    = addr(0x6b, 0x00)
		other    = addr(0x1c, 0x00)
		peer     = addr(0x10)
	)

	m.Valid(radius, chunk, peer)
	m.Valid(radius, other, peer)

	if !m.View(radius, chunk).ProbeDue {
		t.Fatal("probe is not due for a neighborhood that was never probed")
	}
	if !m.ClaimProbe(radius, chunk) {
		t.Fatal("probe that is due was not given")
	}
	if m.ClaimProbe(radius, chunk) {
		t.Fatal("second probe was given right after a probe")
	}
	if !m.View(radius, other).ProbeDue {
		t.Fatal("probe of one neighborhood changed another neighborhood")
	}

	// the removal of the last record does not make the next probe due
	m.Forget(radius, chunk, peer)
	m.Valid(radius, chunk, peer)

	clock.Add(29 * time.Second)
	if m.View(radius, chunk).ProbeDue {
		t.Fatal("probe is due 29s after a probe")
	}
	clock.Add(time.Second)
	if !m.View(radius, chunk).ProbeDue {
		t.Fatal("probe is not due 30s after a probe")
	}

	// a neighborhood without records is removed when the interval ends
	m.Forget(radius, chunk, peer)
	m.Forget(radius, other, peer)
	if neighborhoods, _ := m.Size(); neighborhoods != 0 {
		t.Fatalf("got %d neighborhoods after the interval, want 0", neighborhoods)
	}
}

// TestNeighborhood checks that the chunks that share the first radius bits
// share the records.
func TestNeighborhood(t *testing.T) {
	t.Parallel()

	peer := addr(0x10)

	for _, tc := range []struct {
		name   string
		radius uint8
		chunk  swarm.Address
		same   bool
	}{
		// 0x6b3f shares the first 10 bits with 0x6b00, 0x6b40 does not
		{"radius 10, same 10 bits", 10, addr(0x6b, 0x3f, 0xff), true},
		{"radius 10, bit 10 differs", 10, addr(0x6b, 0x40), false},
		{"radius 10, bit 8 differs", 10, addr(0x6a, 0x00), false},
		{"radius 8, same byte", 8, addr(0x6b, 0xff), true},
		{"radius 8, bit 8 differs", 8, addr(0x6a, 0x00), false},
		{"radius 0", 0, addr(0xff, 0xff), true},
		// the key has at most 14 bits
		{"radius 14, bit 14 differs", 14, addr(0x6b, 0x04), false},
		{"radius 16, bit 14 differs", 16, addr(0x6b, 0x04), false},
		{"radius 16, bit 15 differs", 16, addr(0x6b, 0x02), true},
		{"radius 16, bit 16 differs", 16, addr(0x6b, 0x01), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, _ := newMemory()
			m.Valid(tc.radius, addr(0x6b, 0x00), peer)

			if got := has(m.View(tc.radius, tc.chunk).Valid, peer); got != tc.same {
				t.Fatalf("chunk sees the record: got %v, want %v", got, tc.same)
			}
		})
	}
}

// TestRadiusChange checks that View at a new radius removes the records of
// the old radius, and that a result of a push that began at the old radius
// is ignored from then on, also when the memory has no records.
func TestRadiusChange(t *testing.T) {
	t.Parallel()

	var (
		m, _  = newMemory()
		chunk = addr(0x6b, 0x00)
		peer  = addr(0x10)
		late  = addr(0x20)
	)

	m.Valid(9, chunk, peer)
	m.Pause(9, chunk, late)

	if v := m.View(10, chunk); len(v.Valid) != 0 || len(v.Paused) != 0 {
		t.Fatalf("records of radius 9 are used at radius 10: valid %v paused %v", v.Valid, v.Paused)
	}

	lateResults := func() {
		t.Helper()

		m.Valid(9, chunk, late)
		m.Pause(9, chunk, late)
		m.Forget(9, chunk, peer)
		if m.ClaimProbe(9, chunk) {
			t.Fatal("probe was given for another radius")
		}
	}

	lateResults()
	if got := records(m); got != 0 {
		t.Fatalf("got %d records of the old radius in an empty memory, want 0", got)
	}

	m.Valid(10, chunk, peer)
	lateResults()
	if v := m.View(10, chunk); len(v.Valid) != 1 || !has(v.Valid, peer) || len(v.Paused) != 0 {
		t.Fatalf("got valid %v paused %v, want only the record of radius 10", v.Valid, v.Paused)
	}
}

// TestPausedOrder checks that the peers in a pause are returned in the order
// in which their pauses end.
func TestPausedOrder(t *testing.T) {
	t.Parallel()

	var (
		m, clock = newMemory()
		chunk    = addr(0x6b, 0x00)
		long     = addr(0x6a)
		first    = addr(0x10)
		last     = addr(0x20)
	)

	// two shallow receipts in a row: the pause of this peer ends at 3m
	m.Pause(radius, chunk, long)
	clock.Add(time.Minute)
	m.Pause(radius, chunk, long)
	// these pauses end at 2m10s and 2m20s
	clock.Add(10 * time.Second)
	m.Pause(radius, chunk, first)
	clock.Add(10 * time.Second)
	m.Pause(radius, chunk, last)

	paused := m.View(radius, chunk).Paused
	want := []swarm.Address{first, last, long}
	if len(paused) != len(want) {
		t.Fatalf("got %d peers in a pause, want %d", len(paused), len(want))
	}
	for i := range want {
		if !paused[i].Equal(want[i]) {
			t.Fatalf("position %d: got %s, want %s", i, paused[i], want[i])
		}
	}
}

// TestConcurrent runs all operations at the same time, for the race detector.
func TestConcurrent(t *testing.T) {
	t.Parallel()

	var (
		m, clock = newMemory()
		wg       sync.WaitGroup
	)

	for i := range 16 {
		wg.Go(func() {
			r := rand.New(rand.NewSource(int64(i)))
			for range 200 {
				chunk := addr(byte(r.Intn(4)), byte(r.Intn(256)))
				peer := addr(byte(r.Intn(8)))
				switch r.Intn(6) {
				case 0:
					m.Valid(radius, chunk, peer)
				case 1:
					m.Pause(radius, chunk, peer)
				case 2:
					m.Forget(radius, chunk, peer)
				case 3:
					m.ClaimProbe(radius, chunk)
				case 4:
					clock.Add(time.Second)
				default:
					m.View(radius, chunk)
					m.Size()
				}
			}
		})
	}
	wg.Wait()
}
