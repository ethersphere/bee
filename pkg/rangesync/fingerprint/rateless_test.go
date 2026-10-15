// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	binenc "encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/sha3"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// testMapper is a second, test-only Mapper with integer arithmetic only: it
// proves that the coder works with any Mapper. From index i the gap is
// uniform in [1, i+1], so the density is about 2/i, like ρ.
type testMapper struct{}

func (testMapper) ID() uint32 { return 0x7e57 }

func (testMapper) Indices(id fingerprint.ID) fingerprint.IndexIterator {
	return &testIterator{x: binenc.LittleEndian.Uint64(id[:8]) ^ binenc.LittleEndian.Uint64(id[32:40])}
}

func (testMapper) Expected(count, i uint64) int64 {
	return int64((count / (i/2 + 1)) >> 1)
}

type testIterator struct {
	x       uint64
	i       uint64
	started bool
}

func (it *testIterator) Next() uint64 {
	if !it.started {
		it.started = true
		return 0
	}
	// splitmix64
	it.x += 0x9e3779b97f4a7c15
	z := it.x
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	it.i += 1 + z%(it.i+1)
	return it.i
}

// scriptedMapper maps the ids it knows to fixed indices and every other id
// to symbol 0 only.
type scriptedMapper map[fingerprint.ID][]uint64

func (scriptedMapper) ID() uint32 { return 0x5c }

func (m scriptedMapper) Indices(id fingerprint.ID) fingerprint.IndexIterator {
	idx, ok := m[id]
	if !ok {
		idx = []uint64{0}
	}
	return &sliceIterator{idx: idx}
}

func (scriptedMapper) Expected(uint64, uint64) int64 { return 0 }

type sliceIterator struct{ idx []uint64 }

func (it *sliceIterator) Next() uint64 {
	if len(it.idx) == 0 {
		return fingerprint.EndIndex
	}
	i := it.idx[0]
	it.idx = it.idx[1:]
	return i
}

var mappers = []struct {
	name string
	m    fingerprint.Mapper
}{
	{"float", fingerprint.NewFloatMapper()},
	{"test", testMapper{}},
}

var fullRange = fingerprint.Range{Prefix: swarm.ZeroAddress, Bits: 0}

func newRand(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, 0x5eed))
}

func randIDv2(r *rand.Rand) fingerprint.ID {
	var id fingerprint.ID
	for k := 0; k < fingerprint.IDSize; k += 8 {
		binenc.LittleEndian.PutUint64(id[k:], r.Uint64())
	}
	return id
}

func randIDs(r *rand.Rand, n int) []fingerprint.ID {
	ids := make([]fingerprint.ID, n)
	for k := range ids {
		ids[k] = randIDv2(r)
	}
	return ids
}

func randSymbol(r *rand.Rand) fingerprint.CodedSymbol {
	s := fingerprint.CodedSymbol{Sum: randIDv2(r), Count: int64(r.Uint64())}
	binenc.LittleEndian.PutUint64(s.Check[:], r.Uint64())
	binenc.LittleEndian.PutUint64(s.Check[8:], r.Uint64())
	return s
}

// encodeSet returns symbols [start, start+n) of the stream of ids.
func encodeSet(m fingerprint.Mapper, ids []fingerprint.ID, start uint64, n int) []fingerprint.CodedSymbol {
	syms := make([]fingerprint.CodedSymbol, n)
	for _, id := range ids {
		fingerprint.Encode(m, syms, start, id, 1)
	}
	return syms
}

func sortIDs(ids []fingerprint.ID) []fingerprint.ID {
	out := slices.Clone(ids)
	slices.SortFunc(out, fingerprint.ID.Compare)
	return out
}

func equalIDs(a, b []fingerprint.ID) bool {
	return slices.Equal(sortIDs(a), sortIDs(b))
}

func TestCodedSymbolGroup(t *testing.T) {
	t.Parallel()

	r := newRand(1)
	for range 100 {
		a, b, c := randSymbol(r), randSymbol(r), randSymbol(r)

		ab := a
		ab.Add(&b)
		abc1 := ab
		abc1.Add(&c)
		bc := b
		bc.Add(&c)
		abc2 := a
		abc2.Add(&bc)
		if abc1 != abc2 {
			t.Fatal("Add is not associative")
		}
		ba := b
		ba.Add(&a)
		if ab != ba {
			t.Fatal("Add is not commutative")
		}
		ab.Sub(&b)
		if ab != a {
			t.Fatal("Sub does not invert Add")
		}
		self := a
		self.Sub(&self)
		if self != (fingerprint.CodedSymbol{}) {
			t.Fatal("a - a is not zero")
		}
		z := a
		z.Zero()
		if z != (fingerprint.CodedSymbol{}) {
			t.Fatal("Zero is not the zero value")
		}
		z.Add(&a)
		if z != a {
			t.Fatal("zero is not the identity")
		}
	}

	wrap := fingerprint.CodedSymbol{Count: math.MaxInt64}
	wrap.Add(&fingerprint.CodedSymbol{Count: 1})
	if wrap.Count != math.MinInt64 {
		t.Fatalf("count did not wrap: %d", wrap.Count)
	}
	wrap.Sub(&fingerprint.CodedSymbol{Count: 1})
	if wrap.Count != math.MaxInt64 {
		t.Fatalf("count did not wrap back: %d", wrap.Count)
	}

	// A Fenwick tree of coded symbols gives the sums of bucket runs.
	const n = 37
	buckets := make([]fingerprint.CodedSymbol, n)
	f := fingerprint.NewFenwick[fingerprint.CodedSymbol](n)
	for range 300 {
		b := r.IntN(n)
		s := randSymbol(r)
		buckets[b].Add(&s)
		f.Add(b, &s)
	}
	for i := 0; i <= n; i++ {
		for j := i; j <= n; j++ {
			var want, got fingerprint.CodedSymbol
			for k := i; k < j; k++ {
				want.Add(&buckets[k])
			}
			f.Range(i, j, &got)
			if got != want {
				t.Fatalf("range [%d, %d): got %+v, want %+v", i, j, got, want)
			}
		}
	}
}

