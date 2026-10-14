// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// binary returns the bits of b as a string of '0' and '1', most significant
// bit first. It is the reference the bit-level helpers are checked against.
func binary(b []byte) string {
	var sb strings.Builder
	for _, x := range b {
		fmt.Fprintf(&sb, "%08b", x)
	}
	return sb.String()
}

// addrFromBits returns a 32-byte address whose first bits are given by s
// (a string of '0' and '1') followed by the bits of tail.
func addrFromBits(t *testing.T, s string, tail byte) swarm.Address {
	t.Helper()
	full := s + strings.Repeat(fmt.Sprintf("%08b", tail), swarm.HashSize)
	b := make([]byte, swarm.HashSize)
	for i := range b {
		v, err := strconv.ParseUint(full[8*i:8*i+8], 2, 8)
		if err != nil {
			t.Fatal(err)
		}
		b[i] = byte(v)
	}
	return swarm.NewAddress(b)
}

func randomAddress(r *rand.Rand) swarm.Address {
	b := make([]byte, swarm.HashSize)
	_, _ = r.Read(b)
	return swarm.NewAddress(b)
}

func TestID(t *testing.T) {
	t.Parallel()

	addr := bytes.Repeat([]byte{0xaa}, swarm.HashSize)
	stampHash := bytes.Repeat([]byte{0x55}, swarm.HashSize)

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()

		id := fingerprint.NewID(swarm.NewAddress(addr), stampHash)
		if !bytes.Equal(id[:swarm.HashSize], addr) || !bytes.Equal(id[swarm.HashSize:], stampHash) {
			t.Fatalf("got %x, want %x%x", id, addr, stampHash)
		}
		if !id.Address().Equal(swarm.NewAddress(addr)) {
			t.Fatalf("address: got %s", id.Address())
		}
		if !bytes.Equal(id.StampHash(), stampHash) {
			t.Fatalf("stamp hash: got %x", id.StampHash())
		}
	})

	t.Run("accessors return copies", func(t *testing.T) {
		t.Parallel()

		id := fingerprint.NewID(swarm.NewAddress(addr), stampHash)
		id.Address().Bytes()[0] = 0
		id.StampHash()[0] = 0
		if id[0] != 0xaa || id[swarm.HashSize] != 0x55 {
			t.Fatal("accessor aliases the id")
		}
	})

	t.Run("pad and truncate", func(t *testing.T) {
		t.Parallel()

		id := fingerprint.NewID(swarm.NewAddress([]byte{1, 2}), append(bytes.Repeat([]byte{7}, swarm.HashSize), 9))
		want := fingerprint.ID{}
		want[0], want[1] = 1, 2
		copy(want[swarm.HashSize:], bytes.Repeat([]byte{7}, swarm.HashSize))
		if id != want {
			t.Fatalf("got %x, want %x", id, want)
		}
	})

	t.Run("compare", func(t *testing.T) {
		t.Parallel()

		a := fingerprint.NewID(swarm.NewAddress(addr), stampHash)
		if a.Compare(a) != 0 {
			t.Fatal("compare with itself")
		}
		// a lower address sorts first even with a higher stamp hash
		lowAddr := bytes.Clone(addr)
		lowAddr[31]--
		b := fingerprint.NewID(swarm.NewAddress(lowAddr), bytes.Repeat([]byte{0xff}, swarm.HashSize))
		if b.Compare(a) != -1 || a.Compare(b) != 1 {
			t.Fatal("address must order first")
		}
		// same address, stamp hash decides
		highStamp := bytes.Clone(stampHash)
		highStamp[31]++
		c := fingerprint.NewID(swarm.NewAddress(addr), highStamp)
		if a.Compare(c) != -1 || c.Compare(a) != 1 {
			t.Fatal("stamp hash must order second")
		}
	})
}

