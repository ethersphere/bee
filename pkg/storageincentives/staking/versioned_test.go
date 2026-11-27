// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package staking_test

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/contractversion"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/staking"
)

type backend struct {
	block uint64
	err   error
}

func (b *backend) BlockNumber(context.Context) (uint64, error) { return b.block, b.err }

type fake struct {
	staking.Contract
	calls int
}

func (f *fake) GetMinDeposit(context.Context) (*big.Int, error) {
	f.calls++
	return big.NewInt(1), nil
}

func (f *fake) DepositStake(context.Context, *big.Int) (common.Hash, error) {
	f.calls++
	return common.Hash{}, nil
}

func (f *fake) IsOverlayFrozen(context.Context, uint64) (bool, error) {
	f.calls++
	return false, nil
}

func setup(t *testing.T, b *backend) (staking.Contract, *fake, *fake) {
	t.Helper()
	v0, v1 := &fake{}, &fake{}
	s, err := contractversion.New[staking.Contract](log.Noop, "staking", b, []contractversion.Version[staking.Contract]{
		{FromBlock: 0, Impl: v0}, {FromBlock: 152, Impl: v1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return staking.NewVersioned(s), v0, v1
}

func TestVersionedForwardsToActive(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		block  uint64
		v0, v1 int
	}{{151, 2, 0}, {152, 0, 2}} {
		c, v0, v1 := setup(t, &backend{block: tc.block})
		if _, err := c.GetMinDeposit(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := c.DepositStake(context.Background(), big.NewInt(1)); err != nil {
			t.Fatal(err)
		}
		if v0.calls != tc.v0 || v1.calls != tc.v1 {
			t.Fatalf("block %d: got v0=%d v1=%d", tc.block, v0.calls, v1.calls)
		}
	}
}

func TestVersionedIsOverlayFrozenUsesGivenBlock(t *testing.T) {
	t.Parallel()
	c, v0, v1 := setup(t, &backend{block: 1000})
	if _, err := c.IsOverlayFrozen(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if v0.calls != 1 || v1.calls != 0 {
		t.Fatalf("got v0=%d v1=%d", v0.calls, v1.calls)
	}
}

func TestVersionedBackendError(t *testing.T) {
	t.Parallel()
	want := errors.New("boom")
	c, _, _ := setup(t, &backend{err: want})
	if _, err := c.GetMinDeposit(context.Background()); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
}
