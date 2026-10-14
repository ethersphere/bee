// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	"bytes"
	"errors"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func FuzzParseID(f *testing.F) {
	good := bytes.Repeat([]byte{0xab}, swarm.HashSize)
	f.Add(good, good)
	f.Add([]byte{}, []byte{})
	f.Add(good[:31], good)
	f.Add(good, good[:31])
	f.Add(append(bytes.Clone(good), 0), good)
	f.Add(good, append(bytes.Clone(good), 0))

	f.Fuzz(func(t *testing.T, addr, stampHash []byte) {
		id, err := fingerprint.ParseID(addr, stampHash)
		valid := len(addr) == swarm.HashSize && len(stampHash) == swarm.HashSize
		if !valid {
			if !errors.Is(err, fingerprint.ErrInvalidID) {
				t.Fatalf("lengths %d/%d: got error %v", len(addr), len(stampHash), err)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(id.Address().Bytes(), addr) || !bytes.Equal(id.StampHash(), stampHash) {
			t.Fatal("id does not round trip")
		}
		if id != fingerprint.NewID(swarm.NewAddress(addr), stampHash) {
			t.Fatal("ParseID and NewID differ")
		}
	})
}

// garble returns r with every bit past r.Bits in a full-length prefix set to
// one: the same range in a non-canonical form.
func garble(r fingerprint.Range) fingerprint.Range {
	p := bytes.Repeat([]byte{0xff}, swarm.HashSize)
	for k := 0; k < int(r.Bits); k++ {
		if r.Prefix.Bytes()[k/8]>>(7-k%8)&1 == 0 {
			p[k/8] &^= 0x80 >> (k % 8)
		}
	}
	return fingerprint.Range{Prefix: swarm.NewAddress(p), Bits: r.Bits}
}

func FuzzRange(f *testing.F) {
	full := bytes.Repeat([]byte{0xff}, swarm.HashSize)
	f.Add([]byte{}, uint8(0), full, uint8(4))
	f.Add([]byte{0xa5}, uint8(8), full, uint8(12))
	f.Add([]byte{0xa5, 0x80}, uint8(9), bytes.Repeat([]byte{0xa5}, swarm.HashSize), uint8(64))
	f.Add(full, uint8(255), full, uint8(1))
	f.Add(full, uint8(191), full, uint8(64)) // child at depth MaxBits
	f.Add([]byte{0x01}, uint8(31), []byte{0x01}, uint8(0))
	f.Add([]byte{0xff, 0xff}, uint8(4), []byte{0xff, 0xff}, uint8(12)) // non-canonical prefix

	f.Fuzz(func(t *testing.T, prefix []byte, bits uint8, addrBytes []byte, childBits uint8) {
		r := fingerprint.NewRange(swarm.NewAddress(prefix), bits)
		if r.Bits != bits || len(r.Prefix.Bytes()) != (int(bits)+7)/8 {
			t.Fatalf("not canonical: %x/%d", r.Prefix.Bytes(), r.Bits)
		}
		if !r.Equal(fingerprint.NewRange(r.Prefix, r.Bits)) || r.Key() != fingerprint.NewRange(r.Prefix, r.Bits).Key() {
			t.Fatal("NewRange is not idempotent")
		}
		if s := r.String(); len(s) != int(bits)+1+len(strconv.Itoa(int(bits))) {
			t.Fatalf("String: %q", s)
		}
		if !r.ContainsRange(r) {
			t.Fatal("ContainsRange is not reflexive")
		}

		// the raw (possibly non-canonical, possibly short) prefix selects the
		// same range: missing bytes are zero, bits past Bits are ignored
		raw := fingerprint.Range{Prefix: swarm.NewAddress(prefix), Bits: bits}
		noisy := garble(r)
		for _, nc := range []fingerprint.Range{raw, noisy} {
			if !r.Equal(nc) || !nc.Equal(r) || r.Key() != nc.Key() || r.String() != nc.String() {
				t.Fatalf("non-canonical form %x/%d differs", nc.Prefix.Bytes(), nc.Bits)
			}
			if !nc.ContainsRange(r) || !r.ContainsRange(nc) {
				t.Fatal("ContainsRange depends on bits past Bits")
			}
		}

		addr := swarm.NewAddress(addrBytes)
		contained := r.Contains(addr)
		if len(addrBytes) < (int(bits)+7)/8 {
			if contained || raw.Contains(addr) || noisy.Contains(addr) {
				t.Fatal("address shorter than the prefix is contained")
			}
		} else if contained != fingerprint.NewRange(addr, bits).Equal(r) {
			t.Fatal("Contains disagrees with the range of the address")
		}
		if contained != raw.Contains(addr) || contained != noisy.Contains(addr) {
			t.Fatal("Contains depends on bits past Bits")
		}

		maxBits := fingerprint.MaxBits - int(bits)
		if maxBits > 64 {
			maxBits = 64
		}
		cb := uint8(int(childBits) % (maxBits + 1))
		i := r.ChildIndex(addr, cb)
		if cb < 64 && i >= 1<<cb {
			t.Fatalf("ChildIndex %d out of range for %d bits", i, cb)
		}
		if i != noisy.ChildIndex(addr, cb) {
			t.Fatal("ChildIndex depends on the range prefix")
		}
		child := r.Child(cb, i)
		if !child.Equal(raw.Child(cb, i)) || !child.Equal(noisy.Child(cb, i)) {
			t.Fatal("Child depends on bits past Bits")
		}
		if !r.ContainsRange(child) || !noisy.ContainsRange(garble(child)) {
			t.Fatal("child not inside the range")
		}
		if contained && len(addrBytes) >= (int(child.Bits)+7)/8 && !child.Contains(addr) {
			t.Fatal("Child(ChildIndex(addr)) does not contain addr")
		}

		// BucketSpan of the child below r, with up to 8 more leaf bits
		if cb <= 55 {
			extra := childBits % 9
			leafBits := cb + extra
			for _, root := range []fingerprint.Range{r, raw, noisy} {
				for _, sub := range []fingerprint.Range{child, garble(child)} {
					lo, n, ok := root.BucketSpan(sub, leafBits)
					if !ok || lo != i<<extra || n != 1<<extra {
						t.Fatalf("BucketSpan: got lo %d n %d ok %v, want lo %d n %d", lo, n, ok, i<<extra, 1<<extra)
					}
				}
			}
			if cb > 0 {
				if _, _, ok := child.BucketSpan(r, leafBits); ok {
					t.Fatal("BucketSpan accepted a range shallower than the root")
				}
			}
		}
	})
}
