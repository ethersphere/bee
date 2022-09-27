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

			wait := make(chan struct{})
			addr := test.RandomAddress()

			backend := &mockchainBackend{
				limit:         tc.limit,
				limitCallback: func() { wait <- struct{}{} },
				incrementBy:   tc.incrementBy,
				block:         tc.blocksPerRound}
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
				t.Fatalf("got %d, want %d", contract.revealCalls.Load(), tc.expectedCalls)
			}

			err := service.Close()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWrapCommit(t *testing.T) {

	t.Parallel()

	storageRadius := uint8(0)
	sample := swarm.MustParseHexAddress("8151150019b8c589a8d75df5c6c9c8125e7599fcb75c1c12024c26d50bbd652d")
	overlay := swarm.MustParseHexAddress("954b30794124057409fc3807a1f595fcd9ed94ab80a885f6d67a34b1b42e8fcd")
	obfuscator := swarm.MustParseHexAddress("e3c8da81085b6d375174f395c6f477954758a181cfb048e67ff8869803a8d7a7")
	wantSum := swarm.MustParseHexAddress("23195b391679a7808a7c649e68c48fe6c90d0d6d61f208c74e6cc40bf6fddca7")
	gotSumBytes, err := incentives.WrapCommit(storageRadius, sample.Bytes(), overlay.Bytes(), obfuscator.Bytes())

	if err != nil {
		t.Fatal(err)
	}

	if !wantSum.Equal(swarm.NewAddress(gotSumBytes)) {
		t.Fatal("sum mismatch")
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

	t            *testing.T
	previousCall int
}

func (m *mockContract) ReserveSalt(context.Context) ([]byte, error) {
	return nil, nil
}

func (m *mockContract) IsPlaying(context.Context, uint8) (bool, error) {
	return true, nil
}

func (m *mockContract) IsWinner(context.Context) (bool, bool, error) {
	m.isWinnerCalls.Inc()
	if m.previousCall != revealCall {
		m.t.Fatal("previous call must be reveal")
	}
	m.previousCall = isWinnerCall
	return false, false, nil
}

func (m *mockContract) Claim(context.Context) error {
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
