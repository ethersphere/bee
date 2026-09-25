// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bigint_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bigint"
)

func FuzzBigIntUnmarshalJSON(f *testing.F) {
	// Valid seeds built the production way: construct a real BigInt and
	// use its MarshalJSON output as the seed.
	for _, v := range []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(-1),
		big.NewInt(math.MaxInt64),
		new(big.Int).Mul(big.NewInt(math.MaxInt64), big.NewInt(math.MaxInt64)),
	} {
		b, err := bigint.Wrap(v).MarshalJSON()
		if err != nil {
			f.Fatalf("marshal seed: %v", err)
		}
		f.Add(b)
	}

	// Benign edge seeds.
	f.Add([]byte(`""`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"0"`))
	f.Add([]byte(`"not-a-number"`))
	f.Add([]byte(`x`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var bi bigint.BigInt
		// Invariant (1): never panics on arbitrary bytes.
		err := bi.UnmarshalJSON(data)

		// Invariant (2): round-trip on success. SetString fails silently, so
		// only round-trip when a decode succeeded and produced an Int.
		if err == nil && bi.Int != nil {
			out, mErr := bi.MarshalJSON()
			if mErr != nil {
				t.Fatalf("MarshalJSON after successful UnmarshalJSON: %v", mErr)
			}
			var bi2 bigint.BigInt
			if err := bi2.UnmarshalJSON(out); err != nil {
				t.Fatalf("re-unmarshal of marshaled output failed: %v", err)
			}
			if bi2.Int == nil {
				t.Fatalf("re-unmarshal produced nil Int for output %q", out)
			}
			if bi.Cmp(bi2.Int) != 0 {
				t.Fatalf("round-trip mismatch: got %s want %s", bi2.Int, bi.Int)
			}
		}
	})
}
