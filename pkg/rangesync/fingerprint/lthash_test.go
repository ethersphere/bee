// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"golang.org/x/crypto/sha3"
)

// refLanes is the reference model of an LtHash: plain uint16 lanes, added
// and subtracted one lane at a time.
type refLanes [fingerprint.LtHashLanes]uint16

// refExpand is the reference element expansion: the SHAKE128 output of
// "rangesync/v1" ‖ salt ‖ id read as little-endian uint16 lanes.
func refExpand(salt []byte, id fingerprint.ID) refLanes {
	in := append([]byte("rangesync/v1"), salt...)
	in = append(in, id[:]...)
	out := make([]byte, fingerprint.LtHashSize)
	sha3.ShakeSum128(out, in)
	var v refLanes
	for i := range v {
		v[i] = uint16(out[2*i]) | uint16(out[2*i+1])<<8
	}
	return v
}

func (r *refLanes) add(o *refLanes) {
	for i := range r {
		r[i] += o[i]
	}
}

func (r *refLanes) sub(o *refLanes) {
	for i := range r {
		r[i] -= o[i]
	}
}

// digest is the reference digest: keccak256 of the lanes, little-endian.
func (r *refLanes) digest() [32]byte {
	b := make([]byte, fingerprint.LtHashSize)
	for i, v := range r {
		b[2*i], b[2*i+1] = byte(v), byte(v>>8)
	}
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write(b)
	var d [32]byte
	h.Sum(d[:0])
	return d
}

// equalRef reports the first lane where h and r differ, or -1.
func equalRef(h *fingerprint.LtHash, r *refLanes) int {
	for i, v := range r {
		if h.Lane(i) != v {
			return i
		}
	}
	return -1
}

func randID(r *rand.Rand) fingerprint.ID {
	var id fingerprint.ID
	_, _ = r.Read(id[:])
	return id
}

// randHash returns the sum of n random elements, with the reference model of
// the same sum.
func randHash(r *rand.Rand, n int) (fingerprint.LtHash, refLanes) {
	var (
		h   fingerprint.LtHash
		ref refLanes
	)
	for range n {
		id := randID(r)
		h.Add(id)
		v := refExpand(nil, id)
		ref.add(&v)
	}
	return h, ref
}

func TestLtHashSize(t *testing.T) {
	t.Parallel()

	if fingerprint.LtHashLanes != 1024 || fingerprint.LtHashSize != 2048 {
		t.Fatalf("got %d lanes, %d bytes, want 1024, 2048", fingerprint.LtHashLanes, fingerprint.LtHashSize)
	}
}

func TestLtHashZeroValue(t *testing.T) {
	t.Parallel()

	var h fingerprint.LtHash
	for i := range fingerprint.LtHashLanes {
		if h.Lane(i) != 0 {
			t.Fatalf("lane %d of the zero value is %d", i, h.Lane(i))
		}
	}
	var zero refLanes
	if got, want := h.Digest(), zero.digest(); got != want {
		t.Fatalf("digest of the zero value: got %x, want %x", got, want)
	}
	// The digest of the empty set is not the zero [32]byte: callers that
	// need a zero digest for an empty range must special-case Count == 0.
	if h.Digest() == ([32]byte{}) {
		t.Fatal("digest of the zero value is all zero")
	}
}

