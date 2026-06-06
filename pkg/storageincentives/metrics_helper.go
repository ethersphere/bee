// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ethersphere/bee/v2/pkg/transaction"
)

const (
	actionIsPlaying = "is_playing"
	actionIsWinner  = "is_winner"
	actionCommit    = "commit"
	actionReveal    = "reveal"
	actionClaim     = "claim"
)

func (a *Agent) currentBlockAndOffset(ctx context.Context) (block, offset uint64, err error) {
	a.metrics.BackendCalls.Inc()
	block, err = a.backend.BlockNumber(ctx)
	if err != nil {
		a.metrics.BackendErrors.Inc()
		return 0, 0, err
	}

	a.state.SetCurrentBlock(block)

	return block, block % a.blocksPerRound, nil
}

func (a *Agent) recordActionBlock(action string, block, offset uint64) {
	a.metrics.ActionBlockOffset.WithLabelValues(action).Set(float64(offset))
	a.logger.Debug("contract action", "action", action, "block", block, "block_offset", offset)
}

func (a *Agent) recordPhaseDetected(phase PhaseType, block uint64) {
	a.phaseTimingMu.Lock()
	a.lastPhaseDetectedAt = time.Now()
	a.lastPhaseDetectedPhase = phase
	a.phaseTimingMu.Unlock()

	a.metrics.PhaseDetectedBlock.WithLabelValues(phase.String()).Set(float64(block))
}

func (a *Agent) recordHandlerDelay(phase PhaseType) {
	a.phaseTimingMu.Lock()
	defer a.phaseTimingMu.Unlock()

	if a.lastPhaseDetectedPhase != phase || a.lastPhaseDetectedAt.IsZero() {
		return
	}

	a.metrics.HandlerDelaySeconds.WithLabelValues(phase.String()).Observe(time.Since(a.lastPhaseDetectedAt).Seconds())
}

func (a *Agent) recordHandlerLatency(phase PhaseType, start time.Time) {
	a.metrics.HandlerLatencySeconds.WithLabelValues(phase.String()).Observe(time.Since(start).Seconds())
}

func (a *Agent) recordPhaseSkipped(phase PhaseType, reason string) {
	a.metrics.PhaseSkipped.WithLabelValues(phase.String(), reason).Inc()
	a.logger.Debug("phase skipped", "phase", phase, "reason", reason)
}

func (a *Agent) recordContractError(action string, err error) {
	if err == nil {
		return
	}

	switch {
	case isWrongPhaseError(err):
		a.metrics.WrongPhaseErrors.WithLabelValues(action).Inc()
	case isTxRevertedError(err):
		a.metrics.TxRevertedErrors.WithLabelValues(action).Inc()
	default:
		a.metrics.TxSendErrors.WithLabelValues(action).Inc()
	}
}

func isWrongPhaseError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "WrongPhase") ||
		strings.Contains(msg, "NotCommitPhase") ||
		strings.Contains(msg, "NotRevealPhase") ||
		strings.Contains(msg, "NotClaimPhase") ||
		strings.Contains(msg, "PhaseLastBlock")
}

func isTxRevertedError(err error) bool {
	if errors.Is(err, transaction.ErrTransactionReverted) {
		return true
	}

	return strings.Contains(err.Error(), "execution reverted")
}

// OnTxCompleted implements redistribution.TxObserver.
func (a *Agent) OnTxCompleted(action string, sendTime time.Time, minedBlock uint64, err error) {
	a.metrics.TxConfirmationSeconds.WithLabelValues(action).Observe(time.Since(sendTime).Seconds())

	if minedBlock > 0 {
		offset := minedBlock % a.blocksPerRound
		a.metrics.TxMinedBlockOffset.WithLabelValues(action).Set(float64(offset))
		a.logger.Debug("transaction mined", "action", action, "mined_block", minedBlock, "mined_block_offset", offset)
	}

	if err != nil {
		a.recordContractError(action, err)
	}
}
