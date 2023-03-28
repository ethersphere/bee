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
	"sync/atomic"
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

type sampleData struct {
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
	radius                 postage.RadiusChecker
	sampler                storage.Sampler
	overlay                swarm.Address
	quit                   chan struct{}
	wg                     sync.WaitGroup
	state                  *RedistributionState
}

func New(overlay swarm.Address, ethAddress common.Address, backend ChainBackend, logger log.Logger, monitor Monitor, contract redistribution.Contract, batchExpirer postagecontract.PostageBatchExpirer, redistributionStatuser staking.RedistributionStatuser, radius postage.RadiusChecker, sampler storage.Sampler, blockTime time.Duration, blocksPerRound, blocksPerPhase uint64, stateStore storage.StateStorer, erc20Service erc20.Service, tranService transaction.Service) (*Agent, error) {
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

	var (
		currRound   atomic.Uint64
		phaseEvents = newEvents()
	)
	// cancel all possible running phases
	defer phaseEvents.Close()

	commitF := func(ctx context.Context) {
		phaseEvents.Cancel(claim)

		round := currRound.Load()

		// the sample has to come from previous round to be able to commit it
		sample, err := getSample(a.state.stateStore, round-1)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				// In absence of sample, phase is skipped
				return
			}

			a.logger.Error(err, "getSample for commit phase")
			return
		}

		err = a.commit(ctx, sample, round)
		if err != nil {
			a.logger.Error(err, "commit")
		} else {
			a.logger.Debug("committed the reserve sample and radius")
		}
	}

	// when the sample finishes, if we are in the commit phase, run commit
	phaseEvents.On(sampleEnd, func(ctx context.Context, previous PhaseType) {
		if previous == commit {
			commitF(ctx)
		}
	})

	// when we enter the commit phase, if the sample is already finished, run commit
	phaseEvents.On(commit, func(ctx context.Context, previous PhaseType) {
		if previous == sampleEnd {
			commitF(ctx)
		}
	})

	phaseEvents.On(reveal, func(ctx context.Context, _ PhaseType) {
		// cancel previous executions of the commit and sample phases
		phaseEvents.Cancel(commit, sample, sampleEnd)

		round := currRound.Load()

		// reveal requires the commitKey from the same round
		commitKey, err := getCommitKey(a.state.stateStore, round)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				// In absence of commitKey, phase is skipped
				return
			}

			a.logger.Error(err, "getCommitKey for reveal phase")
			return
		}

		// reveal requires sample from previous round
		sample, err := getSample(a.state.stateStore, round-1)
		if err != nil {
			// Sample must have been saved so far
			a.logger.Error(err, "getSample for reveal phase")
			return
		}

		err = a.reveal(ctx, sample, commitKey, round)
		if err != nil {
			a.logger.Error(err, "reveal")
		} else {
			a.logger.Debug("revealed the sample with the obfuscation key")
		}
	})

	phaseEvents.On(claim, func(ctx context.Context, _ PhaseType) {
		phaseEvents.Cancel(reveal)

		round := currRound.Load()

		err := getRevealRound(a.state.stateStore, round)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				// In absence of reval round, phase is skipped
				return
			}

			a.logger.Error(err, "getRevealRound for claim phase")
			return
		}

		if err := a.claim(ctx, round); err != nil {
			a.logger.Error(err, "claim")
		}
	})

	phaseEvents.On(sample, func(ctx context.Context, _ PhaseType) {
		round := currRound.Load()

		shouldPlayRound, err := a.play(ctx, round)
		if err != nil {
			a.logger.Error(err, "make sample")
		} else if shouldPlayRound {
			a.logger.Info("produced reserve sample", "round", round)
		}

		phaseEvents.Publish(sampleEnd)
	})

	var (
		prevPhase    PhaseType = -1
		currentPhase PhaseType
		checkEvery   uint64 = 1
	)

	// optimization, we do not need to check the phase change at every new block
	if blocksPerPhase > 10 {
		checkEvery = 5
	}

	for {
		select {
		case <-a.quit:
			return
		case <-time.After(blockTime * time.Duration(checkEvery)):
		}

		ctx, cancel := context.WithTimeout(context.Background(), blockTime*time.Duration(blocksPerRound))

		a.metrics.BackendCalls.Inc()
		block, err := a.backend.BlockNumber(context.Background())
		if err != nil {
			a.metrics.BackendErrors.Inc()
			a.logger.Error(err, "getting block number")
			cancel()
			continue
		}

		round := block / blocksPerRound
		currRound.Store(round)

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
		if currentPhase != prevPhase {

			a.metrics.CurrentPhase.Set(float64(currentPhase))

			a.logger.Info("entered new phase", "phase", currentPhase.String(), "round", round, "block", block)

			a.state.SetCurrentEvent(currentPhase, round, block)
			a.state.IsFullySynced(a.monitor.IsFullySynced())

			isFrozen, err := a.redistributionStatuser.IsOverlayFrozen(ctx, block)
			if err != nil {
				a.logger.Error(err, "error checking if stake is frozen")
			} else {
				a.state.SetFrozen(isFrozen, round)
			}

			phaseEvents.Publish(currentPhase)
			if currentPhase == claim {
				phaseEvents.Publish(sample) // trigger sample along side the claim phase
			}
		}
		prevPhase = currentPhase

		cancel()
	}
}

