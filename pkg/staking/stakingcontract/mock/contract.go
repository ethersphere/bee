package mock

import (
	"context"
	"math/big"

	"github.com/ethersphere/bee/pkg/staking/stakingcontract"
)

type stakingContractMock struct {
	depositStake func(ctx context.Context, stakedAmount *big.Int, overlay []byte) error
	getStake     func(ctx context.Context, overlay []byte) (*big.Int, error)
}

func (s *stakingContractMock) DepositStake(ctx context.Context, stakedAmount *big.Int, overlay []byte) error {
	return s.depositStake(ctx, stakedAmount, overlay)
}

func (s *stakingContractMock) GetStake(ctx context.Context, overlay []byte) (*big.Int, error) {
	return s.getStake(ctx, overlay)
}

// Option is a an option passed to New
type Option func(mock *stakingContractMock)

// New creates a new mock BatchStore
func New(opts ...Option) stakingcontract.StakingContract {
	bs := &stakingContractMock{}

	for _, o := range opts {
		o(bs)
	}

	return bs
}

func WithDepositStake(f func(ctx context.Context, stakedAmount *big.Int, overlay []byte) error) Option {
	return func(mock *stakingContractMock) {
		mock.depositStake = f
	}
}