func TestParseID(t *testing.T) {
	t.Parallel()

	good := bytes.Repeat([]byte{1}, swarm.HashSize)
	for _, tc := range []struct {
		name            string
		addr, stampHash []byte
		ok              bool
	}{
		{"valid", good, good, true},
		{"empty address", nil, good, false},
		{"empty stamp hash", good, nil, false},
		{"both empty", nil, nil, false},
		{"short address", good[:31], good, false},
		{"long address", append(bytes.Clone(good), 0), good, false},
		{"short stamp hash", good, good[:31], false},
		{"long stamp hash", good, append(bytes.Clone(good), 0), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			id, err := fingerprint.ParseID(tc.addr, tc.stampHash)
			if !tc.ok {
				if !errors.Is(err, fingerprint.ErrInvalidID) {
					t.Fatalf("got error %v, want %v", err, fingerprint.ErrInvalidID)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if id != fingerprint.NewID(swarm.NewAddress(tc.addr), tc.stampHash) {
				t.Fatalf("got %x", id)
			}
		})
	}
}

func TestNewRange(t *testing.T) {
	t.Parallel()

	addr := swarm.NewAddress(bytes.Repeat([]byte{0xff}, swarm.HashSize))
	for _, bits := range []uint8{0, 1, 7, 8, 9, 31, 255} {
		t.Run(strconv.Itoa(int(bits)), func(t *testing.T) {
			t.Parallel()

			r := fingerprint.NewRange(addr, bits)
			if r.Bits != bits {
				t.Fatalf("bits: got %d", r.Bits)
			}
			p := r.Prefix.Bytes()
			if want := (int(bits) + 7) / 8; len(p) != want {
				t.Fatalf("prefix length: got %d, want %d", len(p), want)
			}
			want := strings.Repeat("1", int(bits)) + strings.Repeat("0", 8*len(p)-int(bits))
			if got := binary(p); got != want {
				t.Fatalf("prefix bits: got %s, want %s", got, want)
			}
		})
	}

	t.Run("does not alias the address", func(t *testing.T) {
		t.Parallel()

		b := bytes.Repeat([]byte{0xff}, swarm.HashSize)
		r := fingerprint.NewRange(swarm.NewAddress(b), 16)
		b[0] = 0
		if r.Prefix.Bytes()[0] != 0xff {
			t.Fatal("prefix aliases the address")
		}
	})

	t.Run("short address is zero padded", func(t *testing.T) {
		t.Parallel()

		r := fingerprint.NewRange(swarm.NewAddress([]byte{0xff}), 12)
		if got := binary(r.Prefix.Bytes()); got != "1111111100000000" {
			t.Fatalf("got %s", got)
		}
	})
}

func TestRangeEqualAndKey(t *testing.T) {
	t.Parallel()

	a := fingerprint.Range{Prefix: swarm.NewAddress([]byte{0b1010_0000}), Bits: 3}
	// same bits, different garbage past Bits and a longer prefix
	b := fingerprint.Range{Prefix: swarm.NewAddress([]byte{0b1011_1111, 0xff}), Bits: 3}
	c := fingerprint.Range{Prefix: swarm.NewAddress([]byte{0b1000_0000}), Bits: 3}
	d := fingerprint.Range{Prefix: swarm.NewAddress([]byte{0b1010_0000}), Bits: 4}

	for _, tc := range []struct {
		name  string
		x, y  fingerprint.Range
		equal bool
	}{
		{"itself", a, a, true},
		{"bits past Bits ignored", a, b, true},
		{"different prefix", a, c, false},
		{"different bits", a, d, false},
		{"empty ranges", fingerprint.Range{}, fingerprint.NewRange(swarm.NewAddress([]byte{0xff}), 0), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.x.Equal(tc.y); got != tc.equal {
				t.Fatalf("Equal: got %v, want %v", got, tc.equal)
			}
			if got := tc.y.Equal(tc.x); got != tc.equal {
				t.Fatalf("Equal is not symmetric")
			}
			if got := tc.x.Key() == tc.y.Key(); got != tc.equal {
				t.Fatalf("Key equality: got %v, want %v", got, tc.equal)
			}
		})
	}

	t.Run("key is a usable map key", func(t *testing.T) {
		t.Parallel()

		m := map[fingerprint.RangeKey]int{a.Key(): 1}
		if m[b.Key()] != 1 {
			t.Fatal("equal range not found")
		}
		if _, ok := m[d.Key()]; ok {
			t.Fatal("different range found")
		}
	})
}

