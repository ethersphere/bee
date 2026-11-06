// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/v2/pkg/storage"
)

const (
	// restartMargin is how long before the claim phase a voluntary restart must
	// begin to count as safe. It has to cover:
	//   - the time for bee to stop;
	//   - the time for systemd, Docker or Kubernetes to start it again,
	//     possibly after fetching a new binary;
	//   - the time for the agent to observe the chain before selection for
	//     the next round is evaluated at the start of the claim phase.
	//
	// It is capped at one phase. This keeps the commit phase of a round the
	// node does not take part in safe, whatever the block time.
	restartMargin = 3 * time.Minute
	// maxBlockAgeChecks is the number of phase checks that may pass without a
	// new recorded block height. After that, the recorded height is too old to
	// decide on.
	maxBlockAgeChecks = 3
)

// phaseCheckPeriod is how often the agent reads the block height.
func phaseCheckPeriod(blockTime time.Duration, blocksPerPhase uint64) time.Duration {
	// optimization: we do not need to check the phase change at every new block
	if blocksPerPhase > 10 {
		return blockTime * 5
	}
	return blockTime
}

// safePointConfig is the round layout used to decide a safe point.
type safePointConfig struct {
	blocksPerRound uint64
	blocksPerPhase uint64
	// marginBlocks is restartMargin expressed in blocks. A safe point needs at
	// least this many blocks left before the claim phase.
	marginBlocks uint64
}

func newSafePointConfig(blockTime time.Duration, blocksPerRound, blocksPerPhase uint64) safePointConfig {
	margin := blocksPerPhase
	if blockTime > 0 {
		margin = min(uint64((restartMargin+blockTime-1)/blockTime), blocksPerPhase)
	}
	return safePointConfig{
		blocksPerRound: blocksPerRound,
		blocksPerPhase: blocksPerPhase,
		marginBlocks:   margin,
	}
}

// SafeToRestart reports whether the node could be stopped now without missing a
// redistribution round it takes part in or may be selected for. If not, it says
// why. It is meant for voluntary restarts, such as a restart to update, that
// can wait for a better moment. It does not block the agent in any way.
//
// The current block is estimated from the last recorded block height and the
// time since it was recorded, because the recorded phase changes only at phase
// boundaries. Without a recent block height, the answer is always unsafe.
func (a *Agent) SafeToRestart() (safe bool, reason string) {
	// Read the observation time before the status. The height is recorded
	// before its time, so the height read is never older than the time.
	at := a.blockObservedAt.Load()
	if at == 0 {
		return false, "no block height observed yet"
	}
	age := time.Since(time.Unix(0, at))
	if maxAge := maxBlockAgeChecks * phaseCheckPeriod(a.blockTime, a.blocksPerPhase); age > maxAge {
		return false, fmt.Sprintf("no block height observed for %s", age.Round(time.Second))
	}

	st, err := a.state.Status()
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return false, fmt.Sprintf("reading redistribution status: %v", err)
		}
		st = NewStatus()
	}

	// The estimate runs ahead of the chain when slots are missed, so the
	// real block is somewhere between the recorded one and the estimate.
	// Every block in that range must be safe.
	block := st.Block
	if a.blockTime > 0 && age > 0 {
		block += uint64(age / a.blockTime)
	}
	return restartSafeRange(st, st.Block, block, newSafePointConfig(a.blockTime, a.blocksPerRound, a.blocksPerPhase))
}

// restartSafeRange reports whether every block from first to last is a safe
// point, with the reason for the first one that is not.
func restartSafeRange(st *Status, first, last uint64, c safePointConfig) (bool, string) {
	for b := first; b <= last; b++ {
		if ok, reason := restartSafePoint(st, b, c); !ok {
			return false, reason
		}
	}
	return true, ""
}

// restartSafePoint uses a recorded status and the current block to decide
// whether the node can restart now. The round and phase come from the block,
// not from the recorded phase, which is updated only when the agent notices a
// phase change.
//
// A round the node plays spans the sample phase, then commit, reveal and claim.
// The sample phase is the claim phase of the previous round and may run into
// the commit phase. The claim phase is always unsafe. Selection for the next
// round is evaluated at its start, and the status cannot tell "not selected"
// from "not evaluated yet". What remains safe is the commit and reveal phases
// of a round the node has not committed to and is not about to commit to, as
// long as at least marginBlocks are left before the claim phase.
func restartSafePoint(st *Status, block uint64, c safePointConfig) (bool, string) {
	round := block / c.blocksPerRound
	pos := block % c.blocksPerRound
	claimStart := 2 * c.blocksPerPhase

	if pos >= claimStart {
		return false, fmt.Sprintf("claim phase of round %d (claiming or sampling for the next round)", round)
	}
	if rd, ok := st.RoundData[round]; ok && rd.CommitKey != nil {
		return false, fmt.Sprintf("committed in round %d", round)
	}
	if st.LastSelectedRound > round {
		return false, fmt.Sprintf("selected for round %d", st.LastSelectedRound)
	}
	// The sample for this round is made in the claim phase of the previous
	// one and may still be in progress. Selection is recorded before
	// sampling starts. Without a sample by the end of the commit phase the
	// node cannot commit, so selection alone only matters until then.
	if pos < c.blocksPerPhase && st.LastSelectedRound == round {
		return false, fmt.Sprintf("selected for round %d, commit pending", round)
	}
	// With a sample, the node may have committed even though no commit key
	// is recorded yet: commit saves it only once the receipt arrives, which
	// can be well into the reveal phase. A restart then would miss the
	// reveal, and the contract freezes the stake. So a round with a sample
	// stays unsafe until the reveal is done.
	if round > 0 {
		if rd, ok := st.RoundData[round-1]; ok && rd.SampleData != nil && !st.RoundData[round].HasRevealed {
			return false, fmt.Sprintf("sample ready in round %d, not revealed", round)
		}
	}
	if left := claimStart - pos; left < c.marginBlocks {
		return false, fmt.Sprintf("%d blocks left before the claim phase of round %d, %d needed", left, round, c.marginBlocks)
	}
	return true, ""
}
