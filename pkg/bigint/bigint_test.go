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

// TestBinaryMarshalingLegacyGob verifies that data written by big.Int.GobEncode
// (the actual on-disk format before this change) is decoded correctly, and that
// the new MarshalBinary produces identical bytes to GobEncode.
func TestBinaryMarshalingLegacyGob(t *testing.T) {
	t.Parallel()

	val := big.NewInt(555000)
	legacyData, err := val.GobEncode()
	if err != nil {
		t.Fatal(err)
	}

	// verify MarshalBinary produces identical bytes to GobEncode
	newData, err := bigint.Wrap(val).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacyData, newData) {
		t.Fatalf("MarshalBinary output differs from GobEncode: got %v, want %v", newData, legacyData)
	}

	var got bigint.BigInt
	if err := got.UnmarshalBinary(legacyData); err != nil {
		t.Fatalf("UnmarshalBinary of legacy gob data: %v", err)
	}

	if got.Cmp(val) != 0 {
		t.Fatalf("got %v, want %v", got.Int, val)
	}
}

// TestBinaryMarshalingLegacyUnquotedJSON verifies that data stored via
// json.Marshal(*big.Int) — an unquoted decimal number like 123 or -456 —
// is decoded correctly. This was the actual format produced by the old
// store.Put(key, bigIntPtr) calls before this change.
func TestBinaryMarshalingLegacyUnquotedJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		data []byte
		want *big.Int
	}{
		{[]byte(`123456`), big.NewInt(123456)},
		{[]byte(`-987654`), big.NewInt(-987654)},
		{[]byte(`0`), big.NewInt(0)},
	}

	for _, tc := range tests {
		var got bigint.BigInt
		if err := got.UnmarshalBinary(tc.data); err != nil {
			t.Fatalf("UnmarshalBinary(%q): %v", tc.data, err)
		}
		if got.Cmp(tc.want) != 0 {
			t.Fatalf("got %v, want %v", got.Int, tc.want)
		}
	}
}

// TestBinaryMarshalingLegacyQuotedJSON verifies that data stored as a
// JSON-quoted decimal string (e.g. "123") is decoded correctly.
func TestBinaryMarshalingLegacyQuotedJSON(t *testing.T) {
	t.Parallel()

	legacyData := []byte(`"999888777"`)

	var got bigint.BigInt
	if err := got.UnmarshalBinary(legacyData); err != nil {
		t.Fatalf("UnmarshalBinary of quoted JSON data: %v", err)
	}

	want := big.NewInt(999888777)
	if got.Cmp(want) != 0 {
		t.Fatalf("got %v, want %v", got.Int, want)
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