func TestExpand(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))
	salt := []byte("anchor of round 7")
	for range 20 {
		id := randID(r)
		e := fingerprint.Expand(nil, id)
		ref := refExpand(nil, id)
		if i := equalRef(&e, &ref); i >= 0 {
			t.Fatalf("unsalted expansion differs from the reference at lane %d", i)
		}
		if e2 := fingerprint.Expand(nil, id); e2 != e {
			t.Fatal("expansion is not deterministic")
		}
		if e2 := fingerprint.Expand([]byte{}, id); e2 != e {
			t.Fatal("empty salt differs from nil salt")
		}

		es := fingerprint.Expand(salt, id)
		refs := refExpand(salt, id)
		if i := equalRef(&es, &refs); i >= 0 {
			t.Fatalf("salted expansion differs from the reference at lane %d", i)
		}
		if es == e {
			t.Fatal("salted expansion equals the unsalted one")
		}
		if es2 := fingerprint.Expand(salt[:len(salt)-1], id); es2 == es {
			t.Fatal("different salts give the same expansion")
		}

		var h fingerprint.LtHash
		h.Add(id)
		if h != e {
			t.Fatal("Add(id) differs from Expand(nil, id)")
		}
		h.Sub(id)
		h.Sub(id)
		var neg fingerprint.LtHash
		neg.SubHash(&e)
		if h != neg {
			t.Fatal("Sub(id) differs from subtracting Expand(nil, id)")
		}
	}
}

func TestLtHashLaneArithmetic(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(2))
	a, refA := randHash(r, 3)
	b, refB := randHash(r, 5)

	sum := a
	sum.AddHash(&b)
	refSum := refA
	refSum.add(&refB)
	if i := equalRef(&sum, &refSum); i >= 0 {
		t.Fatalf("AddHash differs from the reference at lane %d", i)
	}

	diff := a
	diff.SubHash(&b)
	refDiff := refA
	refDiff.sub(&refB)
	if i := equalRef(&diff, &refDiff); i >= 0 {
		t.Fatalf("SubHash differs from the reference at lane %d", i)
	}

	if got, want := sum.Digest(), refSum.digest(); got != want {
		t.Fatalf("digest: got %x, want %x", got, want)
	}
}

// TestLtHashCarry checks that carries and borrows wrap inside a lane and never
// reach the neighboring lane, on the lane values where SWAR arithmetic breaks
// if it is done wrong.
func TestLtHashCarry(t *testing.T) {
	t.Parallel()

	// Random lanes set and clear the top bit of a lane about half the time,
	// so a long mixed sequence of additions and subtractions carries and
	// borrows across every lane boundary many times.
	r := rand.New(rand.NewSource(3))
	var ref refLanes
	var h fingerprint.LtHash
	for n := 0; n < 4000; n++ {
		id := randID(r)
		v := refExpand(nil, id)
		if n%3 == 2 {
			h.Sub(id)
			ref.sub(&v)
		} else {
			h.Add(id)
			ref.add(&v)
		}
		if i := equalRef(&h, &ref); i >= 0 {
			t.Fatalf("after %d operations: lane %d is %#04x, want %#04x", n+1, i, h.Lane(i), ref[i])
		}
	}

	// 0 - x, then x + (0 - x), on the lane values the sequence ended with.
	var zero, neg fingerprint.LtHash
	neg.SubHash(&h)
	neg.AddHash(&h)
	if neg != zero {
		t.Fatal("x + (0 - x) is not zero")
	}
}

