// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package incentives

import (
	"context"
	"crypto/rand"
	"encoding/binary"
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

var (
	ErrSlashed = errors.New("slashed")
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
	IsWinner(context.Context) (bool, bool, error)
	Claim(context.Context) error
	Commit(context.Context, []byte) error
	Reveal(context.Context, uint8, []byte, []byte) error
}

const loggerName = "incentives"

type Service struct {
	logger  log.Logger
	backend ChainBackend
	monitor Monitor

	incentivesContract IncentivesContract
	reserve            postage.Storer
	sampler            Sampler

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
		overlay:            overlay,
		backend:            backend,
		logger:             logger.WithName(loggerName).Register(),
		incentivesContract: incentives,
		reserve:            reserve,
		monitor:            monitor,
		sampler:            sampler,
		cancelC:            make(chan struct{}),
		quit:               make(chan struct{}),
		stopC:              make(chan struct{}),
	}

	s.wg.Add(2)
	go s.start(blockTime, startBlock, blockPerRound, blocksPerPhase)

	return s
}

func (s *Service) start(blockTime time.Duration, startBlock, blocksPerRound, blocksPerPhase uint64) {

	defer s.wg.Done()

	var (
		phaseC = make(chan phase)
	)

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
				select {
				case phaseC <- phase{round: round, phase: currentPhase}:
				case <-s.quit:
					return
				}
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
			switch p.phase {
			case commit:
				if p.round-1 == sampleRound { // the sample has to come from previous round to be able to commit it
					obfuscationKey, err = s.commit(storageRadius, sample)
					if err != nil {
						s.logger.Error(err, "commit")
					} else {
						commitRound = p.round
						s.logger.Debug("commit phase")
					}
				}
			case reveal:
				if p.round == commitRound { // reveal requires the obfuscationKey from the same round
					err = s.reveal(storageRadius, sample, obfuscationKey)
					if err != nil {
						s.logger.Error(err, "reveal")
					} else {
						revealRound = p.round
						s.logger.Debug("reveal phase")
					}
				}
			case claim:
				if p.round == revealRound { // to claim, previous reveal must've happened in the same round
					err = s.claim()
					if err != nil {
						s.logger.Error(err, "attempt claim")
					} else {
						s.logger.Debug("claim phase")
					}
				}
				storageRadius, sample, err = s.play()
				if err != nil {
					s.logger.Error(err, "make sample")
					if errors.Is(err, ErrSlashed) {
						s.logger.Info("slashed error, quiting incentives agent")
						close(s.stopC)
						return
					}
				} else if sample != nil {
					sampleRound = p.round
					s.logger.Debug("made sample", "round", p.round)
				}
			}
		}
	}
}

func (s *Service) reveal(storageRadius uint8, sample, obfuscationKey []byte) error {

	ctx, cancel := s.newContext()
	defer cancel()

	return s.incentivesContract.Reveal(ctx, storageRadius, sample, obfuscationKey)
}

func (s *Service) claim() error {

	ctx, cancel := s.newContext()
	defer cancel()

	isSlashed, isWinner, err := s.incentivesContract.IsWinner(ctx)
	if err != nil {
		return err
	}
	if isSlashed {
		return ErrSlashed
	}
	if isWinner {
		err = s.incentivesContract.Claim(ctx)
		if err != nil {
			return fmt.Errorf("error claiming win: %w", err)
		} else {
			s.logger.Info("claimed win")
		}
	}

	return nil
}

func (s *Service) play() (uint8, []byte, error) {

	ctx, cancel := s.newContext()
	defer cancel()

	storageRadius := s.reserve.GetReserveState().StorageRadius

	isPlaying, err := s.incentivesContract.IsPlaying(ctx, storageRadius)
	if !isPlaying || err != nil {
		return 0, nil, err
	}

	s.logger.Info("neighbourhood chosen")

	salt, err := s.incentivesContract.ReserveSalt(ctx)
	if err != nil {
		return 0, nil, err
	}

	sample, err := s.sampler.ReserveSample(ctx, salt, storageRadius)
	if err != nil {
		return 0, nil, err
	}

	return storageRadius, sample, nil
}

func (s *Service) commit(storageRadius uint8, sample []byte) ([]byte, error) {

	ctx, cancel := s.newContext()
	defer cancel()

	key := make([]byte, swarm.HashSize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}

	orc, err := wrapCommit(storageRadius, sample, s.overlay.Bytes(), key)
	if err != nil {
		return nil, err
	}

	err = s.incentivesContract.Commit(ctx, orc)
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

// wrapCommit concatenates the byte serialisations of all the data needed to apply to
// the lottery and obfuscates it with a nonce that is to be revealed in the subsequent phase
// This should be a contract accessor taking storage radius, and reserve sample and overlay and the obfuscater nonce
func wrapCommit(storageRadius uint8, sample, overlay, obfuscater []byte) (orc []byte, err error) {
	h := swarm.NewHasher()
	if _, err = h.Write(sample); err != nil {
		return nil, err
	}
	sr := make([]byte, 8)
	binary.BigEndian.PutUint64(sr, uint64(storageRadius))
	if _, err = h.Write(sr); err != nil {
		return nil, err
	}
	if _, err = h.Write(overlay); err != nil {
		return nil, err
	}
	if _, err = h.Write(obfuscater); err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
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
