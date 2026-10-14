// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	"math/bits"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
)

// counter is a uint32 under wrapping addition, the group of the Index counts.
type counter uint32

func (c *counter) Add(o *counter) { *c += *o }
func (c *counter) Sub(o *counter) { *c -= *o }
func (c *counter) Zero()          { *c = 0 }

// lanes is a small fixed-size array under lane-wise wrapping addition, the
// shape of an LtHash.
type lanes [4]uint16

func (l *lanes) Add(o *lanes) {
	for k := range l {
		l[k] += o[k]
	}
}

func (l *lanes) Sub(o *lanes) {
	for k := range l {
		l[k] -= o[k]
	}
}

func (l *lanes) Zero() { *l = lanes{} }

// bigLanes is 1024 16-bit lanes under lane-wise wrapping addition, the size
// (2 KB) and the group of an LtHash. Four lanes are packed in each uint64 and
// added without carries between lanes (SWAR), so that the benchmark measures
// the tree rather than a slow per-lane loop.
type bigLanes [256]uint64

// laneHigh has the top bit of every 16-bit lane set.
const laneHigh = 0x8000_8000_8000_8000

func (l *bigLanes) Add(o *bigLanes) {
	for k := range l {
		x, y := l[k], o[k]
		l[k] = ((x &^ laneHigh) + (y &^ laneHigh)) ^ ((x ^ y) & laneHigh)
	}
}

func (l *bigLanes) Sub(o *bigLanes) {
	for k := range l {
		x, y := l[k], o[k]
		l[k] = ((x | laneHigh) - (y &^ laneHigh)) ^ ((x ^ ^y) & laneHigh)
	}
}

func (l *bigLanes) Zero() { *l = bigLanes{} }

func randCounter(r *rand.Rand) counter { return counter(r.Uint32()) }

func randLanes(r *rand.Rand) lanes {
	var l lanes
	for k := range l {
		l[k] = uint16(r.Uint32())
	}
	return l
}

// bruteRange returns the sum of buckets[i:j].
func bruteRange[T any, P fingerprint.Group[T]](buckets []T, i, j int) T {
	var sum T
	P(&sum).Zero()
	for k := i; k < j; k++ {
		P(&sum).Add(&buckets[k])
	}
	return sum
}

// checkTree compares every prefix and a set of ranges of f with the brute
// force sums over buckets. All ranges are checked when len(buckets) is small,
// otherwise every aligned power-of-two span (the ranges BucketSpan returns)
// and some random ones.
func checkTree[T comparable, P fingerprint.Group[T]](t *testing.T, f *fingerprint.Fenwick[T, P], buckets []T, r *rand.Rand) {
	t.Helper()

	n := len(buckets)
	if f.Len() != n {
		t.Fatalf("Len: got %d, want %d", f.Len(), n)
	}
	var got T
	for j := 0; j <= n; j++ {
		f.Prefix(j, &got)
		if want := bruteRange[T, P](buckets, 0, j); got != want {
			t.Fatalf("Prefix(%d): got %v, want %v", j, got, want)
		}
	}
	check := func(i, j int) {
		t.Helper()
		f.Range(i, j, &got)
		if want := bruteRange[T, P](buckets, i, j); got != want {
			t.Fatalf("Range(%d, %d): got %v, want %v", i, j, got, want)
		}
	}
	if n <= 64 {
		for i := 0; i <= n; i++ {
			for j := i; j <= n; j++ {
				check(i, j)
			}
		}
		return
	}
	for size := 1; size <= n; size *= 2 {
		for lo := 0; lo+size <= n; lo += size {
			check(lo, lo+size)
		}
	}
	for range 1000 {
		i, j := r.IntN(n+1), r.IntN(n+1)
		if i > j {
			i, j = j, i
		}
		check(i, j)
	}
}

// testBruteForce applies random Add, Sub and Update operations to a tree of n
// buckets and to a plain slice of buckets, and compares the two.
func testBruteForce[T comparable, P fingerprint.Group[T]](t *testing.T, n int, random func(*rand.Rand) T) {
	t.Helper()

	r := rand.New(rand.NewPCG(uint64(n), 1))
	f := fingerprint.NewFenwick[T, P](n)
	buckets := make([]T, n)
	checkTree(t, f, buckets, r)

	ops := 4 * n
	if ops > 2000 {
		ops = 2000
	}
	for range ops {
		i := r.IntN(n)
		delta := random(r)
		switch r.IntN(3) {
		case 0:
			f.Add(i, &delta)
			P(&buckets[i]).Add(&delta)
		case 1:
			f.Sub(i, &delta)
			P(&buckets[i]).Sub(&delta)
		case 2:
			f.Update(i, func(node *T) { P(node).Add(&delta) })
			P(&buckets[i]).Add(&delta)
		}
	}
	checkTree(t, f, buckets, r)
}

