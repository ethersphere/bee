// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intervalstore_test

import (
	"math"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/puller/intervalstore"
)

// FuzzIntervalsUnmarshalBinary fuzzes Intervals.UnmarshalBinary, the decoder for
// the persisted LevelDB value that records a peer's pull-sync intervals. The
// record is a semicolon-separated, base36-encoded list: element 0 is the start
// value and elements 1..n are "startRange,endRange" pairs. The decoder must
// tolerate any byte slice — truncated, corrupt, or garbage — by returning an
// error rather than panicking.
//
// Only two invariants are asserted, both sound over arbitrary input:
//   - no-panic on any input; and
//   - the marshaled output of a successful decode is always itself decodable
//     without error (encode∘decode yields decoder-acceptable bytes).
//
// Content idempotency (b == decode-then-marshal(b)) is NOT asserted: add()
// normalizes ranges assuming well-formed, start<=end inputs, but arbitrary
// corrupt bytes can carry inverted ranges (end < start) that leave the internal
// ranges array in a state MarshalBinary faithfully emits yet which is not a
// fixed point of the decode/marshal cycle (e.g. adjacent ranges left unmerged).
// That non-idempotency is benign — no panic, crash, or unbounded loop — so it is
// a property the decoder never guaranteed, not a defect.
func FuzzIntervalsUnmarshalBinary(f *testing.F) {
	// Valid seeds built the production way: construct real Intervals and marshal.
	seed := func(start uint64, ranges ...[2]uint64) []byte {
		iv := intervalstore.NewIntervals(start)
		for _, r := range ranges {
			iv.Add(r[0], r[1])
		}
		b, err := iv.MarshalBinary()
		if err != nil {
			f.Fatal(err)
		}
		return b
	}

	f.Add(seed(0))                                      // start only, no ranges
	f.Add(seed(5))                                      // non-zero start, no ranges
	f.Add(seed(0, [2]uint64{1, 10}))                    // single range
	f.Add(seed(0, [2]uint64{1, 10}, [2]uint64{20, 30})) // multiple disjoint ranges
	f.Add(seed(0, [2]uint64{1, math.MaxUint64}))        // range touching MaxUint64

	// Benign edge buffers (not expected to panic).
	f.Add([]byte{})    // empty -> l==0 / start-parse handling
	f.Add([]byte("0")) // start only, l==1
	f.Add([]byte("!")) // single non-base36 byte

	f.Fuzz(func(t *testing.T, data []byte) {
		var iv intervalstore.Intervals
		if err := iv.UnmarshalBinary(data); err != nil { // must not panic on any input
			return
		}

		b1, err := iv.MarshalBinary()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}

		// The encoder must always produce bytes the decoder accepts.
		var iv2 intervalstore.Intervals
		if err := iv2.UnmarshalBinary(b1); err != nil {
			t.Fatalf("re-decode of marshaled output failed: %v", err)
		}
	})
}
