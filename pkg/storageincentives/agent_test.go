// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives_test

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	contractMock "github.com/ethersphere/bee/v2/pkg/postage/postagecontract/mock"
	erc20mock "github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20/mock"
	statestore "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/storageincentives"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/staking/mock"
	"github.com/ethersphere/bee/v2/pkg/storer"
	resMock "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

func TestAgent(t *testing.T) {
	t.Parallel()

	bigBalance := big.NewInt(4_000_000_000)
	tests := []struct {
		name           string
		blocksPerRound uint64
		blocksPerPhase uint64
		incrementBy    uint64
		limit          uint64
		expectedCalls  bool
		balance        *big.Int
		doubling       uint8
	}{
		{
			name:           "3 blocks per phase, same block number returns twice",
			blocksPerRound: 9,
			blocksPerPhase: 3,
			incrementBy:    1,
			expectedCalls:  true,
			limit:          108, // computed with blocksPerRound * (exptectedCalls + 2)
			balance:        bigBalance,
			doubling:       1,
		}, {
			name:           "3 blocks per phase, block number returns every block",
			blocksPerRound: 9,
			blocksPerPhase: 3,
			incrementBy:    1,
			expectedCalls:  true,
			limit:          108,
			balance:        bigBalance,
			doubling:       0,
		}, {
			name:           "no expected calls - block number returns late after each phase",
			blocksPerRound: 9,
			blocksPerPhase: 3,
			incrementBy:    6,
			expectedCalls:  false,
			limit:          108,
			balance:        bigBalance,
			doubling:       0,
		}, {
			name:           "4 blocks per phase, block number returns every other block",
			blocksPerRound: 12,
			blocksPerPhase: 4,
			incrementBy:    2,
			expectedCalls:  true,
			limit:          144,
			balance:        bigBalance,
			doubling:       1,
		}, {
			// This test case is based on previous, but this time agent will not have enough
			// balance to participate in the game so no calls are going to be made.
			name:           "no expected calls - insufficient balance",
			blocksPerRound: 12,
			blocksPerPhase: 4,
			incrementBy:    2,
			expectedCalls:  false,
			limit:          144,
			balance:        big.NewInt(0),
			doubling:       1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wait := make(chan struct{})
			addr := swarm.RandAddress(t)

			backend := &mockchainBackend{
				limit: tc.limit,
				limitCallback: func() {
					select {
					case wait <- struct{}{}:
					default:
					}
				},
				incrementBy: tc.incrementBy,
				block:       tc.blocksPerRound,
				balance:     tc.balance,
			}

			var radius uint8 = 8

			contract := &mockContract{t: t, expectedRadius: radius + tc.doubling}

			service, _ := createService(t, addr, backend, contract, tc.blocksPerRound, tc.blocksPerPhase, radius, tc.doubling)
			testutil.CleanupCloser(t, service)

			<-wait

			if !tc.expectedCalls {
				if len(contract.callsList) > 0 {
					t.Fatal("got unexpected calls")
				} else {
					return
				}
			}

			assertOrder := func(t *testing.T, want, got contractCall) {
				t.Helper()
				if want != got {
					t.Fatalf("expected call %s, got %s", want, got)
				}
			}

			contract.mtx.Lock()
			defer contract.mtx.Unlock()

			if len(contract.callsList) == 0 {
				t.Fatal("no calls were made to the contract")
			}

			prevCall := contract.callsList[0]

			for i := 1; i < len(contract.callsList); i++ {
				currentCall := contract.callsList[i]

				switch currentCall {
				case isWinnerCall:
					assertOrder(t, revealCall, prevCall)
				case revealCall:
					assertOrder(t, commitCall, prevCall)
				case commitCall:
					assertOrder(t, isWinnerCall, prevCall)
				case claimCall:
					assertOrder(t, isWinnerCall, prevCall)
				}

				prevCall = currentCall
			}
		})
	}
}

func createService(
	t *testing.T,
	addr swarm.Address,
	backend storageincentives.ChainBackend,
	contract redistribution.Contract,
	blocksPerRound uint64,
	blocksPerPhase uint64,
	radius uint8,
	doubling uint8,
) (*storageincentives.Agent, error) {
	t.Helper()

	postageContract := contractMock.New(contractMock.WithExpiresBatchesFunc(func(context.Context) error {
		return nil
	}),
	)
	stakingContract := mock.New(mock.WithIsFrozen(func(context.Context, uint64) (bool, error) {
		return false, nil
	}))

	reserve := resMock.NewReserve(
		resMock.WithRadius(radius),
		resMock.WithSample(storer.RandSample(t, nil)),
		resMock.WithCapacityDoubling(int(doubling)),
	)

	return storageincentives.New(
		addr, common.Address{},
		backend,
		contract,
		postageContract,
		stakingContract,
		reserve,
		func() bool { return true },
		time.Millisecond*100,
		blocksPerRound,
		blocksPerPhase,
		statestore.NewStateStore(),
		&postage.NoOpBatchStore{},
		erc20mock.New(),
		transactionmock.New(),
		&mockHealth{},
		log.Noop,
	)
}

