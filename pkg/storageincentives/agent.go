// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/storageincentives/sampler"
	"github.com/ethersphere/bee/pkg/storageincentives/staking"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/transaction"
)

const loggerName = "storageincentives"

const (
	blocksPerRound = 152
	blocksPerPhase = blocksPerRound / 4
	// min # of transactions our wallet should be able to cover
	minTxCountToCover = 15

	// average tx gas used by transactions issued from agent
	avgTxGas = 250_000
)

type Agent struct {
	logger                 log.Logger
	metrics                metrics
	overlay                swarm.Address
	handlers               []func(context.Context, uint64) error
	backend                postage.ChainBackend
	contract               redistribution.Contract
	batchExpirer           postagecontract.PostageBatchExpirer
	redistributionStatuser staking.RedistributionStatuser
	sampler                *sampler.Sampler
	quit                   chan struct{}
	stopped                chan struct{}
	state                  *RedistributionState
}

func handlers(a *Agent) []func(context.Context, uint64) error {
	return []func(context.Context, uint64) error{
		a.handleCommit,
		a.handleReveal,
		a.handleClaim,
		a.handleSample,
	}
}

// handlersFunc is made available in tests
var handlersFunc func(a *Agent) []func(context.Context, uint64) error = handlers

func New(
	logger log.Logger,
	overlay swarm.Address,
	ethAddress common.Address,
	blockTime time.Duration,
	blockbackend postage.ChainBackend,
	contract redistribution.Contract,
	batchExpirer postagecontract.PostageBatchExpirer,
	redistributionStatuser staking.RedistributionStatuser,
	s *sampler.Sampler,
	stateStore storage.StateStorer,
	erc20Service erc20.Service,
	tranService transaction.Service,
) (*Agent, error) {
	a := &Agent{
		logger:                 logger.WithName(loggerName).Register(),
		metrics:                newMetrics(),
		overlay:                overlay,
		backend:                blockbackend,
		sampler:                s,
		contract:               contract,
		batchExpirer:           batchExpirer,
		redistributionStatuser: redistributionStatuser,
		quit:                   make(chan struct{}),
		stopped:                make(chan struct{}),
	}

	a.handlers = handlersFunc(a)

	state, err := NewRedistributionState(logger, ethAddress, stateStore, erc20Service, tranService)
	if err != nil {
		return nil, err
	}

	a.state = state

	go a.start(blockTime)

	return a, nil
}

type phaseAndBlock struct {
	phase PhaseType
	block uint64
}

// start polls the current block number, calculates, and publishes only once the current phase.
// Each round is blocksPerRound long and is divided into three blocksPerPhase long phases: commit, reveal, claim.
// The sample phase is triggered upon entering the claim phase and may run until the end of the commit phase.
// If our neighborhood is selected to participate, a sample is created during the sample phase. In the commit phase,
// the sample is submitted, and in the reveal phase, the obfuscation key from the commit phase is submitted.
// Next, in the claim phase, we check if we've won, and the cycle repeats. The cycle must occur in the length of one round.
func (a *Agent) start(blockTime time.Duration) {
	defer close(a.stopped)
	phaseEvents := NewEvents()
	defer phaseEvents.Close()

	phaseEvents.On(commit, func(quit chan struct{}, blockNumber uint64) {
		phaseEvents.Cancel(claim)
		a.handlePhase(commit, quit, blockNumber)
	})

	phaseEvents.On(reveal, func(quit chan struct{}, blockNumber uint64) {
		phaseEvents.Cancel(commit, sample)
		a.handlePhase(reveal, quit, blockNumber)
	})

	phaseEvents.On(claim, func(quit chan struct{}, blockNumber uint64) {
		phaseEvents.Cancel(reveal)
		phaseEvents.Publish(sample, blockNumber)
		a.handlePhase(claim, quit, blockNumber)
	})

	phaseEvents.On(sample, func(quit chan struct{}, blockNumber uint64) {
		a.handlePhase(sample, quit, blockNumber)
	})
	phaseC := make(chan phaseAndBlock)
	phaseCheckInterval := blockTime
	// optimization, we do not need to check the phase change at every new block
	if blocksPerPhase > 10 {
		phaseCheckInterval = blockTime * 5
	}
	ticker := time.NewTicker(phaseCheckInterval)
	defer ticker.Stop()
	var phase PhaseType
	var block uint64
	ctx, cancel := context.WithCancel(context.Background())
	for {
		select {
		case pr := <-phaseC:
			phase, block = pr.phase, pr.block
			phaseEvents.Publish(phase, block)
			round := block / blocksPerRound

			go a.metrics.CurrentPhase.Set(float64(phase))
			a.metrics.Round.Set(float64(round))

			a.logger.Info("entered new phase", "phase", phase.String(), "round", round, "block", block)
			a.state.SetCurrentBlock(block)
			a.state.SetCurrentEvent(phase, round)
			// a.state.SetFullySynced(a.fullSyncedFunc())
			// a.state.SetHealthy(a.healthyFunc())
			go a.state.purgeStaleRoundData()

		case <-ticker.C:
			cancel()
			ctx, cancel = context.WithCancel(context.Background())
			go func() {
				defer cancel()
				a.getPhase(ctx, phase, phaseC)
			}()

		case <-a.quit:
			return
		}
	}
}

