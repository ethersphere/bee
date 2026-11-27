// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package staking

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/contractversion"
)

type versioned struct {
	s *contractversion.Switch[Contract]
}

var _ Contract = (*versioned)(nil)

// NewVersioned returns a Contract that forwards every call to the version
// active at the current block height.
func NewVersioned(s *contractversion.Switch[Contract]) Contract {
	return &versioned{s: s}
}

func (v *versioned) active(ctx context.Context) (Contract, error) {
	c, err := v.s.Active(ctx)
	if err != nil {
		return nil, fmt.Errorf("active staking contract: %w", err)
	}
	return c, nil
}

func (v *versioned) DepositStake(ctx context.Context, stakedAmount *big.Int) (common.Hash, error) {
	c, err := v.active(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return c.DepositStake(ctx, stakedAmount)
}

func (v *versioned) ChangeStakeOverlay(ctx context.Context, nonce common.Hash) (common.Hash, error) {
	c, err := v.active(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return c.ChangeStakeOverlay(ctx, nonce)
}

func (v *versioned) GetPotentialStake(ctx context.Context) (*big.Int, error) {
	c, err := v.active(ctx)
	if err != nil {
		return nil, err
	}
	return c.GetPotentialStake(ctx)
}

func (v *versioned) GetMinDeposit(ctx context.Context) (*big.Int, error) {
	c, err := v.active(ctx)
	if err != nil {
		return nil, err
	}
	return c.GetMinDeposit(ctx)
}

func (v *versioned) GetWithdrawableStake(ctx context.Context) (*big.Int, error) {
	c, err := v.active(ctx)
	if err != nil {
		return nil, err
	}
	return c.GetWithdrawableStake(ctx)
}

func (v *versioned) WithdrawStake(ctx context.Context) (common.Hash, error) {
	c, err := v.active(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return c.WithdrawStake(ctx)
}

func (v *versioned) MigrateStake(ctx context.Context) (common.Hash, error) {
	c, err := v.active(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return c.MigrateStake(ctx)
}

func (v *versioned) UpdateHeight(ctx context.Context) (common.Hash, bool, error) {
	c, err := v.active(ctx)
	if err != nil {
		return common.Hash{}, false, err
	}
	return c.UpdateHeight(ctx)
}

// IsOverlayFrozen uses the version in force at the given block rather than
// the current one.
func (v *versioned) IsOverlayFrozen(ctx context.Context, block uint64) (bool, error) {
	return v.s.At(block).IsOverlayFrozen(ctx, block)
}
