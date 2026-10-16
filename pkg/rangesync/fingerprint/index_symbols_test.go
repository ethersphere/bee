// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	"errors"
	"math/rand/v2"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
)

// encodeModel returns symbols [start, end) of the elements of m in r,
// brute force through Encode.
func encodeModel(m model, r fingerprint.Range, start, end uint64) []fingerprint.CodedSymbol {
	syms := make([]fingerprint.CodedSymbol, end-start)
	for id := range m {
		if r.ContainsID(id) {
			fingerprint.Encode(fingerprint.FloatMapper{}, syms, start, id, 1)
		}
	}
	return syms
}

// modelScan returns a scan callback over m, counting its calls.
func modelScan(m model, calls *int) func(fingerprint.Range, func(fingerprint.ID) error) error {
	return func(r fingerprint.Range, fn func(fingerprint.ID) error) error {
		if calls != nil {
			*calls++
		}
		for id := range m {
			if err := fn(id); err != nil {
				return err
			}
		}
		return nil
	}
}

// symbolCap returns the cached symbols of r under root: RootSymbols for the
// root, Level1Symbols for a level-1 range, BucketSymbols for a bucket-aligned
// range, 0 otherwise.
func symbolCap(root, r fingerprint.Range, leafBits uint8) uint64 {
	switch {
	case r.Equal(root):
		return fingerprint.RootSymbols
	case r.Bits == root.Bits+fingerprint.FanoutBits:
		return fingerprint.Level1Symbols
	case r.Bits <= root.Bits+leafBits:
		return fingerprint.BucketSymbols
	}
	return 0
}

// windows returns the [start, end) slices checked for a stream of n cached
// symbols: every slice whose ends are boundary points, plus random ones.
func windows(r *rand.Rand, n uint64) [][2]uint64 {
	points := []uint64{0, 1, 2, 31, 32, 33, 511, 512, 513, 4095, 4096}
	var ws [][2]uint64
	for _, a := range points {
		for _, b := range points {
			if a <= b && b <= n {
				ws = append(ws, [2]uint64{a, b})
			}
		}
	}
	for range 8 {
		a := r.Uint64N(n + 1)
		ws = append(ws, [2]uint64{a, a + r.Uint64N(n-a+1)})
	}
	return ws
}

func equalSymbols(a, b []fingerprint.CodedSymbol) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// checkSymbols compares Symbols of every range at every level 0..leafBits
// below root, the level-1 ranges and a few ranges below leaf depth with the
// brute-force encoding of m, for the cached window slices, with and without
// the scan path. ref, if not nil, is a second index (a rebuild of m) whose
// answers must be identical.
func checkSymbols(t *testing.T, r *rand.Rand, ix, ref *fingerprint.Index, m model, root fingerprint.Range) {
	t.Helper()
	leafBits := ix.LeafBits()
	ranges := allRanges(root, leafBits)
	ranges = append(ranges, root.Children(fingerprint.FanoutBits)...)
	for range 4 {
		ranges = append(ranges, root.Child(leafBits+1+uint8(r.IntN(3)), 0))
	}
	scan := modelScan(m, nil)
	for _, rg := range ranges {
		n := symbolCap(root, rg, leafBits)
		full := encodeModel(m, rg, 0, fingerprint.RootSymbols)
		for _, w := range windows(r, n) {
			got, exhausted, err := ix.Symbols(rg, w[0], w[1], nil)
			if err != nil {
				t.Fatalf("symbols %s [%d, %d): %v", rg, w[0], w[1], err)
			}
			if !equalSymbols(got, full[w[0]:w[1]]) {
				t.Fatalf("symbols %s [%d, %d) differ from Encode", rg, w[0], w[1])
			}
			if exhausted != (w[1] == n) {
				t.Fatalf("symbols %s [%d, %d) of %d: exhausted %v", rg, w[0], w[1], n, exhausted)
			}
			if ref != nil {
				want, wantEx, err := ref.Symbols(rg, w[0], w[1], nil)
				if err != nil || wantEx != exhausted || !equalSymbols(got, want) {
					t.Fatalf("symbols %s [%d, %d) differ from a rebuild (%v)", rg, w[0], w[1], err)
				}
			}
		}
		// Past the cache: the scan path, or exhausted and truncated.
		end := n + 40
		got, exhausted, err := ix.Symbols(rg, n/2, end, scan)
		if err != nil || exhausted || !equalSymbols(got, encodeModel(m, rg, n/2, end)) {
			t.Fatalf("scan %s [%d, %d): exhausted %v err %v", rg, n/2, end, exhausted, err)
		}
		got, exhausted, err = ix.Symbols(rg, n/2, end, nil)
		if err != nil || !exhausted || !equalSymbols(got, full[n/2:n]) {
			t.Fatalf("no scan %s [%d, %d): %d symbols, exhausted %v err %v", rg, n/2, end, len(got), exhausted, err)
		}
	}
}