func currentPhase(ctx context.Context, phase PhaseType, block uint64) (changed bool, newphase PhaseType) {
	rem := block % blocksPerRound
	newphase = phase
	switch {
	case rem < blocksPerPhase: // [0, 37]
		newphase = commit
	case rem >= blocksPerPhase && rem < 2*blocksPerPhase: // [38, 75]
		newphase = reveal
	default:
		newphase = claim // [76, 151]
	}
	return newphase == phase, newphase
}

// getPhase feeds the phase to the channel if changed
func (a *Agent) getPhase(ctx context.Context, phase PhaseType, phaseC chan phaseAndBlock) {
	a.metrics.BackendCalls.Inc()
	block, err := a.backend.BlockNumber(ctx)
	if err != nil {
		a.metrics.BackendErrors.Inc()
		a.logger.Error(err, "getting block number from chain backend")
		return
	}

	changed, phase := currentPhase(ctx, phase, block)
	if !changed {
		return
	}
	select {
	case phaseC <- phaseAndBlock{phase, block}:
	case <-ctx.Done():
	}
}

func (a *Agent) handlePhase(phase PhaseType, quit chan struct{}, blockNumber uint64) {
	round := blockNumber / blocksPerRound

	handler := a.handlers[phase]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-quit:
			cancel()
		case <-ctx.Done():
		}
	}()
	err := handler(ctx, round)
	if err != nil {
		a.logger.Error(err, "phase failed", "phase", phase, "round", round)
	}
}

func (a *Agent) handleCommit(ctx context.Context, round uint64) error {

	if _, exists := a.state.CommitKey(round); exists {
		// already committed on this round, phase is skipped
		return nil
	}

	// the sample has to come from previous round to be able to commit it
	sample, exists := a.state.Data(round - 1)
	if !exists {
		// In absence of sample, phase is skipped
		return nil
	}

	err := a.commit(ctx, sample, round)
	if err != nil {
		return err
	}

	a.state.SetLastPlayedRound(round)

	return nil
}

func (a *Agent) handleReveal(ctx context.Context, round uint64) error {
	// reveal requires the commitKey from the same round
	commitKey, exists := a.state.CommitKey(round)
	if !exists {
		// In absence of commitKey, phase is skipped
		return nil
	}

	// reveal requires sample from previous round
	sample, exists := a.state.Data(round - 1)
	if !exists {
		// Sample must have been saved so far
		return fmt.Errorf("sample not found in reveal phase")
	}

	a.metrics.RevealPhase.Inc()

	rsh := sample.Hash.Bytes()
	txHash, err := a.contract.Reveal(ctx, sample.Depth, rsh, commitKey)
	if err != nil {
		a.metrics.ErrReveal.Inc()
		return err
	}
	a.state.AddFee(ctx, txHash)

	a.state.SetHasRevealed(round)

	return nil
}

func (a *Agent) handleClaim(ctx context.Context, round uint64) error {
	hasRevealed := a.state.HasRevealed(round)
	if !hasRevealed {
		// When there was no reveal in same round, phase is skipped
		return nil
	}

	a.metrics.ClaimPhase.Inc()

	isWinner, err := a.contract.IsWinner(ctx)
	if err != nil {
		a.metrics.ErrWinner.Inc()
		return err
	}

	if !isWinner {
		a.logger.Info("not a winner")
		// When there is nothing to claim (node is not a winner), phase is played
		return nil
	}

	a.state.SetLastWonRound(round)
	a.metrics.Winner.Inc()

	// In case when there are too many expired batches, Claim trx could runs out of gas.
	// To prevent this, node should first expire batches before Claiming a reward.
	err = a.batchExpirer.ExpireBatches(ctx)
	if err != nil {
		a.logger.Info("expire batches failed", "err", err)
		// Even when error happens, proceed with claim handler
		// because this should not prevent node from claiming a reward
	}

	errBalance := a.state.SetBalance(ctx)
	if errBalance != nil {
		a.logger.Info("could not set balance", "err", err)
	}

	sampleData, exists := a.state.Data(round - 1)
	if !exists {
		return fmt.Errorf("sample not found")
	}

	anchor, err := a.contract.ReserveSalt(ctx)
	if err != nil {
		a.logger.Info("failed getting anchor after second reveal", "err", err)
	}

	proofs, err := sampler.MakeProofs(sampleData, anchor)

	if err != nil {
		return fmt.Errorf("making inclusion proofs: %w", err)
	}

	txHash, err := a.contract.Claim(ctx, proofs)
	if err != nil {
		a.metrics.ErrClaim.Inc()
		return fmt.Errorf("claiming win: %w", err)
	}

	a.logger.Info("claimed win")

	if errBalance == nil {
		errReward := a.state.CalculateWinnerReward(ctx)
		if errReward != nil {
			a.logger.Info("calculate winner reward", "err", err)
		}
	}

	a.state.AddFee(ctx, txHash)

	return nil
}

