// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package netstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/recovery"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	ErrRecoveryAttempt = errors.New("recovery attempt")
)

type store struct {
	storage.Storer
	retrieval        retrieval.Interface
	validators       []swarm.Validator
	logger           logging.Logger
	recoveryCallback recovery.Callback // this is the callback to be executed when a chunk fails to be retrieved
	callback         func(swarm.Chunk)
}

// New returns a new NetStore that wraps a given Storer.
func New(s storage.Storer, rcb recovery.Callback, r retrieval.Interface, logger logging.Logger,
	validators []swarm.Validator, callback func(swarm.Chunk)) storage.Storer {
	return &store{Storer: s, recoveryCallback: rcb, retrieval: r, logger: logger, validators: validators, callback: callback}
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
				targets := sctx.GetTargets(ctx)
				if targets == nil || s.recoveryCallback == nil {
					return nil, err
				}
				go s.recoveryCallback(addr, targets)
				return nil, ErrRecoveryAttempt
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
		valid, typ := ch.Valid(s.validators...)
		if !valid {
			return nil, storage.ErrInvalidChunk
		}
		if typ == 1 {
			go s.callback(ch)
		}
	}
	return s.Storer.Put(ctx, mode, chs...)
}