type vectors struct {
	MapperID uint32 `json:"mapper_id"`
	IDs      []struct {
		ID       string   `json:"id"`
		Checksum string   `json:"checksum"`
		Indices  []uint64 `json:"indices"`
	} `json:"ids"`
	Expected []struct {
		Count uint64 `json:"count"`
		I     uint64 `json:"i"`
		Want  int64  `json:"want"`
	} `json:"expected"`
}

func loadVectors(t *testing.T) vectors {
	t.Helper()
	b, err := os.ReadFile("testdata/rateless_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.IDs) == 0 || len(v.Expected) == 0 {
		t.Fatal("empty vectors")
	}
	return v
}

func mustID(t *testing.T, s string) fingerprint.ID {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != fingerprint.IDSize {
		t.Fatalf("bad id %q: %v", s, err)
	}
	return fingerprint.ID(b)
}

// The vectors were produced by an independent Python implementation (pure
// Python Keccak-256, xoshiro256**, IEEE double arithmetic, ties rounded away
// from zero) and cross-checked against a second, separately written pure
// Python derivation from the FloatMapper spec. They are checked on amd64
// only; other architectures are T2.8.
// Changing them changes the mapping of mapper ID 1, a wire change.
func TestFloatMapperVectors(t *testing.T) {
	t.Parallel()

	v := loadVectors(t)
	m := fingerprint.NewFloatMapper()
	if v.MapperID != m.ID() || m.ID() != fingerprint.DefaultMapperID {
		t.Fatalf("mapper id %d, vectors %d", m.ID(), v.MapperID)
	}
	for _, e := range v.IDs {
		id := mustID(t, e.ID)
		it := m.Indices(id)
		for k, want := range e.Indices {
			if got := it.Next(); got != want {
				t.Fatalf("id %s: index %d is %d, want %d", e.ID[:8], k, got, want)
			}
		}
	}
	for _, e := range v.Expected {
		if got := m.Expected(e.Count, e.I); got != e.Want {
			t.Errorf("Expected(%d, %d) = %d, want %d", e.Count, e.I, got, e.Want)
		}
	}
}

func TestChecksum(t *testing.T) {
	t.Parallel()

	for _, e := range loadVectors(t).IDs {
		id := mustID(t, e.ID)
		got := fingerprint.Checksum(id)
		if hex.EncodeToString(got[:]) != e.Checksum {
			t.Fatalf("checksum of %s: %x, want %s", e.ID[:8], got, e.Checksum)
		}
		// Legacy Keccak, not SHA3.
		h := sha3.NewLegacyKeccak256()
		_, _ = h.Write([]byte("rangesync/check"))
		_, _ = h.Write(id[:])
		if sum := h.Sum(nil); !slices.Equal(sum[:fingerprint.ChecksumSize], got[:]) {
			t.Fatalf("checksum of %s is not the keccak256 prefix", e.ID[:8])
		}
	}
}

func TestChecksumTruncated(t *testing.T) {
	fingerprint.SetChecksumBytes(t, 2)
	id := randIDv2(newRand(2))
	c := fingerprint.Checksum(id)
	if c[0] == 0 && c[1] == 0 {
		t.Fatal("truncated checksum lost its first bytes")
	}
	for _, b := range c[2:] {
		if b != 0 {
			t.Fatalf("truncated checksum %x has bytes past 2", c)
		}
	}
}

func TestFloatMapperIndices(t *testing.T) {
	t.Parallel()

	m := fingerprint.NewFloatMapper()
	r := newRand(3)
	const ids = 100000
	probe := []uint64{1, 2, 3, 7, 31, 511}
	hits := make([]int, len(probe))
	below := map[uint64]int{32: 0, 512: 0, 4096: 0}
	for range ids {
		id := randIDv2(r)
		it := m.Indices(id)
		prev := it.Next()
		if prev != 0 {
			t.Fatalf("first index %d, want 0", prev)
		}
		for b := range below {
			below[b]++
		}
		for {
			i := it.Next()
			if i <= prev {
				t.Fatalf("index %d after %d", i, prev)
			}
			if i >= 4096 {
				break
			}
			for b := range below {
				if i < b {
					below[b]++
				}
			}
			if k := slices.Index(probe, i); k >= 0 {
				hits[k]++
			}
			prev = i
		}
	}
	// ρ(i) = 1/(1 + i/2) only approximates the probability: the gap sampling
	// gives 0.64 at i = 1 (ceil((1.5)(t-1)) = 1 iff w >= 9/25).
	for k, i := range probe {
		got := float64(hits[k]) / ids
		rho := 1 / (1 + float64(i)/2)
		if math.Abs(got-rho)/rho > 0.10 {
			t.Errorf("P(index %d) = %.4f, ρ = %.4f", i, got, rho)
		}
	}
	if p1 := float64(hits[0]) / ids; math.Abs(p1-0.64) > 0.01 {
		t.Errorf("P(index 1) = %.4f, want 0.64", p1)
	}
	// Proposal §4.3.4: ~6.2 / 11.6 / 15.8 indices below 32 / 512 / 4096.
	for b, want := range map[uint64]float64{32: 6.2, 512: 11.6, 4096: 15.8} {
		got := float64(below[b]) / ids
		if math.Abs(got-want)/want > 0.05 {
			t.Errorf("indices below %d: %.2f, want ~%.1f", b, got, want)
		}
		t.Logf("indices below %d: %.2f", b, got)
	}
}

