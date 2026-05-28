// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"math/big"

	"github.com/ethersphere/bee/v2/pkg/log"
)

var (
	StoredTransactionKey  = storedTransactionKey
	RetryStateKey         = retryStateKey
	PendingTransactionKey = pendingTransactionKey
	ApplyMempoolBump      = applyMempoolBump
)

const (
	FeeTierLow        = feeTierLow
	FeeTierMarket     = feeTierMarket
	FeeTierAggressive = feeTierAggressive

	MempoolBumpPercent     = mempoolBumpPercent
	DefaultAttemptsPerTier = defaultAttemptsPerTier
)

// SuggestGasFeeForTier exposes suggestGasFeeForTier for tests.
func SuggestGasFeeForTier(
	backend Backend,
	maxTxPrice *big.Int,
	ctx context.Context,
	tier int,
	previousTip *big.Int,
	overrides *RetryOverrides,
) (gasFeeCap, gasTipCap *big.Int, err error) {
	svc := &transactionService{
		logger:     log.Noop,
		backend:    backend,
		maxTxPrice: maxTxPrice,
	}
	return svc.suggestGasFeeForTier(ctx, feeTier(tier), previousTip, overrides)
}
