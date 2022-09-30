// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"math/big"

	"github.com/ethersphere/bee/pkg/staking/stakingcontract"
	"github.com/ethersphere/bee/pkg/swarm"
)

type stakingContractMock struct {
	depositStake func(ctx context.Context, stakedAmount *big.Int, overlay swarm.Address) error
	getStake     func(ctx context.Context, overlay swarm.Address) (*big.Int, error)
}

func (s *stakingContractMock) DepositStake(ctx context.Context, stakedAmount *big.Int, overlay swarm.Address) error {
	return s.depositStake(ctx, stakedAmount, overlay)
}

func (s *stakingContractMock) GetStake(ctx context.Context, overlay swarm.Address) (*big.Int, error) {
	return s.getStake(ctx, overlay)
}

// Option is a an option passed to New
type Option func(mock *stakingContractMock)

// New creates a new mock BatchStore
func New(opts ...Option) stakingcontract.Interface {
	bs := &stakingContractMock{}

	for _, o := range opts {
		o(bs)
	}

	return bs
}

func WithDepositStake(f func(ctx context.Context, stakedAmount *big.Int, overlay swarm.Address) error) Option {
	return func(mock *stakingContractMock) {
		mock.depositStake = f
	}
}

func WithGetStake(f func(ctx context.Context, overlay swarm.Address) (*big.Int, error)) Option {
	return func(mock *stakingContractMock) {
		mock.getStake = f
	}
}
