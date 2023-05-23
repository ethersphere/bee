// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives_test

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	mockbatchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	contractMock "github.com/ethersphere/bee/pkg/postage/postagecontract/mock"
	erc20mock "github.com/ethersphere/bee/pkg/settlement/swap/erc20/mock"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storageincentives"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/storageincentives/staking/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	transactionmock "github.com/ethersphere/bee/pkg/transaction/mock"
	"github.com/ethersphere/bee/pkg/util/testutil"
)

func TestAgent(t *testing.T) {
	t.Parallel()

	bigBalance := big.NewInt(4_000_000_000)
	blockTime := time.Millisecond * 10
	tests := []struct {
		name           string
		blocksPerRound uint64
		blocksPerPhase uint64
		incrementBy    uint64
		limit          uint64
		expectedCalls  bool
		balance        *big.Int
	}{{
		name:           "3 blocks per phase, same block number returns twice",
		blocksPerRound: 9,
		blocksPerPhase: 3,
		incrementBy:    1,
		expectedCalls:  true,
		limit:          108, // computed with blocksPerRound * (exptectedCalls + 2)
		balance:        bigBalance,
	}, {
		name:           "3 blocks per phase, block number returns every block",
		blocksPerRound: 9,
		blocksPerPhase: 3,
		incrementBy:    1,
		expectedCalls:  true,
		limit:          108,
		balance:        bigBalance,
	}, {
		name:           "no expected calls - block number returns late after each phase",
		blocksPerRound: 9,
		blocksPerPhase: 3,
		incrementBy:    6,
		expectedCalls:  false,
		limit:          108,
		balance:        bigBalance,
	}, {
		name:           "4 blocks per phase, block number returns every other block",
		blocksPerRound: 12,
		blocksPerPhase: 4,
		incrementBy:    2,
		expectedCalls:  true,
		limit:          144,
		balance:        bigBalance,
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
	},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wait := make(chan struct{})
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
			contract := &mockContract{}

			service, _ := createService(t, backend, contract, blockTime, tc.blocksPerRound, tc.blocksPerPhase)
			testutil.CleanupCloser(t, service)

			<-wait

			if !tc.expectedCalls {
				if len(contract.callsList) > 0 {
					t.Fatal("got unexpected calls")
				} else {
					return
				}
			}

			contract.mtx.Lock()
			defer contract.mtx.Unlock()

			assertContractCallOrder(t, contract.callsList)
		})
	}
}

func TestAgentHalt(t *testing.T) {
	t.Parallel()

	const (
		blocksPerRound = 12
		blocksPerPhase = 4
		blockTime      = time.Millisecond * 10
	)

	tests := []struct {
		name          string
		haltAtBlock   uint64
		expectedCalls int
	}{
		{
			name:          "halt immediately",
			haltAtBlock:   0,
			expectedCalls: 0,
		},
		{
			name:          "halt without committing to the round",
			haltAtBlock:   blocksPerRound - 1,
			expectedCalls: 0, // agent should not wait for round to finish
		},
		{
			name:          "halt after committing to the round (beginning of commit phase)",
			haltAtBlock:   blocksPerRound + 1,
			expectedCalls: 3, // agent has just committed, but should finish round before halting
		},
		{
			name:          "halt after committing to the round (beginning of reveal phase)",
			haltAtBlock:   blocksPerRound + blocksPerPhase,
			expectedCalls: 3, // agent has just revealed, but should finish round before halting
		},
		{
			name:          "halt after committing to the round (beginning of claim phase)",
			haltAtBlock:   blocksPerRound + (2 * blocksPerPhase),
			expectedCalls: 3, // agent has just claimed, but should finish round before halting
		},
		{
			name:          "halt after committing to the round (#3 round)",
			haltAtBlock:   blocksPerRound*2 + 1,
			expectedCalls: 3 * 2,
		},
		{
			name:          "halt after committing to the round (#4 round)",
			haltAtBlock:   blocksPerRound*3 + 1,
			expectedCalls: 3 * 3,
		},
		{
			name:          "halt after committing to the round (#5 round)",
			haltAtBlock:   blocksPerRound*4 + 1,
			expectedCalls: 3 * 4,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wait := make(chan struct{})
			backend := &mockchainBackend{
				incrementBy: 1,
				balance:     big.NewInt(4_000_000_000),
				blockNumberCallback: func(bno uint64) {
					if bno >= tc.haltAtBlock {
						select {
						case <-wait:
						default:
							close(wait)
						}
					}
				},
			}
			contract := &mockContract{}
			service, _ := createService(t, backend, contract, blockTime, blocksPerRound, blocksPerPhase)
			testutil.CleanupCloser(t, service)

			<-wait
			select {
			case <-service.Halt():
			case <-time.After((blocksPerRound + 1) * blockTime):
				t.Fatal("halt signal was not received on time")
			}

			if tc.expectedCalls == 0 {
				return
			}

			contract.mtx.Lock()
			defer contract.mtx.Unlock()

			if got := len(contract.callsList); got != tc.expectedCalls {
				t.Fatalf("contract call list should have size: %d, got: %d", tc.expectedCalls, got)
			}

			assertContractCallOrder(t, contract.callsList)
		})
	}
}

