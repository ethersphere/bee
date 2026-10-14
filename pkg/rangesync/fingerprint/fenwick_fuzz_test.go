// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	bin "encoding/binary"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
)

// FuzzFenwick interprets the input as a tree size followed by a sequence of
// operations, applies them to a Fenwick tree and to a plain slice of buckets,
// and checks every query against the brute-force sums.
//
// Layout: one byte for the size (1..256 buckets), one byte that selects
// NewFenwick or NewFenwickFromBuckets, then 7-byte operations: an opcode
// byte, two big-endian uint16 bucket indices (taken modulo the bounds of the
// operation) and a uint16 value.
func FuzzFenwick(f *testing.F) {
	f.Add([]byte{0, 0})
	f.Add([]byte{1, 1, 0, 0, 0, 0, 1, 0, 5, 2, 0, 0, 0, 2, 0, 0})
	f.Add([]byte{255, 0, 0, 0, 255, 0, 0, 1, 2, 3, 0, 100, 0, 255, 0, 0})
	f.Add([]byte{7, 1, 1, 0, 6, 0, 0, 0xff, 0xff, 4, 0, 1, 0, 7, 0, 0})
	f.Add([]byte{3, 0, 2, 0, 3, 0, 0, 0, 9, 3, 0, 0, 0, 3, 0, 0, 4, 0, 4, 0, 4, 0, 0, 5, 0, 0, 0, 0, 0, 0})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 2 {
			return
		}
		n := int(data[0]) + 1
		buckets := make([]lanes, n)
		var tree *fingerprint.Fenwick[lanes, *lanes]
		if data[1]&1 == 0 {
			tree = fingerprint.NewFenwick[lanes](n)
		} else {
			// Seed the buckets from the input, then build the tree from a copy.
			for k := range buckets {
				buckets[k] = lanes{uint16(k), uint16(data[1]), uint16(k * int(data[1])), 1}
			}
			tree = fingerprint.NewFenwickFromBuckets[lanes](append([]lanes(nil), buckets...))
		}

		for ops := data[2:]; len(ops) >= 7; ops = ops[7:] {
			a := int(bin.BigEndian.Uint16(ops[1:]))
			b := int(bin.BigEndian.Uint16(ops[3:]))
			v := bin.BigEndian.Uint16(ops[5:])
			delta := lanes{v, v ^ 0x5555, uint16(a), uint16(b)}
			switch ops[0] % 7 {
			case 0:
				i := a % n
				tree.Add(i, &delta)
				buckets[i].Add(&delta)
			case 1:
				i := a % n
				tree.Sub(i, &delta)
				buckets[i].Sub(&delta)
			case 2:
				i := a % n
				tree.Update(i, func(node *lanes) { node.Add(&delta) })
				buckets[i].Add(&delta)
			case 3:
				j := a % (n + 1)
				var got lanes
				tree.Prefix(j, &got)
				if want := bruteRange[lanes](buckets, 0, j); got != want {
					t.Fatalf("Prefix(%d): got %v, want %v", j, got, want)
				}
			case 4:
				i, j := a%(n+1), b%(n+1)
				if i > j {
					i, j = j, i
				}
				var got lanes
				tree.Range(i, j, &got)
				if want := bruteRange[lanes](buckets, i, j); got != want {
					t.Fatalf("Range(%d, %d): got %v, want %v", i, j, got, want)
				}
			case 5:
				tree.Reset()
				clear(buckets)
			case 6:
				// Remove what was added: the bucket returns to its old value.
				i := a % n
				before := buckets[i]
				tree.Add(i, &delta)
				tree.Sub(i, &delta)
				var got lanes
				tree.Range(i, i+1, &got)
				if got != before {
					t.Fatalf("Add then Sub at %d: got %v, want %v", i, got, before)
				}
			}
		}

		for j := 0; j <= n; j++ {
			var got lanes
			tree.Prefix(j, &got)
			if want := bruteRange[lanes](buckets, 0, j); got != want {
				t.Fatalf("final Prefix(%d): got %v, want %v", j, got, want)
			}
		}
	})
}