func TestFloatMapperIndicesEnd(t *testing.T) {
	t.Parallel()

	const maxIndex = 1<<53 - 1
	// The smallest gap is 1: from 2^53 - 2 the next index is 2^53 - 1 or the
	// mapping ends.
	for _, s := range [][4]uint64{{1, 2, 3, 4}, {0, 0, 0, 0}, {math.MaxUint64, 5, 7, 9}} {
		it := fingerprint.NewFloatIteratorAt(s, maxIndex-1)
		if i := it.Next(); i != maxIndex && i != fingerprint.EndIndex {
			t.Fatalf("next of 2^53-2 is %d", i)
		}
		it = fingerprint.NewFloatIteratorAt(s, maxIndex)
		for range 3 {
			if i := it.Next(); i != fingerprint.EndIndex {
				t.Fatalf("next of 2^53-1 is %d, want EndIndex", i)
			}
		}
		// From a large index the mapping ends within a few steps.
		it = fingerprint.NewFloatIteratorAt(s, 1<<50)
		ended := false
		prev := uint64(1 << 50)
		for range 10000 {
			i := it.Next()
			if i == fingerprint.EndIndex {
				ended = true
				break
			}
			if i <= prev || i > maxIndex {
				t.Fatalf("index %d after %d", i, prev)
			}
			prev = i
		}
		if !ended {
			t.Fatal("mapping did not end")
		}
		if i := it.Next(); i != fingerprint.EndIndex {
			t.Fatalf("index %d after the end", i)
		}
	}
}

// roundHalfAway returns round(2c/(i+2)) exactly, ties away from zero.
func roundHalfAway(c, i uint64) *big.Int {
	q := new(big.Rat).SetFrac(
		new(big.Int).Lsh(new(big.Int).SetUint64(c), 1),
		new(big.Int).Add(new(big.Int).SetUint64(i), big.NewInt(2)),
	)
	q.Add(q, big.NewRat(1, 2))
	return new(big.Int).Quo(q.Num(), q.Denom()) // floor, q >= 0
}

func TestFloatMapperExpected(t *testing.T) {
	t.Parallel()

	m := fingerprint.NewFloatMapper()
	check := func(c, i uint64) {
		t.Helper()
		want := roundHalfAway(c, i)
		if got := m.Expected(c, i); big.NewInt(got).Cmp(want) != 0 {
			t.Fatalf("Expected(%d, %d) = %d, want %s", c, i, got, want)
		}
	}
	// Ties round away from zero.
	check(1, 2) // 0.5 -> 1
	check(5, 2) // 2.5 -> 3
	check(3, 10)
	check(0, 0)
	check(0, math.MaxUint64)
	r := newRand(4)
	// With 2c < 2^52 the float64 quotient never rounds onto a tie, so the
	// result is exact.
	for range 100000 {
		c := r.Uint64N(1 << 51)
		if r.IntN(2) == 0 {
			c = r.Uint64N(100000)
		}
		check(c, r.Uint64N(1<<40))
		check(c, r.Uint64N(5000))
	}
	// Clamping: a quotient of 2^63 or more is MaxInt64.
	if got := m.Expected(math.MaxUint64, 0); got != math.MaxInt64 {
		t.Fatalf("Expected(MaxUint64, 0) = %d", got)
	}
	if got := m.Expected(1<<63, 0); got != math.MaxInt64 {
		t.Fatalf("Expected(2^63, 0) = %d", got)
	}
	if got := m.Expected(1<<62, 0); got != 1<<62 {
		t.Fatalf("Expected(2^62, 0) = %d", got)
	}
}

func TestEncodeLinearity(t *testing.T) {
	t.Parallel()

	for _, mp := range mappers {
		t.Run(mp.name, func(t *testing.T) {
			t.Parallel()

			r := newRand(5)
			for trial := range 20 {
				common := randIDs(r, r.IntN(200))
				onlyA := randIDs(r, r.IntN(50))
				onlyB := randIDs(r, r.IntN(50))
				a := append(slices.Clone(common), onlyA...)
				b := append(slices.Clone(common), onlyB...)
				const n = 300
				ea := encodeSet(mp.m, a, 0, n)
				eb := encodeSet(mp.m, b, 0, n)
				diff := encodeSet(mp.m, onlyA, 0, n)
				for _, id := range onlyB {
					fingerprint.Encode(mp.m, diff, 0, id, -1)
				}
				for i := range ea {
					ea[i].Sub(&eb[i])
					if ea[i] != diff[i] {
						t.Fatalf("trial %d symbol %d: encode(A)-encode(B) != encode(A\\B)-encode(B\\A)", trial, i)
					}
				}
				// A window [start, start+k) equals the slice of the full stream.
				start, k := uint64(r.IntN(n)), r.IntN(50)
				if int(start)+k > n {
					k = n - int(start)
				}
				full := encodeSet(mp.m, a, 0, n)
				if win := encodeSet(mp.m, a, start, k); !slices.Equal(win, full[start:int(start)+k]) {
					t.Fatalf("trial %d: window [%d, +%d) differs", trial, start, k)
				}
				// Removing every element gives the empty stream.
				for _, id := range a {
					fingerprint.Encode(mp.m, full, 0, id, -1)
				}
				for i := range full {
					if full[i] != (fingerprint.CodedSymbol{}) {
						t.Fatalf("trial %d: symbol %d not empty after removal", trial, i)
					}
				}
			}
		})
	}
}

func TestEncodeSingle(t *testing.T) {
	t.Parallel()

	m := fingerprint.NewFloatMapper()
	id := randIDv2(newRand(6))
	syms := make([]fingerprint.CodedSymbol, 64)
	fingerprint.Encode(m, syms, 0, id, 1)
	one := fingerprint.CodedSymbol{Sum: id, Check: fingerprint.Checksum(id), Count: 1}
	it := m.Indices(id)
	next := it.Next()
	for i := range syms {
		want := fingerprint.CodedSymbol{}
		if uint64(i) == next {
			want = one
			next = it.Next()
		}
		if syms[i] != want {
			t.Fatalf("symbol %d: %+v, want %+v", i, syms[i], want)
		}
	}
	// An empty window and a window at the top of the index space are fine.
	fingerprint.Encode(m, nil, 0, id, 1)
	fingerprint.Encode(m, make([]fingerprint.CodedSymbol, 1), math.MaxUint64-1, id, -1)
}

