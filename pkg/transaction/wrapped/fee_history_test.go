// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
)

func TestSuggestedFeeAndTipsFromFeeHistoryResult(t *testing.T) {
	t.Parallel()

	base := big.NewInt(1000)
	fh := &ethereum.FeeHistory{
		BaseFee: []*big.Int{big.NewInt(1), base},
		Reward: [][]*big.Int{
			{big.NewInt(10), big.NewInt(50), big.NewInt(90)},
			{big.NewInt(20), big.NewInt(60), big.NewInt(100)},
		},
	}

	low, market, agg, err := suggestedFeesFromFeeHistoryResult(fh)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := low.String(), "15"; got != want {
		t.Fatalf("low: got %s want %s", got, want)
	}
	if got, want := market.String(), "55"; got != want {
		t.Fatalf("market: got %s want %s", got, want)
	}
	if got, want := agg.String(), "95"; got != want {
		t.Fatalf("aggressive: got %s want %s", got, want)
	}
}
