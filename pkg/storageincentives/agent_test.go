// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	mockbatchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storageincentives"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"go.uber.org/atomic"
)

func TestAgent(t *testing.T) {
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
		incrementBy:    1,
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
				incrementSleep: time.Millisecond * 10,
				incrementBy:    tc.incrementBy,
				block:          tc.blocksPerRound}
			contract := &mockContract{t: t, baseAddr: addr}

			service := createService(addr, backend, contract, uint64(tc.blocksPerRound), uint64(tc.blocksPerPhase))

			<-wait

			if int(contract.commitCalls.Load()) != tc.expectedCalls {
				t.Fatalf("got %d, want %d", contract.commitCalls.Load(), tc.expectedCalls)
			}

			if int(contract.revealCalls.Load()) != tc.expectedCalls {
				t.Fatalf("got %d, want %d", contract.revealCalls.Load(), tc.expectedCalls)
			}

			if int(contract.isWinnerCalls.Load()) != tc.expectedCalls {
				t.Fatalf("got %d, want %d", contract.isWinnerCalls.Load(), tc.expectedCalls)
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
		time.Millisecond, blocksPerRound, blocksPerPhase,
	)
}

type mockchainBackend struct {
	incrementSleep time.Duration
	incrementBy    float64
	block          float64
	limit          float64
	limitCallback  func()
}

func (m *mockchainBackend) BlockNumber(context.Context) (uint64, error) {

	if m.incrementSleep != 0 {
		time.Sleep(m.incrementSleep)
	}

	ret := uint64(m.block)

	if m.limit == 0 || m.block+m.incrementBy < m.limit {
		m.block += m.incrementBy
	} else if m.limitCallback != nil {
		m.limitCallback()
		return 0, errors.New("reached limit")
	}

	return ret, nil
}

type mockMonitor struct {
}

func (m *mockMonitor) IsFullySynced() bool {
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

	t            *testing.T
	previousCall int
	mtx          sync.Mutex
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
	m.isWinnerCalls.Inc()
	if m.previousCall != revealCall {
		m.t.Fatal("previous call must be reveal")
	}
	m.previousCall = isWinnerCall
	return false, nil
}

func (m *mockContract) Claim(context.Context) error {
	return nil
}

func (m *mockContract) Commit(context.Context, []byte) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.commitCalls.Inc()
	m.previousCall = commitCall
	return nil
}

func (m *mockContract) Reveal(context.Context, uint8, []byte, []byte) error {
	m.t.Helper()
	m.revealCalls.Inc()
	if m.previousCall != commitCall {
		m.t.Fatal("previous call must be commit")
	}
	m.previousCall = revealCall
	return nil
}

type mockSampler struct {
}

func (m *mockSampler) ReserveSample(context.Context, []byte, uint8) (storage.Sample, error) {
	return storage.Sample{
		Hash: test.RandomAddress(),
	}, nil
}
