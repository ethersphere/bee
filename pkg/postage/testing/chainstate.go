// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
)

// NewChainState will create a new ChainState with random values.
func NewChainState() *postage.ChainState {
	return &postage.ChainState{
		Block:        rand.Uint64(), // skipcq: GSC-G404
		CurrentPrice: NewBigInt(),
		TotalAmount:  NewBigInt(),
	}
}

// CompareChainState is a test helper that compares two ChainStates and fails
// the test if they are not exactly equal.
// Fails on first difference and returns a descriptive comparison.
func CompareChainState(t *testing.T, want, got *postage.ChainState) {
	t.Helper()

	if want.Block != got.Block {
		t.Fatalf("block: want %v, got %v", want.Block, got.Block)
	}
	if want.CurrentPrice.Cmp(got.CurrentPrice) != 0 {
		t.Fatalf("price: want %v, got %v", want.CurrentPrice, got.CurrentPrice)
	}
	if want.TotalAmount.Cmp(got.TotalAmount) != 0 {
		t.Fatalf("total: want %v, got %v", want.TotalAmount, got.TotalAmount)
	}
}
