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
	"testing/synctest"
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
			synctest.Test(t, func(t *testing.T) {
				wait := make(chan struct{}, 1)
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

				synctest.Wait()

				calls := contract.getCalls()

				if !tc.expectedCalls {
					if len(calls) > 0 {
						t.Fatal("got unexpected calls")
					}
					return
				}

				if len(calls) == 0 {
					t.Fatal("expected calls but got none")
				}

				assertOrder := func(t *testing.T, want, got contractCall) {
					t.Helper()
					if want != got {
						t.Fatalf("expected call %s, got %s", want, got)
					}
				}

				prevCall := calls[0]

				for i := 1; i < len(calls); i++ {
					switch calls[i] {
					case isWinnerCall:
						assertOrder(t, revealCall, prevCall)
					case revealCall:
						assertOrder(t, commitCall, prevCall)
					case commitCall:
						assertOrder(t, isWinnerCall, prevCall)
					}

					prevCall = calls[i]
				}
			})
		})
	}
}

func TestAgentEnabledDefault(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		backend := &mockchainBackend{
			incrementBy: 1,
			block:       9,
			limit:       18,
			balance:     big.NewInt(4_000_000_000),
		}
		contract := &mockContract{t: t, expectedRadius: 8}
		service, err := createService(t, swarm.RandAddress(t), backend, contract, 9, 3, 8, 0)
		if err != nil {
			t.Fatal(err)
		}
		testutil.CleanupCloser(t, service)

		if !service.IsEnabled() {
			t.Fatal("expected redistribution to be enabled by default")
		}

		service.SetEnabled(false)
		if service.IsEnabled() {
			t.Fatal("expected redistribution to be disabled")
		}

		service.SetEnabled(true)
		if !service.IsEnabled() {
			t.Fatal("expected redistribution to be enabled")
		}
	})
}

