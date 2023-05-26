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
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/pkg/storageincentives/staking"
	"github.com/ethersphere/bee/pkg/transaction"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/swarm"
)

const loggerName = "storageincentives"

const (
	DefaultBlocksPerRound = 152
	DefaultBlocksPerPhase = DefaultBlocksPerRound / 4

	// min # of transactions our wallet should be able to cover
	minTxCountToCover = 25

	// average tx gas used by transactions issued from agent
	avgTxGas = 250_000
)

type ChainBackend interface {
	BlockNumber(context.Context) (uint64, error)
	HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
	BalanceAt(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error)
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
}

type Monitor interface {
	IsFullySynced() bool
}

type Health interface {
	IsHealthy() bool
}

type SampleData struct {
	ReserveSample storage.Sample
	StorageRadius uint8
}

type Agent struct {
	logger                 log.Logger
	metrics                metrics
	backend                ChainBackend
	blocksPerRound         uint64
	monitor                Monitor
	contract               redistribution.Contract
	batchExpirer           postagecontract.PostageBatchExpirer
	redistributionStatuser staking.RedistributionStatuser
	radius                 postage.Radius
	sampler                storage.Sampler
	overlay                swarm.Address
	quit                   chan struct{}
	wg                     sync.WaitGroup
	state                  *RedistributionState
	commitLock             sync.Mutex
	health                 Health
}

func New(overlay swarm.Address, ethAddress common.Address, backend ChainBackend, monitor Monitor, contract redistribution.Contract, batchExpirer postagecontract.PostageBatchExpirer, redistributionStatuser staking.RedistributionStatuser, radius postage.Radius, sampler storage.Sampler, blockTime time.Duration, blocksPerRound, blocksPerPhase uint64, stateStore storage.StateStorer, erc20Service erc20.Service, tranService transaction.Service, health Health, logger log.Logger) (*Agent, error) {
	a := &Agent{
		overlay:                overlay,
		metrics:                newMetrics(),
		backend:                backend,
		logger:                 logger.WithName(loggerName).Register(),
		contract:               contract,
		batchExpirer:           batchExpirer,
		radius:                 radius,
		monitor:                monitor,
		blocksPerRound:         blocksPerRound,
		sampler:                sampler,
		quit:                   make(chan struct{}),
		redistributionStatuser: redistributionStatuser,
		health:                 health,
	}

	state, err := NewRedistributionState(logger, ethAddress, stateStore, erc20Service, tranService)
	if err != nil {
		return nil, err
	}

	a.state = state

	a.wg.Add(1)
	go a.start(blockTime, a.blocksPerRound, blocksPerPhase)

	return a, nil
}

