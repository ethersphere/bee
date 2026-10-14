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

func FuzzRange(f *testing.F) {
	full := bytes.Repeat([]byte{0xff}, swarm.HashSize)
	f.Add([]byte{}, uint8(0), full, uint8(4))
	f.Add([]byte{0xa5}, uint8(8), full, uint8(12))
	f.Add([]byte{0xa5, 0x80}, uint8(9), bytes.Repeat([]byte{0xa5}, swarm.HashSize), uint8(64))
	f.Add(full, uint8(255), full, uint8(1))
	f.Add([]byte{0x01}, uint8(31), []byte{0x01}, uint8(0))

	f.Fuzz(func(t *testing.T, prefix []byte, bits uint8, addrBytes []byte, childBits uint8) {
		r := fingerprint.NewRange(swarm.NewAddress(prefix), bits)
		if r.Bits != bits || len(r.Prefix.Bytes()) != (int(bits)+7)/8 {
			t.Fatalf("not canonical: %x/%d", r.Prefix.Bytes(), r.Bits)
		}
		if !r.Equal(fingerprint.NewRange(r.Prefix, r.Bits)) || r.Key() != fingerprint.NewRange(r.Prefix, r.Bits).Key() {
			t.Fatal("NewRange is not idempotent")
		}
		if len(prefix) >= (int(bits)+7)/8 {
			raw := fingerprint.Range{Prefix: swarm.NewAddress(prefix), Bits: bits}
			if !r.Equal(raw) || r.Key() != raw.Key() || r.String() != raw.String() {
				t.Fatal("canonical form differs from the raw range")
			}
		}
		if s := r.String(); len(s) != int(bits)+1+len(strconv.Itoa(int(bits))) {
			t.Fatalf("String: %q", s)
		}
		if !r.ContainsRange(r) {
			t.Fatal("ContainsRange is not reflexive")
		}

		addr := swarm.NewAddress(addrBytes)
		contained := r.Contains(addr)
		if contained != fingerprint.NewRange(addr, bits).Equal(r) && len(addrBytes) >= (int(bits)+7)/8 {
			t.Fatal("Contains disagrees with the range of the address")
		}

		maxBits := 255 - int(bits)
		if maxBits > 64 {
			maxBits = 64
		}
		cb := uint8(int(childBits) % (maxBits + 1))
		i := r.ChildIndex(addr, cb)
		if cb < 64 && i >= 1<<cb {
			t.Fatalf("ChildIndex %d out of range for %d bits", i, cb)
		}
		child := r.Child(cb, i)
		if !r.ContainsRange(child) {
			t.Fatal("child not inside the range")
		}
		if contained && len(addrBytes) >= (int(child.Bits)+7)/8 && !child.Contains(addr) {
			t.Fatal("Child(ChildIndex(addr)) does not contain addr")
		}
	})
}
