// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	"context"
	"errors"
	"fmt"
	"math/big"
)

const (
	percentageDivisor = 100
	baseFeeMultiplier = 2
)

var (
	ErrEIP1559NotSupported = errors.New("network does not appear to support EIP-1559 (no baseFee)")
)

// SuggestedFeeAndTip calculates the recommended gasFeeCap and gasTipCap for a transaction.
func (b *wrappedBackend) SuggestedFeeAndTip(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error) {
	if gasPrice != nil {
		// 1. gasFeeCap: The absolute maximum price per gas does not exceed the user's specified price.
		// 2. gasTipCap: The entire amount (gasPrice - baseFee) can be used as a priority fee.
		return new(big.Int).Set(gasPrice), new(big.Int).Set(gasPrice), nil
	}

	gasTipCap, err := b.backend.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to suggest gas tip cap: %w", err)
	}

	if boostPercent != 0 {
		// multiplier: 100 + boostPercent (e.g., 110 for 10% boost)
		multiplier := new(big.Int).Add(big.NewInt(int64(percentageDivisor)), big.NewInt(int64(boostPercent)))
		// gasTipCap = gasTipCap * (100 + boostPercent) / 100
		gasTipCap.Mul(gasTipCap, multiplier).Div(gasTipCap, big.NewInt(int64(percentageDivisor)))
	}

	minimumTip := big.NewInt(b.minimumGasTipCap)
	if gasTipCap.Cmp(minimumTip) < 0 {
		gasTipCap.Set(minimumTip)
	}

	latestBlockHeader, err := b.backend.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get latest block header: %w", err)
	}
	if latestBlockHeader == nil || latestBlockHeader.BaseFee == nil {
		return nil, nil, ErrEIP1559NotSupported
	}

	// EIP-1559: gasFeeCap = (2 * baseFee) + gasTipCap
	gasFeeCap := new(big.Int).Mul(latestBlockHeader.BaseFee, big.NewInt(int64(baseFeeMultiplier)))
	gasFeeCap.Add(gasFeeCap, gasTipCap)

	return gasFeeCap, gasTipCap, nil
}