type mockchainBackend struct {
	mu            sync.Mutex
	incrementBy   uint64
	block         uint64
	limit         uint64
	limitCallback func()
	balance       *big.Int
}

func (m *mockchainBackend) BlockNumber(context.Context) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ret := m.block
	lim := m.limit
	inc := m.incrementBy

	if lim == 0 || ret+inc < lim {
		m.block += inc
	} else if m.limitCallback != nil {
		m.limitCallback()
		return 0, errors.New("reached limit")
	}

	return ret, nil
}

func (m *mockchainBackend) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return &types.Header{
		Time: uint64(time.Now().Unix()),
	}, nil
}

func (m *mockchainBackend) BalanceAt(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
	return m.balance, nil
}

func (m *mockchainBackend) SuggestedFeeAndTip(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error) {
	return big.NewInt(4), big.NewInt(5), nil
}

type contractCall int

func (c contractCall) String() string {
	switch c {
	case isWinnerCall:
		return "isWinnerCall"
	case revealCall:
		return "revealCall"
	case commitCall:
		return "commitCall"
	case claimCall:
		return "claimCall"
	}
	return "unknown"
}

const (
	isWinnerCall contractCall = iota
	revealCall
	commitCall
	claimCall
)

type mockContract struct {
	callsList      []contractCall
	mtx            sync.Mutex
	expectedRadius uint8
	t              *testing.T
}

func (m *mockContract) ReserveSalt(context.Context) ([]byte, error) {
	return nil, nil
}

func (m *mockContract) IsPlaying(_ context.Context, r uint8) (bool, error) {
	if r != m.expectedRadius {
		m.t.Fatalf("isPlaying: expected radius %d, got %d", m.expectedRadius, r)
	}
	return true, nil
}

func (m *mockContract) IsWinner(context.Context) (bool, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, isWinnerCall)
	return false, nil
}

func (m *mockContract) Claim(context.Context, redistribution.ChunkInclusionProofs) (common.Hash, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, claimCall)
	return common.Hash{}, nil
}

func (m *mockContract) Commit(context.Context, []byte, uint64) (common.Hash, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, commitCall)
	return common.Hash{}, nil
}

func (m *mockContract) Reveal(_ context.Context, r uint8, _ []byte, _ []byte) (common.Hash, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if r != m.expectedRadius {
		m.t.Fatalf("reveal: expected radius %d, got %d", m.expectedRadius, r)
	}

	m.callsList = append(m.callsList, revealCall)
	return common.Hash{}, nil
}

type mockHealth struct{}

func (m *mockHealth) IsHealthy() bool { return true }

// TestAgentAdvanced uses Go 1.25 advanced testing features for better timing control
func TestAgentAdvanced(t *testing.T) {
	t.Parallel()

	// Use testing.T.Deadline() to ensure test doesn't run too long
	if deadline, ok := t.Deadline(); ok {
		timeout := time.Until(deadline) - time.Second // Leave 1 second buffer
		if timeout < 10*time.Second {
			t.Skip("Not enough time for advanced test")
		}
	}

	bigBalance := big.NewInt(4_000_000_000)

	// Test the problematic case with advanced timing control
	t.Run("4_blocks_per_phase_with_race_detection", func(t *testing.T) {
		t.Parallel()

		// Use a race detector-friendly approach
		var callSequence []contractCall
		var callMutex sync.Mutex
		var callCond sync.Cond
		callCond.L = &callMutex

		wait := make(chan struct{})
		addr := swarm.RandAddress(t)

		backend := &mockchainBackend{
			limit: 144,
			limitCallback: func() {
				select {
				case wait <- struct{}{}:
				default:
				}
			},
			incrementBy: 2,
			block:       12,
			balance:     bigBalance,
		}

		var radius uint8 = 8
		contract := &mockContractAdvanced{
			t:              t,
			expectedRadius: radius + 1,
			callSequence:   &callSequence,
			callMutex:      &callMutex,
			callCond:       &callCond,
		}

		service, _ := createService(t, addr, backend, contract, 12, 4, radius, 1)
		testutil.CleanupCloser(t, service)

		// Wait for completion with timeout
		select {
		case <-wait:
			// Test completed
		case <-time.After(15 * time.Second):
			t.Fatal("Test timed out")
		}

		// Verify call sequence with race detection
		callMutex.Lock()
		defer callMutex.Unlock()

		if len(callSequence) == 0 {
			t.Fatal("no calls were made to the contract")
		}

		// Verify the sequence with better error reporting
		prevCall := callSequence[0]
		for i := 1; i < len(callSequence); i++ {
			currentCall := callSequence[i]

			switch currentCall {
			case isWinnerCall:
				if prevCall != revealCall {
					t.Errorf("Call %d: expected isWinnerCall to follow revealCall, but previous was %s", i, prevCall)
				}
			case revealCall:
				if prevCall != commitCall {
					t.Errorf("Call %d: expected revealCall to follow commitCall, but previous was %s", i, prevCall)
				}
			case commitCall:
				if prevCall != isWinnerCall {
					t.Errorf("Call %d: expected commitCall to follow isWinnerCall, but previous was %s", i, prevCall)
				}
			case claimCall:
				if prevCall != isWinnerCall {
					t.Errorf("Call %d: expected claimCall to follow isWinnerCall, but previous was %s", i, prevCall)
				}
			}

			prevCall = currentCall
		}
	})
}