func (a *Agent) reveal(ctx context.Context, sample sampleData, obfuscationKey []byte, round uint64) error {
	a.metrics.RevealPhase.Inc()
	sampleBytes := sample.ReserveSample.Hash.Bytes()
	txHash, err := a.contract.Reveal(ctx, sample.StorageRadius, sampleBytes, obfuscationKey)
	if err != nil {
		a.metrics.ErrReveal.Inc()
		return err
	}
	a.state.AddFee(ctx, txHash)

	if err := saveRevealRound(a.state.stateStore, round); err != nil {
		return fmt.Errorf("failed to save reveal round: %w", err)
	}

	return nil
}

func (a *Agent) claim(ctx context.Context, round uint64) error {
	a.metrics.ClaimPhase.Inc()
	// event claimPhase was processed

	err := a.batchExpirer.ExpireBatches(ctx)
	if err != nil {
		return err
	}

	isWinner, err := a.contract.IsWinner(ctx)
	if err != nil {
		a.metrics.ErrWinner.Inc()
		return err
	}

	if isWinner {
		a.state.SetLastWonRound(round)
		a.metrics.Winner.Inc()
		errBalance := a.state.SetBalance(ctx)
		if errBalance != nil {
			a.logger.Info("could not set balance", "err", err)
		}

		txHash, err := a.contract.Claim(ctx)
		if err != nil {
			a.metrics.ErrClaim.Inc()
			a.logger.Info("error claiming win", "err", err)
			return fmt.Errorf("error claiming win: %w", err)
		}
		a.logger.Info("claimed win")
		if errBalance == nil {
			errReward := a.state.CalculateWinnerReward(ctx)
			if errReward != nil {
				a.logger.Info("calculate winner reward", "err", err)
			}
		}
		a.state.AddFee(ctx, txHash)

	} else {
		a.logger.Info("claim made, lost round")
	}

	return nil
}

func (a *Agent) play(ctx context.Context, round uint64) (bool, error) {
	status, err := a.state.Status()
	if err != nil {
		return false, err
	}

	if !status.IsFullySynced {
		a.logger.Info("skipping round because node is not fully synced", "round", round)
		return false, nil
	}

	if status.IsFrozen {
		a.logger.Info("skipping round because node is frozen", "round", round)
		return false, nil
	}

	storageRadius := a.radius.StorageRadius()

	isPlaying, err := a.contract.IsPlaying(ctx, storageRadius)
	if err != nil {
		a.metrics.ErrCheckIsPlaying.Inc()
		return false, err
	}
	if !isPlaying {
		return false, nil
	}

	hasFunds, err := a.HasEnoughFundsToPlay(ctx)
	if err != nil {
		a.logger.Error(err, "agent HasEnoughFundsToPlay failed")
		return false, err
	}

	if !hasFunds {
		a.logger.Info("insufficient funds to participate in next round", "round", round)
		a.metrics.InsufficientFundsToPlay.Inc()
		return false, nil
	}

	a.state.SetLastPlayedRound(round)
	a.logger.Info("neighbourhood chosen", "round", round)
	a.metrics.NeighborhoodSelected.Inc()

	sample, err := a.makeSample(ctx, round, storageRadius)
	if err != nil {
		return false, err
	}

	err = saveSample(a.state.stateStore, sample, round)
	if err != nil {
		return false, fmt.Errorf("failed to save sample: %w", err)
	}

	return true, nil
}

func (a *Agent) makeSample(ctx context.Context, round uint64, storageRadius uint8) (sampleData, error) {
	salt, err := a.contract.ReserveSalt(ctx)
	if err != nil {
		return sampleData{}, err
	}

	timeLimiter, err := a.getPreviousRoundTime(ctx)
	if err != nil {
		return sampleData{}, err
	}

	t := time.Now()
	rSample, err := a.sampler.ReserveSample(ctx, salt, storageRadius, uint64(timeLimiter))
	if err != nil {
		return sampleData{}, err
	}
	a.metrics.SampleDuration.Set(time.Since(t).Seconds())

	sample := sampleData{
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

func (a *Agent) commit(ctx context.Context, sample sampleData, round uint64) error {
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

	err = saveCommitKey(a.state.stateStore, key, round)
	if err != nil {
		return fmt.Errorf("failed to save commit key: %w", err)
	}

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

func (a *Agent) HasEnoughFundsToPlay(ctx context.Context) (bool, error) {
	balance, err := a.backend.BalanceAt(ctx, a.state.ethAddress, nil)
	if err != nil {
		return false, err
	}

	price, err := a.backend.SuggestGasPrice(ctx)
	if err != nil {
		return false, err
	}

	avgTxFee := new(big.Int).Mul(big.NewInt(avgTxGas), price)
	minBalance := new(big.Int).Mul(avgTxFee, big.NewInt(minTxCountToCover))

	return balance.Cmp(minBalance) >= 1, nil
}
