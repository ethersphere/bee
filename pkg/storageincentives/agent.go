// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
	"io"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	loggerName              = "storageincentives"
	redistributionStatusKey = "redistribution_state_"
)

const (
	DefaultBlocksPerRound = 152
	DefaultBlocksPerPhase = DefaultBlocksPerRound / 4
)

type ChainBackend interface {
	BlockNumber(context.Context) (uint64, error)
	HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
}

type Monitor interface {
	IsFullySynced() bool
}

type Agent struct {
	logger         log.Logger
	metrics        metrics
	backend        ChainBackend
	blocksPerRound uint64
	monitor        Monitor
	contract       redistribution.Contract
	batchExpirer   postagecontract.PostageBatchExpirer
	reserve        postage.Storer
	nodeState      storage.StateStorer
	sampler        storage.Sampler
	overlay        swarm.Address
	quit           chan struct{}
	wg             sync.WaitGroup
	nodeStatus     NodeStatus
	erc20Service   erc20.Service
	initialBalance *big.Int // current balance of the node before starting the round
}

// NodeStatus provide internal status of the nodes in the redistribution game
type NodeStatus struct {
	State  string   `json:"state"`
	Round  uint64   `json:"round"`
	Block  uint64   `json:"block"`
	Reward *big.Int `json:"reward"`
	Fees   *big.Int `json:"fees"`
}

func New(
	overlay swarm.Address,
	backend ChainBackend,
	logger log.Logger,
	monitor Monitor,
	contract redistribution.Contract,
	batchExpirer postagecontract.PostageBatchExpirer,
	reserve postage.Storer,
	sampler storage.Sampler,
	blockTime time.Duration, blocksPerRound, blocksPerPhase uint64, nodeState storage.StateStorer, erc20Service erc20.Service) *Agent {

	s := &Agent{
		overlay:        overlay,
		metrics:        newMetrics(),
		backend:        backend,
		logger:         logger.WithName(loggerName).Register(),
		contract:       contract,
		batchExpirer:   batchExpirer,
		reserve:        reserve,
		monitor:        monitor,
		blocksPerRound: blocksPerRound,
		sampler:        sampler,
		quit:           make(chan struct{}),
		nodeStatus:     NodeStatus{},
		nodeState:      nodeState,
		erc20Service:   erc20Service,
		initialBalance: big.NewInt(0),
	}

	s.wg.Add(1)
	go s.start(blockTime, blocksPerRound, blocksPerPhase)

	return s
}

func (a *Agent) SetBalance() {
	balance, err := a.erc20Service.BalanceOf(context.Background(), common.HexToAddress(a.overlay.String()))
	if err != nil {
		return
	}
	a.initialBalance.Set(balance)
}

func (a *Agent) GetBalance() *big.Int {
	balance, err := a.erc20Service.BalanceOf(context.Background(), common.HexToAddress(a.overlay.String()))
	if err != nil {
		return nil
	}
	return balance
}

// start polls the current block number, calculates, and publishes only once the current phase.
// Each round is blocksPerRound long and is divided into three blocksPerPhase long phases: commit, reveal, claim.
// The sample phase is triggered upon entering the claim phase and may run until the end of the commit phase.
// If our neighborhood is selected to participate, a sample is created during the sample phase. In the commit phase,
// the sample is submitted, and in the reveal phase, the obfuscation key from the commit phase is submitted.
// Next, in the claim phase, we check if we've won, and the cycle repeats. The cycle must occur in the length of one round.
func (a *Agent) start(blockTime time.Duration, blocksPerRound, blocksPerPhase uint64) {

	// set current balance of the node to calculate the reward
	a.SetBalance()
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
			err := a.claim(ctx)
			if err != nil {
				a.logger.Error(err, "claim")
			}
		}
	})

	phaseEvents.On(sample, func(ctx context.Context, _ PhaseType) {

		mtx.Lock()
		round := round
		mtx.Unlock()

		sr, smpl, err := a.play(ctx)
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

		a.metrics.BackendCalls.Inc()
		block, err := a.backend.BlockNumber(context.Background())
		if err != nil {
			a.metrics.BackendErrors.Inc()
			a.logger.Error(err, "getting block number")
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

			a.logger.Info("entering phase", "phase", currentPhase.String(), "round", round, "block", block)

			phaseEvents.Publish(currentPhase)
			if currentPhase == claim {
				phaseEvents.Publish(sample) // trigger sample along side the claim phase
			}
		}
		a.nodeStatus.Block = block
		a.nodeStatus.State = currentPhase.String()
		a.nodeStatus.Round = round
		a.saveStatus()
		prevPhase = currentPhase

		mtx.Unlock()
	}
}

func (a *Agent) reveal(ctx context.Context, storageRadius uint8, sample, obfuscationKey []byte) error {
	a.metrics.RevealPhase.Inc()
	err := a.contract.Reveal(ctx, storageRadius, sample, obfuscationKey)
	a.setNodeStatusFee(a.contract.GetFee())
	if err != nil {
		a.metrics.ErrReveal.Inc()
	}
	return err
}

func (a *Agent) claim(ctx context.Context) error {
	defer func() {
		a.saveStatus()
	}()
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
		a.nodeStatus.State = winner.String()
		a.metrics.Winner.Inc()
		err = a.contract.Claim(ctx)
		a.setNodeStatusFee(a.contract.GetFee())
		a.calculateWinnerReward()
		if err != nil {
			a.metrics.ErrClaim.Inc()
			return fmt.Errorf("error claiming win: %w", err)
		} else {
			a.logger.Info("claimed win")
		}
	} else {
		a.logger.Info("claim made, lost round")
	}

	return nil
}

func (a *Agent) play(ctx context.Context) (uint8, []byte, error) {

	// get depthmonitor fully synced indicator
	ready := a.monitor.IsFullySynced()
	if !ready {
		return 0, nil, nil
	}

	storageRadius := a.reserve.GetReserveState().StorageRadius

	isPlaying, err := a.contract.IsPlaying(ctx, storageRadius)
	if !isPlaying || err != nil {
		a.metrics.ErrCheckIsPlaying.Inc()
		return 0, nil, err
	}

	a.logger.Info("neighbourhood chosen")
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

	err = a.contract.Commit(ctx, obfuscatedHash, big.NewInt(int64(round)))
	if err != nil {
		a.metrics.ErrCommit.Inc()
		return nil, err
	}
	a.setNodeStatusFee(a.contract.GetFee())
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

func (s *Agent) wrapCommit(storageRadius uint8, sample []byte, key []byte) ([]byte, error) {

	storageRadiusByte := []byte{storageRadius}

	data := append(s.overlay.Bytes(), storageRadiusByte...)
	data = append(data, sample...)
	data = append(data, key...)

	return crypto.LegacyKeccak256(data)
}

// setNodeStatus sets the internal node status
func (s *Agent) setNodeStatusFee(fee *big.Int) {
	if fee != nil {
		s.nodeStatus.Fees = s.nodeStatus.Fees.Add(s.nodeStatus.Fees, fee)
	}
}

func (s *Agent) calculateWinnerReward() {
	// get latest balance
	currentBalance := s.GetBalance()
	if currentBalance != nil {
		s.nodeStatus.Reward = currentBalance.Sub(currentBalance, s.initialBalance)
	}
}

func (s *Agent) saveStatus() {
	s.nodeState.Put(fmt.Sprintf("%s%x", redistributionStatusKey, s.overlay.String()), s.nodeStatus)
}
