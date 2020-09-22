// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package netstore

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/recovery"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/soc"
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
	chunkChanMu      sync.RWMutex
}

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
	}
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
				targets, err1 := sctx.GetTargets(ctx)
				if err1 != nil {
					return nil, err1
				}
				chunkC := make(chan swarm.Chunk, 1)
				socAddress, err1 := s.recoveryCallback(ctx, addr, targets, chunkC)
				if err1 != nil {
					return nil, err1
				}

				// add the expected soc address and the channel in which the chunk is awaited
				s.chunkChanMu.Lock()
				s.chunkChanMap[socAddress.String()] = chunkC
				s.chunkChanMu.Unlock()
				defer func() {
					s.chunkChanMu.Lock()
					delete(s.chunkChanMap, socAddress.String())
					s.chunkChanMu.Unlock()
				}()

				// wait for repair response or context to be cancelled
				var recoveryResponse swarm.Chunk
				select {
				case recoveryResponse = <-chunkC:
				case <-ctx.Done():
					return nil, err // return the original retrieval error
				}
				s, err1 := soc.FromChunk(recoveryResponse)
				if err1 != nil {
					return nil, err1
				}
				ch = s.Chunk
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
	var chunks []swarm.Chunk
	for _, ch := range chs {
		yes, cType := s.validator.Validate(ch)
		if !yes {
			return nil, storage.ErrInvalidChunk
		}

		// check for repair response
		if cType == swarm.SingleOwnerChunk {
			key := ch.Address().String()
			s.chunkChanMu.Lock()
			chunkC, ok := s.chunkChanMap[key]
			s.chunkChanMu.Unlock()
			if ok {
				// the client is still waiting, so send the chunk to him
				chunkC <- ch
				continue
			}
		}
		chunks = append(chunks, ch)
	}
	return s.Storer.Put(ctx, mode, chunks...)
}
