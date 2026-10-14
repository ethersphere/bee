// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
)

// ltHashOp is one step of a fuzzed sequence: add or remove one of a few IDs,
// so that the sequence builds a multiset with repeats and negative
// multiplicities.
type ltHashOp struct {
	sub bool
	id  fingerprint.ID
}

// ltHashOps decodes data into a sequence of operations. Each byte is one
// operation: bit 7 selects Sub, the low 3 bits select one of 8 IDs derived
// from seed.
func ltHashOps(seed byte, data []byte) []ltHashOp {
	var ids [8]fingerprint.ID
	for i := range ids {
		ids[i][0] = seed
		ids[i][fingerprint.IDSize-1] = byte(i)
	}
	ops := make([]ltHashOp, len(data))
	for i, b := range data {
		ops[i] = ltHashOp{sub: b&0x80 != 0, id: ids[b&7]}
	}
	return ops
}

func FuzzLtHashAddSub(f *testing.F) {
	f.Add(byte(0), []byte{})
	f.Add(byte(1), []byte{0})
	f.Add(byte(2), []byte{0x80})
	f.Add(byte(3), []byte{0, 0x80})
	f.Add(byte(4), []byte{0, 1, 2, 3, 4, 5, 6, 7, 0x80, 0x81, 0x82})
	f.Add(byte(5), []byte{0x81, 0x81, 0x81, 1, 2, 2, 2, 2, 0x83})

	f.Fuzz(func(t *testing.T, seed byte, data []byte) {
		if len(data) > 256 {
			data = data[:256]
		}
		ops := ltHashOps(seed, data)

		var (
			h   fingerprint.LtHash
			ref refLanes
		)
		for _, op := range ops {
			v := refExpand(nil, op.id)
			if op.sub {
				h.Sub(op.id)
				ref.sub(&v)
			} else {
				h.Add(op.id)
				ref.add(&v)
			}
		}
		if i := equalRef(&h, &ref); i >= 0 {
			t.Fatalf("lane %d: got %#04x, want %#04x", i, h.Lane(i), ref[i])
		}
		if h.Digest() != ref.digest() {
			t.Fatal("digest differs from the reference")
		}

		// The same operations in reverse order, split into two halves that
		// are combined with AddHash, give the same hash.
		var lo, hi fingerprint.LtHash
		for i := len(ops) - 1; i >= 0; i-- {
			part := &lo
			if i >= len(ops)/2 {
				part = &hi
			}
			if ops[i].sub {
				part.Sub(ops[i].id)
			} else {
				part.Add(ops[i].id)
			}
		}
		lo.AddHash(&hi)
		if lo != h {
			t.Fatal("order or grouping changes the hash")
		}

		// Undoing every operation gives the hash of the empty set.
		for _, op := range ops {
			if op.sub {
				h.Add(op.id)
			} else {
				h.Sub(op.id)
			}
		}
		if h != (fingerprint.LtHash{}) {
			t.Fatal("undoing every operation is not zero")
		}
	})
}