// start polls the current block number, calculates, and publishes only once the current phase.
// Each round is blocksPerRound long and is divided into three blocksPerPhase long phases: commit, reveal, claim.
// The sample phase is triggered upon entering the claim phase and may run until the end of the commit phase.
// If our neighborhood is selected to participate, a sample is created during the sample phase. In the commit phase,
// the sample is submitted, and in the reveal phase, the obfuscation key from the commit phase is submitted.
// Next, in the claim phase, we check if we've won, and the cycle repeats. The cycle must occur in the length of one round.
func (a *Agent) start(blockTime time.Duration, blocksPerRound, blocksPerPhase uint64) {

	defer a.wg.Done()

	phaseEvents := newEvents()
	defer phaseEvents.Close()

	logPhaseResult := func(phase PhaseType, round uint64, err error, isPhasePlayed bool) {
		if err != nil {
			a.logger.Error(err, "phase failed", "phase", phase, "round", round)
		} else if isPhasePlayed {
			a.logger.Info("phase played", "phase", phase, "round", round)
		} else {
			a.logger.Info("phase skipped", "phase", phase, "round", round)
		}
	}

	phaseEvents.On(commit, func(ctx context.Context) {
		phaseEvents.Cancel(claim)

		round, _ := a.state.currentRoundAndPhase()
		isPhasePlayed, err := a.handleCommit(ctx, round)
		logPhaseResult(commit, round, err, isPhasePlayed)
	})

	phaseEvents.On(reveal, func(ctx context.Context) {
		phaseEvents.Cancel(commit, sample)

		round, _ := a.state.currentRoundAndPhase()
		isPhasePlayed, err := a.handleReveal(ctx, round)
		logPhaseResult(reveal, round, err, isPhasePlayed)
	})

	phaseEvents.On(claim, func(ctx context.Context) {
		phaseEvents.Cancel(reveal)
		phaseEvents.Publish(sample)

		round, _ := a.state.currentRoundAndPhase()
		isPhasePlayed, err := a.handleClaim(ctx, round)
		logPhaseResult(claim, round, err, isPhasePlayed)
	})

	phaseEvents.On(sample, func(ctx context.Context) {
		round, _ := a.state.currentRoundAndPhase()
		isPhasePlayed, err := a.handleSample(ctx, round)
		logPhaseResult(sample, round, err, isPhasePlayed)

		// Sample handled could potentially take long time, therefore it could overlap with commit
		// phase of next round. When that case happens commit event needs to be triggered once more
		// in order to handle commit phase with delay.
		currentRound, currentPhase := a.state.currentRoundAndPhase()
		if isPhasePlayed &&
			currentPhase == commit &&
			currentRound-1 == round {
			phaseEvents.Publish(commit)
		}
	})

	var (
		prevPhase    PhaseType = -1
		currentPhase PhaseType
	)

	phaseCheck := func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, blockTime*time.Duration(blocksPerRound))
		defer cancel()

		a.metrics.BackendCalls.Inc()
		block, err := a.backend.BlockNumber(ctx)
		if err != nil {
			a.metrics.BackendErrors.Inc()
			a.logger.Error(err, "getting block number")
			return
		}

		round := block / blocksPerRound

		a.metrics.Round.Set(float64(round))

		p := block % blocksPerRound
		if p < blocksPerPhase {
			currentPhase = commit // [0, 37]
		} else if p >= blocksPerPhase && p < 2*blocksPerPhase { // [38, 75]
			currentPhase = reveal
		} else if p >= 2*blocksPerPhase {
			currentPhase = claim // [76, 151]
		}

		// write the current phase only once
		if currentPhase == prevPhase {
			return
		}

		prevPhase = currentPhase
		a.metrics.CurrentPhase.Set(float64(currentPhase))

		a.logger.Info("entered new phase", "phase", currentPhase.String(), "round", round, "block", block)

		a.state.SetCurrentEvent(currentPhase, round, block)
		a.state.SetFullySynced(a.monitor.IsFullySynced())
		go a.state.purgeStaleRoundData()

		isFrozen, err := a.redistributionStatuser.IsOverlayFrozen(ctx, block)
		if err != nil {
			a.logger.Error(err, "error checking if stake is frozen")
		} else {
			a.state.SetFrozen(isFrozen, round)
		}

		phaseEvents.Publish(currentPhase)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-a.quit
		cancel()
	}()

	// manually invoke phaseCheck initially in order to set initial data asap
	phaseCheck(ctx)

	phaseCheckInterval := blockTime
	// optimization, we do not need to check the phase change at every new block
	if blocksPerPhase > 10 {
		phaseCheckInterval = blockTime * 5
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(phaseCheckInterval):
			phaseCheck(ctx)
		}
	}
}

func (a *Agent) handleCommit(ctx context.Context, round uint64) (bool, error) {
	// commit event handler has to be guarded with lock to avoid
	// race conditions when handler is triggered again from sample phase
	a.commitLock.Lock()
	defer a.commitLock.Unlock()

	if _, exists := a.state.CommitKey(round); exists {
		// already committed on this round, phase is skipped
		return false, nil
	}

	// the sample has to come from previous round to be able to commit it
	sample, exists := a.state.SampleData(round - 1)
	if !exists {
		// In absence of sample, phase is skipped
		return false, nil
	}

	err := a.commit(ctx, sample, round)
	if err != nil {
		return false, err
	}

	a.state.SetLastPlayedRound(round)

	return true, nil
}

func (a *Agent) handleReveal(ctx context.Context, round uint64) (bool, error) {
	// reveal requires the commitKey from the same round
	commitKey, exists := a.state.CommitKey(round)
	if !exists {
		// In absence of commitKey, phase is skipped
		return false, nil
	}

	// reveal requires sample from previous round
	sample, exists := a.state.SampleData(round - 1)
	if !exists {
		// Sample must have been saved so far
		return false, fmt.Errorf("sample not found in reveal phase")
	}

	a.metrics.RevealPhase.Inc()
	sampleBytes := sample.ReserveSample.Hash.Bytes()
	txHash, err := a.contract.Reveal(ctx, sample.StorageRadius, sampleBytes, commitKey)
	if err != nil {
		a.metrics.ErrReveal.Inc()
		return false, err
	}
	a.state.AddFee(ctx, txHash)

	a.state.SetHasRevealed(round)

	return true, nil
}