func TestAgentParticipationToggle(t *testing.T) {
	t.Parallel()

	const (
		blocksPerRound = uint64(9)
		blocksPerPhase = uint64(3)
		limit          = uint64(108)
	)
	bigBalance := big.NewInt(4_000_000_000)

	t.Run("disabled before sample still samples but skips commit", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			wait := make(chan struct{}, 1)
			backend := &mockchainBackend{
				limit: limit,
				limitCallback: func() {
					select {
					case wait <- struct{}{}:
					default:
					}
				},
				incrementBy: 1,
				block:       blocksPerRound,
				balance:     bigBalance,
			}
			contract := &mockContract{t: t, expectedRadius: 8}
			service, err := createService(t, swarm.RandAddress(t), backend, contract, blocksPerRound, blocksPerPhase, 8, 0)
			if err != nil {
				t.Fatal(err)
			}
			testutil.CleanupCloser(t, service)

			service.SetEnabled(false)

			<-wait
			synctest.Wait()

			if got := contract.playingCount(); got == 0 {
				t.Fatal("expected sampling to run while disabled")
			}
			if got := contract.countCalls(commitCall); got != 0 {
				t.Fatalf("expected no commit calls, got %d", got)
			}
			if got := contract.countCalls(revealCall); got != 0 {
				t.Fatalf("expected no reveal calls, got %d", got)
			}
		})
	})

	t.Run("disable after sample skips commit", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			wait := make(chan struct{}, 1)
			started := make(chan struct{})
			unblock := make(chan struct{})
			var once sync.Once
			backend := &mockchainBackend{
				limit: limit,
				limitCallback: func() {
					select {
					case wait <- struct{}{}:
					default:
					}
				},
				incrementBy: 1,
				block:       blocksPerRound,
				balance:     bigBalance,
			}
			contract := &mockContract{
				t:              t,
				expectedRadius: 8,
				beforeIsPlaying: func() {
					once.Do(func() { close(started) })
					<-unblock
				},
			}
			service, err := createService(t, swarm.RandAddress(t), backend, contract, blocksPerRound, blocksPerPhase, 8, 0)
			if err != nil {
				t.Fatal(err)
			}
			testutil.CleanupCloser(t, service)

			<-started
			service.SetEnabled(false)
			close(unblock)

			<-wait
			synctest.Wait()

			if got := contract.playingCount(); got == 0 {
				t.Fatal("expected isPlaying to run before disable")
			}
			if got := contract.countCalls(commitCall); got != 0 {
				t.Fatalf("expected no commit calls, got %d", got)
			}
		})
	})

	t.Run("disable after commit still reveals", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			wait := make(chan struct{}, 1)
			committed := make(chan struct{})
			var once sync.Once
			backend := &mockchainBackend{
				limit: limit,
				limitCallback: func() {
					select {
					case wait <- struct{}{}:
					default:
					}
				},
				incrementBy: 1,
				block:       blocksPerRound,
				balance:     bigBalance,
			}
			contract := &mockContract{
				t:              t,
				expectedRadius: 8,
				beforeCommit: func() {
					once.Do(func() { close(committed) })
				},
			}
			service, err := createService(t, swarm.RandAddress(t), backend, contract, blocksPerRound, blocksPerPhase, 8, 0)
			if err != nil {
				t.Fatal(err)
			}
			testutil.CleanupCloser(t, service)

			<-committed
			service.SetEnabled(false)

			<-wait
			synctest.Wait()

			if got := contract.countCalls(commitCall); got != 1 {
				t.Fatalf("expected exactly one commit, got %d", got)
			}
			if got := contract.countCalls(revealCall); got != 1 {
				t.Fatalf("expected reveal after commit, got %d", got)
			}
			if got := contract.countCalls(isWinnerCall); got != 1 {
				t.Fatalf("expected claim-phase winner check, got %d", got)
			}
		})
	})

	t.Run("disable during commit still reveals", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			wait := make(chan struct{}, 1)
			started := make(chan struct{})
			unblock := make(chan struct{})
			var once sync.Once
			backend := &mockchainBackend{
				limit: limit,
				limitCallback: func() {
					select {
					case wait <- struct{}{}:
					default:
					}
				},
				incrementBy: 1,
				block:       blocksPerRound,
				balance:     bigBalance,
			}
			contract := &mockContract{
				t:              t,
				expectedRadius: 8,
				beforeCommit: func() {
					once.Do(func() { close(started) })
					<-unblock
				},
			}
			service, err := createService(t, swarm.RandAddress(t), backend, contract, blocksPerRound, blocksPerPhase, 8, 0)
			if err != nil {
				t.Fatal(err)
			}
			testutil.CleanupCloser(t, service)

			<-started
			service.SetEnabled(false)
			close(unblock)

			<-wait
			synctest.Wait()

			if got := contract.countCalls(commitCall); got != 1 {
				t.Fatalf("expected in-flight commit to finish, got %d", got)
			}
			if got := contract.countCalls(revealCall); got != 1 {
				t.Fatalf("expected reveal after in-flight commit, got %d", got)
			}
		})
	})

	t.Run("re-enable in same commit phase does not retry", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			wait := make(chan struct{}, 1)
			started := make(chan struct{})
			unblock := make(chan struct{})
			var once sync.Once
			backend := &mockchainBackend{
				limit: limit,
				limitCallback: func() {
					select {
					case wait <- struct{}{}:
					default:
					}
				},
				incrementBy: 1,
				block:       blocksPerRound,
				balance:     bigBalance,
			}
			contract := &mockContract{
				t:              t,
				expectedRadius: 8,
				beforeIsPlaying: func() {
					once.Do(func() { close(started) })
					<-unblock
				},
			}
			service, err := createService(t, swarm.RandAddress(t), backend, contract, blocksPerRound, blocksPerPhase, 8, 0)
			if err != nil {
				t.Fatal(err)
			}
			testutil.CleanupCloser(t, service)

			<-started
			service.SetEnabled(false)
			close(unblock)

			waitUntilStatus(t, service, func(status *storageincentives.Status) bool {
				return status.Phase.String() == "commit" && status.Round >= 2
			})
			synctest.Wait()

			if got := contract.countCalls(commitCall); got != 0 {
				t.Fatalf("expected skipped commit, got %d", got)
			}

			service.SetEnabled(true)
			time.Sleep(200 * time.Millisecond)
			synctest.Wait()

			if got := contract.countCalls(commitCall); got != 0 {
				t.Fatalf("re-enable in the same commit phase should not commit, got %d", got)
			}

			<-wait
			synctest.Wait()

			if got := contract.countCalls(commitCall); got == 0 {
				t.Fatal("expected commit after the next sample cycle")
			}
		})
	})
}

// Claim receives no bypass options unless the node setting is enabled. When it is,
// bypass starts DelayBlocks into the claim phase, and a delay longer than the phase is clamped.
func TestAgentClaimPriceBypass(t *testing.T) {
	t.Parallel()

	const (
		blocksPerPhase   = uint64(3)
		claimPhaseBlocks = winningAgentBlocksPerRound - 2*blocksPerPhase
	)

	tests := []struct {
		name      string
		bypass    storageincentives.ClaimMaxPriceBypass
		wantNil   bool
		wantDelay uint64
	}{
		{
			name:    "disabled by default",
			wantNil: true,
		},
		{
			name:      "enabled after the configured delay",
			bypass:    storageincentives.ClaimMaxPriceBypass{Enabled: true, DelayBlocks: 1},
			wantDelay: 1,
		},
		{
			name:      "delay above the claim phase is clamped",
			bypass:    storageincentives.ClaimMaxPriceBypass{Enabled: true, DelayBlocks: 100},
			wantDelay: claimPhaseBlocks,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				_, contract := runWinningAgent(t, nil, withClaimPriceBypass(tc.bypass))

				claims := contract.getClaims()
				if len(claims) == 0 {
					t.Fatal("expected a claim")
				}
				claim := claims[0]
				if tc.wantNil {
					if claim.hasOpts {
						t.Fatal("bypass is disabled, claim must not receive options")
					}
					return
				}
				if !claim.hasOpts {
					t.Fatal("bypass is enabled, claim must receive options")
				}
				phaseOffset := 2*blocksPerPhase + tc.wantDelay
				if got := claim.opts.BypassFromBlock; got < phaseOffset || (got-phaseOffset)%winningAgentBlocksPerRound != 0 {
					t.Fatalf("got BypassFromBlock %d, want a round start plus %d blocks", got, phaseOffset)
				}
			})
		})
	}
}