func TestLtHashGroupLaws(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(4))
	a, _ := randHash(r, 2)
	b, _ := randHash(r, 3)
	c, _ := randHash(r, 4)
	var zero fingerprint.LtHash

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		x := a
		x.AddHash(&zero)
		if x != a {
			t.Fatal("a + 0 != a")
		}
		x.SubHash(&zero)
		if x != a {
			t.Fatal("a - 0 != a")
		}
		y := zero
		y.AddHash(&a)
		if y != a {
			t.Fatal("0 + a != a")
		}
	})

	t.Run("inverse", func(t *testing.T) {
		t.Parallel()
		x := a
		x.SubHash(&a)
		if x != zero {
			t.Fatal("a - a != 0")
		}
		var neg fingerprint.LtHash
		neg.SubHash(&a)
		neg.AddHash(&a)
		if neg != zero {
			t.Fatal("(0 - a) + a != 0")
		}
	})

	t.Run("self", func(t *testing.T) {
		t.Parallel()
		// The receiver and the argument may be the same value.
		x := a
		x.SubHash(&x)
		if x != zero {
			t.Fatal("a - a (aliased) != 0")
		}
		y := a
		y.AddHash(&y)
		z := a
		z.AddHash(&a)
		if y != z {
			t.Fatal("a + a (aliased) differs")
		}
	})

	t.Run("commutativity", func(t *testing.T) {
		t.Parallel()
		x := a
		x.AddHash(&b)
		y := b
		y.AddHash(&a)
		if x != y {
			t.Fatal("a + b != b + a")
		}
	})

	t.Run("associativity", func(t *testing.T) {
		t.Parallel()
		x := a
		x.AddHash(&b)
		x.AddHash(&c)
		bc := b
		bc.AddHash(&c)
		y := a
		y.AddHash(&bc)
		if x != y {
			t.Fatal("(a + b) + c != a + (b + c)")
		}
		// And mixed with subtraction: (a - b) + c == a - (b - c).
		x = a
		x.SubHash(&b)
		x.AddHash(&c)
		bmc := b
		bmc.SubHash(&c)
		y = a
		y.SubHash(&bmc)
		if x != y {
			t.Fatal("(a - b) + c != a - (b - c)")
		}
	})

	t.Run("order", func(t *testing.T) {
		t.Parallel()
		ids := make([]fingerprint.ID, 50)
		for i := range ids {
			ids[i] = randID(rand.New(rand.NewSource(int64(i))))
		}
		var fwd, rev fingerprint.LtHash
		for i := range ids {
			fwd.Add(ids[i])
			rev.Add(ids[len(ids)-1-i])
		}
		if fwd != rev || fwd.Digest() != rev.Digest() {
			t.Fatal("insertion order changes the hash")
		}
		for _, id := range ids {
			fwd.Sub(id)
		}
		if fwd != zero {
			t.Fatal("adding then removing every element is not zero")
		}
	})
}

func TestLtHashDigestChanges(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(5))
	h, _ := randHash(r, 10)
	d := h.Digest()
	if h.Digest() != d {
		t.Fatal("digest is not deterministic")
	}
	h.Add(randID(r))
	if h.Digest() == d {
		t.Fatal("one more element does not change the digest")
	}
}

type ltHashVector struct {
	Name      string   `json:"name"`
	Salt      string   `json:"salt"`
	Add       []string `json:"add"`
	Sub       []string `json:"sub"`
	LanesHead []uint16 `json:"lanes_head"`
	Digest    string   `json:"digest"`
}

func decodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func decodeID(t *testing.T, s string) fingerprint.ID {
	t.Helper()
	var id fingerprint.ID
	b := decodeHex(t, s)
	if len(b) != len(id) {
		t.Fatalf("id %q has %d bytes", s, len(b))
	}
	copy(id[:], b)
	return id
}

// TestLtHashVectors pins the element expansion, the lane order and the digest
// byte order. The vectors in testdata/lthash_vectors.json were produced by an
// independent implementation (Python: hashlib.shake_128, a pure-Python
// Keccak-256 and plain uint16 lanes); a change here is a wire change.
func TestLtHashVectors(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("testdata", "lthash_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors []ltHashVector
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) == 0 {
		t.Fatal("no vectors")
	}
	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			t.Parallel()
			salt := decodeHex(t, v.Salt)
			var h fingerprint.LtHash
			for _, s := range v.Add {
				id := decodeID(t, s)
				if len(salt) == 0 {
					h.Add(id)
				} else {
					e := fingerprint.Expand(salt, id)
					h.AddHash(&e)
				}
			}
			for _, s := range v.Sub {
				id := decodeID(t, s)
				if len(salt) == 0 {
					h.Sub(id)
				} else {
					e := fingerprint.Expand(salt, id)
					h.SubHash(&e)
				}
			}
			for i, want := range v.LanesHead {
				if got := h.Lane(i); got != want {
					t.Fatalf("lane %d: got %#04x, want %#04x", i, got, want)
				}
			}
			d := h.Digest()
			if got := hex.EncodeToString(d[:]); got != v.Digest {
				t.Fatalf("digest: got %s, want %s", got, v.Digest)
			}
		})
	}
}