func TestEncodePanics(t *testing.T) {
	t.Parallel()

	m := fingerprint.NewFloatMapper()
	for _, tc := range []struct {
		name  string
		start uint64
		n     int
		sign  int64
	}{
		{"sign 0", 0, 4, 0},
		{"sign 2", 0, 4, 2},
		{"overflow", math.MaxUint64, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Fatal("no panic")
				}
			}()
			fingerprint.Encode(m, make([]fingerprint.CodedSymbol, tc.n), tc.start, fingerprint.ID{}, tc.sign)
		})
	}
}

// diffStreams returns symbols [start, start+n) of the local stream of
// onlyLocal and the remote stream of onlyRemote, both plus common.
func diffStreams(m fingerprint.Mapper, common, onlyLocal, onlyRemote []fingerprint.ID, start uint64, n int) (local, remote []fingerprint.CodedSymbol) {
	local = encodeSet(m, common, start, n)
	remote = slices.Clone(local)
	for _, id := range onlyLocal {
		fingerprint.Encode(m, local, start, id, 1)
	}
	for _, id := range onlyRemote {
		fingerprint.Encode(m, remote, start, id, 1)
	}
	return local, remote
}

// The worked example of proposal §4.3.7: A holds x, B holds y and z, with
// x -> {0, 1}, y -> {0, 2}, z -> {0, 1, 3}. A decodes B's stream.
func TestDecoderWorkedExample(t *testing.T) {
	t.Parallel()

	r := newRand(7)
	x, y, z := randIDv2(r), randIDv2(r), randIDv2(r)
	m := scriptedMapper{x: {0, 1}, y: {0, 2}, z: {0, 1, 3}}
	local, remote := diffStreams(m, nil, []fingerprint.ID{x}, []fingerprint.ID{y, z}, 0, 4)

	// After subtraction (d = local - remote):
	// s0 = x, y, z (count -1); s1 = x, z (count 0); s2 = y (-1); s3 = z (-1).
	wantCounts := []int64{-1, 0, -1, -1}
	for i := range local {
		d := local[i]
		d.Sub(&remote[i])
		if d.Count != wantCounts[i] {
			t.Fatalf("s%d count %d, want %d", i, d.Count, wantCounts[i])
		}
	}

	dec := fingerprint.NewDecoder(m, fullRange)
	steps := []struct {
		recovered int
		done      bool
	}{
		{0, false}, // s0: three elements, not pure
		{0, false}, // s1: count 0
		{1, false}, // s2 is pure: y only on B
		{3, true},  // s3 is pure: z only on B; then s0 = x (+1): x only on A
	}
	for i, st := range steps {
		if err := dec.Add(uint64(i), remote[i:i+1], local[i:i+1]); err != nil {
			t.Fatalf("symbol %d: %v", i, err)
		}
		if dec.Recovered() != st.recovered || dec.Done() != st.done {
			t.Fatalf("after s%d: recovered %d done %v, want %d %v", i, dec.Recovered(), dec.Done(), st.recovered, st.done)
		}
	}
	onlyLocal, onlyRemote := dec.Result()
	if !slices.Equal(onlyLocal, []fingerprint.ID{x}) || !slices.Equal(onlyRemote, sortIDs([]fingerprint.ID{y, z})) {
		t.Fatalf("result %x / %x", onlyLocal, onlyRemote)
	}
	if dec.Received() != 4 {
		t.Fatalf("received %d", dec.Received())
	}
}

// decodeOneByOne feeds the stream one symbol at a time until the decoder is
// done and returns the number of symbols it took. It checks that Done is
// reported exactly when the difference is complete.
func decodeOneByOne(t *testing.T, m fingerprint.Mapper, common, onlyLocal, onlyRemote []fingerprint.ID) uint64 {
	t.Helper()
	dec := fingerprint.NewDecoder(m, fullRange)
	d := len(onlyLocal) + len(onlyRemote)
	window := 2*d + 16
	var start uint64
	for {
		local, remote := diffStreams(m, common, onlyLocal, onlyRemote, start, window)
		for k := range local {
			if err := dec.Add(dec.Received(), remote[k:k+1], local[k:k+1]); err != nil {
				t.Fatalf("d=%d symbol %d: %v", d, dec.Received(), err)
			}
			complete := dec.Recovered() == d
			if dec.Done() != complete {
				t.Fatalf("d=%d after %d symbols: done %v, recovered %d", d, dec.Received(), dec.Done(), dec.Recovered())
			}
			if complete {
				gotLocal, gotRemote := dec.Result()
				if !equalIDs(gotLocal, onlyLocal) || !equalIDs(gotRemote, onlyRemote) {
					t.Fatalf("d=%d: wrong result", d)
				}
				return dec.Received()
			}
		}
		start += uint64(window)
	}
}

func TestDecoderCost(t *testing.T) {
	t.Parallel()

	r := newRand(8)
	m := fingerprint.NewFloatMapper()
	// The cost of one decode varies a lot at small d (standard deviation 1.0
	// at d = 2, 0.76 at d = 4), and the mean peaks at ~1.76 near d = 4, so
	// small differences need thousands of trials to keep the sample mean
	// inside the band.
	cases := []struct{ d, trials int }{
		{1, 200},
		{2, 8000},
		{3, 6000},
		{4, 5000},
		{5, 4000},
		{8, 2500},
		{10, 2000},
		{20, 1000},
		{50, 200},
		{100, 100},
		{200, 40},
		{500, 16},
		{1000, 8},
		{2000, 4},
		{5000, 2},
		{10000, 1},
	}
	if testing.Short() {
		cases = cases[:len(cases)-3]
	}
	for _, tc := range cases {
		var total uint64
		for range tc.trials {
			nl := r.IntN(tc.d + 1)
			common := randIDs(r, r.IntN(20))
			total += decodeOneByOne(t, m, common, randIDs(r, nl), randIDs(r, tc.d-nl))
		}
		cost := float64(total) / float64(tc.d*tc.trials)
		t.Logf("d=%5d trials=%3d symbols/difference=%.3f", tc.d, tc.trials, cost)
		lo, hi := 1.3, 1.8
		if tc.d == 1 {
			// One difference is the whole of symbol 0: always 1 symbol.
			lo, hi = 1, 1
		}
		if cost < lo || cost > hi {
			t.Errorf("d=%d: %.3f symbols per difference, want [%.1f, %.1f]", tc.d, cost, lo, hi)
		}
	}
}

