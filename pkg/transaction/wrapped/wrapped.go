// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

const (
	percentageDivisor = 100
	baseFeeMultiplier = 2
)

var (
	_                      transaction.Backend = (*wrappedBackend)(nil)
	ErrEIP1559NotSupported                     = errors.New("network does not appear to support EIP-1559 (no baseFee)")
)

type wrappedBackend struct {
	backend *ethclient.Client
	metrics metrics
}

func NewBackend(backend *ethclient.Client) transaction.Backend {
	return &wrappedBackend{
		backend: backend,
		metrics: newMetrics(),
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
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.BlockNumberCalls.Inc()
	blockNumber, err := b.backend.BlockNumber(ctx)
	if err != nil {
		b.metrics.TotalRPCErrors.Inc()
		return 0, err
	}
	return blockNumber, nil
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

func (b *wrappedBackend) EstimateGas(ctx context.Context, call ethereum.CallMsg) (gas uint64, err error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.EstimateGasCalls.Inc()
	gas, err = b.backend.EstimateGas(ctx, call)
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

// BlockByNumber implements transaction.Backend.
func (b *wrappedBackend) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	b.metrics.TotalRPCCalls.Inc()
	b.metrics.BlockByNumberCalls.Inc()
	block, err := b.backend.BlockByNumber(ctx, number)
	if err != nil {
		if !errors.Is(err, ethereum.NotFound) {
			b.metrics.TotalRPCErrors.Inc()
		}
		return nil, err
	}
	return block, nil
}

func (b *wrappedBackend) Close() error {
	b.backend.Close()
	return nil
}

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

	minimumTip := big.NewInt(transaction.MinimumGasTipCap)
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