// TestIndexSymbolsConsistency applies random adds, replaces and evictions
// and checks, after each round, that all three caches equal Encode over the
// brute-force set and a rebuilt index. Fixed seeds, plain test (not a fuzz
// target).
func TestIndexSymbolsConsistency(t *testing.T) {
	t.Parallel()

	for _, seed := range []uint64{1, 2, 3} {
		r := rand.New(rand.NewPCG(seed, 31))
		leafBits := uint8(2 + r.IntN(5)) // 2..6, above and below FanoutBits
		root := randomRange(r, uint8(r.IntN(4)))
		ix := newIndex(leafBits)
		m := make(model)
		for range 50 {
			m[randomIDIn(r, root)] = struct{}{}
		}
		if err := ix.Rebuild(root, sliceIterate(m.ids())); err != nil {
			t.Fatal(err)
		}
		for round := range 4 {
			for range 30 {
				var add, remove []fingerprint.ID
				ids := m.ids()
				switch op := r.IntN(3); {
				case op == 0 || len(ids) == 0: // put, sometimes outside the root
					rg := root
					if r.IntN(5) == 0 {
						rg = fingerprint.Range{}
					}
					add = append(add, randomIDIn(r, rg))
				case op == 1: // replace
					remove = append(remove, ids[r.IntN(len(ids))])
					add = append(add, randomIDIn(r, root))
				default: // evict
					remove = append(remove, ids[r.IntN(len(ids))])
				}
				for _, id := range remove {
					delete(m, id)
				}
				for _, id := range add {
					m[id] = struct{}{}
				}
				ix.Apply(add, remove)
			}
			ref := newIndex(leafBits)
			if err := ref.Rebuild(root, sliceIterate(m.ids())); err != nil {
				t.Fatal(err)
			}
			t.Logf("seed %d round %d: leaf bits %d, %d elements", seed, round, leafBits, len(m))
			checkSymbols(t, r, ix, ref, m, root)
		}
	}
}