func TestDecoderPluggable(t *testing.T) {
	t.Parallel()

	for _, mp := range mappers {
		t.Run(mp.name, func(t *testing.T) {
			t.Parallel()

			r := newRand(9)
			for _, d := range []int{0, 1, 2, 5, 30, 300} {
				nl := r.IntN(d + 1)
				onlyLocal, onlyRemote := randIDs(r, nl), randIDs(r, d-nl)
				common := randIDs(r, 50)
				if d == 0 {
					dec := fingerprint.NewDecoder(mp.m, fullRange)
					local, remote := diffStreams(mp.m, common, nil, nil, 0, 1)
					if err := dec.Add(0, remote, local); err != nil || !dec.Done() {
						t.Fatalf("equal sets: err %v done %v", err, dec.Done())
					}
					continue
				}
				decodeOneByOne(t, mp.m, common, onlyLocal, onlyRemote)
			}
		})
	}
}

func TestDecoderIncremental(t *testing.T) {
	t.Parallel()

	r := newRand(10)
	m := fingerprint.NewFloatMapper()
	onlyLocal, onlyRemote := randIDs(r, 40), randIDs(r, 30)
	const n = 400
	local, remote := diffStreams(m, randIDs(r, 100), onlyLocal, onlyRemote, 0, n)

	// One batch.
	one := fingerprint.NewDecoder(m, fullRange)
	if err := one.Add(0, remote, local); err != nil || !one.Done() {
		t.Fatalf("one batch: err %v done %v", err, one.Done())
	}
	wantLocal, wantRemote := one.Result()
	if !equalIDs(wantLocal, onlyLocal) || !equalIDs(wantRemote, onlyRemote) {
		t.Fatal("one batch: wrong result")
	}
	if !slices.IsSortedFunc(wantLocal, fingerprint.ID.Compare) || !slices.IsSortedFunc(wantRemote, fingerprint.ID.Compare) {
		t.Fatal("result not sorted")
	}

	// Growing batches as T2.4 sends them, stopping once done.
	many := fingerprint.NewDecoder(m, fullRange)
	var sent uint64
	for b := uint64(4); !many.Done(); b *= 4 {
		end := min(sent+b, n)
		if err := many.Add(sent, remote[sent:end], local[sent:end]); err != nil {
			t.Fatal(err)
		}
		sent = end
		if many.Received() != sent {
			t.Fatalf("received %d, want %d", many.Received(), sent)
		}
	}
	gotLocal, gotRemote := many.Result()
	if !slices.Equal(gotLocal, wantLocal) || !slices.Equal(gotRemote, wantRemote) || many.Recovered() != 70 {
		t.Fatal("batches: wrong result")
	}

	// Result returns copies.
	gotLocal[0] = fingerprint.ID{}
	if again, _ := many.Result(); again[0] == (fingerprint.ID{}) {
		t.Fatal("Result shares its slice")
	}

	// Empty local symbols: len(local) == 0.
	empty := fingerprint.NewDecoder(m, fullRange)
	only := encodeSet(m, onlyRemote, 0, 200)
	if err := empty.Add(0, only, nil); err != nil || !empty.Done() {
		t.Fatalf("nil local: err %v done %v", err, empty.Done())
	}
	if l, rr := empty.Result(); len(l) != 0 || !equalIDs(rr, onlyRemote) {
		t.Fatal("nil local: wrong result")
	}

	// Equal sets: done after symbol 0, empty result.
	same := fingerprint.NewDecoder(m, fullRange)
	if same.Done() {
		t.Fatal("done before any symbol")
	}
	common := encodeSet(m, randIDs(r, 10), 0, 1)
	if err := same.Add(0, common, common); err != nil || !same.Done() || same.Recovered() != 0 {
		t.Fatalf("equal sets: err %v done %v", err, same.Done())
	}
}

func TestDecoderInvalidBatch(t *testing.T) {
	t.Parallel()

	r := newRand(11)
	m := fingerprint.NewFloatMapper()
	local, remote := diffStreams(m, nil, randIDs(r, 3), randIDs(r, 3), 0, 64)
	dec := fingerprint.NewDecoder(m, fullRange)

	expectInvalid := func(start uint64, rem, loc []fingerprint.CodedSymbol) {
		t.Helper()
		before := dec.Received()
		if err := dec.Add(start, rem, loc); !errors.Is(err, fingerprint.ErrInvalidBatch) {
			t.Fatalf("got %v, want ErrInvalidBatch", err)
		}
		if dec.Received() != before {
			t.Fatal("invalid batch changed the decoder")
		}
	}
	expectInvalid(1, remote[1:2], local[1:2])    // gap
	expectInvalid(0, remote[0:2], local[0:1])    // length mismatch
	expectInvalid(5, nil, nil)                   // empty, but wrong start
	if err := dec.Add(0, nil, nil); err != nil { // empty batch: no-op
		t.Fatal(err)
	}
	if err := dec.Add(0, remote[:2], local[:2]); err != nil {
		t.Fatal(err)
	}
	expectInvalid(0, remote[:2], local[:2]) // repeated
	if err := dec.Add(2, remote[2:], local[2:]); err != nil || !dec.Done() {
		t.Fatalf("err %v done %v", err, dec.Done())
	}
	if err := dec.Add(dec.Received(), nil, nil); err != nil { // empty once done: no-op
		t.Fatal(err)
	}
	more := encodeSet(m, nil, 64, 1)
	expectInvalid(64, more, more) // non-empty once done
	if !dec.Done() {
		t.Fatal("no longer done")
	}
}

