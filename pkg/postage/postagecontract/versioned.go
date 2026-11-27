// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postagecontract

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/contractversion"
)

type versioned struct {
	s *contractversion.Switch[Interface]
}

var _ Interface = (*versioned)(nil)

// NewVersioned returns an Interface that forwards every call to the version
// active at the current block height.
func NewVersioned(s *contractversion.Switch[Interface]) Interface {
	return &versioned{s: s}
}

func (v *versioned) active(ctx context.Context) (Interface, error) {
	c, err := v.s.Active(ctx)
	if err != nil {
		return nil, fmt.Errorf("active postage contract: %w", err)
	}
	return c, nil
}

func (v *versioned) CreateBatch(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) (common.Hash, []byte, error) {
	c, err := v.active(ctx)
	if err != nil {
		return common.Hash{}, nil, err
	}
	return c.CreateBatch(ctx, initialBalance, depth, immutable, label)
}

func (v *versioned) TopUpBatch(ctx context.Context, batchID []byte, topupBalance *big.Int) (common.Hash, error) {
	c, err := v.active(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return c.TopUpBatch(ctx, batchID, topupBalance)
}

func (v *versioned) DiluteBatch(ctx context.Context, batchID []byte, newDepth uint8) (common.Hash, error) {
	c, err := v.active(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return c.DiluteBatch(ctx, batchID, newDepth)
}

func (v *versioned) Paused(ctx context.Context) (bool, error) {
	c, err := v.active(ctx)
	if err != nil {
		return false, err
	}
	return c.Paused(ctx)
}

func (v *versioned) MinimumValidityBlocks(ctx context.Context) (uint64, error) {
	c, err := v.active(ctx)
	if err != nil {
		return 0, err
	}
	return c.MinimumValidityBlocks(ctx)
}

func (v *versioned) ExpireBatches(ctx context.Context) error {
	c, err := v.active(ctx)
	if err != nil {
		return err
	}
	return c.ExpireBatches(ctx)
}
