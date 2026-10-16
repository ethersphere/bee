// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	"errors"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// model is the brute-force reference: the set of elements of the reserve.
type model map[fingerprint.ID]struct{}

func (m model) clone() model {
	c := make(model, len(m))
	for id := range m {
		c[id] = struct{}{}
	}
	return c
}

func (m model) ids() []fingerprint.ID {
	ids := make([]fingerprint.ID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	return ids
}

// summaries returns the brute-force summary of every range at every level
// 0..leafBits below root, keyed by range.
func (m model) summaries(root fingerprint.Range, leafBits uint8) map[fingerprint.RangeKey]fingerprint.Summary {
	n := 1 << leafBits
	counts := make([]uint64, n)
	hashes := make([]fingerprint.LtHash, n)
	for id := range m {
		if !root.ContainsID(id) {
			continue
		}
		b := root.ChildIndex(swarm.NewAddress(id[:swarm.HashSize]), leafBits)
		counts[b]++
		hashes[b].Add(id)
	}
	out := make(map[fingerprint.RangeKey]fingerprint.Summary)
	for level := uint8(0); level <= leafBits; level++ {
		width := 1 << (leafBits - level)
		for i, r := range root.Children(level) {
			var c uint64
			var h fingerprint.LtHash
			for b := i * width; b < (i+1)*width; b++ {
				c += counts[b]
				h.AddHash(&hashes[b])
			}
			s := fingerprint.Summary{Count: c}
			if c > 0 {
				s.Digest = h.Digest()
			}
			out[r.Key()] = s
		}
	}
	return out
}

// allRanges returns every range at every level 0..leafBits below root.
func allRanges(root fingerprint.Range, leafBits uint8) []fingerprint.Range {
	var rs []fingerprint.Range
	for level := uint8(0); level <= leafBits; level++ {
		rs = append(rs, root.Children(level)...)
	}
	return rs
}

// checkIndex compares Summary for every range at every level with m.
func checkIndex(t *testing.T, ix *fingerprint.Index, m model, root fingerprint.Range) {
	t.Helper()
	want := m.summaries(root, ix.LeafBits())
	for _, r := range allRanges(root, ix.LeafBits()) {
		got, err := ix.Summary(r)
		if err != nil {
			t.Fatalf("summary %s: %v", r, err)
		}
		if got != want[r.Key()] {
			t.Fatalf("summary %s: got count %d digest %x, want count %d digest %x", r, got.Count, got.Digest, want[r.Key()].Count, want[r.Key()].Digest)
		}
	}
	gotRoot, s, err := ix.RootSummary()
	if err != nil {
		t.Fatalf("root summary: %v", err)
	}
	if !gotRoot.Equal(root) || s != want[root.Key()] {
		t.Fatalf("root summary: got %s %+v, want %s %+v", gotRoot, s, root, want[root.Key()])
	}
}

// fill sets b to random bytes from r.
func fill(r *rand.Rand, b []byte) {
	for i := range b {
		b[i] = byte(r.Uint32())
	}
}

// randomIDIn returns a random ID whose address lies in root.
func randomIDIn(r *rand.Rand, root fingerprint.Range) fingerprint.ID {
	var id fingerprint.ID
	fill(r, id[:])
	p := root.Prefix.Bytes()
	for k := 0; k < int(root.Bits); k++ {
		mask := byte(0x80) >> (k % 8)
		id[k/8] = id[k/8]&^mask | p[k/8]&mask
	}
	return id
}

// randomRange returns a random canonical range of the given depth.
func randomRange(r *rand.Rand, bits uint8) fingerprint.Range {
	var a [swarm.HashSize]byte
	fill(r, a[:])
	return fingerprint.NewRange(swarm.NewAddress(a[:]), bits)
}

func sliceIterate(ids []fingerprint.ID) func(fn func(fingerprint.ID) error) error {
	return func(fn func(fingerprint.ID) error) error {
		for _, id := range ids {
			if err := fn(id); err != nil {
				return err
			}
		}
		return nil
	}
}

// store is a reserve model with snapshot iteration: a commit changes the set
// atomically, an iteration reads a copy taken when it starts.
type store struct {
	mu  sync.Mutex
	set model

	iterating atomic.Int64 // iterations in progress
	overlap   atomic.Int64 // commits made while an iteration was in progress
}

func newStore() *store { return &store{set: make(model)} }

// commit applies exact changes through ix.Commit: an add only for an absent
// element, a remove only for a present one.
func (s *store) commit(ix *fingerprint.Index, add, remove []fingerprint.ID) {
	_ = ix.Commit(func() ([]fingerprint.ID, []fingerprint.ID, error) {
		if s.iterating.Load() > 0 {
			s.overlap.Add(1)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		var adds, removes []fingerprint.ID
		for _, id := range add {
			if _, ok := s.set[id]; !ok {
				s.set[id] = struct{}{}
				adds = append(adds, id)
			}
		}
		for _, id := range remove {
			if _, ok := s.set[id]; ok {
				delete(s.set, id)
				removes = append(removes, id)
			}
		}
		return adds, removes, nil
	})
}

func (s *store) snapshot() model {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.set.clone()
}

func (s *store) iterate(fn func(fingerprint.ID) error) error {
	return sliceIterate(s.snapshot().ids())(fn)
}

func newIndex(leafBits uint8) *fingerprint.Index {
	return fingerprint.NewIndex(fingerprint.IndexOptions{LeafBits: leafBits})
}

func TestIndexAdapters(t *testing.T) {
	t.Parallel()

	if got := unsafe.Sizeof(fingerprint.CountNode(0)); got != 4 {
		t.Fatalf("count node size %d, want 4", got)
	}
	if got := unsafe.Sizeof(fingerprint.LtHashNode{}); got != fingerprint.LtHashSize {
		t.Fatalf("lthash node size %d, want %d", got, fingerprint.LtHashSize)
	}

	var c fingerprint.CountNode
	one := fingerprint.CountNode(1)
	c.Sub(&one)
	if c != 1<<32-1 {
		t.Fatalf("count wraps to %d", c)
	}
	c.Add(&one)
	if c != 0 {
		t.Fatalf("count %d after +1, want 0", c)
	}
	c = 5
	c.Zero()
	if c != 0 {
		t.Fatalf("count %d after zero", c)
	}

	r := rand.New(rand.NewPCG(1, 1))
	a, b := randomIDIn(r, fingerprint.Range{}), randomIDIn(r, fingerprint.Range{})
	ea, eb := fingerprint.Expand(nil, a), fingerprint.Expand(nil, b)
	var want fingerprint.LtHash
	want.Add(a)
	want.Add(b)
	h := fingerprint.LtHashNode(ea)
	nb := fingerprint.LtHashNode(eb)
	h.Add(&nb)
	if fingerprint.LtHash(h) != want {
		t.Fatal("adapter Add differs from AddHash")
	}
	h.Sub(&nb)
	if fingerprint.LtHash(h) != ea {
		t.Fatal("adapter Sub differs from SubHash")
	}
	h.Zero()
	if fingerprint.LtHash(h) != (fingerprint.LtHash{}) {
		t.Fatal("adapter Zero is not the empty set")
	}
}

func TestNewIndex(t *testing.T) {
	t.Parallel()

	if got := newIndex(0).LeafBits(); got != fingerprint.LeafBits {
		t.Fatalf("default leaf bits %d, want %d", got, fingerprint.LeafBits)
	}
	if got := newIndex(4).LeafBits(); got != 4 {
		t.Fatalf("leaf bits %d, want 4", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("leaf bits 17 did not panic")
		}
	}()
	newIndex(17)
}

// TestIndexBeforeRebuild checks that an index with no rebuild answers Busy
// and drops deltas.
func TestIndexBeforeRebuild(t *testing.T) {
	t.Parallel()

	ix := newIndex(4)
	if _, err := ix.Summary(fingerprint.Range{}); !errors.Is(err, fingerprint.ErrBusy) {
		t.Fatalf("summary before rebuild: %v, want ErrBusy", err)
	}
	if _, _, err := ix.RootSummary(); !errors.Is(err, fingerprint.ErrBusy) {
		t.Fatalf("root summary before rebuild: %v, want ErrBusy", err)
	}
	r := rand.New(rand.NewPCG(2, 2))
	ix.Apply([]fingerprint.ID{randomIDIn(r, fingerprint.Range{})}, nil)
	if got := fingerprint.IndexOutsideRoot(ix); got != 0 {
		t.Fatalf("dropped delta counted outside root: %v", got)
	}
	if err := ix.Rebuild(fingerprint.Range{}, sliceIterate(nil)); err != nil {
		t.Fatal(err)
	}
	checkIndex(t, ix, model{}, fingerprint.Range{})
}

func TestIndexSummary(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(3, 3))
	for _, bits := range []uint8{0, 3, 8, 13} {
		root := randomRange(r, bits)
		ix := newIndex(5)
		m := make(model)
		for range 300 {
			m[randomIDIn(r, root)] = struct{}{}
		}
		if err := ix.Rebuild(root, sliceIterate(m.ids())); err != nil {
			t.Fatal(err)
		}
		checkIndex(t, ix, m, root)

		// A non-canonical prefix: bits past Bits are ignored.
		child := root.Child(3, 5)
		p := child.Prefix.Clone().Bytes()
		if rem := child.Bits % 8; rem != 0 {
			p[len(p)-1] |= 0xff >> rem
		}
		p = append(p, 0xff, 0xff)
		nonCanonical := fingerprint.Range{Prefix: swarm.NewAddress(p), Bits: child.Bits}
		got, err := ix.Summary(nonCanonical)
		if err != nil {
			t.Fatal(err)
		}
		if want, _ := ix.Summary(child); got != want {
			t.Fatalf("non-canonical %s: %+v, want %+v", child, got, want)
		}

		// Below the leaf depth.
		if _, err := ix.Summary(root.Child(6, 1)); !errors.Is(err, fingerprint.ErrBelowLeaf) {
			t.Fatalf("below leaf: %v, want ErrBelowLeaf", err)
		}
		// Outside the root.
		if bits > 0 {
			outside := siblingOf(root)
			if _, err := ix.Summary(outside); !errors.Is(err, fingerprint.ErrOutsideRoot) {
				t.Fatalf("outside root: %v, want ErrOutsideRoot", err)
			}
			if _, err := ix.Summary(fingerprint.Range{}); !errors.Is(err, fingerprint.ErrOutsideRoot) {
				t.Fatalf("range wider than root: %v, want ErrOutsideRoot", err)
			}
		}
	}
}

// siblingOf returns the range of the same depth as r that differs from it in
// its last bit. r.Bits must be positive.
func siblingOf(r fingerprint.Range) fingerprint.Range {
	p := r.Prefix.Clone().Bytes()
	k := int(r.Bits) - 1
	p[k/8] ^= 0x80 >> (k % 8)
	return fingerprint.NewRange(swarm.NewAddress(p), r.Bits)
}

func TestIndexEmptyRangeZeroDigest(t *testing.T) {
	t.Parallel()

	ix := newIndex(2)
	if err := ix.Rebuild(fingerprint.Range{}, sliceIterate(nil)); err != nil {
		t.Fatal(err)
	}
	s, err := ix.Summary(fingerprint.Range{})
	if err != nil {
		t.Fatal(err)
	}
	if s != (fingerprint.Summary{}) {
		t.Fatalf("empty summary %+v, want zero", s)
	}
}

func TestIndexApply(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(4, 4))
	root := randomRange(r, 4)
	ix := newIndex(4)
	m := make(model)
	for range 50 {
		m[randomIDIn(r, root)] = struct{}{}
	}
	if err := ix.Rebuild(root, sliceIterate(m.ids())); err != nil {
		t.Fatal(err)
	}
	adds := make([]fingerprint.ID, 0, 31)
	var removes []fingerprint.ID
	for range 30 {
		id := randomIDIn(r, root)
		m[id] = struct{}{}
		adds = append(adds, id)
	}
	for id := range m {
		if len(removes) == 20 {
			break
		}
		delete(m, id)
		removes = append(removes, id)
	}
	// An element added and removed in the same delta is a no-op.
	both := randomIDIn(r, root)
	ix.Apply(append(adds, both), append(removes, both))
	checkIndex(t, ix, m, root)

	outside := randomIDIn(r, siblingOf(root))
	ix.Apply([]fingerprint.ID{outside}, []fingerprint.ID{outside, outside})
	if got := fingerprint.IndexOutsideRoot(ix); got != 3 {
		t.Fatalf("outside root %v, want 3", got)
	}
	checkIndex(t, ix, m, root)
}

