// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"context"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	erc20mock "github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20/mock"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
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
	log := testutil.NewLogger(t)
	state, err := NewRedistributionState(log, common.Address{}, mock.NewStateStore(), erc20mock.New(erc20Opts...), transactionmock.New(txOpts...))
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
	state.SetCurrentBlock(input.Block)
	state.SetCurrentEvent(input.Phase, input.Round)
	state.SetFullySynced(input.IsFullySynced)
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
			ReserveSampleHash: swarm.RandAddress(t),
			StorageRadius:     3,
		}
		state.SetSampleData(1, savedSample, 0)

		sample, exists := state.SampleData(1)
		if !exists {
			t.Error("should exist")
		}
		if diff := cmp.Diff(savedSample, sample); diff != "" {
			t.Errorf("sample mismatch (-want +have):\n%s", diff)
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
		if diff := cmp.Diff(savedKey, key); diff != "" {
			t.Errorf("key mismatch (-want +have):\n%s", diff)
		}
	})

	t.Run("has revealed", func(t *testing.T) {
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

func TestPurgeRoundData(t *testing.T) {
	t.Parallel()

	state := createRedistribution(t, nil, nil)

	// helper function which populates data at specified round
	populateDataAtRound := func(round uint64) {
		savedSample := SampleData{
			ReserveSampleHash: swarm.RandAddress(t),
			StorageRadius:     3,
		}
		commitKey := testutil.RandBytes(t, swarm.HashSize)

		state.SetSampleData(round, savedSample, 0)
		state.SetCommitKey(round, commitKey)
		state.SetHasRevealed(round)
	}

	// asserts if there is, or there isn't, data at specified round
	assertHasDataAtRound := func(round uint64, shouldHaveData bool) {
		check := func(exists bool) {
			if shouldHaveData && !exists {
				t.Error("should have data")
			} else if !shouldHaveData && exists {
				t.Error("should not have data")
			}
		}

		_, exists1 := state.SampleData(round)
		_, exists2 := state.CommitKey(round)
		exists3 := state.HasRevealed(round)

		check(exists1)
		check(exists2)
		check(exists3)
	}

	const roundsCount = 100
	hasRoundData := make([]bool, roundsCount)

	// Populate data at random rounds
	for i := uint64(0); i < roundsCount; i++ {
		v := rand.Int()%2 == 0
		hasRoundData[i] = v
		if v {
			populateDataAtRound(i)
		}
		assertHasDataAtRound(i, v)
	}

	// Run purge successively and assert that all data is purged up to
	// currentRound - purgeDataOlderThenXRounds
	for i := uint64(0); i < roundsCount; i++ {
		state.SetCurrentEvent(0, i)
		state.purgeStaleRoundData()

		if i <= purgeStaleDataThreshold {
			assertHasDataAtRound(i, hasRoundData[i])
		} else {
			for j := uint64(0); j < i-purgeStaleDataThreshold; j++ {
				assertHasDataAtRound(j, false)
			}
		}
	}

	// Purge remaining data in single go
	round := uint64(roundsCount + purgeStaleDataThreshold)
	state.SetCurrentEvent(0, round)
	state.purgeStaleRoundData()

	// One more time assert that everything was purged
	for i := uint64(0); i < roundsCount; i++ {
		assertHasDataAtRound(i, false)
	}
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
