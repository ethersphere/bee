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

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	mockbatchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storageincentives"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
)

func TestAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		blocksPerRound float64
		blocksPerPhase float64
		incrementBy    float64
		limit          float64
		expectedCalls  bool
	}{{
		name:           "3 blocks per phase, same block number returns twice",
		blocksPerRound: 9,
		blocksPerPhase: 3,
		incrementBy:    1,
		expectedCalls:  true,
		limit:          108, // computed with blocksPerRound * (exptectedCalls + 2)
	}, {
		name:           "3 blocks per phase, block number returns every block",
		blocksPerRound: 9,
		blocksPerPhase: 3,
		incrementBy:    1,
		expectedCalls:  true,
		limit:          108,
	}, {
		name:           "3 blocks per phase, block number returns every other block",
		blocksPerRound: 9,
		blocksPerPhase: 3,
		incrementBy:    2,
		expectedCalls:  true,
		limit:          108,
	}, {
		name:           "no expected calls - block number returns late after each phase",
		blocksPerRound: 9,
		blocksPerPhase: 3,
		incrementBy:    6,
		expectedCalls:  false,
		limit:          108,
	}, {
		name:           "4 blocks per phase, block number returns every other block",
		blocksPerRound: 12,
		blocksPerPhase: 4,
		incrementBy:    2,
		expectedCalls:  true,
		limit:          144,
	},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wait := make(chan struct{})
			addr := test.RandomAddress()

			backend := &mockchainBackend{
				limit: tc.limit,
				limitCallback: func() {
					select {
					case wait <- struct{}{}:
					default:
					}
				},
				incrementBy: tc.incrementBy,
				block:       tc.blocksPerRound}
			contract := &mockContract{}

			service := createService(addr, backend, contract, uint64(tc.blocksPerRound), uint64(tc.blocksPerPhase))

			<-wait

			if !tc.expectedCalls {
				if len(contract.callsList) > 0 {
					t.Fatal("got unexpected calls")
				} else {
					err := service.Close()
					if err != nil {
						t.Fatal(err)
					}

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

			prevCall := contract.callsList[0]

			for i := 1; i < len(contract.callsList); i++ {

				switch contract.callsList[i] {
				case isWinnerCall:
					assertOrder(t, revealCall, prevCall)
				case revealCall:
					assertOrder(t, commitCall, prevCall)
				case commitCall:
					assertOrder(t, isWinnerCall, prevCall)
				}

				prevCall = contract.callsList[i]
			}

			err := service.Close()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func createService(
	addr swarm.Address,
	backend storageincentives.ChainBackend,
	contract redistribution.Contract,
	blocksPerRound uint64,
	blocksPerPhase uint64) *storageincentives.Agent {

	return storageincentives.New(
		addr,
		backend,
		log.Noop,
		&mockMonitor{},
		contract,
		mockbatchstore.New(mockbatchstore.WithReserveState(&postage.ReserveState{StorageRadius: 0})),
		&mockSampler{},
		time.Millisecond*10, blocksPerRound, blocksPerPhase,
	)
}

type mockchainBackend struct {
	incrementBy   float64
	block         float64
	limit         float64
	limitCallback func()
}

func (m *mockchainBackend) BlockNumber(context.Context) (uint64, error) {

	ret := uint64(m.block)

	if m.limit == 0 || m.block+m.incrementBy < m.limit {
		m.block += m.incrementBy
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

type mockMonitor struct {
}

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

func (m *mockContract) Claim(context.Context) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, claimCall)
	return nil
}

func (m *mockContract) Commit(context.Context, []byte, *big.Int) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, commitCall)
	return nil
}

func (m *mockContract) Reveal(context.Context, uint8, []byte, []byte) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, revealCall)
	return nil
}

type mockSampler struct{}

func (m *mockSampler) ReserveSample(context.Context, []byte, uint8, uint64) (storage.Sample, error) {
	return storage.Sample{
		Hash: test.RandomAddress(),
	}, nil
}