func TestIndexCommit(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(5, 5))
	ix := newIndex(3)
	if err := ix.Rebuild(fingerprint.Range{}, sliceIterate(nil)); err != nil {
		t.Fatal(err)
	}
	id := randomIDIn(r, fingerprint.Range{})
	errRun := errors.New("run failed")
	calls := 0
	err := ix.Commit(func() ([]fingerprint.ID, []fingerprint.ID, error) {
		calls++
		return []fingerprint.ID{id}, nil, errRun
	})
	if err != errRun { //nolint:errorlint // the error is returned as is
		t.Fatalf("commit error %v, want the run error as is", err)
	}
	if calls != 1 {
		t.Fatalf("run called %d times", calls)
	}
	checkIndex(t, ix, model{}, fingerprint.Range{})

	calls = 0
	if err := ix.Commit(func() ([]fingerprint.ID, []fingerprint.ID, error) {
		calls++
		return []fingerprint.ID{id}, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("run called %d times", calls)
	}
	checkIndex(t, ix, model{id: {}}, fingerprint.Range{})
}

// TestIndexRebuildSnapshot checks the rebuild protocol: commits made before
// the snapshot are in it, commits made after it opened are delivered as
// deltas, and the result equals the reserve.
func TestIndexRebuildSnapshot(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(6, 6))
	root := randomRange(r, 2)
	ix := newIndex(4)
	s := newStore()
	for range 100 {
		s.commit(ix, []fingerprint.ID{randomIDIn(r, root)}, nil)
	}
	if err := ix.Rebuild(root, s.iterate); err != nil {
		t.Fatal(err)
	}
	checkIndex(t, ix, s.snapshot(), root)

	// The second rebuild, to a wider root, commits while it iterates.
	wider := fingerprint.NewRange(root.Prefix, 1)
	snap := s.snapshot().ids()
	err := ix.Rebuild(wider, func(fn func(fingerprint.ID) error) error {
		for i, id := range snap {
			if err := fn(id); err != nil {
				return err
			}
			if i%10 == 0 {
				// One element of the old root, one of the new half, and a
				// removal of an element of the snapshot.
				s.commit(ix, []fingerprint.ID{randomIDIn(r, root), randomIDIn(r, wider)}, []fingerprint.ID{snap[(i+50)%len(snap)]})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	checkIndex(t, ix, s.snapshot(), wider)
}

// TestIndexRebuildConversionWindow commits between the conversion of the new
// slab and the swap (rebuild step 5).
func TestIndexRebuildConversionWindow(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(7, 7))
	root := randomRange(r, 3)
	ix := newIndex(4)
	s := newStore()
	for range 64 {
		s.commit(ix, []fingerprint.ID{randomIDIn(r, root)}, nil)
	}
	if err := ix.Rebuild(root, s.iterate); err != nil {
		t.Fatal(err)
	}
	called := false
	fingerprint.SetAfterConvertHook(ix, func() {
		called = true
		snap := s.snapshot().ids()
		s.commit(ix, []fingerprint.ID{randomIDIn(r, root), randomIDIn(r, root)}, snap[:3])
	})
	if err := ix.Rebuild(root, s.iterate); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("hook not called")
	}
	checkIndex(t, ix, s.snapshot(), root)
}

// TestIndexRebuildIterateExits checks that the write lock is released and
// the rebuild state cleared on every exit from iterate.
func TestIndexRebuildIterateExits(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(8, 8))
	root := randomRange(r, 2)
	errIter := errors.New("iterate failed")
	ids := []fingerprint.ID{randomIDIn(r, root), randomIDIn(r, root)}

	for _, tc := range []struct {
		name    string
		iterate func(fn func(fingerprint.ID) error) error
		panics  bool
	}{
		{"error before first element", func(func(fingerprint.ID) error) error { return errIter }, false},
		{"error after first element", func(fn func(fingerprint.ID) error) error {
			_ = fn(ids[0])
			return errIter
		}, false},
		{"panic before first element", func(func(fingerprint.ID) error) error { panic("boom") }, true},
		{"panic after first element", func(fn func(fingerprint.ID) error) error {
			_ = fn(ids[0])
			panic("boom")
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ix := newIndex(3)
			func() {
				defer func() {
					if p := recover(); (p != nil) != tc.panics {
						t.Fatalf("panic %v, want panic %v", p, tc.panics)
					}
				}()
				err := ix.Rebuild(root, tc.iterate)
				if !tc.panics && !errors.Is(err, errIter) {
					t.Fatalf("rebuild error %v, want %v", err, errIter)
				}
			}()
			// The lock is free and the index is back to dropping deltas.
			done := make(chan struct{})
			go func() {
				defer close(done)
				ix.Apply(ids[:1], nil)
				if _, err := ix.Summary(root); !errors.Is(err, fingerprint.ErrBusy) {
					t.Errorf("summary after failed first rebuild: %v, want ErrBusy", err)
				}
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("index lock not released")
			}
			if err := ix.Rebuild(root, sliceIterate(ids[1:])); err != nil {
				t.Fatalf("rebuild after failure: %v", err)
			}
			checkIndex(t, ix, model{ids[1]: {}}, root)
		})
	}
}

