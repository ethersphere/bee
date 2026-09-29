// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

var (
	NewEvents           = newEvents
	SampleChunk         = sampleChunk
	MakeInclusionProofs = makeInclusionProofs

	PhaseCommit = commit
	PhaseReveal = reveal
	PhaseClaim  = claim
)

// RestartSafePoint runs restartSafePoint using the round layout of an agent
// that has the given block time and the default round length.
func RestartSafePoint(st *Status, block uint64, blockTime time.Duration) (bool, string) {
	return restartSafePoint(st, block, newSafePointConfig(blockTime, DefaultBlocksPerRound, DefaultBlocksPerPhase))
}

// NewSafePointAgent returns an agent that is not started. It reads its
// redistribution status from store and is meant for testing SafeToRestart.
func NewSafePointAgent(tb testing.TB, store storage.StateStorer, blockTime time.Duration) *Agent {
	tb.Helper()
	state, err := NewRedistributionState(log.Noop, common.Address{}, store, nil, nil)
	if err != nil {
		tb.Fatal(err)
	}
	return &Agent{
		state:          state,
		blockTime:      blockTime,
		blocksPerRound: DefaultBlocksPerRound,
		blocksPerPhase: DefaultBlocksPerPhase,
	}
}

// ObserveBlock records block as the current block height, the same way the
// agent does on each phase check.
func (a *Agent) ObserveBlock(block uint64) {
	a.state.SetCurrentBlock(block)
	a.blockObservedAt.Store(time.Now().UnixNano())
}
