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
	EscalateGasTip        = escalateGasTip
)

// SuggestGasFeeGasTipCapWithHistory exposes suggestGasFeeGasTipCapWithHistory for tests.
func SuggestGasFeeGasTipCapWithHistory(
	backend Backend,
	gasIncreasePercent int,
	maxTxPrice *big.Int,
	ctx context.Context,
	prevGasTipCap *big.Int,
	overrides *RetryOverrides,
) (gasFeeCap, gasTipCap *big.Int, err error) {
	svc := &transactionService{
		logger:                    log.Noop,
		backend:                   backend,
		txRetryGasIncreasePercent: gasIncreasePercent,
		maxTxPrice:                maxTxPrice,
	}
	return svc.suggestGasFeeGasTipCapWithHistory(ctx, prevGasTipCap, overrides)
}
