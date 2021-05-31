// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap_test

import (
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/p2p"
	swap "github.com/ethersphere/bee/pkg/settlement/swap/headers"
)

func TestParseSettlementResponseHeaders(t *testing.T) {
	headers := p2p.Headers{
		"exchange":  []byte{10},
		"deduction": []byte{20},
	}

	ex, ded, err := swap.ParseSettlementResponseHeaders(headers)

	if err != nil {
		t.Fatal("unexpected error", err)
	}

	if new(big.Int).SetInt64(10).Cmp(ex) != 0 {
		t.Fatal("expected 0, got", ex)
	}

	if new(big.Int).SetInt64(20).Cmp(ded) != 0 {
		t.Fatal("expected 0, got", ded)
	}
}
