// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"math/big"
	"time"

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
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	BalanceAt(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error)
}

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

func WaitSynced(ctx context.Context, backend Backend, maxDelay time.Duration) error {
	for {
		synced, err := IsSynced(ctx, backend, maxDelay)
		if err != nil {
			return err
		}

		if synced {
			return nil
		}
	}
}