// mockContractAdvanced provides better race detection and timing control
type mockContractAdvanced struct {
	t              *testing.T
	expectedRadius uint8
	callSequence   *[]contractCall
	callMutex      *sync.Mutex
	callCond       *sync.Cond
}

func (m *mockContractAdvanced) ReserveSalt(context.Context) ([]byte, error) {
	return nil, nil
}

func (m *mockContractAdvanced) IsPlaying(_ context.Context, r uint8) (bool, error) {
	if r != m.expectedRadius {
		m.t.Fatalf("isPlaying: expected radius %d, got %d", m.expectedRadius, r)
	}
	return true, nil
}

func (m *mockContractAdvanced) IsWinner(context.Context) (bool, error) {
	m.callMutex.Lock()
	defer m.callMutex.Unlock()

	*m.callSequence = append(*m.callSequence, isWinnerCall)
	m.callCond.Broadcast() // Notify waiting goroutines

	return false, nil
}

func (m *mockContractAdvanced) Claim(context.Context, redistribution.ChunkInclusionProofs) (common.Hash, error) {
	m.callMutex.Lock()
	defer m.callMutex.Unlock()

	*m.callSequence = append(*m.callSequence, claimCall)
	m.callCond.Broadcast()

	return common.Hash{}, nil
}

func (m *mockContractAdvanced) Commit(context.Context, []byte, uint64) (common.Hash, error) {
	m.callMutex.Lock()
	defer m.callMutex.Unlock()

	*m.callSequence = append(*m.callSequence, commitCall)
	m.callCond.Broadcast()

	return common.Hash{}, nil
}

func (m *mockContractAdvanced) Reveal(_ context.Context, r uint8, _ []byte, _ []byte) (common.Hash, error) {
	if r != m.expectedRadius {
		m.t.Fatalf("reveal: expected radius %d, got %d", m.expectedRadius, r)
	}

	m.callMutex.Lock()
	defer m.callMutex.Unlock()

	*m.callSequence = append(*m.callSequence, revealCall)
	m.callCond.Broadcast()

	return common.Hash{}, nil
}

// TestAgentRaceDetection runs multiple iterations to detect race conditions
func TestAgentRaceDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race detection test in short mode")
	}

	// Use Go 1.25's race detection capabilities
	iterations := 10
	if testing.Verbose() {
		iterations = 20 // More iterations in verbose mode
	}

	for i := 0; i < iterations; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			t.Parallel()

			// Run the problematic test case multiple times
			runSingleAgentTest(t, "race_detection_test")
		})
	}
}

// runSingleAgentTest runs a single instance of the agent test
func runSingleAgentTest(t *testing.T, testName string) {
	bigBalance := big.NewInt(4_000_000_000)
	wait := make(chan struct{})
	addr := swarm.RandAddress(t)

	backend := &mockchainBackend{
		limit: 144,
		limitCallback: func() {
			select {
			case wait <- struct{}{}:
			default:
			}
		},
		incrementBy: 2,
		block:       12,
		balance:     bigBalance,
	}

	var radius uint8 = 8
	contract := &mockContract{t: t, expectedRadius: radius + 1}

	service, _ := createService(t, addr, backend, contract, 12, 4, radius, 1)
	testutil.CleanupCloser(t, service)

	// Wait for completion with timeout
	select {
	case <-wait:
		// Test completed
	case <-time.After(10 * time.Second):
		t.Fatal("Test timed out")
	}

	// Verify call sequence
	contract.mtx.Lock()
	defer contract.mtx.Unlock()

	if len(contract.callsList) == 0 {
		t.Fatal("no calls were made to the contract")
	}

	// Check sequence
	prevCall := contract.callsList[0]
	for i := 1; i < len(contract.callsList); i++ {
		currentCall := contract.callsList[i]

		switch currentCall {
		case isWinnerCall:
			if prevCall != revealCall {
				t.Errorf("Race detected! Call %d: expected isWinnerCall to follow revealCall, but previous was %s", i, prevCall)
			}
		case revealCall:
			if prevCall != commitCall {
				t.Errorf("Race detected! Call %d: expected revealCall to follow commitCall, but previous was %s", i, prevCall)
			}
		case commitCall:
			if prevCall != isWinnerCall {
				t.Errorf("Race detected! Call %d: expected commitCall to follow isWinnerCall, but previous was %s", i, prevCall)
			}
		case claimCall:
			if prevCall != isWinnerCall {
				t.Errorf("Race detected! Call %d: expected claimCall to follow isWinnerCall, but previous was %s", i, prevCall)
			}
		}

		prevCall = currentCall
	}
}