func TestRangeContains(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		prefix string // range prefix bits
		addr   string // leading address bits
		tail   byte   // rest of the address
		want   bool
	}{
		{"empty range", "", "", 0xff, true},
		{"one bit match", "1", "1", 0x00, true},
		{"one bit mismatch", "1", "0", 0xff, false},
		{"byte boundary match", "10100101", "10100101", 0x00, true},
		{"byte boundary mismatch last bit", "10100101", "10100100", 0xff, false},
		{"unaligned match", "101001011", "101001011", 0x00, true},
		{"unaligned mismatch last bit", "101001011", "101001010", 0xff, false},
		{"unaligned mismatch first bit", "101001011", "001001011", 0xff, false},
		{"deep match", strings.Repeat("01", 100), strings.Repeat("01", 100), 0xff, true},
		{"deep mismatch", strings.Repeat("01", 100), strings.Repeat("01", 99) + "00", 0xff, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := fingerprint.NewRange(addrFromBits(t, tc.prefix, 0x00), uint8(len(tc.prefix)))
			addr := addrFromBits(t, tc.addr, tc.tail)
			if got := r.Contains(addr); got != tc.want {
				t.Fatalf("Contains: got %v, want %v", got, tc.want)
			}
			id := fingerprint.NewID(addr, bytes.Repeat([]byte{0xee}, swarm.HashSize))
			if got := r.ContainsID(id); got != tc.want {
				t.Fatalf("ContainsID: got %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("bits past Bits in the prefix are ignored", func(t *testing.T) {
		t.Parallel()

		r := fingerprint.Range{Prefix: swarm.NewAddress([]byte{0b1011_1111}), Bits: 3}
		if !r.Contains(addrFromBits(t, "101", 0x00)) {
			t.Fatal("not contained")
		}
	})

	t.Run("short address is not contained", func(t *testing.T) {
		t.Parallel()

		r := fingerprint.NewRange(swarm.NewAddress([]byte{0xab, 0xcd}), 9)
		if r.Contains(swarm.NewAddress([]byte{0xab})) {
			t.Fatal("short address contained")
		}
		if !r.Contains(swarm.NewAddress([]byte{0xab, 0x80})) {
			t.Fatal("address of exactly ceil(bits/8) bytes not contained")
		}
	})
}

func TestRangeContainsRange(t *testing.T) {
	t.Parallel()

	rng := func(s string) fingerprint.Range {
		return fingerprint.NewRange(addrFromBits(t, s, 0x00), uint8(len(s)))
	}
	for _, tc := range []struct {
		name string
		r, o fingerprint.Range
		want bool
	}{
		{"reflexive", rng("1011"), rng("1011"), true},
		{"empty contains all", rng(""), rng("0110101"), true},
		{"empty contains empty", rng(""), rng(""), true},
		{"nested", rng("1011"), rng("10110"), true},
		{"nested deep unaligned", rng("101"), rng("101" + strings.Repeat("1", 30)), true},
		{"shallower in deeper", rng("10110"), rng("1011"), false},
		{"sibling", rng("1011"), rng("1010"), false},
		{"disjoint deeper", rng("1011"), rng("10100"), false},
		{"non-canonical other", rng("101"), fingerprint.Range{Prefix: swarm.NewAddress([]byte{0b1011_1111}), Bits: 4}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.r.ContainsRange(tc.o); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRangeString(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		r    fingerprint.Range
		want string
	}{
		{fingerprint.Range{}, "/0"},
		{fingerprint.NewRange(swarm.NewAddress([]byte{0b0110_1111}), 4), "0110/4"},
		{fingerprint.Range{Prefix: swarm.NewAddress([]byte{0b0110_1111}), Bits: 4}, "0110/4"},
		{fingerprint.NewRange(swarm.NewAddress([]byte{0xff, 0x00}), 10), "1111111100/10"},
	} {
		if got := tc.r.String(); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

func TestRangeChild(t *testing.T) {
	t.Parallel()

	t.Run("bits appended msb first", func(t *testing.T) {
		t.Parallel()

		r := fingerprint.NewRange(addrFromBits(t, "101", 0xff), 3)
		c := r.Child(4, 0b0110)
		if c.String() != "1010110/7" {
			t.Fatalf("got %s", c)
		}
		if !c.Equal(fingerprint.NewRange(c.Prefix, c.Bits)) || len(c.Prefix.Bytes()) != 1 {
			t.Fatal("child is not canonical")
		}
		// only the low bits of i are used
		if !r.Child(4, 0xf0|0b0110).Equal(c) {
			t.Fatal("high bits of i used")
		}
	})

	t.Run("zero bits is the range itself", func(t *testing.T) {
		t.Parallel()

		r := fingerprint.NewRange(addrFromBits(t, "10111", 0xff), 5)
		if !r.Child(0, 7).Equal(r) {
			t.Fatal("Child(0) differs")
		}
		children := r.Children(0)
		if len(children) != 1 || !children[0].Equal(r) {
			t.Fatalf("Children(0): got %v", children)
		}
		if r.ChildIndex(addrFromBits(t, "10111", 0xff), 0) != 0 {
			t.Fatal("ChildIndex(0) not 0")
		}
	})

	t.Run("non-canonical parent", func(t *testing.T) {
		t.Parallel()

		r := fingerprint.Range{Prefix: swarm.NewAddress([]byte{0b1011_1111}), Bits: 3}
		if got := r.Child(2, 0).String(); got != "10100/5" {
			t.Fatalf("got %s", got)
		}
	})

	t.Run("64 bits", func(t *testing.T) {
		t.Parallel()

		r := fingerprint.NewRange(addrFromBits(t, "1", 0x00), 1)
		c := r.Child(64, 1<<63|1)
		want := "1" + "1" + strings.Repeat("0", 62) + "1"
		if got := c.String(); got != want+"/65" {
			t.Fatalf("got %s", got)
		}
		if got := r.ChildIndex(c.Prefix, 64); got != 1<<63|1 {
			t.Fatalf("ChildIndex: got %x", got)
		}
	})
}

func TestRangeChildrenPartition(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))
	parents := []fingerprint.Range{
		fingerprint.NewRange(randomAddress(r), 0),
		fingerprint.NewRange(randomAddress(r), 3),
		fingerprint.NewRange(randomAddress(r), 8),
		fingerprint.NewRange(randomAddress(r), 13),
		fingerprint.NewRange(randomAddress(r), 30),
	}
	for _, parent := range parents {
		for _, bits := range []uint8{1, fingerprint.FanoutBits, 8, fingerprint.LeafBits} {
			t.Run(fmt.Sprintf("%s/%d", parent, bits), func(t *testing.T) {
				t.Parallel()

				children := parent.Children(bits)
				if len(children) != 1<<bits {
					t.Fatalf("got %d children, want %d", len(children), 1<<bits)
				}
				seen := make(map[fingerprint.RangeKey]bool, len(children))
				for i, c := range children {
					if c.Bits != parent.Bits+bits {
						t.Fatalf("child %d: bits %d", i, c.Bits)
					}
					if !c.Equal(parent.Child(bits, uint64(i))) {
						t.Fatalf("child %d differs from Child", i)
					}
					if !c.Equal(fingerprint.NewRange(c.Prefix, c.Bits)) || len(c.Prefix.Bytes()) != (int(c.Bits)+7)/8 {
						t.Fatalf("child %d not canonical: %x", i, c.Prefix.Bytes())
					}
					if !parent.ContainsRange(c) || c.ContainsRange(parent) {
						t.Fatalf("child %d not strictly inside the parent", i)
					}
					if i > 0 && bytes.Compare(children[i-1].Prefix.Bytes(), c.Prefix.Bytes()) >= 0 {
						t.Fatalf("child %d not in address order", i)
					}
					if seen[c.Key()] {
						t.Fatalf("child %d duplicated", i)
					}
					seen[c.Key()] = true
				}
			})
		}
	}

	t.Run("random addresses fall in exactly one child", func(t *testing.T) {
		t.Parallel()

		r := rand.New(rand.NewSource(2))
		for n := 0; n < 200; n++ {
			parentBits := uint8(r.Intn(40))
			bits := uint8(1 + r.Intn(fingerprint.LeafBits))
			addr := randomAddress(r)
			parent := fingerprint.NewRange(addr, parentBits)
			matches := 0
			for i, c := range parent.Children(bits) {
				if c.Contains(addr) {
					matches++
					if got := parent.ChildIndex(addr, bits); got != uint64(i) {
						t.Fatalf("ChildIndex: got %d, want %d", got, i)
					}
				}
			}
			if matches != 1 {
				t.Fatalf("address in %d children", matches)
			}
		}
	})
}

func TestRangeChildIndex(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(3))
	for n := 0; n < 1000; n++ {
		addr := randomAddress(r)
		parentBits := uint8(r.Intn(8 * swarm.HashSize / 2))
		maxBits := 8*swarm.HashSize - int(parentBits)
		if maxBits > 64 {
			maxBits = 64
		}
		bits := uint8(r.Intn(maxBits + 1))
		parent := fingerprint.NewRange(addr, parentBits)

		// reference: read the bits from the binary string
		s := binary(addr.Bytes())[parentBits : int(parentBits)+int(bits)]
		var want uint64
		if bits > 0 {
			v, err := strconv.ParseUint(s, 2, 64)
			if err != nil {
				t.Fatal(err)
			}
			want = v
		}
		got := parent.ChildIndex(addr, bits)
		if got != want {
			t.Fatalf("parent %s bits %d: got %d, want %d", parent, bits, got, want)
		}
		if !parent.Child(bits, got).Contains(addr) {
			t.Fatalf("parent %s bits %d: Child(ChildIndex) does not contain the address", parent, bits)
		}
	}

	t.Run("bucket and level-1 index below the root", func(t *testing.T) {
		t.Parallel()

		// root of radius 5 with prefix 10110; next 12 bits are 0011 0101 1100
		addr := addrFromBits(t, "10110"+"0011"+"0101"+"1100", 0xff)
		root := fingerprint.NewRange(addr, 5)
		if got := root.ChildIndex(addr, fingerprint.LeafBits); got != 0b0011_0101_1100 {
			t.Fatalf("bucket: got %b", got)
		}
		if got := root.ChildIndex(addr, fingerprint.FanoutBits); got != 0b0011 {
			t.Fatalf("level-1: got %b", got)
		}
	})

	t.Run("missing address bytes are zero", func(t *testing.T) {
		t.Parallel()

		root := fingerprint.NewRange(swarm.NewAddress([]byte{0xff}), 4)
		if got := root.ChildIndex(swarm.NewAddress([]byte{0xff}), 8); got != 0xf0 {
			t.Fatalf("got %x", got)
		}
	})
}

// mustPanic fails the test unless f panics.
func mustPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: no panic", name)
		}
	}()
	f()
}

