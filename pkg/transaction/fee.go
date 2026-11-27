// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

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

const DefaultFeeHistoryBlockCount = 100

// Geth and Nethermind reject eth_feeHistory counts above 1024. That rejection
// makes every fee-priority price fall back, so a higher configuration value is clamped.
const MaxFeeHistoryBlockCount = 1000

var feeHistoryRewardPercentiles = []float64{10, 50, 90}

var ErrEIP1559NotSupported = errors.New("network does not appear to support EIP-1559 (no baseFee)")

func normalizeFeeHistoryBlocks(count uint64) uint64 {
	if count == 0 {
		return DefaultFeeHistoryBlockCount
	}
	if count > MaxFeeHistoryBlockCount {
		return MaxFeeHistoryBlockCount
	}
	return count
}

func (t *transactionService) SuggestFees(ctx context.Context, opts ...Option) (*big.Int, *big.Int, error) {
	txOpts, err := resolveTxOptions(opts...)
	if err != nil {
		return nil, nil, err
	}
	return t.fees(ctx, txOpts)
}

func (t *transactionService) fees(ctx context.Context, txOpts TxOptions) (*big.Int, *big.Int, error) {
	if t.suggestFees != nil {
		return t.suggestFees(ctx, txOpts)
	}
	if txOpts.Fee.Priority != 0 {
		return t.priorityFees(ctx, txOpts.Fee.Priority)
	}
	return t.suggestedFees(ctx, txOpts.Fee.GasPrice, txOpts.Fee.TipBoostPercent)
}

// suggestedFees calculates the recommended gasFeeCap (maxFeePerGas) and gasTipCap (maxPriorityFeePerGas) for a transaction.
// If gasPrice is provided (legacy mode):
//   - On EIP-1559 networks: gasFeeCap = gasPrice; gasTipCap = max(gasPrice - baseFee, minimumTip) to respect the total cap while enforcing a tip floor where possible.
//   - On pre-EIP-1559 networks: returns (gasPrice, gasPrice) for legacy transaction compatibility.
//
// If gasPrice is nil: Uses suggested tip with optional boost, enforces minimum, and sets gasFeeCap = 2 * baseFee + gasTipCap.
func (t *transactionService) suggestedFees(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error) {
	if gasPrice != nil {
		latestBlockHeader, err := t.backend.HeaderByNumber(ctx, nil)
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

	gasTipCap, err := t.backend.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to suggest gas tip cap: %w", err)
	}
	gasTipCap = new(big.Int).Set(gasTipCap)

	if boostPercent != 0 {
		// multiplier: 100 + boostPercent (e.g., 110 for 10% boost)
		multiplier := new(big.Int).Add(big.NewInt(int64(percentageDivisor)), big.NewInt(int64(boostPercent)))
		// gasTipCap = gasTipCap * (100 + boostPercent) / 100
		gasTipCap.Mul(gasTipCap, multiplier).Div(gasTipCap, big.NewInt(int64(percentageDivisor)))
	}

	minimumTip := big.NewInt(t.minimumGasTipCap)
	if gasTipCap.Cmp(minimumTip) < 0 {
		gasTipCap.Set(minimumTip)
	}

	latestBlockHeader, err := t.backend.HeaderByNumber(ctx, nil)
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

// priorityFees calculates gasFeeCap and gasTipCap for tier from eth_feeHistory.
// gasFeeCap is twice the next block base fee carried in that response plus the tier tip.
// When eth_feeHistory fails or has no usable rewards, it falls back to suggestedFees
// with the tier's fallback tip boost.
// It returns ErrEIP1559NotSupported when the latest block header has no base fee.
func (t *transactionService) priorityFees(ctx context.Context, tier int) (*big.Int, *big.Int, error) {
	tips, baseFee, err := t.suggestedFeeAndTipsFromHistory(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, err
		}
		t.metrics.FeeHistoryFallbacks.Inc()
		gasFeeCap, gasTipCap, fallbackErr := t.suggestedFees(ctx, nil, fallbackTipBoostPercent(tier))
		if fallbackErr != nil {
			return nil, nil, fmt.Errorf("fee history: %w; fallback suggestion: %w", err, fallbackErr)
		}
		return gasFeeCap, gasTipCap, nil
	}
	// eth_feeHistory reports zero base fees instead of an error on networks without EIP-1559.
	if baseFee.Sign() == 0 {
		latestBlockHeader, err := t.backend.HeaderByNumber(ctx, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get latest block header: %w", err)
		}
		if latestBlockHeader == nil || latestBlockHeader.BaseFee == nil {
			return nil, nil, ErrEIP1559NotSupported
		}
	}
	gasTipCap := new(big.Int).Set(tipForPriority(tier, tips))
	gasFeeCap := new(big.Int).Mul(baseFee, big.NewInt(int64(baseFeeMultiplier)))
	gasFeeCap.Add(gasFeeCap, gasTipCap)
	return gasFeeCap, gasTipCap, nil
}

func (t *transactionService) suggestedFeeAndTipsFromHistory(ctx context.Context) (*feeHistorySuggestedFeeAndTips, *big.Int, error) {
	t.metrics.FeeHistoryCalls.Inc()
	fh, err := t.backend.FeeHistory(ctx, t.feeHistoryBlocks, nil, feeHistoryRewardPercentiles)
	if err != nil {
		return nil, nil, err
	}
	low, market, aggressive, err := suggestedFeesFromFeeHistoryResult(fh)
	if err != nil {
		t.metrics.FeeHistoryParseErrors.Inc()
		return nil, nil, err
	}
	// eth_feeHistory appends the next block's base fee after the requested blocks.
	baseFee := fh.BaseFee[len(fh.BaseFee)-1]
	if baseFee == nil {
		t.metrics.FeeHistoryParseErrors.Inc()
		return nil, nil, errors.New("fee history: missing next base fee")
	}
	return &feeHistorySuggestedFeeAndTips{
		LowTip:        clampTip(low, t.minimumGasTipCap),
		MarketTip:     clampTip(market, t.minimumGasTipCap),
		AggressiveTip: clampTip(aggressive, t.minimumGasTipCap),
	}, new(big.Int).Set(baseFee), nil
}

// feeHistorySuggestedFeeAndTips holds the median priority fee at the 10th, 50th, and 90th reward percentiles.
type feeHistorySuggestedFeeAndTips struct {
	LowTip        *big.Int
	MarketTip     *big.Int
	AggressiveTip *big.Int
}

// tipForPriority expects a tier already checked by ValidateFeePriority.
func tipForPriority(tier int, fh *feeHistorySuggestedFeeAndTips) *big.Int {
	switch tier {
	case FeePriorityLow:
		return fh.LowTip
	case FeePriorityAggressive:
		return fh.AggressiveTip
	default:
		return fh.MarketTip
	}
}

// fallbackTipBoostPercent is the boost applied to the default tip suggestion when eth_feeHistory is unusable.
// Market uses the same boost as a transaction without a fee priority.
// It expects a tier already checked by ValidateFeePriority.
func fallbackTipBoostPercent(tier int) int {
	switch tier {
	case FeePriorityLow:
		return 15
	case FeePriorityAggressive:
		return 80
	default:
		return DefaultTipBoostPercent
	}
}

// clampTip raises tip to minimum when the history tip is below the node floor.
func clampTip(tip *big.Int, minimum int64) *big.Int {
	floor := big.NewInt(minimum)
	if tip.Cmp(floor) < 0 {
		return floor
	}
	return tip
}