// TestIndexRebuildFailure checks that a failed rebuild keeps the live slab,
// which stays current.
func TestIndexRebuildFailure(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(9, 9))
	root := randomRange(r, 3)
	ix := newIndex(4)
	s := newStore()
	for range 40 {
		s.commit(ix, []fingerprint.ID{randomIDIn(r, root)}, nil)
	}
	if err := ix.Rebuild(root, s.iterate); err != nil {
		t.Fatal(err)
	}
	errIter := errors.New("iterate failed")
	err := ix.Rebuild(fingerprint.NewRange(root.Prefix, 2), func(fn func(fingerprint.ID) error) error {
		_ = fn(randomIDIn(r, root))
		s.commit(ix, []fingerprint.ID{randomIDIn(r, root)}, nil)
		return errIter
	})
	if !errors.Is(err, errIter) {
		t.Fatalf("rebuild error %v", err)
	}
	s.commit(ix, []fingerprint.ID{randomIDIn(r, root)}, nil)
	checkIndex(t, ix, s.snapshot(), root)
}

// TestIndexRebuildBusy checks the answers while a rebuild to a new root runs
// and that a second rebuild is refused.
func TestIndexRebuildBusy(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(10, 10))
	root := randomRange(r, 3)
	ix := newIndex(4)
	s := newStore()
	for range 40 {
		s.commit(ix, []fingerprint.ID{randomIDIn(r, root)}, nil)
		s.commit(ix, []fingerprint.ID{randomIDIn(r, siblingOf(root))}, nil)
	}
	if err := ix.Rebuild(root, s.iterate); err != nil {
		t.Fatal(err)
	}
	wider := fingerprint.NewRange(root.Prefix, 2)
	snap := s.snapshot()
	err := ix.Rebuild(wider, func(fn func(fingerprint.ID) error) error {
		if err := sliceIterate(snap.ids())(fn); err != nil {
			return err
		}
		if err := ix.Rebuild(root, sliceIterate(nil)); !errors.Is(err, fingerprint.ErrBusy) {
			t.Errorf("concurrent rebuild: %v, want ErrBusy", err)
		}
		// Ranges of the new root outside the old one are busy.
		for _, rg := range []fingerprint.Range{wider, siblingOf(root), siblingOf(root).Child(2, 1)} {
			if _, err := ix.Summary(rg); !errors.Is(err, fingerprint.ErrBusy) {
				t.Errorf("summary %s during rebuild: %v, want ErrBusy", rg, err)
			}
		}
		// Ranges of the old root are answered by the old slab.
		want := snap.summaries(root, 4)
		for _, rg := range allRanges(root, 4) {
			got, err := ix.Summary(rg)
			if err != nil || got != want[rg.Key()] {
				t.Errorf("summary %s during rebuild: %+v %v, want %+v", rg, got, err, want[rg.Key()])
			}
		}
		// Ranges outside both roots.
		if _, err := ix.Summary(siblingOf(wider)); !errors.Is(err, fingerprint.ErrOutsideRoot) {
			t.Errorf("summary outside both roots: %v, want ErrOutsideRoot", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	checkIndex(t, ix, s.snapshot(), wider)
}

// TestIndexRebuildRootMove moves the root up and down and ignores elements
// outside it.
func TestIndexRebuildRootMove(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(11, 11))
	overlay := randomRange(r, 255).Prefix
	ix := newIndex(4)
	s := newStore()
	for range 400 {
		s.commit(ix, []fingerprint.ID{randomIDIn(r, fingerprint.NewRange(overlay, uint8(r.IntN(6))))}, nil)
	}
	outside := 0.0
	for _, radius := range []uint8{3, 2, 4, 4, 0, 5} {
		root := fingerprint.NewRange(overlay, radius)
		snap := s.snapshot()
		for id := range snap {
			if !root.ContainsID(id) {
				outside++
			}
		}
		if err := ix.Rebuild(root, sliceIterate(snap.ids())); err != nil {
			t.Fatal(err)
		}
		checkIndex(t, ix, snap, root)
		if got := fingerprint.IndexOutsideRoot(ix); got != outside {
			t.Fatalf("outside root %v, want %v", got, outside)
		}
	}
}

func TestIndexRebuildInvalidRoot(t *testing.T) {
	t.Parallel()

	ix := newIndex(12)
	root := fingerprint.NewRange(swarm.ZeroAddress, 244)
	if err := ix.Rebuild(root, sliceIterate(nil)); err == nil {
		t.Fatal("rebuild of a root too deep for the leaf bits succeeded")
	}
	if err := ix.Rebuild(fingerprint.NewRange(swarm.ZeroAddress, 243), sliceIterate(nil)); err != nil {
		t.Fatalf("rebuild at the deepest root: %v", err)
	}
}

func TestIndexSubscribeRootChange(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(12, 12))
	root := randomRange(r, 2)

	expect := func(t *testing.T, c <-chan struct{}, want bool) {
		t.Helper()
		select {
		case _, ok := <-c:
			if !want || !ok {
				t.Fatalf("unexpected signal (open %v)", ok)
			}
		default:
			if want {
				t.Fatal("no signal")
			}
		}
	}

	t.Run("immediate", func(t *testing.T) {
		t.Parallel()

		ix := newIndex(3)
		c, unsubscribe := ix.SubscribeRootChange()
		c2, unsubscribe2 := ix.SubscribeRootChange()
		defer unsubscribe2()

		ix.Apply([]fingerprint.ID{randomIDIn(r, root)}, nil) // dropped
		expect(t, c, false)
		if err := ix.Rebuild(root, sliceIterate(nil)); err != nil {
			t.Fatal(err)
		}
		expect(t, c, true)
		expect(t, c2, true)

		ix.Apply([]fingerprint.ID{randomIDIn(r, siblingOf(root))}, nil)
		expect(t, c, false)

		// Signals coalesce.
		ix.Apply([]fingerprint.ID{randomIDIn(r, root)}, nil)
		ix.Apply([]fingerprint.ID{randomIDIn(r, root)}, nil)
		expect(t, c, true)
		expect(t, c, false)

		unsubscribe()
		unsubscribe()
		if _, ok := <-c; ok {
			t.Fatal("channel open after unsubscribe")
		}
		ix.Apply([]fingerprint.ID{randomIDIn(r, root)}, nil)
		expect(t, c2, true)

		if err := ix.Close(); err != nil {
			t.Fatal(err)
		}
		ix.Apply([]fingerprint.ID{randomIDIn(r, root)}, nil)
		expect(t, c2, false)
	})

	t.Run("debounced", func(t *testing.T) {
		t.Parallel()

		const debounce = 50 * time.Millisecond
		ix := fingerprint.NewIndex(fingerprint.IndexOptions{LeafBits: 3, RootChangeDebounce: debounce})
		defer ix.Close()
		c, unsubscribe := ix.SubscribeRootChange()
		defer unsubscribe()

		if err := ix.Rebuild(root, sliceIterate(nil)); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		for range 20 {
			ix.Apply([]fingerprint.ID{randomIDIn(r, root)}, nil)
		}
		expect(t, c, false)
		select {
		case <-c:
		case <-time.After(5 * time.Second):
			t.Fatal("no debounced signal")
		}
		if d := time.Since(start); d < debounce/2 {
			t.Fatalf("signal after %v, want about %v", d, debounce)
		}
		time.Sleep(2 * debounce)
		expect(t, c, false)
	})

	t.Run("close stops a pending signal", func(t *testing.T) {
		t.Parallel()

		ix := fingerprint.NewIndex(fingerprint.IndexOptions{LeafBits: 3, RootChangeDebounce: 20 * time.Millisecond})
		c, unsubscribe := ix.SubscribeRootChange()
		defer unsubscribe()
		if err := ix.Rebuild(root, sliceIterate(nil)); err != nil {
			t.Fatal(err)
		}
		if err := ix.Close(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(60 * time.Millisecond)
		expect(t, c, false)
	})
}

// TestIndexMemory checks the slab sizes of proposal §5.3 at B = 4096: 16 KB
// of counts and 8 MB of LtHash, twice that while a rebuild runs.
func TestIndexMemory(t *testing.T) {
	t.Parallel()

	const (
		buckets = 1 << fingerprint.LeafBits
		counts  = buckets * 4                      // 16 KB
		hashes  = buckets * fingerprint.LtHashSize // 8 MB
		// The symbol caches: bucket Fenwick, level-1 and root (§5.3).
		symbols = (buckets*fingerprint.BucketSymbols + 1<<fingerprint.FanoutBits*fingerprint.Level1Symbols + fingerprint.RootSymbols) * 88
	)
	if counts != 16<<10 || hashes != 8<<20 {
		t.Fatalf("budget %d + %d", counts, hashes)
	}
	ix := newIndex(0)
	if got := fingerprint.IndexMemoryBytes(ix); got != 0 {
		t.Fatalf("memory before rebuild %v", got)
	}
	var during float64
	err := ix.Rebuild(fingerprint.Range{}, func(fn func(fingerprint.ID) error) error {
		during = fingerprint.IndexMemoryBytes(ix)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if during != counts+hashes+symbols {
		t.Fatalf("memory during the first rebuild %v, want %d", during, counts+hashes+symbols)
	}
	if got := fingerprint.IndexMemoryBytes(ix); got != counts+hashes+symbols {
		t.Fatalf("memory %v, want %d", got, counts+hashes+symbols)
	}
	err = ix.Rebuild(fingerprint.Range{}, func(fn func(fingerprint.ID) error) error {
		during = fingerprint.IndexMemoryBytes(ix)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if during != 2*(counts+hashes+symbols) {
		t.Fatalf("memory during a rebuild %v, want %d", during, 2*(counts+hashes+symbols))
	}
	if got := fingerprint.IndexMemoryBytes(ix); got != counts+hashes+symbols {
		t.Fatalf("memory after the rebuild %v, want %d", got, counts+hashes+symbols)
	}
}

// TestIndexConcurrent runs writers and Summary readers while the index is
// rebuilt repeatedly, until enough commits overlapped a rebuild's iteration;
// readers see only valid answers and the result equals the reserve. Run it
// with -race.
func TestIndexConcurrent(t *testing.T) {
	t.Parallel()

	const (
		leafBits   = 5
		writers    = 4
		maxOwned   = 150
		minOverlap = 20
	)
	r := rand.New(rand.NewPCG(13, 13))
	overlay := randomRange(r, 255).Prefix
	ix := newIndex(leafBits)
	s := newStore()
	if err := ix.Rebuild(fingerprint.NewRange(overlay, 1), s.iterate); err != nil {
		t.Fatal(err)
	}
	// A slowed-down iteration, so that commits overlap it.
	iterate := func(fn func(fingerprint.ID) error) error {
		s.iterating.Add(1)
		defer s.iterating.Add(-1)
		for i, id := range s.snapshot().ids() {
			if err := fn(id); err != nil {
				return err
			}
			if i%32 == 0 {
				time.Sleep(20 * time.Microsecond)
			}
		}
		return nil
	}

	// Writers run until the rebuild loop below has seen enough overlap.
	var wg sync.WaitGroup
	stopWriters := make(chan struct{})
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewPCG(uint64(w), 99))
			var mine []fingerprint.ID
			for {
				select {
				case <-stopWriters:
					return
				default:
				}
				if len(mine) > maxOwned || len(mine) > 0 && r.IntN(3) == 0 {
					k := r.IntN(len(mine))
					s.commit(ix, nil, mine[k:k+1])
					mine = append(mine[:k], mine[k+1:]...)
					continue
				}
				id := randomIDIn(r, fingerprint.NewRange(overlay, uint8(r.IntN(3))))
				mine = append(mine, id)
				s.commit(ix, []fingerprint.ID{id}, nil)
			}
		}()
	}
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for range 2 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			r := rand.New(rand.NewPCG(5, 6))
			for {
				select {
				case <-stop:
					return
				default:
				}
				// At most leafBits deep, so never below the leaf depth.
				rg := fingerprint.NewRange(overlay, uint8(r.IntN(leafBits+1)))
				if _, err := ix.Summary(rg); err != nil && !errors.Is(err, fingerprint.ErrOutsideRoot) && !errors.Is(err, fingerprint.ErrBusy) {
					t.Errorf("summary %s: %v", rg, err)
				}
				if _, _, err := ix.RootSummary(); err != nil {
					t.Errorf("root summary: %v", err)
				}
			}
		}()
	}

	var radius uint8
	deadline := time.Now().Add(20 * time.Second)
	for i := 0; i < 6 || s.overlap.Load() < minOverlap; i++ {
		if time.Now().After(deadline) {
			break // reported below, after the goroutines stopped
		}
		radius = uint8(i % 3)
		if err := ix.Rebuild(fingerprint.NewRange(overlay, radius), iterate); err != nil {
			t.Fatal(err)
		}
	}
	close(stopWriters)
	wg.Wait()
	close(stop)
	readers.Wait()
	if n := s.overlap.Load(); n < minOverlap {
		t.Fatalf("%d commits overlapped a rebuild iteration, want at least %d", n, minOverlap)
	}
	checkIndex(t, ix, s.snapshot(), fingerprint.NewRange(overlay, radius))
}

// TestIndexNoPartialCommit checks that a reader never sees part of a
// commit: under concurrent replaces (one removal and one addition per
// commit) the reserve always holds the same number of elements, and so must
// every root summary.
func TestIndexNoPartialCommit(t *testing.T) {
	t.Parallel()

	const (
		writers  = 2
		perOwner = 32
		replaces = 3000
	)
	r := rand.New(rand.NewPCG(17, 17))
	root := randomRange(r, 2)
	ix := newIndex(4)
	s := newStore()
	owned := make([][]fingerprint.ID, writers)
	for w := range owned {
		for range perOwner {
			id := randomIDIn(r, root)
			owned[w] = append(owned[w], id)
			s.commit(ix, []fingerprint.ID{id}, nil)
		}
	}
	if err := ix.Rebuild(root, s.iterate); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var bad, reads atomic.Int64
	var reader sync.WaitGroup
	reader.Add(1)
	go func() {
		defer reader.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_, sm, err := ix.RootSummary()
			if err != nil {
				t.Error(err)
				return
			}
			reads.Add(1)
			if sm.Count != writers*perOwner {
				bad.Add(1)
			}
		}
	}()
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewPCG(uint64(w), 18))
			for range replaces {
				k := r.IntN(perOwner)
				id := randomIDIn(r, root)
				s.commit(ix, []fingerprint.ID{id}, []fingerprint.ID{owned[w][k]})
				owned[w][k] = id
			}
		}()
	}
	wg.Wait()
	close(stop)
	reader.Wait()
	if n := bad.Load(); n > 0 {
		t.Fatalf("%d of %d root summaries saw a count the reserve never held", n, reads.Load())
	}
	checkIndex(t, ix, s.snapshot(), root)
}

