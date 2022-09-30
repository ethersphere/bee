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
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const loggerName = "incentives"

type ChainBackend interface {
	BlockNumber(context.Context) (uint64, error)
}

type Sampler interface {
	ReserveSample(context.Context, []byte, uint8) (storage.Sample, error)
}

type Monitor interface {
	IsStable() bool
}

type IncentivesContract interface {
	ReserveSalt(context.Context) ([]byte, error)
	IsPlaying(context.Context, uint8) (bool, error)
	IsWinner(context.Context) (bool, error)
	Claim(context.Context) error
	Commit(context.Context, []byte) error
	Reveal(context.Context, uint8, []byte, []byte) error
	WrapCommit(uint8, []byte, []byte, []byte) ([]byte, error)
}

type Agent struct {
	logger   log.Logger
	backend  ChainBackend
	monitor  Monitor
	contract IncentivesContract
	reserve  postage.Storer
	sampler  Sampler
	overlay  swarm.Address
	quit     chan struct{}
	wg       sync.WaitGroup
}

func New(
	overlay swarm.Address,
	backend ChainBackend,
	logger log.Logger,
	monitor Monitor,
	incentives IncentivesContract,
	reserve postage.Storer,
	sampler Sampler,
	blockTime time.Duration, blockPerRound, blocksPerPhase uint64) *Agent {

	s := &Agent{
		overlay:  overlay,
		backend:  backend,
		logger:   logger.WithName(loggerName).Register(),
		contract: incentives,
		reserve:  reserve,
		monitor:  monitor,
		sampler:  sampler,
		quit:     make(chan struct{}),
	}

	s.wg.Add(1)
	go s.start(blockTime, blockPerRound, blocksPerPhase)

	return s
}

// start polls the current block number, calculates, and publishes only once the current phase.
// Each round is blocksPerRound long and is divided in to three blocksPerPhase long phases: commit, reveal, claim.
// The sample phase is triggered upon entering the claim phase and may run until the end of the commit phase.
// If our neighborhood is selected to participate, a sample is created during the sample phase. In the commit phase,
// the sample is submitted, and in the reveal phase, the obfuscation key from the commit phase is submitted.
// Next, in the claim phase, we check if we've won, and the cycle repeats. The cycle must occur in the length of one round.
func (s *Agent) start(blockTime time.Duration, blocksPerRound, blocksPerPhase uint64) {

	defer s.wg.Done()

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
			obf, err := s.commit(ctx, storageRadius, reserveSample)
			if err != nil {
				s.logger.Error(err, "commit")
			} else {
				mtx.Lock()
				obfuscationKey = obf
				commitRound = round
				mtx.Unlock()
				s.logger.Debug("commited the reserve sample and radius")
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
			err := s.reveal(ctx, storageRadius, reserveSample, obfuscationKey)
			if err != nil {
				s.logger.Error(err, "reveal")
			} else {
				mtx.Lock()
				revealRound = round
				mtx.Unlock()
				s.logger.Debug("revealed the sample with the obfuscation key")
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
			err := s.claim(ctx)
			if err != nil {
				s.logger.Error(err, "claim")
			} else {
				s.logger.Debug("claim made")
			}
		}
	})

	phaseEvents.On(sample, func(ctx context.Context, _ PhaseType) {

		mtx.Lock()
		round := round
		mtx.Unlock()

		sr, smpl, err := s.play(ctx)
		if err != nil {
			s.logger.Error(err, "make sample")
		} else if smpl != nil {
			mtx.Lock()
			sampleRound = round
			reserveSample = smpl
			storageRadius = sr
			s.logger.Info("produced reserve sample", "round", round)
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
		case <-s.quit:
			return
		case <-time.After(blockTime * time.Duration(checkEvery)):
		}

		// skip when the depthmonitor is unstable
		if !s.monitor.IsStable() {
			continue
		}

		block, err := s.backend.BlockNumber(context.Background())
		if err != nil {
			s.logger.Error(err, "getting block number")
			continue
		}

		mtx.Lock()
		round = block / blocksPerRound

		// compute the current phase
		p := block % blocksPerRound
		if p < blocksPerPhase {
			currentPhase = commit
		} else if p >= blocksPerPhase && p < 2*blocksPerPhase {
			currentPhase = reveal
		} else {
			currentPhase = claim
		}

		// write the current phase only once
		if currentPhase != prevPhase {
			s.logger.Info("entering phase", "phase", currentPhase.String(), "round", round, "block", block)

			phaseEvents.Publish(currentPhase)
			if currentPhase == claim {
				phaseEvents.Publish(sample) // trigger sample along side the claim phase
			}
		}

		prevPhase = currentPhase

		mtx.Unlock()
	}
}

func (s *Agent) reveal(ctx context.Context, storageRadius uint8, sample, obfuscationKey []byte) error {
	return s.contract.Reveal(ctx, storageRadius, sample, obfuscationKey)
}

func (s *Agent) claim(ctx context.Context) error {

	isWinner, err := s.contract.IsWinner(ctx)
	if err != nil {
		return err
	}

	if isWinner {
		err = s.contract.Claim(ctx)
		if err != nil {
			return fmt.Errorf("error claiming win: %w", err)
		} else {
			s.logger.Info("claimed win")
		}
	}

	return nil
}

func (s *Agent) play(ctx context.Context) (uint8, []byte, error) {

	storageRadius := s.reserve.GetReserveState().StorageRadius

	isPlaying, err := s.contract.IsPlaying(ctx, storageRadius)
	if !isPlaying || err != nil {
		return 0, nil, err
	}

	s.logger.Info("neighbourhood chosen")

	salt, err := s.contract.ReserveSalt(ctx)
	if err != nil {
		return 0, nil, err
	}

	sample, err := s.sampler.ReserveSample(ctx, salt, storageRadius)
	if err != nil {
		return 0, nil, err
	}

	return storageRadius, sample.Hash.Bytes(), nil
}

func (s *Agent) commit(ctx context.Context, storageRadius uint8, sample []byte) ([]byte, error) {

	key := make([]byte, swarm.HashSize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}

	orc, err := s.contract.WrapCommit(storageRadius, sample, s.overlay.Bytes(), key)
	if err != nil {
		return nil, err
	}

	err = s.contract.Commit(ctx, orc)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func (s *Agent) Close() error {
	close(s.quit)

	stopped := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(stopped)
	}()

	select {
	case <-stopped:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("stopping incentives with ongoing worker goroutine")
	}
}
