// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package abiutil

import (
	"math/big"
	"strings"
	"testing"
)

func TestMustParseABI(t *testing.T) {
	t.Parallel()

	defer func() {
		switch err := recover(); {
		case err == nil:
			t.Error("expected panic")
		case !strings.Contains(err.(error).Error(), "unable to parse ABI:"):
			t.Errorf("unexpected panic: %v", err)
		}
	}()

	MustParseABI("invalid abi")
}

func Test_Convert_BigInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input     []interface{}
		wantError bool
	}{
		// valid
		{input: toResult(big.NewInt(111))},
		{input: toResult(big.NewInt(0))},

		// nil value
		{input: nil, wantError: true},
		{input: toResult(nil), wantError: true},

		// wrong types
		{input: toResult([]byte{}), wantError: true},
		{input: toResult([30]byte{}), wantError: true},
		{input: toResult("digital-freedom"), wantError: true},
		{input: toResult(111), wantError: true},
		{input: toResult(true), wantError: true},
	}

	for i, tc := range tests {
		_, err := ConvertBigInt(tc.input)

		if tc.wantError && err == nil {
			t.Fatalf("error expected %d", i)
		} else if !tc.wantError && err != nil {
			t.Fatalf("unexpected error %d", i)
		}
	}
}

func Test_Convert_Bool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input     []interface{}
		wantError bool
	}{
		// valid
		{input: toResult(true)},
		{input: toResult(false)},

		// nil value
		{input: nil, wantError: true},
		{input: toResult(nil), wantError: true},

		// wrong types
		{input: toResult([]byte{}), wantError: true},
		{input: toResult([30]byte{}), wantError: true},
		{input: toResult("digital-freedom"), wantError: true},
		{input: toResult(1), wantError: true},
		{input: toResult(big.NewInt(1)), wantError: true},
	}

	for _, tc := range tests {
		_, err := ConvertBool(tc.input)

		if tc.wantError && err == nil {
			t.Fatal("error expected")
		} else if !tc.wantError && err != nil {
			t.Fatal("unexpected error")
		}
	}
}

func Test_Convert_Bytes32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input     []interface{}
		wantError bool
	}{
		// valid
		{input: toResult([32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2})},

		// nil value
		{input: nil, wantError: true},
		{input: toResult(nil), wantError: true},

		// wrong types
		{input: toResult([]byte{}), wantError: true},
		{input: toResult([30]byte{}), wantError: true},
		{input: toResult("digital-freedom"), wantError: true},
		{input: toResult(1), wantError: true},
		{input: toResult(big.NewInt(1)), wantError: true},
	}

	for _, tc := range tests {
		_, err := ConvertBytes32(tc.input)

		if tc.wantError && err == nil {
			t.Fatal("error expected")
		} else if !tc.wantError && err != nil {
			t.Fatal("unexpected error")
		}
	}
}

func toResult(v interface{}) []interface{} {
	return []interface{}{v}
}
