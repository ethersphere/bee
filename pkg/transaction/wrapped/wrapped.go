// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/backend"
	"github.com/ethersphere/bee/v2/pkg/transaction/wrapped/cache"
)

var _ transaction.Backend = (*wrappedBackend)(nil)

type blockNumberAnchor struct {
	number           uint64
	timestamp        time.Time
	averageBlockTime time.Duration
}

type wrappedBackend struct {
	backend           backend.Geth
	metrics           metrics
	minimumGasTipCap  int64
	blockTime         time.Duration
	blockSyncInterval uint64
	blockNumberCache  *cache.SingleFlightCache[blockNumberAnchor]
}

func NewBackend(
	backend backend.Geth,
	minimumGasTipCap uint64,
	blockTime time.Duration,
	blockSyncInterval uint64,
) transaction.Backend {
	if blockSyncInterval == 0 {
		blockSyncInterval = 1
	}

	return &wrappedBackend{
		backend:           backend,
		minimumGasTipCap:  int64(minimumGasTipCap),
		blockTime:         blockTime,
		metrics:           newMetrics(),
		blockSyncInterval: blockSyncInterval,
		blockNumberCache:  cache.NewSingleFlightCache[blockNumberAnchor]("block_number"),
	}
}

func (b *wrappedBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.TransactionReceiptCalls.Inc()
	receipt, err := b.backend.TransactionReceipt(ctx, txHash)
	if err != nil {
		if !errors.Is(err, ethereum.NotFound) {
			b.metrics.TotalRPCErrors.Inc()
		}
		return nil, err
	}
	return receipt, nil
}

func (b *wrappedBackend) TransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.TransactionCalls.Inc()
	tx, isPending, err := b.backend.TransactionByHash(ctx, hash)
	if err != nil {
		if !errors.Is(err, ethereum.NotFound) {
			b.metrics.TotalRPCErrors.Inc()
		}
		return nil, false, err
	}
	return tx, isPending, err
}

func (b *wrappedBackend) BlockNumber(ctx context.Context) (uint64, error) {
	canReuseBlockAnchorFn := func(anchor blockNumberAnchor) bool {
		_, elapsedBlocks := b.estimatedBlockNumberWithElapsed(anchor, time.Now().UTC())
		return elapsedBlocks < b.blockSyncInterval
	}

	loadFreshBlockFn := func(prev blockNumberAnchor) (blockNumberAnchor, error) {
		b.metrics.TotalRPCCalls.Inc()
		b.metrics.BlockHeaderAsBlockNumberCalls.Inc()

		header, err := b.backend.HeaderByNumber(ctx, nil)
		if err != nil {
			b.metrics.TotalRPCErrors.Inc()
			return blockNumberAnchor{}, err
		}
		if header == nil || header.Number == nil {
			b.metrics.TotalRPCErrors.Inc()
			return blockNumberAnchor{}, errors.New("latest block header unavailable")
		}

		newNumber := header.Number.Uint64()
		newTime := time.Unix(int64(header.Time), 0).UTC()
		averageBlockTime := b.computeAverageBlockTime(prev, newNumber, newTime)
		b.metrics.AverageBlockTimeSeconds.Set(averageBlockTime.Seconds())

		return blockNumberAnchor{
			number:           newNumber,
			timestamp:        newTime,
			averageBlockTime: averageBlockTime,
		}, nil
	}

	anchor, err := b.blockNumberCache.PeekOrLoad(
		ctx,
		canReuseBlockAnchorFn,
		loadFreshBlockFn,
	)
	if err != nil {
		return 0, err
	}
	return b.estimatedBlockNumber(anchor, time.Now().UTC()), nil
}

func (b *wrappedBackend) estimatedBlockNumber(anchor blockNumberAnchor, now time.Time) uint64 {
	currentBlock, _ := b.estimatedBlockNumberWithElapsed(anchor, now)
	return currentBlock
}

