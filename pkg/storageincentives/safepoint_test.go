// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives_test

import (
	"math/big"
	"testing"
	"testing/synctest"
	"time"

	statestore "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	si "github.com/ethersphere/bee/v2/pkg/storageincentives"
)

// gnosisBlockTime makes the restart margin 36 blocks. A safe point then needs a
// block position of at most 76-36 = 40 in the round.
const gnosisBlockTime = 5 * time.Second

// at returns the block at position pos of round.
func at(round, pos uint64) uint64 { return round*si.DefaultBlocksPerRound + pos }

// status returns a status as the agent records it. The phase and round are
// written at phase changes, and the block at every phase check.
func status(phase si.PhaseType, round, block uint64) *si.Status {
	st := si.NewStatus()
	st.Phase, st.Round, st.Block = phase, round, block
	st.IsFullySynced, st.IsHealthy = true, true
	st.LastPlayedRound, st.LastSelectedRound, st.LastWonRound = 900, 900, 850
	st.Reward, st.Fees = big.NewInt(0), big.NewInt(0)
	// Data from rounds long past does not matter.
	st.RoundData[899] = si.RoundData{SampleData: &si.SampleData{}}
	st.RoundData[900] = si.RoundData{CommitKey: []byte{1}, HasRevealed: true}
	return st
}

func TestRestartSafePoint(t *testing.T) {
	t.Parallel()

	with := func(st *si.Status, f func(*si.Status)) *si.Status { f(st); return st }

	for _, tc := range []struct {
		name  string
		st    *si.Status
		block uint64 // current block, st.Block when 0
		want  bool
	}{
		{"no state", si.NewStatus(), at(1000, 0), true},
		{"idle, start of commit", status(si.PhaseCommit, 1000, at(1000, 1)), 0, true},
		{"idle, reveal with the margin left", status(si.PhaseReveal, 1000, at(1000, 40)), 0, true},
		{"idle, reveal short of the margin", status(si.PhaseReveal, 1000, at(1000, 41)), 0, false},
		{"idle, last block before claim", status(si.PhaseReveal, 1000, at(1000, 75)), 0, false},
		{"claim", status(si.PhaseClaim, 1000, at(1000, 76)), 0, false},
		{"end of claim", status(si.PhaseClaim, 1000, at(1000, 151)), 0, false},

		// The recorded phase lags behind the block, so the block decides.
		{"stale reveal phase, block in claim", status(si.PhaseReveal, 1000, at(1000, 80)), 0, false},
		{"stale commit phase, block past the margin", status(si.PhaseCommit, 1000, at(1000, 50)), 0, false},
		{"stale claim phase, block in next commit", status(si.PhaseClaim, 1000, at(1001, 2)), 0, true},
		{"block estimated past the recorded one", status(si.PhaseCommit, 1000, at(1000, 10)), at(1000, 77), false},

		{"selected, sampling into commit", with(status(si.PhaseClaim, 1000, at(1001, 3)), func(st *si.Status) {
			st.LastSelectedRound = 1001
		}), 0, false},
		{"selected, sampling in claim", with(status(si.PhaseClaim, 1000, at(1000, 90)), func(st *si.Status) {
			st.LastSelectedRound = 1001
		}), 0, false},
		{"selected without sample, commit window missed", with(status(si.PhaseReveal, 1001, at(1001, 39)), func(st *si.Status) {
			st.LastSelectedRound = 1001
		}), 0, true},
		{"sample ready, commit pending", with(status(si.PhaseCommit, 1001, at(1001, 5)), func(st *si.Status) {
			st.LastSelectedRound = 1001
			st.RoundData[1000] = si.RoundData{SampleData: &si.SampleData{}}
		}), 0, false},
		{"sample ready, selection not recorded", with(status(si.PhaseCommit, 1001, at(1001, 5)), func(st *si.Status) {
			st.RoundData[1000] = si.RoundData{SampleData: &si.SampleData{}}
		}), 0, false},
		{"committed, commit phase", with(status(si.PhaseCommit, 1001, at(1001, 20)), func(st *si.Status) {
			st.LastSelectedRound = 1001
			st.RoundData[1000] = si.RoundData{SampleData: &si.SampleData{}}
			st.RoundData[1001] = si.RoundData{CommitKey: []byte{1}}
		}), 0, false},
		{"committed, reveal phase", with(status(si.PhaseReveal, 1001, at(1001, 38)), func(st *si.Status) {
			st.LastSelectedRound = 1001
			st.RoundData[1000] = si.RoundData{SampleData: &si.SampleData{}}
			st.RoundData[1001] = si.RoundData{CommitKey: []byte{1}, HasRevealed: true}
		}), 0, false},
		{"played round over, next round idle", with(status(si.PhaseCommit, 1002, at(1002, 4)), func(st *si.Status) {
			st.LastSelectedRound, st.LastPlayedRound = 1001, 1001
			st.RoundData[1000] = si.RoundData{SampleData: &si.SampleData{}}
			st.RoundData[1001] = si.RoundData{CommitKey: []byte{1}, HasRevealed: true}
		}), 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			block := tc.block
			if block == 0 {
				block = tc.st.Block
			}
			got, reason := si.RestartSafePoint(tc.st, block, gnosisBlockTime)
			if got != tc.want {
				t.Fatalf("got %v (%q), want %v", got, reason, tc.want)
			}
			if !got && reason == "" {
				t.Fatal("unsafe without a reason")
			}
		})
	}
}