func TestFenwickBruteForce(t *testing.T) {
	t.Parallel()

	for _, n := range []int{1, 2, 3, 5, 8, 13, 64, 100, 4096} {
		t.Run("counter/"+strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			testBruteForce[counter](t, n, randCounter)
		})
		t.Run("lanes/"+strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			testBruteForce[lanes](t, n, randLanes)
		})
	}
}

func TestFenwickAddSubIdentity(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(7, 7))
	const n = 4096
	f := fingerprint.NewFenwick[lanes](n)
	deltas := make([]lanes, 500)
	at := make([]int, len(deltas))
	for k := range deltas {
		deltas[k], at[k] = randLanes(r), r.IntN(n)
		f.Add(at[k], &deltas[k])
	}
	for k := range deltas {
		f.Sub(at[k], &deltas[k])
	}
	checkTree(t, f, make([]lanes, n), r)
}

func TestFenwickUpdateVisitsLogNodes(t *testing.T) {
	t.Parallel()

	for _, n := range []int{1, 2, 3, 4096, 4097} {
		f := fingerprint.NewFenwick[counter](n)
		maxNodes := bits.Len(uint(n))
		for i := 0; i < n; i++ {
			visits := 0
			f.Update(i, func(*counter) { visits++ })
			if visits < 1 || visits > maxNodes {
				t.Fatalf("n %d, Update(%d): %d nodes visited, want 1..%d", n, i, visits, maxNodes)
			}
		}
	}
}

func TestNewFenwickFromBuckets(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(3, 3))
	for _, n := range []int{0, 1, 2, 3, 7, 64, 4096} {
		buckets := make([]lanes, n)
		input := make([]lanes, n)
		for k := range buckets {
			buckets[k] = randLanes(r)
		}
		copy(input, buckets)

		f := fingerprint.NewFenwickFromBuckets[lanes](input)
		checkTree(t, f, buckets, r)

		// The tree keeps working after the conversion.
		for range 100 {
			if n == 0 {
				break
			}
			i, delta := r.IntN(n), randLanes(r)
			f.Add(i, &delta)
			buckets[i].Add(&delta)
		}
		checkTree(t, f, buckets, r)
	}
}

// Not parallel: testing.AllocsPerRun panics in parallel tests.
func TestNewFenwickFromBucketsInPlace(t *testing.T) {
	// The slab is allocated once outside the measured function, so only the
	// conversion is counted. Converting it again on every run leaves
	// meaningless bucket values, which is fine: only allocations are checked.
	buckets := make([]counter, 64)
	allocs := testing.AllocsPerRun(10, func() {
		f := fingerprint.NewFenwickFromBuckets[counter](buckets)
		_ = f.Len()
	})
	if allocs > 1 {
		t.Fatalf("got %v allocations, want at most 1 (the tree header)", allocs)
	}
}

func TestFenwickReset(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(5, 5))
	f := fingerprint.NewFenwick[lanes](100)
	for range 300 {
		delta := randLanes(r)
		f.Add(r.IntN(100), &delta)
	}
	f.Reset()
	checkTree(t, f, make([]lanes, 100), r)
}

func TestFenwickOverwritesOut(t *testing.T) {
	t.Parallel()

	f := fingerprint.NewFenwick[counter](8)
	one := counter(1)
	f.Add(3, &one)

	out := counter(12345)
	f.Prefix(8, &out)
	if out != 1 {
		t.Fatalf("Prefix: got %d, want 1", out)
	}
	out = 12345
	f.Range(4, 8, &out)
	if out != 0 {
		t.Fatalf("Range: got %d, want 0", out)
	}
	out = 12345
	f.Range(3, 3, &out)
	if out != 0 {
		t.Fatalf("empty Range: got %d, want 0", out)
	}
}

