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
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/recovery"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	maxBgPutters int = 16
)

type store struct {
	storage.Storer
	retrieval        retrieval.Interface
	logger           logging.Logger
	validStamp       postage.ValidStampFn
	recoveryCallback recovery.Callback // this is the callback to be executed when a chunk fails to be retrieved
	bgWorkers        chan struct{}
	sCtx             context.Context
	sCancel          context.CancelFunc
	wg               sync.WaitGroup
}

var (
	ErrRecoveryAttempt = errors.New("failed to retrieve chunk, recovery initiated")
)

// New returns a new NetStore that wraps a given Storer.
func New(s storage.Storer, validStamp postage.ValidStampFn, rcb recovery.Callback, r retrieval.Interface, logger logging.Logger) storage.Storer {
	ns := &store{Storer: s, validStamp: validStamp, recoveryCallback: rcb, retrieval: r, logger: logger}
	ns.sCtx, ns.sCancel = context.WithCancel(context.Background())
	ns.bgWorkers = make(chan struct{}, maxBgPutters)
	return ns
}

// Get retrieves a given chunk address.
// It will request a chunk from the network whenever it cannot be found locally.
// If the network path is taken, the method also stores the found chunk into the
// local-store.
func (s *store) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {
	ch, err = s.Storer.Get(ctx, mode, addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// request from network
			ch, err = s.retrieval.RetrieveChunk(ctx, addr, true)
			if err != nil {
				targets := sctx.GetTargets(ctx)
				if targets == nil || s.recoveryCallback == nil {
					return nil, err
				}
				go s.recoveryCallback(addr, targets)
				return nil, ErrRecoveryAttempt
			}
			s.put(ch, mode)
			return ch, nil
		}
		return nil, fmt.Errorf("netstore get: %w", err)
	}
	return ch, nil
}

// put will store the chunk into storage asynchronously
func (s *store) put(ch swarm.Chunk, mode storage.ModeGet) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		select {
		case <-s.sCtx.Done():
			s.logger.Debug("netstore: stopping netstore")
			return
		case s.bgWorkers <- struct{}{}:
		}
		defer func() {
			<-s.bgWorkers
		}()

		stamp, err := ch.Stamp().MarshalBinary()
		if err != nil {
			s.logger.Errorf("netstore: failed to marshal stamp from chunk %s Err:%s", ch.Address(), err.Error())
			return
		}

		putMode := storage.ModePutRequest
		if mode == storage.ModeGetRequestPin {
			putMode = storage.ModePutRequestPin
		}

		cch, err := s.validStamp(ch, stamp)
		if err != nil {
			// if a chunk with an invalid postage stamp was received
			// we force it into the cache.
			putMode = storage.ModePutRequestCache
			cch = ch
		}

		_, err = s.Storer.Put(s.sCtx, putMode, cch)
		if err != nil {
			s.logger.Errorf("netstore: failed to put chunk %s Err: %s", cch.Address(), err.Error())
		}
	}()
}

// The underlying store is not the netstore's responsibility to close
func (s *store) Close() error {
	s.sCancel()

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		s.wg.Wait()
	}()

	select {
	case <-stopped:
		return nil
	case <-time.After(time.Second * 5):
		return errors.New("netstore: waited 5 seconds to close active goroutines")
	}
}
