// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package incentives

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
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
type phase struct {
	round uint64
	phase int
}

const (
	commit = iota
	reveal
	claim
)

type IncentivesContract interface {
	RandomSeedAnchor() ([]byte, error)
	RandomSeedNeighbourhood() ([]byte, error)
	IsWinner() (bool, bool, error)
	ClaimWin() error
	Commit([]byte) error
	Reveal(uint8, []byte, []byte) error
}

const loggerName = "incentives"

type Service struct {
	logger  log.Logger
	backend ChainBackend
	monitor Monitor

	incentivesConract IncentivesContract
	reserve           postage.Storer
	sampler           Sampler

	overlay swarm.Address

	quit chan struct{}
	wg   sync.WaitGroup
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
		overlay:           overlay,
		backend:           backend,
		logger:            logger.WithName(loggerName).Register(),
		incentivesConract: incentives,
		reserve:           reserve,
		monitor:           monitor,
		sampler:           sampler,
		quit:              make(chan struct{}),
	}

	s.wg.Add(2)
	go s.manage(blockTime, startBlock, blockPerRound, blocksPerPhase)

	return s
}

func (s *Service) manage(blockTime time.Duration, startBlock, blocksPerRound, blocksPerPhase uint64) {

	defer s.wg.Done()

	var (
		newRoundC = make(chan struct{})
		phaseC    = make(chan phase)
	)

	go func() {

		defer s.wg.Done()

		var (
			round        uint64
			prevPhase    int = -1
			currentPhase int
		)

		for {
			select {
			case <-s.quit:
				return
			case <-time.After(blockTime):
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

			// fmt.Println("block", block)

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

			// write the new phase only once
			if currentPhase != prevPhase {
				if currentPhase == commit {
					select {
					case newRoundC <- struct{}{}: // on commit phase, we enter a new round, cancel all previus executions
					default:
					}
				}
				// fmt.Println("block", block, "blocks", blocks, "blocksPerRound", blocksPerRound, "phase", currentPhase, "round", round)
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
				if p.round-1 == sampleRound {
					// fmt.Println("commit", sampleRound)
					obfuscationKey, err = s.commit(storageRadius, sample)
					if err != nil {
						s.logger.Error(err, "commit")
					} else {
						commitRound = p.round
					}
				}
			case reveal:
				if p.round == commitRound {
					// fmt.Println("reveal", commitRound)
					err = s.incentivesConract.Reveal(storageRadius, sample, obfuscationKey)
					if err != nil {
						s.logger.Error(err, "reveal")
					} else {
						revealRound = p.round
					}
				}
			case claim:
				if p.round == revealRound {
					// fmt.Println("claim")
					err = s.attempClaim()
					if err != nil {
						s.logger.Error(err, "attempt claim")
					}
				}
				// fmt.Println("sample")
				storageRadius, sample, err = s.makeSample(newRoundC)
				if err != nil {
					s.logger.Error(err, "creating reserve sampler")
				} else {
					sampleRound = p.round
				}
			}
		}
	}
}

func (s *Service) attempClaim() error {
	isSlashed, isWinner, err := s.incentivesConract.IsWinner()
	if err != nil {
		return err
	}
	if isSlashed {
		return ErrSlashed
	}
	if isWinner {
		return s.incentivesConract.ClaimWin()
	}

	return nil
}

func (s *Service) makeSample(newRouncC chan struct{}) (uint8, []byte, error) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		select {
		case <-newRouncC: // cancel context if a new round happens, terminating current call
			cancel()
		case <-ctx.Done(): // return if context is already done
			return
		}
	}()

	neighbourhoodRnd, err := s.incentivesConract.RandomSeedNeighbourhood()
	if err != nil {
		return 0, nil, err
	}

	storageRadius := s.reserve.GetReserveState().StorageRadius

	if swarm.Proximity(s.overlay.Bytes(), neighbourhoodRnd) < storageRadius {
		return 0, nil, nil
	}

	salt, err := s.incentivesConract.RandomSeedAnchor()
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

	key := make([]byte, swarm.HashSize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}

	orc, err := wrapCommit(storageRadius, sample, s.overlay.Bytes(), key)
	if err != nil {
		return nil, err
	}

	err = s.incentivesConract.Commit(orc)
	if err != nil {
		return nil, err
	}

	return key, nil
}

// wrapCommit concatenates the byte serialisations of all the data needed to apply to
// the lottery and obfuscates it with a nonce that is to be revealed in the subsequent phase
// This should be a contract accessor taking `SD`, `RC` and overlay and the obfuscater nonce
func wrapCommit(storageRadius uint8, sample, overlay, key []byte) (orc []byte, err error) {
	h := swarm.NewHasher()
	if _, err = h.Write(sample); err != nil {
		return nil, err
	}
	sdb := make([]byte, 8)
	binary.BigEndian.PutUint64(sdb, uint64(storageRadius))
	if _, err = h.Write(sdb); err != nil {
		return nil, err
	}
	if _, err = h.Write(overlay); err != nil {
		return nil, err
	}
	if _, err = h.Write(key); err != nil {
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
