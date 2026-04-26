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

func TestMarshalBinaryNilPanics(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on MarshalBinary with nil Int, got none")
		}
	}()

	var w bigint.BigInt // Int is nil
	_, _ = w.MarshalBinary()
}

func TestUnmarshalBinaryEmptyErrors(t *testing.T) {
	t.Parallel()

	var w bigint.BigInt
	if err := w.UnmarshalBinary([]byte{}); err == nil {
		t.Fatal("expected error on UnmarshalBinary with empty data, got nil")
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

func TestUnmarshalJSONAcceptsStringAndNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "string", input: `"123456789"`, want: "123456789"},
		{name: "number", input: `123456789`, want: "123456789"},
		{name: "negative number", input: `-42`, want: "-42"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got bigint.BigInt
			if err := got.UnmarshalJSON([]byte(tc.input)); err != nil {
				t.Fatalf("UnmarshalJSON: %v", err)
			}

			if got.String() != tc.want {
				t.Fatalf("got %s, want %s", got.String(), tc.want)
			}
		})
	}
}
