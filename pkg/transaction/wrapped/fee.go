// Copyright 2025 The Swarm Authors. All rights reserved.
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

// SuggestedFeeAndTip calculates the recommended gasFeeCap (maxFeePerGas) and gasTipCap (maxPriorityFeePerGas) for a transaction.
// If gasPrice is provided (legacy mode):
//   - On EIP-1559 networks: gasFeeCap = gasPrice; gasTipCap = max(gasPrice - baseFee, minimumTip) to respect the total cap while enforcing a tip floor where possible.
//   - On pre-EIP-1559 networks: returns (gasPrice, gasPrice) for legacy transaction compatibility.
//
// If gasPrice is nil: Uses suggested tip with optional boost, enforces minimum, and sets gasFeeCap = 2 * baseFee + gasTipCap.
func (b *wrappedBackend) SuggestedFeeAndTip(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error) {
	if gasPrice != nil {
		latestBlockHeader, err := b.backend.HeaderByNumber(ctx, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get latest block header: %w", err)
		}
		if latestBlockHeader == nil || latestBlockHeader.BaseFee == nil {
			return new(big.Int).Set(gasPrice), new(big.Int).Set(gasPrice), nil
		}

		baseFee := latestBlockHeader.BaseFee
		if gasPrice.Cmp(baseFee) < 0 {
			return nil, nil, fmt.Errorf("specified gas price %s is below current base fee %s", gasPrice, baseFee)
		}

		// nominal tip = gasPrice - baseFee
		gasTipCap := new(big.Int).Sub(gasPrice, baseFee)
		gasFeeCap := new(big.Int).Set(gasPrice)

		return gasFeeCap, gasTipCap, nil
	}

	gasTipCap, err := b.backend.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to suggest gas tip cap: %w", err)
	}
	gasTipCap = new(big.Int).Set(gasTipCap)

	if boostPercent != 0 {
		if boostPercent < 0 {
			return nil, nil, fmt.Errorf("negative boostPercent (%d) not allowed", boostPercent)
		}
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

	// gasFeeCap = (2 * baseFee) + gasTipCap
	gasFeeCap := new(big.Int).Mul(latestBlockHeader.BaseFee, big.NewInt(int64(baseFeeMultiplier)))
	gasFeeCap.Add(gasFeeCap, gasTipCap)

	return gasFeeCap, gasTipCap, nil
}