// TestIndexSymbolsSources checks the source selection of proposal §4.3.5:
// the root and level-1 caches serve only the index's own root, a narrower
// range uses the bucket Fenwick or the scan path, and a range over ScanLimit
// elements is exhausted.
func TestIndexSymbolsSources(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(41, 41))
	root := randomRange(r, 1)
	ix := newIndex(6)
	m := make(model)
	for range fingerprint.ScanLimit + 200 {
		m[randomIDIn(r, root)] = struct{}{}
	}
	if err := ix.Rebuild(root, sliceIterate(m.ids())); err != nil {
		t.Fatal(err)
	}

	var calls int
	scan := modelScan(m, &calls)
	// The root: cached up to RootSymbols, then exhausted (count > ScanLimit).
	got, exhausted, err := ix.Symbols(root, 4000, 5000, scan)
	if err != nil || !exhausted || calls != 0 || !equalSymbols(got, encodeModel(m, root, 4000, fingerprint.RootSymbols)) {
		t.Fatalf("root past cache: %d symbols, exhausted %v, %d scans, err %v", len(got), exhausted, calls, err)
	}
	// A narrower session root (one bit below): not the own root, so only
	// the bucket Fenwick (count over ScanLimit/2, scanned only if small).
	narrow := root.Child(1, 0)
	got, exhausted, err = ix.Symbols(narrow, 0, 100, nil)
	if err != nil || !exhausted || !equalSymbols(got, encodeModel(m, narrow, 0, fingerprint.BucketSymbols)) {
		t.Fatalf("narrow root: %d symbols, exhausted %v, err %v", len(got), exhausted, err)
	}
	got, exhausted, err = ix.Symbols(narrow, 0, 100, scan)
	if err != nil || exhausted || calls != 1 || !equalSymbols(got, encodeModel(m, narrow, 0, 100)) {
		t.Fatalf("narrow root scan: %d symbols, exhausted %v, %d scans, err %v", len(got), exhausted, calls, err)
	}
	// A level-1 range inside the cache does not scan.
	l1 := root.Child(fingerprint.FanoutBits, 3)
	if _, _, err := ix.Symbols(l1, 0, fingerprint.Level1Symbols, scan); err != nil || calls != 1 {
		t.Fatalf("level-1: %d scans, err %v", calls, err)
	}
	// The returned symbols are copies: changing them changes nothing.
	got, _, _ = ix.Symbols(root, 0, 4, nil)
	got[0].Count = 1 << 40
	again, _, _ := ix.Symbols(root, 0, 4, nil)
	if again[0].Count != int64(len(m)) {
		t.Fatalf("symbol 0 count %d after editing a copy, want %d", again[0].Count, len(m))
	}
	// A scan error is returned wrapped.
	errScan := errors.New("scan")
	if _, _, err := ix.Symbols(narrow, 0, 100, func(fingerprint.Range, func(fingerprint.ID) error) error { return errScan }); !errors.Is(err, errScan) {
		t.Fatalf("scan error %v", err)
	}
}

func TestIndexSymbolsErrors(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(43, 43))
	root := randomRange(r, 3)
	ix := newIndex(4)
	if _, _, err := ix.Symbols(root, 0, 1, nil); !errors.Is(err, fingerprint.ErrBusy) {
		t.Fatalf("before rebuild: %v", err)
	}
	if err := ix.Rebuild(root, sliceIterate(nil)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ix.Symbols(siblingOf(root), 0, 1, nil); !errors.Is(err, fingerprint.ErrOutsideRoot) {
		t.Fatalf("outside root: %v", err)
	}
	if _, _, err := ix.Symbols(root, 2, 1, nil); !errors.Is(err, fingerprint.ErrInvalidBatch) {
		t.Fatalf("end before start: %v", err)
	}
	if got, exhausted, err := ix.Symbols(root, 7, 7, nil); err != nil || len(got) != 0 || exhausted {
		t.Fatalf("empty window: %d %v %v", len(got), exhausted, err)
	}
	if ix.Mapper().ID() != fingerprint.DefaultMapperID {
		t.Fatalf("mapper %d", ix.Mapper().ID())
	}
}

// countingMapper is FloatMapper counting its Indices calls.
type countingMapper struct {
	fingerprint.FloatMapper
	mu    sync.Mutex
	calls int
}

func (c *countingMapper) Indices(id fingerprint.ID) fingerprint.IndexIterator {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.FloatMapper.Indices(id)
}

// TestIndexSymbolsMapper checks that the caches are built through the
// Index's Mapper: once per element on rebuild and on apply.
func TestIndexSymbolsMapper(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(47, 47))
	cm := &countingMapper{}
	ix := fingerprint.NewIndex(fingerprint.IndexOptions{LeafBits: 4, Mapper: cm})
	ids := []fingerprint.ID{randomIDIn(r, fingerprint.Range{}), randomIDIn(r, fingerprint.Range{})}
	if err := ix.Rebuild(fingerprint.Range{}, sliceIterate(ids)); err != nil {
		t.Fatal(err)
	}
	ix.Apply([]fingerprint.ID{randomIDIn(r, fingerprint.Range{})}, ids[:1])
	if cm.calls != 4 {
		t.Fatalf("%d Indices calls, want 4", cm.calls)
	}
	if ix.Mapper() != fingerprint.Mapper(cm) {
		t.Fatal("mapper not kept")
	}
}