func TestLtHashLanePanics(t *testing.T) {
	t.Parallel()

	for _, i := range []int{-1, fingerprint.LtHashLanes} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("Lane(%d) did not panic", i)
				}
			}()
			var h fingerprint.LtHash
			_ = h.Lane(i)
		}()
	}
}

func TestLtHashAllocs(t *testing.T) {
	// Not parallel: AllocsPerRun counts allocations of the whole process.
	r := rand.New(rand.NewSource(6))
	id := randID(r)
	a, _ := randHash(r, 2)
	var h fingerprint.LtHash
	var sink [32]byte

	for _, tc := range []struct {
		name string
		f    func()
	}{
		{"Add", func() { h.Add(id) }},
		{"Sub", func() { h.Sub(id) }},
		{"AddHash", func() { h.AddHash(&a) }},
		{"SubHash", func() { h.SubHash(&a) }},
		{"Digest", func() { sink = h.Digest() }},
		{"Expand", func() { h = fingerprint.Expand(nil, id) }},
	} {
		if n := testing.AllocsPerRun(100, tc.f); n != 0 {
			t.Errorf("%s: %v allocations per run, want 0", tc.name, n)
		}
	}
	_ = sink
}

var benchSink [32]byte

func BenchmarkLtHashAdd(b *testing.B) {
	r := rand.New(rand.NewSource(7))
	ids := make([]fingerprint.ID, 1024)
	for i := range ids {
		ids[i] = randID(r)
	}
	var h fingerprint.LtHash
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		h.Add(ids[i%len(ids)])
		i++
	}
}

func BenchmarkLtHashExpand(b *testing.B) {
	r := rand.New(rand.NewSource(8))
	id := randID(r)
	salt := make([]byte, 32)
	var h fingerprint.LtHash
	b.ReportAllocs()
	for b.Loop() {
		h = fingerprint.Expand(salt, id)
	}
	benchSink[0] = byte(h.Lane(0))
}

func BenchmarkLtHashAddHash(b *testing.B) {
	r := rand.New(rand.NewSource(9))
	h, _ := randHash(r, 1)
	o, _ := randHash(r, 1)
	b.SetBytes(fingerprint.LtHashSize)
	b.ReportAllocs()
	for b.Loop() {
		h.AddHash(&o)
	}
}

func BenchmarkLtHashSubHash(b *testing.B) {
	r := rand.New(rand.NewSource(10))
	h, _ := randHash(r, 1)
	o, _ := randHash(r, 1)
	b.SetBytes(fingerprint.LtHashSize)
	b.ReportAllocs()
	for b.Loop() {
		h.SubHash(&o)
	}
}

// BenchmarkLtHashRange24 is the vector work of the worst-case Fenwick range
// query at B = 4096 (§4.3.5): 24 stored nodes, half added and half
// subtracted, into a zeroed accumulator. It excludes the tree walk itself.
func BenchmarkLtHashRange24(b *testing.B) {
	r := rand.New(rand.NewSource(11))
	nodes := make([]fingerprint.LtHash, 24)
	for i := range nodes {
		nodes[i], _ = randHash(r, 1)
	}
	var out fingerprint.LtHash
	b.ReportAllocs()
	for b.Loop() {
		out = fingerprint.LtHash{}
		for i := range nodes {
			if i%2 == 0 {
				out.AddHash(&nodes[i])
			} else {
				out.SubHash(&nodes[i])
			}
		}
	}
}

func BenchmarkLtHashDigest(b *testing.B) {
	r := rand.New(rand.NewSource(12))
	h, _ := randHash(r, 1)
	b.ReportAllocs()
	for b.Loop() {
		benchSink = h.Digest()
	}
}
