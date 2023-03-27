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
	"math"
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
		mtx            sync.Mutex
		sampleRound    uint64 = math.MaxUint64
		commitRound    uint64 = math.MaxUint64
		revealRound    uint64 = math.MaxUint64
		round          uint64
		reserveSample  []byte
		obfuscationKey []byte
		storageRadius  uint8
		phaseEvents    = newEvents()
	)
	// cancel all possible running phases
	defer phaseEvents.Close()

	commitF := func(ctx context.Context) {
		phaseEvents.Cancel(claim)

		mtx.Lock()
		round := round
		sampleRound := sampleRound
		storageRadius := storageRadius
		reserveSample := reserveSample
		mtx.Unlock()

		if round-1 == sampleRound { // the sample has to come from previous round to be able to commit it
			obf, err := a.commit(ctx, storageRadius, reserveSample, round)
			if err != nil {
				a.logger.Error(err, "commit")
			} else {
				mtx.Lock()
				obfuscationKey = obf
				commitRound = round
				mtx.Unlock()
				a.logger.Debug("committed the reserve sample and radius")
			}
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

		mtx.Lock()
		round := round
		commitRound := commitRound
		storageRadius := storageRadius
		reserveSample := reserveSample
		obfuscationKey := obfuscationKey
		mtx.Unlock()

		if round == commitRound { // reveal requires the obfuscationKey from the same round
			err := a.reveal(ctx, storageRadius, reserveSample, obfuscationKey)
			if err != nil {
				a.logger.Error(err, "reveal")
			} else {
				mtx.Lock()
				revealRound = round
				mtx.Unlock()
				a.logger.Debug("revealed the sample with the obfuscation key")
			}
		}
	})

	phaseEvents.On(claim, func(ctx context.Context, _ PhaseType) {

		phaseEvents.Cancel(reveal)

		mtx.Lock()
		round := round
		revealRound := revealRound
		mtx.Unlock()

		if round == revealRound { // to claim, previous reveal must've happened in the same round
			err := a.claim(ctx, round)
			if err != nil {
				a.logger.Error(err, "claim")
			}
		}
	})

	phaseEvents.On(sample, func(ctx context.Context, _ PhaseType) {

		mtx.Lock()
		round := round
		mtx.Unlock()

		sr, smpl, err := a.play(ctx, round)
		if err != nil {
			a.logger.Error(err, "make sample")
		} else if smpl != nil {
			mtx.Lock()
			sampleRound = round
			reserveSample = smpl
			storageRadius = sr
			a.logger.Info("produced reserve sample", "round", round)
			mtx.Unlock()
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

		mtx.Lock()
		round = block / blocksPerRound
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
		mtx.Unlock()
	}
}

func (a *Agent) reveal(ctx context.Context, storageRadius uint8, sample, obfuscationKey []byte) error {
	a.metrics.RevealPhase.Inc()
	txHash, err := a.contract.Reveal(ctx, storageRadius, sample, obfuscationKey)
	if err != nil {
		a.metrics.ErrReveal.Inc()
		return err
	}
	a.state.AddFee(ctx, txHash)
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

func (a *Agent) play(ctx context.Context, round uint64) (uint8, []byte, error) {

	status, err := a.state.Status()
	if err != nil {
		return 0, nil, err
	}

	if !status.IsFullySynced {
		a.logger.Info("skipping round because node is not fully synced", "round", round)
		return 0, nil, nil
	}

	if status.IsFrozen {
		a.logger.Info("skipping round because node is frozen", "round", round)
		return 0, nil, nil
	}

	storageRadius := a.radius.StorageRadius()

	isPlaying, err := a.contract.IsPlaying(ctx, storageRadius)
	if err != nil {
		a.metrics.ErrCheckIsPlaying.Inc()
		return 0, nil, err
	}
	if !isPlaying {
		return 0, nil, nil
	}

	hasFunds, err := a.HasEnoughFundsToPlay(ctx)
	if err != nil {
		a.logger.Error(err, "agent HasEnoughFundsToPlay failed")
		return 0, nil, nil
	}

	if !hasFunds {
		a.logger.Info("insufficient funds to participate in next round", "round", round)
		a.metrics.InsufficientFundsToPlay.Inc()
		return 0, nil, nil
	}

	a.state.SetLastPlayedRound(round)
	a.logger.Info("neighbourhood chosen", "round", round)
	a.metrics.NeighborhoodSelected.Inc()

	salt, err := a.contract.ReserveSalt(ctx)
	if err != nil {
		return 0, nil, err
	}

	t := time.Now()

	timeLimiter, err := a.getPreviousRoundTime(ctx)
	if err != nil {
		return 0, nil, err
	}

	sample, err := a.sampler.ReserveSample(ctx, salt, storageRadius, uint64(timeLimiter))
	if err != nil {
		return 0, nil, err
	}
	a.metrics.SampleDuration.Set(time.Since(t).Seconds())

	return storageRadius, sample.Hash.Bytes(), nil
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

func (a *Agent) commit(ctx context.Context, storageRadius uint8, sample []byte, round uint64) ([]byte, error) {
	a.metrics.CommitPhase.Inc()

	key := make([]byte, swarm.HashSize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}

	obfuscatedHash, err := a.wrapCommit(storageRadius, sample, key)
	if err != nil {
		return nil, err
	}

	txHash, err := a.contract.Commit(ctx, obfuscatedHash, big.NewInt(int64(round)))
	if err != nil {
		a.metrics.ErrCommit.Inc()
		return nil, err
	}
	a.state.AddFee(ctx, txHash)
	return key, nil
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