func corruptMessage(t *testing.T, err error, phrase string) {
	t.Helper()
	if !errors.Is(err, fingerprint.ErrCorrupt) {
		t.Fatalf("got %v, want ErrCorrupt", err)
	}
	if !strings.Contains(err.Error(), phrase) {
		t.Fatalf("got %q, want the %q rule", err, phrase)
	}
}

func checkSticky(t *testing.T, dec *fingerprint.Decoder) {
	t.Helper()
	if dec.Done() {
		t.Fatal("corrupt decoder is done")
	}
	if err := dec.Add(dec.Received(), nil, nil); !errors.Is(err, fingerprint.ErrCorrupt) {
		t.Fatalf("corruption not sticky: %v", err)
	}
	if err := dec.Add(12345, nil, nil); !errors.Is(err, fingerprint.ErrCorrupt) {
		t.Fatalf("corruption not reported first: %v", err)
	}
}

func pureSymbol(id fingerprint.ID, count int64) fingerprint.CodedSymbol {
	return fingerprint.CodedSymbol{Sum: id, Check: fingerprint.Checksum(id), Count: count}
}

// idNotMappedTo returns an id whose FloatMapper mapping skips index j.
func idNotMappedTo(t *testing.T, r *rand.Rand, j uint64) fingerprint.ID {
	t.Helper()
	m := fingerprint.NewFloatMapper()
	for range 1000 {
		id := randIDv2(r)
		it := m.Indices(id)
		i := it.Next()
		for i < j {
			i = it.Next()
		}
		if i != j {
			return id
		}
	}
	t.Fatal("no id found")
	return fingerprint.ID{}
}

func firstIndexAfterZero(id fingerprint.ID) uint64 {
	it := fingerprint.NewFloatMapper().Indices(id)
	it.Next()
	return it.Next()
}

func TestDecoderCorruption(t *testing.T) {
	t.Parallel()

	m := fingerprint.NewFloatMapper()

	t.Run("outside range", func(t *testing.T) {
		t.Parallel()
		r := newRand(12)
		rg := fingerprint.NewRange(swarm.NewAddress([]byte{0x00}), 8)
		id := randIDv2(r)
		id[0] = 0xff
		dec := fingerprint.NewDecoder(m, rg)
		err := dec.Add(0, []fingerprint.CodedSymbol{pureSymbol(id, 1)}, nil)
		corruptMessage(t, err, "outside range")
		checkSticky(t, dec)
	})

	t.Run("recovered twice", func(t *testing.T) {
		t.Parallel()
		r := newRand(13)
		x := randIDv2(r)
		k := firstIndexAfterZero(x)
		// remote: s0 = {x} and s_k = {0, 0, count 2}. Peeling x from s0
		// turns s_k into {x, count -1}: x again.
		remote := make([]fingerprint.CodedSymbol, k+1)
		remote[0] = pureSymbol(x, 1)
		remote[k].Count = 2
		dec := fingerprint.NewDecoder(m, fullRange)
		corruptMessage(t, dec.Add(0, remote, nil), "recovered twice")
		checkSticky(t, dec)
	})

	t.Run("pure symbol not in mapping", func(t *testing.T) {
		t.Parallel()
		r := newRand(14)
		x := idNotMappedTo(t, r, 1)
		remote := []fingerprint.CodedSymbol{{Count: 5}, pureSymbol(x, 1)}
		dec := fingerprint.NewDecoder(m, fullRange)
		corruptMessage(t, dec.Add(0, remote, nil), "does not map")
		checkSticky(t, dec)
	})

	t.Run("symbol 0 empty", func(t *testing.T) {
		t.Parallel()
		r := newRand(15)
		remote := []fingerprint.CodedSymbol{{}, randSymbol(r)}
		remote[1].Count = 7
		dec := fingerprint.NewDecoder(m, fullRange)
		corruptMessage(t, dec.Add(0, remote, nil), "symbol 0 empty")
		checkSticky(t, dec)
	})
}

// The cap on peel steps (2 recoveries per received symbol) is a backstop
// that honest streams never reach: honest decoding recovers at most one id
// per received symbol, which a cap of 1 checks. A cap of 0 shows the rule
// fires.
func TestDecoderPeelSteps(t *testing.T) {
	r := newRand(16)
	m := fingerprint.NewFloatMapper()

	fingerprint.SetPeelFactor(t, 1)
	for _, d := range []int{1, 2, 3, 10, 100, 1000} {
		nl := r.IntN(d + 1)
		decodeOneByOne(t, m, nil, randIDs(r, nl), randIDs(r, d-nl))
		local, remote := diffStreams(m, nil, randIDs(r, nl), randIDs(r, d-nl), 0, 2*d+32)
		dec := fingerprint.NewDecoder(m, fullRange)
		if err := dec.Add(0, remote, local); err != nil {
			t.Fatalf("d=%d one batch: %v", d, err)
		}
	}

	fingerprint.SetPeelFactor(t, 0)
	dec := fingerprint.NewDecoder(m, fullRange)
	local, remote := diffStreams(m, nil, randIDs(r, 1), nil, 0, 4)
	corruptMessage(t, dec.Add(0, remote, local), "peel steps")
	checkSticky(t, dec)
}

