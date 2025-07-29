// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/transaction/backend"
)

// Backend is the minimum of blockchain backend functions we need.
type Backend interface {
	backend.Geth
	SuggestedFeeAndTip(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error)
}

// IsSynced will check if we are synced with the given blockchain backend. This
// is true if the current wall clock is after the block time of last block
// with the given maxDelay as the maximum duration we can be behind the block
// time.
func IsSynced(ctx context.Context, backend Backend, maxDelay time.Duration) (bool, time.Time, error) {
	number, err := backend.BlockNumber(ctx)
	if err != nil {
		return false, time.Time{}, err
	}
	header, err := backend.HeaderByNumber(ctx, big.NewInt(int64(number)))
	if errors.Is(err, ethereum.NotFound) {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, err
	}

	blockTime := time.Unix(int64(header.Time), 0)

	return blockTime.After(time.Now().UTC().Add(-maxDelay)), blockTime, nil
}

// WaitSynced will wait until we are synced with the given blockchain backend,
// with the given maxDelay duration as the maximum time we can be behind the
// last block.
func WaitSynced(ctx context.Context, logger log.Logger, backend Backend, maxDelay time.Duration) error {
	logger = logger.WithName(loggerName).Register()

	for {
		synced, blockTime, err := IsSynced(ctx, backend, maxDelay)
		if err != nil {
			return err
		}

		if synced {
			return nil
		}

		logger.Info("still waiting for blockchain RPC to sync", "block_time", blockTime)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func WaitBlockAfterTransaction(ctx context.Context, backend Backend, pollingInterval time.Duration, txHash common.Hash, additionalConfirmations uint64) (*types.Header, error) {
	for {
		receipt, err := backend.TransactionReceipt(ctx, txHash)
		if err != nil {
			if !errors.Is(err, ethereum.NotFound) {
				return nil, err
			}
			select {
			case <-time.After(pollingInterval):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			continue
		}

		bn, err := backend.BlockNumber(ctx)
		if err != nil {
			return nil, err
		}

		nextBlock := receipt.BlockNumber.Uint64() + 1

		if bn >= nextBlock+additionalConfirmations {
			header, err := backend.HeaderByNumber(ctx, new(big.Int).SetUint64(nextBlock))
			if err != nil {
				if !errors.Is(err, ethereum.NotFound) {
					return nil, err
				}
				// in the case where we cannot find the block even though we already saw a higher number we keep on trying
			} else {
				return header, nil
			}
		}

		select {
		case <-time.After(pollingInterval):
		case <-ctx.Done():
			return nil, errors.New("context timeout")
		}
	}
}