// The margin is a time, so with longer blocks it covers fewer blocks.
func TestRestartSafePointMarginFollowsBlockTime(t *testing.T) {
	t.Parallel()

	st := status(si.PhaseReveal, 1000, at(1000, 55))
	if safe, _ := si.RestartSafePoint(st, st.Block, gnosisBlockTime); safe {
		t.Fatal("safe with 5s blocks, want unsafe")
	}
	// 12s blocks: the margin is 15 blocks.
	if safe, reason := si.RestartSafePoint(st, st.Block, 12*time.Second); !safe {
		t.Fatalf("unsafe with 12s blocks: %s", reason)
	}
	// Very long blocks: the margin is capped at one phase.
	st = status(si.PhaseReveal, 1000, at(1000, 38))
	if safe, reason := si.RestartSafePoint(st, st.Block, time.Hour); !safe {
		t.Fatalf("unsafe with 1h blocks: %s", reason)
	}
}

func TestSafeToRestart(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		store := statestore.NewStateStore()
		a := si.NewSafePointAgent(t, store, gnosisBlockTime)

		if safe, _ := a.SafeToRestart(); safe {
			t.Fatal("safe before any block was observed")
		}

		a.ObserveBlock(at(1000, 2))
		if safe, reason := a.SafeToRestart(); !safe {
			t.Fatalf("unsafe at the start of an idle round: %s", reason)
		}

		// 50s later the round is estimated at block 12, even though no newer
		// height was recorded.
		time.Sleep(50 * time.Second)
		if safe, reason := a.SafeToRestart(); !safe {
			t.Fatalf("unsafe at block 12: %s", reason)
		}

		// After 75s more (block 27), the recorded height is too old to decide
		// on.
		time.Sleep(75 * time.Second)
		if safe, _ := a.SafeToRestart(); safe {
			t.Fatal("safe with a stale block height")
		}

		// A fresh height close to the claim phase is unsafe. The elapsed time
		// then moves the estimate into claim.
		a.ObserveBlock(at(1000, 38))
		if safe, reason := a.SafeToRestart(); !safe {
			t.Fatalf("unsafe at block 38: %s", reason)
		}
		time.Sleep(15 * time.Second)
		if safe, _ := a.SafeToRestart(); safe {
			t.Fatal("safe at estimated block 41")
		}
	})
}
