// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package incentives

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
	"github.com/ethersphere/bee/pkg/swarm"
)

type ChainBackend interface {
	BlockNumber(context.Context) (uint64, error)
}

type Sampler interface {
	ReserveSample(context.Context, []byte, uint8) ([]byte, error)
}

type Monitor interface {
	IsStable() bool
}

type phaseType int
type phase struct {
	round uint64
	phase phaseType
}

const (
	commit phaseType = iota
	reveal
	claim
)

func (p phaseType) string() string {
	switch p {
	case commit:
		return "commit"
	case reveal:
		return "reveal"
	case claim:
		return "claim"
	default:
		return "unknown"
	}
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

const loggerName = "incentives"

type Service struct {
	logger  log.Logger
	backend ChainBackend
	monitor Monitor

	contract IncentivesContract
	reserve  postage.Storer
	sampler  Sampler

	overlay swarm.Address

	cancelC chan struct{}
	stopC   chan struct{}
	quit    chan struct{}
	wg      sync.WaitGroup
}

func New(
	overlay swarm.Address,
	backend ChainBackend,
	logger log.Logger,
	monitor Monitor,
	incentives IncentivesContract,
	reserve postage.Storer,
	sampler Sampler,
	blockTime time.Duration, startBlock, blockPerRound, blocksPerPhase uint64) *Service {

	s := &Service{
		overlay:  overlay,
		backend:  backend,
		logger:   logger.WithName(loggerName).Register(),
		contract: incentives,
		reserve:  reserve,
		monitor:  monitor,
		sampler:  sampler,
		cancelC:  make(chan struct{}),
		quit:     make(chan struct{}),
		stopC:    make(chan struct{}),
	}

	s.wg.Add(2)
	go s.start(blockTime, startBlock, blockPerRound, blocksPerPhase)

	return s
}

func (s *Service) start(blockTime time.Duration, startBlock, blocksPerRound, blocksPerPhase uint64) {

	defer s.wg.Done()

	var phaseC = make(chan phase)

	// drain the channel on shutdown
	defer func() {
		select {
		case <-phaseC:
		default:
		}
	}()

	// the goroutine polls the current block number, calculates,
	// and writes only once the current phase and round. If a new round is detected,
	// it writes to a new round channel to exterminate any previous makeSample execution.
	go func() {

		defer s.wg.Done()

		var (
			round        uint64
			prevPhase    phaseType = -1
			currentPhase phaseType
		)

		// optimization, we do not need to check the phase change at every new block
		var checkEvery uint64 = 1
		if blocksPerPhase > 10 {
			checkEvery = 5
		}

		for {
			select {
			case <-s.quit:
				return
			case <-s.stopC:
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

			blocks := (block - startBlock)

			round = blocks / blocksPerRound

			// compute the current phase
			p := blocks % blocksPerRound
			if p < blocksPerPhase {
				currentPhase = commit
			} else if p >= blocksPerPhase && p < 2*blocksPerPhase {
				currentPhase = reveal
			} else {
				currentPhase = claim
			}

			// write the current phase only once
			if currentPhase != prevPhase {
				s.logger.Info("entering phase", "phase", currentPhase.string(), "round", round, "block", block)
				select {
				case s.cancelC <- struct{}{}: // we enter a new phase, cancel all previous executions
				default:
				}
				phaseC <- phase{round: round, phase: currentPhase}
			}
			prevPhase = currentPhase
		}
	}()

	var (
		sampleRound    uint64 = math.MaxUint64
		commitRound    uint64 = math.MaxUint64
		revealRound    uint64 = math.MaxUint64
		sample         []byte
		obfuscationKey []byte
		storageRadius  uint8
		err            error
	)

	for {
		select {
		case <-s.quit:
			return
		case p := <-phaseC:
			ctx, cancel := s.newContext()
			switch p.phase {
			case commit:
				if p.round-1 == sampleRound { // the sample has to come from previous round to be able to commit it
					obfuscationKey, err = s.commit(ctx, storageRadius, sample)
					if err != nil {
						s.logger.Error(err, "commit")
					} else {
						commitRound = p.round
						s.logger.Debug("commit phase")
					}
				}
			case reveal:
				if p.round == commitRound { // reveal requires the obfuscationKey from the same round
					err = s.reveal(ctx, storageRadius, sample, obfuscationKey)
					if err != nil {
						s.logger.Error(err, "reveal")
					} else {
						revealRound = p.round
						s.logger.Debug("reveal phase")
					}
				}
			case claim:
				if p.round == revealRound { // to claim, previous reveal must've happened in the same round
					err = s.claim(ctx)
					if err != nil {
						s.logger.Error(err, "attempt claim")
					} else {
						s.logger.Debug("claim phase")
					}
				}
				storageRadius, sample, err = s.play(ctx)
				if err != nil {
					s.logger.Error(err, "make sample")

				} else if sample != nil {
					sampleRound = p.round
					s.logger.Debug("made sample", "round", p.round)
				}
			}
			cancel()
		}
	}

}

func (s *Service) reveal(ctx context.Context, storageRadius uint8, sample, obfuscationKey []byte) error {
	return s.contract.Reveal(ctx, storageRadius, sample, obfuscationKey)
}

func (s *Service) claim(ctx context.Context) error {

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

func (s *Service) play(ctx context.Context) (uint8, []byte, error) {

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

	return storageRadius, sample, nil
}

func (s *Service) commit(ctx context.Context, storageRadius uint8, sample []byte) ([]byte, error) {

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

func (s *Service) newContext() (context.Context, context.CancelFunc) {

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-s.quit: // cancel context on quit call
			cancel()
		case <-s.cancelC: // cancel context if a new round happens, terminating current call
			cancel()
		case <-ctx.Done(): // return if context is already done
			return
		}
	}()

	return ctx, cancel
}

func (s *Service) Close() error {
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