func (a *Agent) isPlaying(ctx context.Context, round uint64) (bool, uint8, error) {
	isFrozen, err := a.redistributionStatuser.IsOverlayFrozen(ctx, round*blocksPerRound)
	if err != nil {
		a.logger.Error(err, "error checking if stake is frozen")
	} else {
		a.state.SetFrozen(isFrozen, round)
	}

	if a.state.IsFrozen() {
		a.logger.Info("skipping round because node is frozen")
		return false, 0, nil
	}

	depth := a.sampler.StorageRadius()
	isPlaying, err := a.contract.IsPlaying(ctx, depth)
	if err != nil {
		a.metrics.ErrCheckIsPlaying.Inc()
		return false, 0, err
	}
	if !isPlaying {
		a.logger.Info("not playing in this round")
		return false, 0, nil
	}
	a.state.SetLastSelectedRound(round + 1)
	a.metrics.NeighborhoodSelected.Inc()
	a.logger.Info("neighbourhood chosen", "round", round)

	if !a.state.IsFullySynced() {
		a.logger.Info("skipping round because node is not fully synced")
		return false, 0, nil
	}

	if !a.state.IsHealthy() {
		a.logger.Info("skipping round because node is unhealhy", "round", round)
		return false, 0, nil
	}

	_, hasFunds, err := a.HasEnoughFundsToPlay(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("has enough funds to play: %w", err)
	}
	if !hasFunds {
		a.logger.Info("insufficient funds to play in next round", "round", round)
		a.metrics.InsufficientFundsToPlay.Inc()
		return false, 0, nil
	}
	return true, depth, nil
}

func (a *Agent) handleSample(ctx context.Context, round uint64) error {
	isPlaying, depth, err := a.isPlaying(ctx, round)
	if !isPlaying {
		return nil
	}

	salt, err := a.contract.ReserveSalt(ctx)
	if err != nil {
		return err
	}

	sample, err := a.sampler.MakeSample(ctx, salt, depth, round)
	if err != nil {
		return err
	}

	a.logger.Info("produced sample", "hash", sample.Hash, "depth", sample.Depth, "round", round)

	a.state.SetData(round, sample)

	return nil
}

func (a *Agent) commit(ctx context.Context, sample sampler.Data, round uint64) error {
	a.metrics.CommitPhase.Inc()

	key := make([]byte, swarm.HashSize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return err
	}

	obfuscatedHash, err := a.wrapCommit(sample.Depth, sample.Hash.Bytes(), key)
	if err != nil {
		return err
	}

	txHash, err := a.contract.Commit(ctx, obfuscatedHash, round)
	if err != nil {
		a.metrics.ErrCommit.Inc()
		return err
	}
	a.state.AddFee(ctx, txHash)

	a.state.SetCommitKey(round, key)

	return nil
}

func (a *Agent) Close() error {
	close(a.quit)

	select {
	case <-a.stopped:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("stopping incentives with ongoing worker goroutine")
	}
}

func (a *Agent) wrapCommit(depth uint8, sample []byte, key []byte) ([]byte, error) {
	data := append([]byte{}, a.overlay.Bytes()...)
	data = append(data, depth)
	data = append(data, sample...)
	data = append(data, key...)

	return crypto.LegacyKeccak256(data)
}

// Status returns the node status
func (a *Agent) Status() (*Status, error) {
	return a.state.Status()
}

func (a *Agent) HasEnoughFundsToPlay(ctx context.Context) (*big.Int, bool, error) {
	balance, err := a.backend.BalanceAt(ctx, a.state.ethAddress, nil)
	if err != nil {
		return nil, false, err
	}

	price, err := a.backend.SuggestGasPrice(ctx)
	if err != nil {
		return nil, false, err
	}

	avgTxFee := new(big.Int).Mul(big.NewInt(avgTxGas), price)
	minBalance := new(big.Int).Mul(avgTxFee, big.NewInt(minTxCountToCover))
	return minBalance, balance.Cmp(minBalance) >= 1, nil
}
