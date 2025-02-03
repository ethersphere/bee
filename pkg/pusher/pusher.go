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
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	olog "github.com/opentracing/opentracing-go/log"
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

type Service struct {
	networkID         uint64
	storer            Storer
	pushSyncer        pushsync.PushSyncer
	batchExist        postage.BatchExist
	logger            log.Logger
	metrics           metrics
	quit              chan struct{}
	chunksWorkerQuitC chan struct{}
	inflight          *inflight
	attempts          *attempts
	smuggler          chan OpChan
}

const (
	traceDuration     = 30 * time.Second // duration for every root tracing span
	ConcurrentPushes  = swarm.Branches   // how many chunks to push simultaneously
	DefaultRetryCount = 6
)

func New(
	networkID uint64,
	storer Storer,
	pushSyncer pushsync.PushSyncer,
	batchExist postage.BatchExist,
	logger log.Logger,
	warmupTime time.Duration,
	retryCount int,
) *Service {
	p := &Service{
		networkID:         networkID,
		storer:            storer,
		pushSyncer:        pushSyncer,
		batchExist:        batchExist,
		logger:            logger.WithName(loggerName).Register(),
		metrics:           newMetrics(),
		quit:              make(chan struct{}),
		chunksWorkerQuitC: make(chan struct{}),
		inflight:          newInflight(),
		attempts:          &attempts{retryCount: retryCount, attempts: make(map[string]int)},
		smuggler:          make(chan OpChan),
	}
	go p.chunksWorker(warmupTime)
	return p
}

// chunksWorker is a loop that keeps looking for chunks that are locally uploaded ( by monitoring pushIndex )
// and pushes them to the closest peer and get a receipt.
func (s *Service) chunksWorker(warmupTime time.Duration) {
	defer close(s.chunksWorkerQuitC)
	select {
	case <-time.After(warmupTime):
	case <-s.quit:
		return
	}

	var (
		ctx, cancel = context.WithCancel(context.Background())
		sem         = make(chan struct{}, ConcurrentPushes)
		cc          = make(chan *Op)
	)

	// inflight.set handles the backpressure for the maximum amount of inflight chunks
	// and duplicate handling.
	chunks, unsubscribe := s.storer.SubscribePush(ctx)
	defer func() {
		unsubscribe()
		cancel()
	}()

	var wg sync.WaitGroup

	push := func(op *Op) {
		var (
			err      error
			doRepeat bool
		)

		defer func() {
			// no peer was found which may mean that the node is suffering from connections issues
			// we must slow down the pusher to prevent constant retries
			if errors.Is(err, topology.ErrNotFound) {
				select {
				case <-time.After(time.Second * 5):
				case <-s.quit:
				}
			}

			wg.Done()
			<-sem
			if doRepeat {
				select {
				case cc <- op:
				case <-s.quit:
				}
			}
		}()

		s.metrics.TotalToPush.Inc()
		startTime := time.Now()

		spanCtx := ctx
		if op.Span != nil {
			spanCtx = tracing.WithContext(spanCtx, op.Span.Context())
		} else {
			op.Span = opentracing.NoopTracer{}.StartSpan("noOp")
		}

		if op.Direct {
			err = s.pushDirect(spanCtx, s.logger, op)
		} else {
			doRepeat, err = s.pushDeferred(spanCtx, s.logger, op)
		}

		if err != nil {
			s.metrics.TotalErrors.Inc()
			s.metrics.ErrorTime.Observe(time.Since(startTime).Seconds())
			ext.LogError(op.Span, err)
		} else {
			op.Span.LogFields(olog.Bool("success", true))
		}

		s.metrics.SyncTime.Observe(time.Since(startTime).Seconds())
		s.metrics.TotalSynced.Inc()
	}

	go func() {
		for {
			select {
			case ch, ok := <-chunks:
				if !ok {
					chunks = nil
					continue
				}
				select {
				case cc <- &Op{Chunk: ch, Direct: false}:
				case <-s.quit:
					return
				}
			case apiC := <-s.smuggler:
				go func() {
					for {
						select {
						case op := <-apiC:
							select {
							case cc <- op:
							case <-s.quit:
								return
							}
						case <-s.quit:
							return
						}
					}
				}()
			case <-s.quit:
				return
			}
		}
	}()

	defer wg.Wait()

	for {
		select {
		case op := <-cc:
			idAddress, err := storage.IdentityAddress(op.Chunk)
			if err != nil {
				op.Err <- err
				continue
			}
			op.identityAddress = idAddress
			if s.inflight.set(idAddress, op.Chunk.Stamp().BatchID()) {
				if op.Direct {
					select {
					case op.Err <- nil:
					default:
						s.logger.Debug("chunk already in flight, skipping", "chunk", op.Chunk.Address())
					}
				}
				continue
			}
			select {
			case sem <- struct{}{}:
				wg.Add(1)
				go push(op)
			case <-s.quit:
				return
			}
		case <-s.quit:
			return
		}
	}

}

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
