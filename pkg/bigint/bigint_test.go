// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bigint_test

import (
	"encoding/json"
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bigint"
)

func TestBinaryMarshalingRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  *big.Int
	}{
		{"positive", big.NewInt(123456789)},
		{"negative", big.NewInt(-987654321)},
		{"zero", big.NewInt(0)},
		{"large", new(big.Int).Mul(big.NewInt(math.MaxInt64), big.NewInt(math.MaxInt64))},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := bigint.Wrap(tc.val)
			data, err := original.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary: %v", err)
			}

			var got bigint.BigInt
			if err := got.UnmarshalBinary(data); err != nil {
				t.Fatalf("UnmarshalBinary: %v", err)
			}

			if got.Cmp(tc.val) != 0 {
				t.Fatalf("got %v, want %v", got.Int, tc.val)
			}
		})
	}
}

// TestBinaryMarshalingGobCompatibility verifies that MarshalBinary produces
// byte-identical output to big.Int.GobEncode, confirming that nodes upgrading
// from the old code (which stored raw *big.Int via GobEncode) will write
// identical bytes after migration.
func TestBinaryMarshalingGobCompatibility(t *testing.T) {
	t.Parallel()

	val := big.NewInt(555000)
	gobData, err := val.GobEncode()
	if err != nil {
		t.Fatal(err)
	}

	newData, err := bigint.Wrap(val).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(gobData, newData) {
		t.Fatalf("MarshalBinary output differs from GobEncode: got %v, want %v", newData, gobData)
	}

	var got bigint.BigInt
	if err := got.UnmarshalBinary(gobData); err != nil {
		t.Fatalf("UnmarshalBinary of gob data: %v", err)
	}
	if got.Cmp(val) != 0 {
		t.Fatalf("got %v, want %v", got.Int, val)
	}
}

func TestMarshaling(t *testing.T) {
	t.Parallel()

	mar, err := json.Marshal(struct {
		Bg *bigint.BigInt
	}{
		Bg: bigint.Wrap(new(big.Int).Mul(big.NewInt(math.MaxInt64), big.NewInt(math.MaxInt64))),
	})
	if err != nil {
		t.Errorf("Marshaling failed: %v", err)
	}
	if !reflect.DeepEqual(mar, []byte("{\"Bg\":\"85070591730234615847396907784232501249\"}")) {
		t.Error("Wrongly marshaled data")
	}
}
