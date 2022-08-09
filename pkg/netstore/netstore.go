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

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "netstore"

const (
	maxBgPutters int = 16
)

type store struct {
	storage.Storer
	retrieval  retrieval.Interface
	logger     log.Logger
	validStamp postage.ValidStampFn
	bgWorkers  chan struct{}
	sCtx       context.Context
	sCancel    context.CancelFunc
	wg         sync.WaitGroup
	metrics    metrics
}

var (
	errInvalidLocalChunk = errors.New("invalid chunk found locally")
)

// New returns a new NetStore that wraps a given Storer.
func New(s storage.Storer, validStamp postage.ValidStampFn, r retrieval.Interface, logger log.Logger) storage.Storer {
	ns := &store{
		Storer:     s,
		validStamp: validStamp,
		retrieval:  r,
		logger:     logger.WithName(loggerName).Register(),
		bgWorkers:  make(chan struct{}, maxBgPutters),
		metrics:    newMetrics(),
	}
	ns.sCtx, ns.sCancel = context.WithCancel(context.Background())
	return ns
}

// Get retrieves a given chunk address.
// It will request a chunk from the network whenever it cannot be found locally.
// If the network path is taken, the method also stores the found chunk into the
// local-store.
func (s *store) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {
	ch, err = s.Storer.Get(ctx, mode, addr)
	if err == nil {
		s.metrics.LocalChunksCounter.Inc()
		// ensure the chunk we get locally is valid. If not, retrieve the chunk
		// from network. If there is any corruption of data in the local storage,
		// this would ensure it is retrieved again from network and added back with
		// the correct data
		if !cac.Valid(ch) && !soc.Valid(ch) {
			err = errInvalidLocalChunk
			ch = nil
			s.logger.Warning("netstore: got invalid chunk from localstore, falling back to retrieval")
			s.metrics.InvalidLocalChunksCounter.Inc()
		}
	}
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, errInvalidLocalChunk) {
			// request from network
			ch, err = s.retrieval.RetrieveChunk(ctx, addr, swarm.ZeroAddress)
			if err != nil {
				return nil, err
			}
			s.wg.Add(1)
			s.put(ch, mode)
			s.metrics.RetrievedChunksCounter.Inc()
			return ch, nil
		}
		return nil, fmt.Errorf("netstore get: %w", err)
	}
	return ch, nil
}

// put will store the chunk into storage asynchronously
func (s *store) put(ch swarm.Chunk, mode storage.ModeGet) {
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
			s.logger.Error(err, "failed to marshal stamp from chunk", "address", ch.Address())
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
			s.logger.Error(err, "failed to put chunk", "address", cch.Address())
		}
	}()
}

// The underlying store is not the netstore's responsibility to close
func (s *store) Close() error {
	s.sCancel()

	stopped := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(stopped)
	}()

	select {
	case <-stopped:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("netstore: waited 5 seconds to close active goroutines")
	}
}
