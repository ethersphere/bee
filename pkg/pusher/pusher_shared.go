// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pusher provides protocol-orchestrating functionality
// over the pushsync protocol. It makes sure that chunks meant
// to be distributed over the network are sent used using the
// pushsync protocol.
package pusher

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/opentracing/opentracing-go"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "pusher"

type Op struct {
	Chunk  swarm.Chunk
	Err    chan error
	Direct bool
	Span   opentracing.Span

	identityAddress swarm.Address
}

type OpChan <-chan *Op

type Storer interface {
	storage.PushReporter
	storage.PushSubscriber
	ReservePutter() storage.Putter
}

const (
	traceDuration     = 30 * time.Second // duration for every root tracing span
	ConcurrentPushes  = swarm.Branches   // how many chunks to push simultaneously
	DefaultRetryCount = 6
)

func (s *Service) pushDeferred(ctx context.Context, logger log.Logger, op *Op) (bool, error) {
	loggerV1 := logger.V(1).Build()

	defer s.inflight.delete(op.identityAddress, op.Chunk.Stamp().BatchID())

	ok, err := s.batchExist.Exists(op.Chunk.Stamp().BatchID())
	if !ok || err != nil {
		loggerV1.Warning(
			"stamp is no longer valid, skipping syncing for chunk",
			"batch_id", hex.EncodeToString(op.Chunk.Stamp().BatchID()),
			"chunk_address", op.Chunk.Address(),
			"error", err,
		)
		return false, errors.Join(err, s.storer.Report(ctx, op.Chunk, storage.ChunkCouldNotSync))
	}

	switch _, err := s.pushSyncer.PushChunkToClosest(ctx, op.Chunk); {
	case errors.Is(err, topology.ErrWantSelf):
		// store the chunk
		loggerV1.Debug("chunk stays here, i'm the closest node", "chunk_address", op.Chunk.Address())
		err = s.storer.ReservePutter().Put(ctx, op.Chunk)
		if err != nil {
			loggerV1.Error(err, "pusher: failed to store chunk")
			return true, err
		}
		err = s.storer.Report(ctx, op.Chunk, storage.ChunkStored)
		if err != nil {
			loggerV1.Error(err, "pusher: failed reporting chunk")
			return true, err
		}
	case errors.Is(err, pushsync.ErrShallowReceipt):
		if s.shallowReceipt(op.identityAddress) {
			return true, err
		}
		if err := s.storer.Report(ctx, op.Chunk, storage.ChunkSynced); err != nil {
			loggerV1.Error(err, "pusher: failed to report sync status")
			return true, err
		}
	case err == nil:
		if err := s.storer.Report(ctx, op.Chunk, storage.ChunkSynced); err != nil {
			loggerV1.Error(err, "pusher: failed to report sync status")
			return true, err
		}
	default:
		loggerV1.Error(err, "pusher: failed PushChunkToClosest")
		return true, err
	}

	return false, nil
}

func (s *Service) pushDirect(ctx context.Context, logger log.Logger, op *Op) error {
	loggerV1 := logger.V(1).Build()

	var err error

	defer func() {
		s.inflight.delete(op.identityAddress, op.Chunk.Stamp().BatchID())
		select {
		case op.Err <- err:
		default:
			loggerV1.Error(err, "pusher: failed to return error for direct upload")
		}
	}()

	ok, err := s.batchExist.Exists(op.Chunk.Stamp().BatchID())
	if !ok || err != nil {
		loggerV1.Warning(
			"stamp is no longer valid, skipping direct upload for chunk",
			"batch_id", hex.EncodeToString(op.Chunk.Stamp().BatchID()),
			"chunk_address", op.Chunk.Address(),
			"error", err,
		)
		return err
	}

	switch _, err = s.pushSyncer.PushChunkToClosest(ctx, op.Chunk); {
	case errors.Is(err, topology.ErrWantSelf):
		// store the chunk
		loggerV1.Debug("chunk stays here, i'm the closest node", "chunk_address", op.Chunk.Address())
		err = s.storer.ReservePutter().Put(ctx, op.Chunk)
		if err != nil {
			loggerV1.Error(err, "pusher: failed to store chunk")
		}
	case errors.Is(err, pushsync.ErrShallowReceipt):
		if s.shallowReceipt(op.identityAddress) {
			return err
		}
		// out of attempts for retry, swallow error
		err = nil
	case err != nil:
		loggerV1.Error(err, "pusher: failed PushChunkToClosest")
	}

	return err
}

func (s *Service) shallowReceipt(idAddress swarm.Address) bool {
	if s.attempts.try(idAddress) {
		return true
	}
	s.attempts.delete(idAddress)
	return false
}

func (s *Service) AddFeed(c <-chan *Op) {
	go func() {
		select {
		case s.smuggler <- c:
			s.logger.Info("got a chunk being smuggled")
		case <-s.quit:
		}
	}()
}

func (s *Service) Close() error {
	s.logger.Info("pusher shutting down")
	close(s.quit)

	// Wait for chunks worker to finish
	select {
	case <-s.chunksWorkerQuitC:
	case <-time.After(10 * time.Second):
	}
	return nil
}