func createService(
	t *testing.T,
	backend storageincentives.ChainBackend,
	contract redistribution.Contract,
	blockTime time.Duration,
	blocksPerRound uint64,
	blocksPerPhase uint64) (*storageincentives.Agent, error) {
	t.Helper()

	addr := swarm.RandAddress(t)
	postageContract := contractMock.New(contractMock.WithExpiresBatchesFunc(func(context.Context) error {
		return nil
	}),
	)
	stakingContract := mock.New(mock.WithIsFrozen(func(context.Context, uint64) (bool, error) {
		return false, nil
	}))

	return storageincentives.New(addr, common.Address{}, backend, &mockMonitor{}, contract, postageContract, stakingContract, mockbatchstore.New(mockbatchstore.WithReserveState(&postage.ReserveState{StorageRadius: 0})), &mockSampler{t: t}, blockTime, blocksPerRound, blocksPerPhase, statestore.NewStateStore(), erc20mock.New(), transactionmock.New(), &mockHealth{}, log.Noop)
}

type mockchainBackend struct {
	mu                  sync.Mutex
	incrementBy         uint64
	block               uint64
	limit               uint64
	limitCallback       func()
	balance             *big.Int
	blockNumberCallback func(uint64)
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

	if m.blockNumberCallback != nil {
		m.blockNumberCallback(ret)
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

func (m *mockchainBackend) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return big.NewInt(4), nil
}

type mockMonitor struct{}

func (m *mockMonitor) IsFullySynced() bool {
	return true
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
	callsList []contractCall
	mtx       sync.Mutex
}

func (m *mockContract) ReserveSalt(context.Context) ([]byte, error) {
	return nil, nil
}

func (m *mockContract) IsPlaying(context.Context, uint8) (bool, error) {
	return true, nil
}

func (m *mockContract) IsWinner(context.Context) (bool, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, isWinnerCall)
	return false, nil
}

func (m *mockContract) Claim(context.Context) (common.Hash, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, claimCall)
	return common.Hash{}, nil
}

func (m *mockContract) Commit(context.Context, []byte, *big.Int) (common.Hash, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, commitCall)
	return common.Hash{}, nil
}

func (m *mockContract) Reveal(context.Context, uint8, []byte, []byte) (common.Hash, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, revealCall)
	return common.Hash{}, nil
}

type mockSampler struct {
	t *testing.T
}

func (m *mockSampler) ReserveSample(context.Context, []byte, uint8, uint64) (storage.Sample, error) {
	return storage.Sample{
		Hash: swarm.RandAddress(m.t),
	}, nil
}

type mockHealth struct{}

func (m *mockHealth) IsHealthy() bool { return true }

func assertContractCallOrder(t *testing.T, callsList []contractCall) {
	t.Helper()

	if len(callsList) == 0 {
		t.Fatal("contract calls list should not be empty")
	}

	prevCall := callsList[0]
	assertPrevCall := func(call contractCall) {
		if prevCall != call {
			t.Fatalf("expected call %s, got %s", call, prevCall)
		}
	}

	for i := 1; i < len(callsList); i++ {
		switch callsList[i] {
		case isWinnerCall:
			assertPrevCall(revealCall)
		case revealCall:
			assertPrevCall(commitCall)
		case commitCall:
			assertPrevCall(isWinnerCall)
		}

		prevCall = callsList[i]
	}
}
