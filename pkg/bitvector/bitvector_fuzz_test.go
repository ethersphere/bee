// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bitvector_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bitvector"
)

// FuzzNewFromBytes fuzzes the bit-vector constructor, which the pullsync client
// builds from a peer-supplied "want" bitvector and a length derived from the
// offer. The target asserts that any length/backing-slice pair the constructor
// accepts is safe to index across its whole range via Get and Set — the exact
// operations processWant performs — so a malformed bitvector cannot drive an
// out-of-bounds access.
func FuzzNewFromBytes(f *testing.F) {
	f.Add([]byte{0x00}, 1)
	f.Add([]byte{0xff, 0xff}, 16)
	f.Add([]byte{}, 0)
	f.Add([]byte{0x01}, 8)
	f.Add([]byte{0xaa, 0x55}, 9)

	f.Fuzz(func(t *testing.T, b []byte, l int) {
		// Bound the length so the target exercises indexing logic rather than
		// the allocator; very large lengths only measure make() behaviour.
		if l < 0 || l > 1<<20 {
			return
		}

		bv, err := bitvector.NewFromBytes(b, l)
		if err != nil {
			return
		}

		if len(bv.Bytes())*8 < l {
			t.Fatalf("backing slice of %d bytes too small for length %d", len(bv.Bytes()), l)
		}

		for i := 0; i < l; i++ {
			_ = bv.Get(i)
			bv.Set(i)
			if !bv.Get(i) {
				t.Fatalf("bit %d reads false immediately after Set", i)
			}
		}
	})
}