func (a *Agent) handleClaim(ctx context.Context, round uint64) (bool, error) {
	hasRevealed := a.state.HasRevealed(round)
	if !hasRevealed {
		// When there was no reveal in same round, phase is skipped
		return false, nil
	}

	a.metrics.ClaimPhase.Inc()

	isWinner, err := a.contract.IsWinner(ctx)
	if err != nil {
		a.metrics.ErrWinner.Inc()
		return false, err
	}

	if !isWinner {
		a.logger.Info("not a winner")
		// When there is nothing to claim (node is not a winner), phase is played
		return true, nil
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

	txHash, err := a.contract.Claim(ctx)
	if err != nil {
		a.metrics.ErrClaim.Inc()
		return false, fmt.Errorf("claiming win: %w", err)
	}

	a.logger.Info("claimed win")

	if errBalance == nil {
		errReward := a.state.CalculateWinnerReward(ctx)
		if errReward != nil {
			a.logger.Info("calculate winner reward", "err", err)
		}
	}

	a.state.AddFee(ctx, txHash)

	return true, nil
}

func (a *Agent) handleSample(ctx context.Context, round uint64) (bool, error) {

	a.state.SetLastSelectedRound(round)

	if !a.state.IsFullySynced() {
		a.logger.Info("skipping round because node is not fully synced")
		return false, nil
	}

	if a.state.IsFrozen() {
		a.logger.Info("skipping round because node is frozen")
		return false, nil
	}

	if !a.health.IsHealthy() {
		a.logger.Info("skipping round because node is unhealhy", "round", round)
		return false, nil
	}

	storageRadius := a.radius.StorageRadius()

	isPlaying, err := a.contract.IsPlaying(ctx, storageRadius)
	if err != nil {
		a.metrics.ErrCheckIsPlaying.Inc()
		return false, err
	}
	if !isPlaying {
		a.logger.Info("not playing in this round")
		return false, nil
	}

	_, hasFunds, err := a.HasEnoughFundsToPlay(ctx)
	if err != nil {
		return false, fmt.Errorf("has enough funds to play: %w", err)
	}

	if !hasFunds {
		a.logger.Info("insufficient funds to participate in next round", "round", round)
		a.metrics.InsufficientFundsToPlay.Inc()
		return false, nil
	}

	a.logger.Info("neighbourhood chosen", "round", round)
	a.metrics.NeighborhoodSelected.Inc()

	sample, err := a.makeSample(ctx, storageRadius)
	if err != nil {
		return false, err
	}

	a.state.SetSampleData(round, sample)

	return true, nil
}

func (a *Agent) makeSample(ctx context.Context, storageRadius uint8) (SampleData, error) {
	salt, err := a.contract.ReserveSalt(ctx)
	if err != nil {
		return SampleData{}, err
	}

	timeLimiter, err := a.getPreviousRoundTime(ctx)
	if err != nil {
		return SampleData{}, err
	}

	t := time.Now()
	rSample, err := a.sampler.ReserveSample(ctx, salt, storageRadius, uint64(timeLimiter))
	if err != nil {
		return SampleData{}, err
	}
	a.metrics.SampleDuration.Set(time.Since(t).Seconds())

	sample := SampleData{
		ReserveSample: rSample,
		StorageRadius: storageRadius,
	}

	return sample, nil
}

func (a *Agent) getPreviousRoundTime(ctx context.Context) (time.Duration, error) {

	a.metrics.BackendCalls.Inc()
	block, err := a.backend.BlockNumber(ctx)
	if err != nil {
		a.metrics.BackendErrors.Inc()
		return 0, err
	}

	previousRoundBlockNumber := ((block / a.blocksPerRound) - 1) * a.blocksPerRound

	a.metrics.BackendCalls.Inc()
	timeLimiterBlock, err := a.backend.HeaderByNumber(ctx, new(big.Int).SetUint64(previousRoundBlockNumber))
	if err != nil {
		a.metrics.BackendErrors.Inc()
		return 0, err
	}

	return time.Duration(timeLimiterBlock.Time) * time.Second / time.Nanosecond, nil
}

func (a *Agent) commit(ctx context.Context, sample SampleData, round uint64) error {
	a.metrics.CommitPhase.Inc()

	key := make([]byte, swarm.HashSize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return err
	}

	sampleBytes := sample.ReserveSample.Hash.Bytes()
	obfuscatedHash, err := a.wrapCommit(sample.StorageRadius, sampleBytes, key)
	if err != nil {
		return err
	}

	txHash, err := a.contract.Commit(ctx, obfuscatedHash, big.NewInt(int64(round)))
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

	stopped := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(stopped)
	}()

	select {
	case <-stopped:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("stopping incentives with ongoing worker goroutine")
	}
}

func (a *Agent) wrapCommit(storageRadius uint8, sample []byte, key []byte) ([]byte, error) {

	storageRadiusByte := []byte{storageRadius}

	data := append(a.overlay.Bytes(), storageRadiusByte...)
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