// collision finds x, y, z whose truncated checksums make {x, y, z} look like
// the pure symbol of x^y^z.
func collision(t *testing.T, r *rand.Rand) (x, y, z fingerprint.ID) {
	t.Helper()
	x, y = randIDv2(r), randIDv2(r)
	hx, hy := fingerprint.Checksum(x), fingerprint.Checksum(y)
	for range 1 << 22 {
		z = randIDv2(r)
		var s fingerprint.ID
		for k := range s {
			s[k] = x[k] ^ y[k] ^ z[k]
		}
		hz, hs := fingerprint.Checksum(z), fingerprint.Checksum(s)
		ok := true
		for k := range hs {
			if hx[k]^hy[k]^hz[k] != hs[k] {
				ok = false
				break
			}
		}
		if ok {
			return x, y, z
		}
	}
	t.Fatal("no collision found")
	return
}

// With a 16-bit checksum, colliding ids make a false pure symbol 0. Symbol 0
// alone is then a wrong Done; a batch of MinBatch symbols either reports
// corruption or, peeling the real pure symbols first, the correct difference.
func TestDecoderTruncatedChecksumCollision(t *testing.T) {
	fingerprint.SetChecksumBytes(t, 2)
	r := newRand(17)
	m := fingerprint.NewFloatMapper()
	for range 20 {
		x, y, z := collision(t, r)
		local, remote := diffStreams(m, nil, []fingerprint.ID{x}, []fingerprint.ID{y, z}, 0, 4)

		// Symbol 0 alone is a wrong Done: x^y^z "only remote".
		alone := fingerprint.NewDecoder(m, fullRange)
		if err := alone.Add(0, remote[:1], local[:1]); err != nil || !alone.Done() {
			t.Fatalf("symbol 0 alone: err %v done %v", err, alone.Done())
		}

		dec := fingerprint.NewDecoder(m, fullRange)
		err := dec.Add(0, remote, local)
		if err != nil {
			if !errors.Is(err, fingerprint.ErrCorrupt) {
				t.Fatalf("got %v, want ErrCorrupt", err)
			}
			if dec.Done() {
				t.Fatal("corrupt decoder is done")
			}
			continue
		}
		if !dec.Done() {
			continue // a batch of 4 need not suffice
		}
		gotLocal, gotRemote := dec.Result()
		if !equalIDs(gotLocal, []fingerprint.ID{x}) || !equalIDs(gotRemote, []fingerprint.ID{y, z}) {
			t.Fatal("wrong Done with a batch of 4")
		}
	}

	// Rule (a) catches the collision: x, y and z all map to {0, 1}, so
	// symbol 1 looks like the pure symbol of x^y^z, which maps to 0 only.
	x, y, z := collision(t, r)
	sm := scriptedMapper{x: {0, 1}, y: {0, 1}, z: {0, 1}}
	local, remote := diffStreams(sm, nil, []fingerprint.ID{x}, []fingerprint.ID{y, z}, 0, 2)
	dec := fingerprint.NewDecoder(sm, fullRange)
	corruptMessage(t, dec.Add(0, remote, local), "does not map")
	checkSticky(t, dec)
}

// With a 16-bit checksum and batches of at least MinBatch symbols, decoding
// random differences rarely ends in a wrong Done: the outcomes are a correct
// Done, ErrCorrupt, or (rarely) a wrong Done, which the session catches later
// (undeliverable ids, fingerprints that still differ).
func TestDecoderTruncatedChecksumRate(t *testing.T) {
	fingerprint.SetChecksumBytes(t, 2)
	r := newRand(18)
	m := fingerprint.NewFloatMapper()
	var correct, corrupt, wrong, trials int
	for _, d := range []int{2, 3, 5, 10, 30, 100, 300} {
		for range 200 {
			trials++
			nl := r.IntN(d + 1)
			onlyLocal, onlyRemote := randIDs(r, nl), randIDs(r, d-nl)
			n := 4*d + 64
			local, remote := diffStreams(m, nil, onlyLocal, onlyRemote, 0, n)
			dec := fingerprint.NewDecoder(m, fullRange)
			var sent int
			var err error
			for b := 4; err == nil && !dec.Done() && sent < n; b *= 4 {
				end := min(sent+b, n)
				err = dec.Add(uint64(sent), remote[sent:end], local[sent:end])
				sent = end
			}
			switch {
			case err != nil:
				if !errors.Is(err, fingerprint.ErrCorrupt) {
					t.Fatal(err)
				}
				corrupt++
			case !dec.Done():
				t.Fatalf("d=%d: not done after %d symbols", d, n)
			default:
				gl, gr := dec.Result()
				if equalIDs(gl, onlyLocal) && equalIDs(gr, onlyRemote) {
					correct++
				} else {
					wrong++
				}
			}
		}
	}
	t.Logf("16-bit checksum: %d trials, %d correct, %d corrupt, %d wrong Done", trials, correct, corrupt, wrong)
	if rate := float64(wrong) / float64(trials); rate > 0.005 {
		t.Fatalf("wrong Done rate %.4f over 0.5%%", rate)
	}
}

func TestSymbolDataRoundTrip(t *testing.T) {
	t.Parallel()

	for _, mp := range mappers {
		t.Run(mp.name, func(t *testing.T) {
			t.Parallel()

			r := newRand(19)
			for trial := range 200 {
				n := r.IntN(100)
				syms := make([]fingerprint.CodedSymbol, n)
				for k := range syms {
					syms[k] = randSymbol(r)
					switch r.IntN(4) {
					case 0:
						syms[k].Count = int64(r.IntN(10)) - 5
					case 1:
						syms[k].Count = []int64{math.MinInt64, math.MaxInt64, 0, -1}[r.IntN(4)]
					}
				}
				count := r.Uint64()
				if trial%2 == 0 {
					count = r.Uint64N(10000)
				}
				start := r.Uint64N(1 << 20)
				if trial%7 == 0 {
					start = math.MaxUint64 - uint64(n)
				}
				data := fingerprint.AppendSymbolData([]byte{0xaa}, mp.m, count, start, syms)
				if data[0] != 0xaa {
					t.Fatal("dst prefix lost")
				}
				data = data[1:]
				if len(data) < n*fingerprint.MinSymbolSize || len(data) > n*fingerprint.MaxSymbolSize {
					t.Fatalf("%d symbols in %d bytes", n, len(data))
				}
				got, err := fingerprint.ParseSymbolData(mp.m, count, start, data)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != n || (n > 0 && !slices.Equal(got, syms)) {
					t.Fatalf("trial %d: round trip differs", trial)
				}
			}
		})
	}
}

