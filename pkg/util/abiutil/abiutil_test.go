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

func TestUnpack(t *testing.T) {
	t.Parallel()
	resultBigInt := []interface{}{big.NewInt(10)}
	resultBool := []interface{}{true}
	resultBytes := []interface{}{[32]uint8{}}

	t.Run("bigint ok", func(t *testing.T) {
		t.Parallel()
		_, err := ConvertBigInt(resultBigInt)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
	})

	t.Run("bool ok", func(t *testing.T) {
		t.Parallel()
		_, err := UnpackBool(resultBool)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
	})

	t.Run("bytes ok", func(t *testing.T) {
		t.Parallel()
		_, err := UnpackBytes32(resultBytes)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
	})

	t.Run("bigint fail", func(t *testing.T) {
		t.Parallel()
		_, err := ConvertBigInt(resultBytes)
		if err == nil {
			t.Fatal(err)
		}
	})

	t.Run("bool fail", func(t *testing.T) {
		t.Parallel()
		_, err := UnpackBool(resultBigInt)
		if err == nil {
			t.Fatal(err)
		}
	})

	t.Run("bytes fail", func(t *testing.T) {
		t.Parallel()
		_, err := UnpackBytes32(resultBool)
		if err == nil {
			t.Fatal(err)
		}
	})
}
