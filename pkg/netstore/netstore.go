// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package netstore provides an abstraction layer over the
// Swarm local storage layer that leverages connectivity
// with other peers in order to retrieve chunks from the network that cannot
// be found locally.
package netstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/recovery"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type store struct {
	storage.Storer
	retrieval        retrieval.Interface
	logger           logging.Logger
	validStamp       postage.ValidStampFn
	recoveryCallback recovery.Callback // this is the callback to be executed when a chunk fails to be retrieved
}

var (
	ErrRecoveryAttempt = errors.New("failed to retrieve chunk, recovery initiated")
)

// New returns a new NetStore that wraps a given Storer.
func New(s storage.Storer, validStamp postage.ValidStampFn, rcb recovery.Callback, r retrieval.Interface, logger logging.Logger) storage.Storer {
	return &store{Storer: s, validStamp: validStamp, recoveryCallback: rcb, retrieval: r, logger: logger}
}

type result struct {
	Chunk     swarm.Chunk
	Retrieved bool
}

// Get retrieves a given chunk address.
// It will request a chunk from the network whenever it cannot be found locally.
// If the network path is taken, the method also stores the found chunk into the
// local-store.
func (s *store) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {

	// request from network

	resultC := make(chan result, 2)
	errC := make(chan struct{}, 2)

	go func() {

		ch, err = s.Storer.Get(ctx, mode, addr)
		if err == nil {
			select {
			case resultC <- result{Chunk: ch, Retrieved: false}:
			case <-ctx.Done():
			}
		} else {
			select {
			case errC <- struct{}{}:
			}
		}
	}()

	go func() {

		chunk, err := s.retrieval.RetrieveChunk(ctx, addr, true)
		if err != nil {
			targets := sctx.GetTargets(ctx)
			if targets != nil && s.recoveryCallback != nil {
				go s.recoveryCallback(addr, targets)
			}
			select {
			case errC <- struct{}{}:
			}
		} else {

			select {
			case resultC <- result{Chunk: chunk, Retrieved: true}:
			case <-ctx.Done():
			}
			stamp, err := chunk.Stamp().MarshalBinary()
			if err != nil {
				return
			}

			putMode := storage.ModePutRequest
			if mode == storage.ModeGetRequestPin {
				putMode = storage.ModePutRequestPin
			}

			cch, err := s.validStamp(chunk, stamp)
			if err != nil {
				// if a chunk with an invalid postage stamp was received
				// we force it into the cache.
				putMode = storage.ModePutRequestCache
				cch = chunk
			}
			_, _ = s.Storer.Put(ctx, putMode, cch)

		}

	}()

	hit := 0

	for {
		select {
		case result := <-resultC:

			if result.Retrieved {
				go func(chunk swarm.Chunk) {
					_, err = s.Storer.Put(context.Background(), storage.ModePutRequest, chunk)
				}(result.Chunk)
			}
			return result.Chunk, nil
		case <-errC:
			hit++

			if hit >= 2 {
				return nil, fmt.Errorf("netstore get")
			}
		case <-ctx.Done():
			return nil, fmt.Errorf("magic")
		}
	}

}
