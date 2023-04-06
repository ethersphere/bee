// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"bytes"
	"context"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/log"
	erc20mock "github.com/ethersphere/bee/pkg/settlement/swap/erc20/mock"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	transactionmock "github.com/ethersphere/bee/pkg/transaction/mock"
	"github.com/ethersphere/bee/pkg/util/testutil"
	"github.com/google/go-cmp/cmp"
)

func createRedistribution(t *testing.T, erc20Opts []erc20mock.Option, txOpts []transactionmock.Option) *RedistributionState {
	t.Helper()
	if erc20Opts == nil {
		erc20Opts = []erc20mock.Option{
			erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
				return big.NewInt(1000), nil
			}),
		}
	}
	if txOpts == nil {
		txOpts = []transactionmock.Option{
			transactionmock.WithTransactionFeeFunc(func(ctx context.Context, txHash common.Hash) (*big.Int, error) {
				return big.NewInt(1000), nil
			}),
		}
	}
	state, err := NewRedistributionState(log.Noop, common.Address{}, mock.NewStateStore(), erc20mock.New(erc20Opts...), transactionmock.New(txOpts...))
	if err != nil {
		t.Fatal("failed to connect")
	}
	return state
}

func TestState(t *testing.T) {
	t.Parallel()
	input := Status{
		Phase:           commit,
		IsFrozen:        true,
		IsFullySynced:   true,
		Round:           2,
		LastWonRound:    2,
		LastPlayedRound: 2,
		LastFrozenRound: 2,
		Block:           2,
	}
	want := Status{
		Phase:           commit,
		IsFrozen:        true,
		IsFullySynced:   true,
		Round:           2,
		LastWonRound:    2,
		LastPlayedRound: 2,
		LastFrozenRound: 2,
		Block:           2,
		Fees:            big.NewInt(0),
		Reward:          big.NewInt(0),
		RoundData:       make(map[uint64]RoundData),
	}
	state := createRedistribution(t, nil, nil)
	state.SetCurrentEvent(input.Phase, input.Round, input.Block)
	state.IsFullySynced(input.IsFullySynced)
	state.SetLastWonRound(input.LastWonRound)
	state.SetFrozen(input.IsFrozen, input.LastFrozenRound)
	state.SetLastPlayedRound(input.LastPlayedRound)
	got, err := state.Status()
	if err != nil {
		t.Fatal("failed to get state")
	}

	opt := []cmp.Option{
		cmp.AllowUnexported(big.Int{}),
		cmp.AllowUnexported(Status{}),
	}
	if diff := cmp.Diff(want, *got, opt...); diff != "" {
		t.Errorf("result mismatch (-want +have):\n%s", diff)
	}

}

func TestStateRoundData(t *testing.T) {
	t.Parallel()

	t.Run("sample data", func(t *testing.T) {
		t.Parallel()

		state := createRedistribution(t, nil, nil)

		_, exists := state.SampleData(1)
		if exists {
			t.Error("should not exists")
		}

		savedSample := SampleData{
			ReserveSample: storage.Sample{
				Hash: swarm.RandAddress(t),
			},
			StorageRadius: 3,
		}
		state.SetSampleData(1, savedSample)

		sample, exists := state.SampleData(1)
		if !exists {
			t.Error("should exist")
		}
		if !reflect.DeepEqual(savedSample, sample) {
			t.Errorf("sample does not match saved sample")
		}
	})

	t.Run("commit key", func(t *testing.T) {
		t.Parallel()

		state := createRedistribution(t, nil, nil)

		_, exists := state.CommitKey(1)
		if exists {
			t.Error("should not exists")
		}

		savedKey := testutil.RandBytes(t, swarm.HashSize)
		state.SetCommitKey(1, savedKey)

		key, exists := state.CommitKey(1)
		if !exists {
			t.Error("should exist")
		}
		if !bytes.Equal(savedKey, key) {
			t.Errorf("key does not match saved key")
		}
	})

	t.Run("has reveal", func(t *testing.T) {
		t.Parallel()

		state := createRedistribution(t, nil, nil)

		if state.HasRevealed(1) {
			t.Error("should not be revealed")
		}

		state.SetHasRevealed(1)

		if !state.HasRevealed(1) {
			t.Error("should be revealed")
		}
	})

}

