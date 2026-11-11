// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
)

func TestSuggestedFeeAndTipsFromFeeHistoryResult_NoRewardEntries(t *testing.T) {
	t.Parallel()

	fh := &ethereum.FeeHistory{
		BaseFee: []*big.Int{big.NewInt(1000)},
		Reward:  [][]*big.Int{},
	}

	low, market, aggressive, err := suggestedFeesFromFeeHistoryResult(fh)
	if err == nil {
		t.Fatal("expected error when fee history has no reward entries")
	}
	if low != nil || market != nil || aggressive != nil {
		t.Fatal("expected nil tips on error")
	}
}
