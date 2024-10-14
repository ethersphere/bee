// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/staking"
)

type stakingContractMock struct {
	depositStake     func(ctx context.Context, stakedAmount *big.Int) (common.Hash, error)
	getStake         func(ctx context.Context) (*big.Int, error)
	withdrawAllStake func(ctx context.Context) (common.Hash, error)
	migrateStake     func(ctx context.Context) (common.Hash, error)
	isFrozen         func(ctx context.Context, block uint64) (bool, error)
}

func (s *stakingContractMock) DepositStake(ctx context.Context, stakedAmount *big.Int) (common.Hash, error) {
	return s.depositStake(ctx, stakedAmount)
}

func (s *stakingContractMock) ChangeStakeOverlay(_ context.Context, h common.Hash) (common.Hash, error) {
	return h, nil
}

func (s *stakingContractMock) UpdateHeight(_ context.Context) (common.Hash, bool, error) {
	return common.Hash{}, false, nil
}

func (s *stakingContractMock) GetPotentialStake(ctx context.Context) (*big.Int, error) {
	return s.getStake(ctx)
}

func (s *stakingContractMock) GetWithdrawableStake(ctx context.Context) (*big.Int, error) {
	return s.getStake(ctx)
}

func (s *stakingContractMock) WithdrawStake(ctx context.Context) (common.Hash, error) {
	return s.withdrawAllStake(ctx)
}

func (s *stakingContractMock) MigrateStake(ctx context.Context) (common.Hash, error) {
	return s.migrateStake(ctx)
}

func (s *stakingContractMock) IsOverlayFrozen(ctx context.Context, block uint64) (bool, error) {
	return s.isFrozen(ctx, block)
}

// Option is a an option passed to New
type Option func(mock *stakingContractMock)

// New creates a new mock BatchStore.
func New(opts ...Option) staking.Contract {
	bs := &stakingContractMock{}

	for _, o := range opts {
		o(bs)
	}

	return bs
}

func WithDepositStake(f func(ctx context.Context, stakedAmount *big.Int) (common.Hash, error)) Option {
	return func(mock *stakingContractMock) {
		mock.depositStake = f
	}
}

func WithGetStake(f func(ctx context.Context) (*big.Int, error)) Option {
	return func(mock *stakingContractMock) {
		mock.getStake = f
	}
}

func WithWithdrawStake(f func(ctx context.Context) (common.Hash, error)) Option {
	return func(mock *stakingContractMock) {
		mock.withdrawAllStake = f
	}
}

func WithMigrateStake(f func(ctx context.Context) (common.Hash, error)) Option {
	return func(mock *stakingContractMock) {
		mock.migrateStake = f
	}
}

func WithIsFrozen(f func(ctx context.Context, block uint64) (bool, error)) Option {
	return func(mock *stakingContractMock) {
		mock.isFrozen = f
	}
}