func TestSymbolDataSize(t *testing.T) {
	t.Parallel()

	// An honest root stream: 10000 elements, 4096 symbols. Counts are close
	// to their expectation, so a symbol takes about 82 bytes (§6.2).
	r := newRand(20)
	m := fingerprint.NewFloatMapper()
	const count = 10000
	syms := encodeSet(m, randIDs(r, count), 0, 4096)
	data := fingerprint.AppendSymbolData(nil, m, count, 0, syms)
	per := float64(len(data)) / float64(len(syms))
	t.Logf("%.2f bytes per symbol", per)
	if per > 82.5 {
		t.Fatalf("%.2f bytes per symbol", per)
	}
}

func TestParseSymbolDataStrict(t *testing.T) {
	t.Parallel()

	m := fingerprint.NewFloatMapper()
	sym := randSymbol(newRand(21))
	sym.Count = 3
	good := fingerprint.AppendSymbolData(nil, m, 100, 0, []fingerprint.CodedSymbol{sym})
	head := good[:fingerprint.IDSize+fingerprint.ChecksumSize]
	withVarint := func(v ...byte) []byte { return append(slices.Clone(head), v...) }

	for _, tc := range []struct {
		name  string
		start uint64
		data  []byte
	}{
		{"partial head", 0, good[:50]},
		{"no varint", 0, head},
		{"truncated varint", 0, withVarint(0x80)},
		{"non-minimal varint", 0, withVarint(0x80, 0x00)},
		{"non-minimal long varint", 0, withVarint(0x81, 0x80, 0x00)},
		{"varint overflow", 0, withVarint(0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x02)},
		{"varint too long", 0, withVarint(0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01)},
		{"trailing bytes", 0, append(slices.Clone(good), 0x00)},
		{"second symbol partial", 0, append(slices.Clone(good), good[:81]...)},
		{"start overflow", math.MaxUint64, good},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := fingerprint.ParseSymbolData(m, 100, tc.start, tc.data); !errors.Is(err, fingerprint.ErrInvalidSymbolData) {
				t.Fatalf("got %v, want ErrInvalidSymbolData", err)
			}
		})
	}

	// Valid edge cases.
	if syms, err := fingerprint.ParseSymbolData(m, 100, 0, nil); err != nil || len(syms) != 0 {
		t.Fatalf("empty data: %v %d", err, len(syms))
	}
	if syms, err := fingerprint.ParseSymbolData(m, 100, math.MaxUint64-1, good); err != nil || len(syms) != 1 {
		t.Fatalf("last index: %v", err)
	}
	if _, err := fingerprint.ParseSymbolData(m, 100, 0, withVarint(0x7f)); err != nil {
		t.Fatalf("one-byte varint: %v", err)
	}
	maxVarint := withVarint(0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01)
	if len(maxVarint) != fingerprint.MaxSymbolSize {
		t.Fatal("bad max size")
	}
	if _, err := fingerprint.ParseSymbolData(m, 100, 0, maxVarint); err != nil {
		t.Fatalf("10-byte varint: %v", err)
	}
	// The varint is the zigzag difference from Expected(count, start+k).
	if want := binenc.AppendVarint(slices.Clone(head), sym.Count-m.Expected(100, 0)); !slices.Equal(good, want) {
		t.Fatalf("encoding %x, want %x", good, want)
	}
}

func BenchmarkFloatMapperIndices(b *testing.B) {
	m := fingerprint.NewFloatMapper()
	ids := randIDs(newRand(30), 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		it := m.Indices(ids[k%len(ids)])
		for it.Next() < fingerprint.RootSymbols {
		}
	}
}

func BenchmarkChecksum(b *testing.B) {
	ids := randIDs(newRand(31), 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		_ = fingerprint.Checksum(ids[k%len(ids)])
	}
}

func BenchmarkEncode(b *testing.B) {
	m := fingerprint.NewFloatMapper()
	ids := randIDs(newRand(32), 1024)
	syms := make([]fingerprint.CodedSymbol, fingerprint.RootSymbols)
	b.ReportAllocs()
	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		fingerprint.Encode(m, syms, 0, ids[k%len(ids)], 1)
	}
}

func BenchmarkDecoder(b *testing.B) {
	for _, d := range []int{10, 100, 1000} {
		r := newRand(33)
		m := fingerprint.NewFloatMapper()
		local, remote := diffStreams(m, nil, randIDs(r, d/2), randIDs(r, d-d/2), 0, 2*d+32)
		b.Run("d="+strconv.Itoa(d), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				dec := fingerprint.NewDecoder(m, fullRange)
				if err := dec.Add(0, remote, local); err != nil || !dec.Done() {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSymbolData(b *testing.B) {
	m := fingerprint.NewFloatMapper()
	syms := encodeSet(m, randIDs(newRand(34), 10000), 0, fingerprint.RootSymbols)
	data := fingerprint.AppendSymbolData(nil, m, 10000, 0, syms)
	b.Run("append", func(b *testing.B) {
		b.ReportAllocs()
		buf := make([]byte, 0, len(data))
		for range b.N {
			buf = fingerprint.AppendSymbolData(buf[:0], m, 10000, 0, syms)
		}
	})
	b.Run("parse", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := fingerprint.ParseSymbolData(m, 10000, 0, data); err != nil {
				b.Fatal(err)
			}
		}
	})
}