const winningAgentBlocksPerRound = uint64(9)

// runWinningAgent runs an agent that wins every round until the chain mock stops.
func runWinningAgent(t *testing.T, claimErr error, opts ...serviceOption) (*storageincentives.Agent, *mockContract) {
	t.Helper()

	const radius = uint8(8)
	wait := make(chan struct{}, 1)
	backend := &mockchainBackend{
		limit: 12 * winningAgentBlocksPerRound,
		limitCallback: func() {
			select {
			case wait <- struct{}{}:
			default:
			}
		},
		incrementBy: 1,
		block:       winningAgentBlocksPerRound,
		balance:     big.NewInt(4_000_000_000),
	}
	contract := &mockContract{
		t:              t,
		expectedRadius: radius,
		salt:           testutil.RandBytes(t, swarm.HashSize),
		winner:         true,
		claimErr:       claimErr,
	}
	agent, err := createService(t, swarm.RandAddress(t), backend, contract, winningAgentBlocksPerRound, 3, radius, 0, opts...)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, agent)

	<-wait
	synctest.Wait()
	return agent, contract
}

func waitUntilStatus(t *testing.T, agent *storageincentives.Agent, pred func(*storageincentives.Status) bool) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		status, err := agent.Status()
		if err == nil && pred(status) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for redistribution status")
		}
		time.Sleep(time.Millisecond)
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
	opts ...serviceOption,
) (*storageincentives.Agent, error) {
	t.Helper()

	var deps serviceDeps
	for _, opt := range opts {
		opt(&deps)
	}

	postageContract := contractMock.New(contractMock.WithExpiresBatchesFunc(func(context.Context) error {
		return nil
	}))
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
		deps.bypass,
	)
}

type serviceDeps struct {
	bypass storageincentives.ClaimMaxPriceBypass
}

type serviceOption func(*serviceDeps)

func withClaimPriceBypass(bypass storageincentives.ClaimMaxPriceBypass) serviceOption {
	return func(d *serviceDeps) { d.bypass = bypass }
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
	callsList       []contractCall
	mtx             sync.Mutex
	expectedRadius  uint8
	t               *testing.T
	isPlayingCount  int
	beforeIsPlaying func()
	beforeCommit    func()
	salt            []byte
	winner          bool
	claimErr        error
	claims          []claimRecord
}

// claimRecord is what the agent handed to Claim.
type claimRecord struct {
	opts    redistribution.ClaimOpts
	hasOpts bool
}

// getCalls returns a snapshot of the calls list
// even after synctest.Wait() all goroutines are blocked, we still should use locking
// for defensive programming.
func (m *mockContract) getCalls() []contractCall {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	// return a copy to avoid external modifications
	calls := make([]contractCall, len(m.callsList))
	copy(calls, m.callsList)
	return calls
}

func (m *mockContract) countCalls(call contractCall) int {
	n := 0
	for _, c := range m.getCalls() {
		if c == call {
			n++
		}
	}
	return n
}

func (m *mockContract) playingCount() int {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.isPlayingCount
}

func (m *mockContract) ReserveSalt(context.Context) ([]byte, error) {
	return m.salt, nil
}

func (m *mockContract) IsPlaying(_ context.Context, r uint8) (bool, error) {
	if m.beforeIsPlaying != nil {
		m.beforeIsPlaying()
	}
	if r != m.expectedRadius {
		m.t.Fatalf("isPlaying: expected radius %d, got %d", m.expectedRadius, r)
	}
	m.mtx.Lock()
	m.isPlayingCount++
	m.mtx.Unlock()
	return true, nil
}

func (m *mockContract) IsWinner(context.Context) (bool, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, isWinnerCall)
	return m.winner, nil
}

func (m *mockContract) Claim(_ context.Context, _ redistribution.ChunkInclusionProofs, opts *redistribution.ClaimOpts) (common.Hash, error) {
	var record claimRecord
	if opts != nil {
		record = claimRecord{opts: *opts, hasOpts: true}
	}
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, claimCall)
	m.claims = append(m.claims, record)
	return common.Hash{}, m.claimErr
}

func (m *mockContract) getClaims() []claimRecord {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return append([]claimRecord(nil), m.claims...)
}

func (m *mockContract) Commit(context.Context, []byte, uint64) (common.Hash, error) {
	if m.beforeCommit != nil {
		m.beforeCommit()
	}
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