func TestFenwickZeroValue(t *testing.T) {
	t.Parallel()

	var f fingerprint.Fenwick[counter, *counter]
	if f.Len() != 0 {
		t.Fatalf("Len: got %d, want 0", f.Len())
	}
	out := counter(7)
	f.Prefix(0, &out)
	if out != 0 {
		t.Fatalf("Prefix(0): got %d, want 0", out)
	}
	out = 7
	f.Range(0, 0, &out)
	if out != 0 {
		t.Fatalf("Range(0, 0): got %d, want 0", out)
	}
	f.Reset()
}

func TestFenwickBounds(t *testing.T) {
	t.Parallel()

	f := fingerprint.NewFenwick[counter](4)
	var c counter
	for _, tc := range []struct {
		name string
		f    func()
	}{
		{"Add(-1)", func() { f.Add(-1, &c) }},
		{"Add(n)", func() { f.Add(4, &c) }},
		{"Sub(-1)", func() { f.Sub(-1, &c) }},
		{"Sub(n)", func() { f.Sub(4, &c) }},
		{"Update(-1)", func() { f.Update(-1, func(*counter) {}) }},
		{"Update(n)", func() { f.Update(4, func(*counter) {}) }},
		{"Prefix(-1)", func() { f.Prefix(-1, &c) }},
		{"Prefix(n+1)", func() { f.Prefix(5, &c) }},
		{"Range(-1, 2)", func() { f.Range(-1, 2, &c) }},
		{"Range(2, 1)", func() { f.Range(2, 1, &c) }},
		{"Range(0, n+1)", func() { f.Range(0, 5, &c) }},
		{"NewFenwick(-1)", func() { fingerprint.NewFenwick[counter](-1) }},
	} {
		mustPanic(t, tc.name, tc.f)
	}
}

// Not parallel: testing.AllocsPerRun panics in parallel tests.
func TestFenwickNoAllocs(t *testing.T) {
	f := fingerprint.NewFenwick[bigLanes](4096)
	var delta, out bigLanes
	delta[0] = 1
	for _, tc := range []struct {
		name string
		f    func()
	}{
		{"Add", func() { f.Add(0, &delta) }},
		{"Sub", func() { f.Sub(0, &delta) }},
		{"Update", func() { f.Update(0, func(node *bigLanes) { node[0]++ }) }},
		{"Prefix", func() { f.Prefix(4095, &out) }},
		{"Range", func() { f.Range(2047, 4095, &out) }},
	} {
		if allocs := testing.AllocsPerRun(100, tc.f); allocs != 0 {
			t.Errorf("%s: got %v allocations, want 0", tc.name, allocs)
		}
	}
}

// BenchmarkFenwick measures the tree over 2 KB elements (the size of an
// LtHash) at B = 4096 buckets. Range(2047, 4095) is the worst case: it reads
// 23 stored nodes, the most any range of a 4096-bucket tree reads.
func BenchmarkFenwick(b *testing.B) {
	const n = 4096
	r := rand.New(rand.NewPCG(1, 2))
	buckets := make([]bigLanes, n)
	for k := range buckets {
		for l := range buckets[k] {
			buckets[k][l] = r.Uint64()
		}
	}
	f := fingerprint.NewFenwickFromBuckets[bigLanes](buckets)
	var delta, out bigLanes
	for l := range delta {
		delta[l] = r.Uint64()
	}

	b.Run("Range/worst", func(b *testing.B) {
		for b.Loop() {
			f.Range(2047, 4095, &out)
		}
	})
	b.Run("Range/random", func(b *testing.B) {
		spans := make([][2]int, 1024)
		for k := range spans {
			i, j := r.IntN(n+1), r.IntN(n+1)
			if i > j {
				i, j = j, i
			}
			spans[k] = [2]int{i, j}
		}
		k := 0
		for b.Loop() {
			s := spans[k%len(spans)]
			f.Range(s[0], s[1], &out)
			k++
		}
	})
	b.Run("Prefix/worst", func(b *testing.B) {
		for b.Loop() {
			f.Prefix(4095, &out)
		}
	})
	b.Run("Add/worst", func(b *testing.B) {
		for b.Loop() {
			f.Add(0, &delta)
		}
	})
	b.Run("Build", func(b *testing.B) {
		slab := make([]bigLanes, n)
		for b.Loop() {
			copy(slab, buckets)
			fingerprint.NewFenwickFromBuckets[bigLanes](slab)
		}
	})
}