// TestReward test reward calculations. It also checks whether reward is incremented after second win.
func TestReward(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// first win reward calculation
	initialBalance := big.NewInt(3000)
	state := createRedistribution(t, []erc20mock.Option{
		erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
			return initialBalance, nil
		}),
	}, nil)
	err := state.SetBalance(ctx)
	if err != nil {
		t.Fatal("failed to set balance")
	}
	balanceAfterFirstWin := big.NewInt(4000)
	state.erc20Service = erc20mock.New([]erc20mock.Option{
		erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
			return big.NewInt(4000), nil
		}),
	}...)

	err = state.CalculateWinnerReward(ctx)
	if err != nil {
		t.Fatal("failed to calculate reward")
	}
	firstWinResult, err := state.Status()
	if err != nil {
		t.Fatal("failed to get status")
	}
	expectedReward := balanceAfterFirstWin.Sub(balanceAfterFirstWin, initialBalance)
	if firstWinResult.Reward.Cmp(expectedReward) != 0 {
		t.Fatalf("expect reward %d got %d", expectedReward, firstWinResult.Reward)
	}

	// Second win reward calculation. The reward should add up
	err = state.SetBalance(ctx)
	if err != nil {
		t.Fatal("failed to set balance")
	}
	// Set latest balance
	newCurrentBalance := state.currentBalance
	balanceAfterSecondWin := big.NewInt(7000)
	state.erc20Service = erc20mock.New([]erc20mock.Option{
		erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
			return big.NewInt(7000), nil
		}),
	}...)

	err = state.CalculateWinnerReward(ctx)
	if err != nil {
		t.Fatal("failed to calculate reward")
	}
	secondWinResult, err := state.Status()
	if err != nil {
		t.Fatal("failed to get status")
	}
	expectedSecondReward := firstWinResult.Reward.Add(firstWinResult.Reward, balanceAfterSecondWin.Sub(balanceAfterSecondWin, newCurrentBalance))
	if secondWinResult.Reward.Cmp(expectedSecondReward) != 0 {
		t.Fatalf("expect reward %d got %d", expectedSecondReward, secondWinResult.Reward)
	}
}

// TestFee check if fees increments when called multiple times
func TestFee(t *testing.T) {
	t.Parallel()
	firstFee := big.NewInt(10)
	state := createRedistribution(t, nil, []transactionmock.Option{
		transactionmock.WithTransactionFeeFunc(func(ctx context.Context, txHash common.Hash) (*big.Int, error) {
			return firstFee, nil
		}),
	})
	ctx := context.Background()
	state.AddFee(ctx, common.Hash{})
	gotFirstResult, err := state.Status()
	if err != nil {
		t.Fatal("failed to get status")
	}
	if gotFirstResult.Fees.Cmp(firstFee) != 0 {
		t.Fatalf("expected fee %d got %d", firstFee, gotFirstResult.Fees)
	}
	secondFee := big.NewInt(15)
	state.txService = transactionmock.New([]transactionmock.Option{
		transactionmock.WithTransactionFeeFunc(func(ctx context.Context, txHash common.Hash) (*big.Int, error) {
			return secondFee, nil
		}),
	}...)

	state.AddFee(ctx, common.Hash{})
	gotSecondResult, err := state.Status()
	if err != nil {
		t.Fatal("failed to get status")
	}
	expectedResult := secondFee.Add(secondFee, firstFee)
	if gotSecondResult.Fees.Cmp(expectedResult) != 0 {
		t.Fatalf("expected fee %d got %d", expectedResult, gotSecondResult.Fees)
	}
}
