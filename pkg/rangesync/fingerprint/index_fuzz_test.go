// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	"math/rand/v2"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzIndex interprets the input as a sequence of operations on a reserve
// model and an Index kept current through Commit, with rebuilds that commit
// while they iterate. After every rebuild and at the end it checks every
// range at every level against the brute-force sums, and at the end that a
// fresh Rebuild of the reserve equals the live index.
//
// Layout: a byte of leaf bits (1..5), a byte of root depth (0..8), eight
// bytes of random seed, then two-byte operations: an opcode byte and an
// argument byte.
func FuzzIndex(f *testing.F) {
	f.Add([]byte{0, 0, 1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte{3, 2, 9, 9, 9, 9, 9, 9, 9, 9, 0, 0, 0, 1, 0, 2, 1, 0, 2, 3, 3, 0})
	f.Add([]byte{4, 8, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 2, 1, 1, 0, 2, 7, 0, 5, 1, 2, 2, 0})
	f.Add([]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 9, 0, 9, 2, 255, 1, 0, 1, 0, 2, 0})
	f.Add([]byte{2, 5, 4, 3, 2, 1, 0, 1, 2, 3, 0, 1, 0, 2, 0, 3, 0, 4, 3, 1, 2, 4, 3, 2, 1, 1, 2, 9})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 10 {
			return
		}
		leafBits := 1 + data[0]%5
		var seed uint64
		for _, b := range data[2:10] {
			seed = seed<<8 | uint64(b)
		}
		r := rand.New(rand.NewPCG(seed, 1))
		var overlay [swarm.HashSize]byte
		fill(r, overlay[:])
		rootAt := func(b byte) fingerprint.Range {
			return fingerprint.NewRange(swarm.NewAddress(overlay[:]), b%9)
		}
		root := rootAt(data[1])
		// Elements near the overlay, some of them outside the root.
		newID := func() fingerprint.ID {
			return randomIDIn(r, fingerprint.NewRange(swarm.NewAddress(overlay[:]), uint8(r.IntN(10))))
		}
		pick := func(s *store, b byte) []fingerprint.ID {
			ids := s.snapshot().ids()
			if len(ids) == 0 {
				return nil
			}
			return ids[int(b)%len(ids) : int(b)%len(ids)+1]
		}

		ix := newIndex(leafBits)
		s := newStore()
		built := false
		ops := data[10:]
		if len(ops) > 400 {
			ops = ops[:400]
		}
		for len(ops) >= 2 {
			op, arg := ops[0], ops[1]
			ops = ops[2:]
			switch op % 4 {
			case 0: // add
				s.commit(ix, []fingerprint.ID{newID()}, nil)
			case 1: // remove
				s.commit(ix, nil, pick(s, arg))
			case 2: // rebuild, committing every arg%8+1 elements
				every := int(arg%8) + 1
				next := rootAt(arg >> 3)
				err := ix.Rebuild(next, func(fn func(fingerprint.ID) error) error {
					for i, id := range s.snapshot().ids() {
						if err := fn(id); err != nil {
							return err
						}
						if i%every == 0 {
							s.commit(ix, []fingerprint.ID{newID()}, pick(s, byte(i)))
						}
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				root, built = next, true
				checkIndex(t, ix, s.snapshot(), root)
			case 3: // replace: remove one and add one in one commit
				s.commit(ix, []fingerprint.ID{newID()}, pick(s, arg))
			}
		}
		if !built {
			if err := ix.Rebuild(root, s.iterate); err != nil {
				t.Fatal(err)
			}
		}
		m := s.snapshot()
		checkIndex(t, ix, m, root)

		fresh := newIndex(leafBits)
		if err := fresh.Rebuild(root, sliceIterate(m.ids())); err != nil {
			t.Fatal(err)
		}
		for _, rg := range allRanges(root, leafBits) {
			a, errA := ix.Summary(rg)
			b, errB := fresh.Summary(rg)
			if errA != nil || errB != nil || a != b {
				t.Fatalf("range %s: live %+v %v, rebuilt %+v %v", rg, a, errA, b, errB)
			}
		}
	})
}
