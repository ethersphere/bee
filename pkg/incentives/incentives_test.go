// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package incentives_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/incentives"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	mockbatchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"go.uber.org/atomic"
)

func Test(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		blocksPerRound float64
		blocksPerPhase float64
		incrementBy    float64
		limit          float64
		expectedCalls  int
	}{{
		name:           "3 blocks per phase, same block number returns twice",
		blocksPerRound: 9,
		blocksPerPhase: 3,
		incrementBy:    0.5,
		expectedCalls:  10,
		limit:          108, // computed with blocksPerRound * (exptectedCalls + 2)
	}, {
		name:           "3 blocks per phase, block number returns every block",
		blocksPerRound: 9,
		blocksPerPhase: 3,
		incrementBy:    1,
		expectedCalls:  10,
		limit:          108,
	}, {
		name:           "3 blocks per phase, block number returns every other block",
		blocksPerRound: 9,
		blocksPerPhase: 3,
		incrementBy:    2,
		expectedCalls:  10,
		limit:          108,
	}, {
		name:           "no expected calls - block number returns late after each phase",
		blocksPerRound: 9,
		blocksPerPhase: 3,
		incrementBy:    6,
		expectedCalls:  0,
		limit:          108,
	}, {
		name:           "4 blocks per phase, block number returns every other block",
		blocksPerRound: 12,
		blocksPerPhase: 4,
		incrementBy:    2,
		expectedCalls:  10,
		limit:          144,
	}}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			addr := test.RandomAddress()

			backend := &mockchainBackend{limit: tc.limit, incrementBy: tc.incrementBy, block: tc.blocksPerRound}
			contract := &mockContract{t: t, baseAddr: addr, neighbourhoodSeed: test.RandomAddressAt(addr, 1).Bytes()}

			service := createService(addr, backend, contract, uint64(tc.blocksPerRound), uint64(tc.blocksPerPhase))

			time.Sleep(time.Second)

			if int(contract.commitCalls.Load()) != tc.expectedCalls {
				t.Fatalf("got %d, want %d", contract.commitCalls.Load(), tc.expectedCalls)
			}

			if int(contract.revealCalls.Load()) != tc.expectedCalls {
				t.Fatalf("got %d, want %d", contract.revealCalls.Load(), tc.expectedCalls)
			}

			if int(contract.isWinnerCalls.Load()) != tc.expectedCalls {
				t.Fatalf("got %d, want %d", contract.revealCalls.Load(), tc.expectedCalls)
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
	backend incentives.ChainBackend,
	contract incentives.IncentivesContract,
	blocksPerRound uint64,
	blocksPerPhase uint64) *incentives.Service {

	return incentives.New(
		addr,
		backend,
		log.Noop,
		&mockMonitor{},
		contract,
		mockbatchstore.New(mockbatchstore.WithReserveState(&postage.ReserveState{StorageRadius: 0})),
		&mockSampler{},
		time.Millisecond, 0, blocksPerRound, blocksPerPhase,
	)
}

type mockchainBackend struct {
	incrementBy float64
	block       float64
	limit       float64
}

func (m *mockchainBackend) BlockNumber(context.Context) (uint64, error) {

	ret := uint64(m.block)

	if m.limit == 0 || m.block+m.incrementBy < m.limit {
		m.block += m.incrementBy
	}
	return ret, nil
}

type mockMonitor struct {
}

func (m *mockMonitor) IsStable() bool {
	return true
}

const (
	isWinnerCall = iota
	revealCall
	commitCall
)

type mockContract struct {
	baseAddr      swarm.Address
	commitCalls   atomic.Int32
	revealCalls   atomic.Int32
	isWinnerCalls atomic.Int32

	neighbourhoodSeed []byte

	t            *testing.T
	previousCall int
}

func (m *mockContract) RandomSeedAnchor(context.Context) ([]byte, error) {
	return nil, nil
}

func (m *mockContract) RandomSeedNeighbourhood(context.Context) ([]byte, error) {
	return m.neighbourhoodSeed, nil
}

func (m *mockContract) IsWinner(context.Context) (bool, bool, error) {
	m.isWinnerCalls.Inc()
	if m.previousCall != revealCall {
		m.t.Fatal("previous call must be reveal")
	}
	m.previousCall = isWinnerCall
	return false, false, nil
}

func (m *mockContract) ClaimWin(context.Context) error {
	return nil
}

func (m *mockContract) Commit(context.Context, []byte) error {
	m.commitCalls.Inc()
	m.previousCall = commitCall
	return nil
}

func (m *mockContract) Reveal(context.Context, uint8, []byte, []byte) error {
	m.revealCalls.Inc()
	if m.previousCall != commitCall {
		m.t.Fatal("previous call must be commit")
	}
	m.previousCall = revealCall
	return nil
}

type mockSampler struct {
}

func (m *mockSampler) ReserveSample(context.Context, []byte, uint8) ([]byte, error) {
	return test.RandomAddress().Bytes(), nil
}

// func newChainBackend(opts ...Option) *chainBackend {
// 	mock := &chainBackend{}
// 	for _, o := range opts {
// 		o.apply(mock)
// 	}
// 	return mock
// }

// func Withlol() Option {
// 	return optionFunc(func(s *chainBackend) {
// 		// s.filterLogEvents = events
// 	})
// }

// type Option interface {
// 	apply(*chainBackend)
// }

// type optionFunc func(*chainBackend)

// func (f optionFunc) apply(r *chainBackend) { f(r) }