// TestIndexSymbolsRebuildConcurrent runs writers and Symbols readers while
// the index is rebuilt repeatedly, until enough commits overlapped a
// rebuild's iteration; the caches then equal Encode over the reserve. Run it
// with -race.
func TestIndexSymbolsRebuildConcurrent(t *testing.T) {
	t.Parallel()

	const (
		leafBits   = 5
		writers    = 4
		maxOwned   = 100
		minOverlap = 20
	)
	r := rand.New(rand.NewPCG(53, 53))
	root := randomRange(r, 1)
	ix := newIndex(leafBits)
	s := newStore()
	if err := ix.Rebuild(root, s.iterate); err != nil {
		t.Fatal(err)
	}
	iterate := func(fn func(fingerprint.ID) error) error {
		s.iterating.Add(1)
		defer s.iterating.Add(-1)
		for i, id := range s.snapshot().ids() {
			if err := fn(id); err != nil {
				return err
			}
			if i%16 == 0 {
				time.Sleep(20 * time.Microsecond)
			}
		}
		return nil
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewPCG(uint64(w), 54))
			var mine []fingerprint.ID
			for {
				select {
				case <-stop:
					return
				default:
				}
				if len(mine) > maxOwned || len(mine) > 0 && r.IntN(3) == 0 {
					k := r.IntN(len(mine))
					s.commit(ix, nil, mine[k:k+1])
					mine = append(mine[:k], mine[k+1:]...)
					continue
				}
				id := randomIDIn(r, root)
				mine = append(mine, id)
				s.commit(ix, []fingerprint.ID{id}, nil)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		r := rand.New(rand.NewPCG(55, 55))
		for {
			select {
			case <-stop:
				return
			default:
			}
			rg := root.Child(uint8(r.IntN(leafBits+1)), 0)
			if _, _, err := ix.Symbols(rg, 0, 600, nil); err != nil {
				t.Errorf("symbols %s: %v", rg, err)
			}
		}
	}()

	deadline := time.Now().Add(20 * time.Second)
	for i := 0; i < 4 || s.overlap.Load() < minOverlap; i++ {
		if time.Now().After(deadline) {
			break
		}
		if err := ix.Rebuild(root, iterate); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
	if n := s.overlap.Load(); n < minOverlap {
		t.Fatalf("%d commits overlapped a rebuild iteration, want at least %d", n, minOverlap)
	}
	checkSymbols(t, r, ix, nil, s.snapshot(), root)
}

// TestIndexSymbolsMemory checks the symbol cache budget of proposal §5.3 at
// B = 4096: 11.5 MB of bucket symbols, 0.7 MB level-1, 0.36 MB root.
func TestIndexSymbolsMemory(t *testing.T) {
	t.Parallel()

	const (
		sym     = int(unsafe.Sizeof(fingerprint.CodedSymbol{}))
		buckets = 1 << fingerprint.LeafBits
		bucket  = buckets * fingerprint.BucketSymbols * sym
		level1  = 1 << fingerprint.FanoutBits * fingerprint.Level1Symbols * sym
		rootSym = fingerprint.RootSymbols * sym
		summary = buckets * (4 + fingerprint.LtHashSize)
	)
	if sym != 88 || bucket != 11534336 || level1 != 720896 || rootSym != 360448 {
		t.Fatalf("symbol %d B, caches %d + %d + %d", sym, bucket, level1, rootSym)
	}
	ix := newIndex(0)
	if err := ix.Rebuild(fingerprint.Range{}, sliceIterate(nil)); err != nil {
		t.Fatal(err)
	}
	if got := fingerprint.IndexMemoryBytes(ix); got != float64(summary+bucket+level1+rootSym) {
		t.Fatalf("memory %v, want %d", got, summary+bucket+level1+rootSym)
	}
}

// symbolsIDs returns n random ids under the zero root.
func symbolsIDs(r *rand.Rand, n int) []fingerprint.ID {
	ids := make([]fingerprint.ID, n)
	for i := range ids {
		ids[i] = randomIDIn(r, fingerprint.Range{})
	}
	return ids
}

func BenchmarkIndexRebuild(b *testing.B) {
	ids := symbolsIDs(rand.New(rand.NewPCG(5, 6)), 20000)
	ix := newIndex(0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ix.Rebuild(fingerprint.Range{}, sliceIterate(ids)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(ids)), "ns/element")
}

func BenchmarkIndexSymbols(b *testing.B) {
	ids := symbolsIDs(rand.New(rand.NewPCG(7, 8)), 10000)
	ix := newIndex(0)
	if err := ix.Rebuild(fingerprint.Range{}, sliceIterate(ids)); err != nil {
		b.Fatal(err)
	}
	cases := []struct {
		name string
		r    fingerprint.Range
		end  uint64
	}{
		{"root", fingerprint.Range{}, fingerprint.RootSymbols},
		{"level1", fingerprint.Range{}.Child(fingerprint.FanoutBits, 7), fingerprint.Level1Symbols},
		{"bucketRun", fingerprint.Range{}.Child(8, 100), fingerprint.BucketSymbols},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, _, err := ix.Symbols(c.r, 0, c.end, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestIndexSymbolsBelowLeaf covers a level-1 range below leaf depth
// (leafBits < FanoutBits), whose scan bound is its bucket's count, and a
// window starting at or past the cache with no scan path.
func TestIndexSymbolsBelowLeaf(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(59, 59))
	root := randomRange(r, 2)
	ix := newIndex(2) // 4 buckets, each holding 4 level-1 ranges
	l1 := root.Child(fingerprint.FanoutBits, 0)
	bucket := root.Child(2, 0)
	m := make(model)
	// Fill bucket 0 past ScanLimit, but only outside l1, so l1 is sparse.
	for len(m) < fingerprint.ScanLimit+10 {
		id := randomIDIn(r, bucket)
		if !l1.ContainsID(id) {
			m[id] = struct{}{}
		}
	}
	inL1 := randomIDIn(r, l1)
	m[inL1] = struct{}{}
	if err := ix.Rebuild(root, sliceIterate(m.ids())); err != nil {
		t.Fatal(err)
	}
	var calls int
	scan := modelScan(m, &calls)
	// The whole level-1 cache: exhausted, since the bucket bound rules out
	// the scan path.
	got, exhausted, err := ix.Symbols(l1, 0, fingerprint.Level1Symbols, scan)
	if err != nil || !exhausted || calls != 0 || !equalSymbols(got, encodeModel(m, l1, 0, fingerprint.Level1Symbols)) {
		t.Fatalf("level-1 cache: exhausted %v, %d scans, err %v", exhausted, calls, err)
	}
	// Past it: the bucket count (> ScanLimit) bounds the sparse l1, so
	// exhausted (conservative) although l1 holds one element.
	got, exhausted, err = ix.Symbols(l1, 500, 600, scan)
	if err != nil || !exhausted || calls != 0 || !equalSymbols(got, encodeModel(m, l1, 500, fingerprint.Level1Symbols)) {
		t.Fatalf("level-1 past cache: %d symbols, exhausted %v, %d scans, err %v", len(got), exhausted, calls, err)
	}
	// In a small bucket the level-1 range is scanned past its cache.
	l1b := root.Child(fingerprint.FanoutBits, 4) // in bucket 1, empty
	if _, exhausted, err := ix.Symbols(l1b, 500, 600, scan); err != nil || exhausted || calls != 1 {
		t.Fatalf("small bucket: exhausted %v, %d scans, err %v", exhausted, calls, err)
	}
	// start at or past the cache, no scan: no symbols, exhausted.
	for _, start := range []uint64{fingerprint.Level1Symbols, fingerprint.Level1Symbols + 7} {
		got, exhausted, err := ix.Symbols(l1, start, start+10, nil)
		if err != nil || !exhausted || len(got) != 0 {
			t.Fatalf("start %d: %d symbols, exhausted %v, err %v", start, len(got), exhausted, err)
		}
	}
	got, exhausted, err = ix.Symbols(root, fingerprint.RootSymbols, fingerprint.RootSymbols+1, nil)
	if err != nil || !exhausted || len(got) != 0 {
		t.Fatalf("root past cache: %d symbols, exhausted %v, err %v", len(got), exhausted, err)
	}
}