// TestIndexSummaryProductionLeafBits checks every range at every level at
// the protocol leaf bits (4096 buckets).
func TestIndexSummaryProductionLeafBits(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(19, 19))
	root := randomRange(r, 7)
	ix := newIndex(0)
	m := make(model)
	for range 400 {
		m[randomIDIn(r, root)] = struct{}{}
	}
	if err := ix.Rebuild(root, sliceIterate(m.ids())); err != nil {
		t.Fatal(err)
	}
	checkIndex(t, ix, m, root)
	for range 50 {
		id := randomIDIn(r, root)
		m[id] = struct{}{}
		ix.Apply([]fingerprint.ID{id}, nil)
	}
	checkIndex(t, ix, m, root)
}

func BenchmarkIndexApply(b *testing.B) {
	r := rand.New(rand.NewPCG(1, 2))
	ix := newIndex(0)
	if err := ix.Rebuild(fingerprint.Range{}, sliceIterate(nil)); err != nil {
		b.Fatal(err)
	}
	ids := make([]fingerprint.ID, 1024)
	for i := range ids {
		ids[i] = randomIDIn(r, fingerprint.Range{})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ix.Apply(ids[i%len(ids):i%len(ids)+1], nil)
	}
}

func BenchmarkIndexSummary(b *testing.B) {
	r := rand.New(rand.NewPCG(3, 4))
	ix := newIndex(0)
	ids := make([]fingerprint.ID, 10000)
	for i := range ids {
		ids[i] = randomIDIn(r, fingerprint.Range{})
	}
	if err := ix.Rebuild(fingerprint.Range{}, sliceIterate(ids)); err != nil {
		b.Fatal(err)
	}
	// The worst-case bucket run, (2047, 4095), as in BenchmarkFenwick, is a
	// depth-12 range; use a level-1 range and a bucket instead.
	ranges := []fingerprint.Range{
		fingerprint.Range{}.Child(4, 7),
		fingerprint.Range{}.Child(fingerprint.LeafBits, 2047),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ix.Summary(ranges[i%len(ranges)]); err != nil {
			b.Fatal(err)
		}
	}
}
