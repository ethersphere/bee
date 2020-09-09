// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package netstore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/recovery"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type store struct {
	storage.Storer
	retrieval        retrieval.Interface
	validator        swarm.Validator
	logger           logging.Logger
	recoveryCallback recovery.RecoveryHook // this is the callback to be executed when a chunk fails to be retrieved
	chunkChanMap     map[string]chan swarm.Chunk
	chunkChanMu      *sync.RWMutex
}

var (
	repairTimeout = 10 * time.Second
)

var (
	ErrRecoveryTimeout = errors.New("timeout during chunk repair")
)

// New returns a new NetStore that wraps a given Storer.
func New(s storage.Storer, rcb recovery.RecoveryHook, r retrieval.Interface, logger logging.Logger,
	validator swarm.Validator) storage.Storer {
	return &store{
		Storer:           s,
		recoveryCallback: rcb,
		retrieval:        r,
		logger:           logger,
		validator:        validator,
		chunkChanMap:     make(map[string]chan swarm.Chunk),
		chunkChanMu:      &sync.RWMutex{},
	}
}

func SetTimeout(timeout time.Duration) {
	repairTimeout = timeout
}

// Get retrieves a given chunk address.
// It will request a chunk from the network whenever it cannot be found locally.
func (s *store) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {
	ch, err = s.Storer.Get(ctx, mode, addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// request from network
			ch, err = s.retrieval.RetrieveChunk(ctx, addr)
			if err != nil {
				if s.recoveryCallback == nil {
					return nil, err
				}
				targets, err := sctx.GetTargets(ctx)
				if err != nil {
					return nil, err
				}
				chunkC := make(chan swarm.Chunk, 1)
				socAddress, err := s.recoveryCallback(addr, targets, chunkC)
				if err != nil {
					s.logger.Debugf("netstore: error while recovering chunk: %v", err)
				}
				// add the expected soc address and the channel in which the chunk is awaited
				s.chunkChanMu.Lock()
				s.chunkChanMap[socAddress.String()] = chunkC
				s.chunkChanMu.Unlock()

				// wait for chunk or timeout
				select {
				case ch = <-chunkC:
				case <-time.After(repairTimeout):
					return nil, ErrRecoveryTimeout
				}
			}
			_, err = s.Storer.Put(ctx, storage.ModePutRequest, ch)
			if err != nil {
				return nil, fmt.Errorf("netstore retrieve put: %w", err)
			}
			return ch, nil
		}
		return nil, fmt.Errorf("netstore get: %w", err)
	}
	return ch, nil
}

// Put stores a given chunk in the local storage.
// returns a storage.ErrInvalidChunk error when
// encountering an invalid chunk.
func (s *store) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {

	for _, ch := range chs {
		if !s.validator.Validate(ch) {
			return nil, storage.ErrInvalidChunk
		}

		// if the chunk is expected as part of the repair process, then deliver it
		s.chunkChanMu.Lock()
		key := ch.Address().String()
		if chunkC, ok := s.chunkChanMap[key]; ok {
			chunkC <- ch
			close(chunkC)
			delete(s.chunkChanMap, key)
		}
		s.chunkChanMu.Unlock()
	}

	return s.Storer.Put(ctx, mode, chs...)
}