func (b *wrappedBackend) estimatedBlockNumberWithElapsed(anchor blockNumberAnchor, now time.Time) (uint64, uint64) {
	if now.Before(anchor.timestamp) {
		return anchor.number, 0
	}

	blockTime := anchor.averageBlockTime
	if blockTime == 0 {
		blockTime = b.blockTime
	}

	elapsedBlocks := uint64(now.Sub(anchor.timestamp) / blockTime)
	return anchor.number + elapsedBlocks, elapsedBlocks
}

// computeAverageBlockTime returns the observed block time between prev and the
// freshly loaded header. Falls back to the configured block time only when prev
// is unset. When the chain block number did not advance, wall-clock elapsed
// since prev is treated as the time for one block.
func (b *wrappedBackend) computeAverageBlockTime(prev blockNumberAnchor, newNumber uint64, newTime time.Time) time.Duration {
	if prev.number == 0 || newNumber <= prev.number {
		return b.blockTime
	}
	if !newTime.After(prev.timestamp) {
		return b.blockTime
	}

	elapsed := newTime.Sub(prev.timestamp)
	blocks := newNumber - prev.number

	return elapsed / time.Duration(blocks)
}

func (b *wrappedBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.BlockHeaderCalls.Inc()
	header, err := b.backend.HeaderByNumber(ctx, number)
	if err != nil {
		if !errors.Is(err, ethereum.NotFound) {
			b.metrics.TotalRPCErrors.Inc()
		}
		return nil, err
	}
	return header, nil
}

func (b *wrappedBackend) BalanceAt(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.BalanceCalls.Inc()
	balance, err := b.backend.BalanceAt(ctx, address, block)
	if err != nil {
		b.metrics.TotalRPCErrors.Inc()
		return nil, err
	}
	return balance, nil
}

func (b *wrappedBackend) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.NonceAtCalls.Inc()
	nonce, err := b.backend.NonceAt(ctx, account, blockNumber)
	if err != nil {
		b.metrics.TotalRPCErrors.Inc()
		return 0, err
	}
	return nonce, nil
}

func (b *wrappedBackend) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.CallContractCalls.Inc()
	result, err := b.backend.CallContract(ctx, call, blockNumber)
	if err != nil {
		b.metrics.TotalRPCErrors.Inc()
		return nil, err
	}
	return result, nil
}

func (b *wrappedBackend) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.CodeAtCalls.Inc()
	code, err := b.backend.CodeAt(ctx, contract, blockNumber)
	if err != nil {
		b.metrics.TotalRPCErrors.Inc()
		return nil, err
	}
	return code, nil
}

func (b *wrappedBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.PendingNonceCalls.Inc()
	nonce, err := b.backend.PendingNonceAt(ctx, account)
	if err != nil {
		b.metrics.TotalRPCErrors.Inc()
		return 0, err
	}
	return nonce, nil
}

func (b *wrappedBackend) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.SuggestGasTipCapCalls.Inc()
	gasTipCap, err := b.backend.SuggestGasTipCap(ctx)
	if err != nil {
		b.metrics.TotalRPCErrors.Inc()
		return nil, err
	}
	return gasTipCap, nil
}

func (b *wrappedBackend) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.EstimateGasCalls.Inc()
	gas, err := b.backend.EstimateGas(ctx, msg)
	if err != nil {
		b.metrics.TotalRPCErrors.Inc()
		return 0, err
	}
	return gas, nil
}

func (b *wrappedBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.SendTransactionCalls.Inc()
	err := b.backend.SendTransaction(ctx, tx)
	if err != nil {
		b.metrics.TotalRPCErrors.Inc()
		return err
	}
	return nil
}

func (b *wrappedBackend) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.FilterLogsCalls.Inc()
	logs, err := b.backend.FilterLogs(ctx, query)
	if err != nil {
		b.metrics.TotalRPCErrors.Inc()
		return nil, err
	}
	return logs, nil
}

func (b *wrappedBackend) ChainID(ctx context.Context) (*big.Int, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.ChainIDCalls.Inc()
	chainID, err := b.backend.ChainID(ctx)
	if err != nil {
		b.metrics.TotalRPCErrors.Inc()
		return nil, err
	}
	return chainID, nil
}

func (b *wrappedBackend) Close() {
	b.backend.Close()
}