func TestRangeLimits(t *testing.T) {
	t.Parallel()

	if fingerprint.MaxBits != 255 {
		t.Fatalf("MaxBits: got %d, want 255", fingerprint.MaxBits)
	}
	full := swarm.NewAddress(bytes.Repeat([]byte{0xff}, swarm.HashSize))

	t.Run("child at the largest depth", func(t *testing.T) {
		t.Parallel()

		r := fingerprint.NewRange(full, fingerprint.MaxBits-64)
		c := r.Child(64, 1<<64-1)
		if c.Bits != fingerprint.MaxBits {
			t.Fatalf("bits: got %d", c.Bits)
		}
		if !c.Equal(fingerprint.NewRange(full, fingerprint.MaxBits)) || len(c.Prefix.Bytes()) != swarm.HashSize {
			t.Fatalf("got %x", c.Prefix.Bytes())
		}
		if got := binary(c.Prefix.Bytes()); got != strings.Repeat("1", 255)+"0" {
			t.Fatalf("got %s", got)
		}
		if !c.Contains(full) {
			t.Fatal("deepest child does not contain the address")
		}
		if got := r.ChildIndex(full, 64); got != 1<<64-1 {
			t.Fatalf("ChildIndex: got %x", got)
		}
		children := fingerprint.NewRange(full, fingerprint.MaxBits-1).Children(1)
		if len(children) != 2 || children[1].Bits != fingerprint.MaxBits {
			t.Fatalf("Children at the largest depth: %v", children)
		}
	})

	t.Run("documented panics", func(t *testing.T) {
		t.Parallel()

		r := fingerprint.NewRange(full, 8)
		mustPanic(t, "Child over 64 bits", func() { r.Child(65, 0) })
		mustPanic(t, "Child past the largest depth", func() { fingerprint.NewRange(full, 192).Child(64, 0) })
		mustPanic(t, "Child past the largest depth by one bit", func() { fingerprint.NewRange(full, fingerprint.MaxBits).Child(1, 0) })
		mustPanic(t, "Children over the limit", func() { r.Children(17) })
		mustPanic(t, "Children past the largest depth", func() { fingerprint.NewRange(full, fingerprint.MaxBits).Children(1) })
		mustPanic(t, "ChildIndex over 64 bits", func() { r.ChildIndex(full, 65) })
	})
}

