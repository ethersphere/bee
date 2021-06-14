// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Backend is the minimum of blockchain backend functions we need.
type Backend interface {
	bind.ContractBackend
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error)
	BlockNumber(ctx context.Context) (uint64, error)
	BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	BalanceAt(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error)
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
}

// IsSynced will check if we are synced with the given blockchain backend. This
// is true if the current wall clock is after the block time of last block
// with the given maxDelay as the maximum duration we can be behind the block
// time.
func IsSynced(ctx context.Context, backend Backend, maxDelay time.Duration) (bool, error) {
	number, err := backend.BlockNumber(ctx)
	if err != nil {
		return false, err
	}

	header, err := backend.HeaderByNumber(ctx, big.NewInt(int64(number)))
	if err != nil {
		return false, err
	}

	blockTime := time.Unix(int64(header.Time), 0)

	return blockTime.After(time.Now().UTC().Add(-maxDelay)), nil
}

// WaitSynced will wait until we are synced with the given blockchain backend,
// with the given maxDelay duration as the maximum time we can be behind the
// last block.
func WaitSynced(ctx context.Context, backend Backend, maxDelay time.Duration) error {
	for {
		synced, err := IsSynced(ctx, backend, maxDelay)
		if err != nil {
			return err
		}

		if synced {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// ParseABIUnchecked will parse a valid json abi. Only use this with string constants known to be correct.
func ParseABIUnchecked(json string) abi.ABI {
	cabi, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(fmt.Sprintf("error creating ABI for contract: %v", err))
	}
	return cabi
}