func TestRangeChildIndexIgnoresPrefixGarbage(t *testing.T) {
	t.Parallel()

	// ChildIndex reads the address it is given; a non-canonical range prefix
	// passed as the address carries garbage bits into the index, which is why
	// BucketSpan canonicalizes first.
	root := fingerprint.NewRange(swarm.NewAddress(bytes.Repeat([]byte{0xff}, swarm.HashSize)), 4)
	r := fingerprint.Range{Prefix: swarm.NewAddress([]byte{0xff, 0xff}), Bits: 8}
	canonical := fingerprint.NewRange(r.Prefix, r.Bits)
	if got := root.ChildIndex(canonical.Prefix, fingerprint.LeafBits); got != 0xf00 {
		t.Fatalf("canonical: got %x, want f00", got)
	}
	lo, n, ok := root.BucketSpan(r, fingerprint.LeafBits)
	if !ok || lo != 0xf00 || n != 256 {
		t.Fatalf("BucketSpan: got lo %x n %d ok %v, want f00 256 true", lo, n, ok)
	}
}

func TestRangeBucketSpan(t *testing.T) {
	t.Parallel()

	addr := addrFromBits(t, "10110"+"0011"+"0101"+"1100", 0xff)
	root := fingerprint.NewRange(addr, 5)
	other := fingerprint.NewRange(addrFromBits(t, "10111", 0x00), 5)

	for _, tc := range []struct {
		name     string
		root     fingerprint.Range
		r        fingerprint.Range
		leafBits uint8
		lo, n    uint64
		ok       bool
	}{
		{"root itself", root, root, fingerprint.LeafBits, 0, 1 << fingerprint.LeafBits, true},
		{"level-1 range", root, fingerprint.NewRange(addr, 5+4), fingerprint.LeafBits, 0b0011 << 8, 256, true},
		{"level-2 range", root, fingerprint.NewRange(addr, 5+8), fingerprint.LeafBits, 0b0011_0101 << 4, 16, true},
		{"bucket", root, fingerprint.NewRange(addr, 5+12), fingerprint.LeafBits, 0b0011_0101_1100, 1, true},
		{"unaligned depth", root, fingerprint.NewRange(addr, 5+3), fingerprint.LeafBits, 0b001 << 9, 512, true},
		{"non-canonical range", root, fingerprint.Range{Prefix: addr, Bits: 5 + 4}, fingerprint.LeafBits, 0b0011 << 8, 256, true},
		{"non-canonical root", fingerprint.Range{Prefix: addr, Bits: 5}, fingerprint.NewRange(addr, 5+4), fingerprint.LeafBits, 0b0011 << 8, 256, true},
		{"zero leaf bits", root, root, 0, 0, 1, true},
		{"empty root", fingerprint.Range{}, fingerprint.NewRange(addr, 4), 8, 0b1011 << 4, 16, true},
		{"below leaf depth", root, fingerprint.NewRange(addr, 5+13), fingerprint.LeafBits, 0, 0, false},
		{"outside the root", root, fingerprint.NewRange(other.Prefix, 9), fingerprint.LeafBits, 0, 0, false},
		{"shallower than the root", root, fingerprint.NewRange(addr, 4), fingerprint.LeafBits, 0, 0, false},
		{"leaf bits over 63", fingerprint.Range{}, fingerprint.Range{}, 64, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lo, n, ok := tc.root.BucketSpan(tc.r, tc.leafBits)
			if ok != tc.ok || lo != tc.lo || n != tc.n {
				t.Fatalf("got lo %b n %d ok %v, want lo %b n %d ok %v", lo, n, ok, tc.lo, tc.n, tc.ok)
			}
		})
	}

	t.Run("matches ChildIndex of every bucket", func(t *testing.T) {
		t.Parallel()

		r := rand.New(rand.NewSource(4))
		for n := 0; n < 500; n++ {
			a := randomAddress(r)
			rootBits := uint8(r.Intn(40))
			leafBits := uint8(r.Intn(fingerprint.LeafBits + 1))
			depth := uint8(r.Intn(int(leafBits) + 1))
			root := fingerprint.NewRange(a, rootBits)
			sub := fingerprint.NewRange(a, rootBits+depth)
			lo, cnt, ok := root.BucketSpan(sub, leafBits)
			if !ok {
				t.Fatalf("root %s sub %s: not ok", root, sub)
			}
			// the buckets [lo, lo+cnt) are exactly those inside sub
			for i := uint64(0); i < 1<<leafBits; i++ {
				inside := sub.ContainsRange(root.Child(leafBits, i))
				if inside != (i >= lo && i < lo+cnt) {
					t.Fatalf("root %s sub %s leaf %d: bucket %d inside %v, span [%d, %d)", root, sub, leafBits, i, inside, lo, lo+cnt)
				}
			}
		}
	})
}
