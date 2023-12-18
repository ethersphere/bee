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
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pushsync"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/tracing"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "pusher"

type Op struct {
	Chunk  swarm.Chunk
	Err    chan error
	Direct bool
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
	validStamp        postage.ValidStampFn
	radius            func() (uint8, error)
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
	ConcurrentPushes  = 100              // how many chunks to push simultaneously
	DefaultRetryCount = 6
)

var (
	ErrInvalidAddress = errors.New("invalid address")
	ErrShallowReceipt = errors.New("shallow recipt")
)

func New(
	networkID uint64,
	storer Storer,
	radius func() (uint8, error),
	pushSyncer pushsync.PushSyncer,
	validStamp postage.ValidStampFn,
	logger log.Logger,
	tracer *tracing.Tracer,
	warmupTime time.Duration,
	retryCount int,
) *Service {
	p := &Service{
		networkID:         networkID,
		storer:            storer,
		pushSyncer:        pushSyncer,
		validStamp:        validStamp,
		radius:            radius,
		logger:            logger.WithName(loggerName).Register(),
		metrics:           newMetrics(),
		quit:              make(chan struct{}),
		chunksWorkerQuitC: make(chan struct{}),
		inflight:          newInflight(),
		attempts:          &attempts{retryCount: retryCount, attempts: make(map[string]int)},
		smuggler:          make(chan OpChan),
	}
	go p.chunksWorker(warmupTime, tracer)
	return p
}

// chunksWorker is a loop that keeps looking for chunks that are locally uploaded ( by monitoring pushIndex )
// and pushes them to the closest peer and get a receipt.
func (s *Service) chunksWorker(warmupTime time.Duration, tracer *tracing.Tracer) {
	defer close(s.chunksWorkerQuitC)
	select {
	case <-time.After(warmupTime):
	case <-s.quit:
		return
	}

	// fetch the network radius before starting pusher worker
	r, err := s.radius()
	if err != nil {
		s.logger.Error(err, "pusher: initial radius error")
	} else {
		s.logger.Info("pusher: warmup period complete", log.LogItem{"radius", r})
	}

	var (
		cctx, cancel      = context.WithCancel(context.Background())
		mtx               sync.Mutex
		wg                sync.WaitGroup
		span, logger, ctx = tracer.StartSpanFromContext(cctx, "pusher-sync-batch", s.logger)
		timer             = time.NewTimer(traceDuration)
		sem               = make(chan struct{}, ConcurrentPushes)
		cc                = make(chan *Op)
	)

	// inflight.set handles the backpressure for the maximum amount of inflight chunks
	// and duplicate handling.
	chunks, unsubscribe := s.storer.SubscribePush(cctx)
	defer func() {
		unsubscribe()
		cancel()
	}()

	ctxLogger := func() (context.Context, log.Logger) {
		mtx.Lock()
		defer mtx.Unlock()
		return ctx, logger
	}

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
		ctx, logger := ctxLogger()
		startTime := time.Now()

		if op.Direct {
			err = s.pushDirect(ctx, logger, op)
		} else {
			doRepeat, err = s.pushDeferred(ctx, logger, op)
		}

		if err != nil {
			s.metrics.TotalErrors.Inc()
			s.metrics.ErrorTime.Observe(time.Since(startTime).Seconds())
		}

		s.metrics.SyncTime.Observe(time.Since(startTime).Seconds())
		s.metrics.TotalSynced.Inc()
	}

	go func() {
		for {
			select {
			case <-s.quit:
				return
			case <-timer.C:
				// reset the span
				mtx.Lock()
				span.Finish()
				span, logger, ctx = tracer.StartSpanFromContext(cctx, "pusher-sync-batch", s.logger)
				mtx.Unlock()
			}
		}
	}()

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
			if s.inflight.set(op.Chunk) {
				if op.Direct {
					select {
					case op.Err <- nil:
					default:
						s.logger.Debug("chunk already in flight, skipping", log.LogItem{"chunk", op.Chunk.Address()})
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

	defer s.inflight.delete(op.Chunk)

	if _, err := s.validStamp(op.Chunk); err != nil {
		loggerV1.Warning(
			"stamp with is no longer valid, skipping syncing for chunk",
			log.LogItem{"batch_id", hex.EncodeToString(op.Chunk.Stamp().BatchID())},
			log.LogItem{"chunk_address", op.Chunk.Address()},
			log.LogItem{"error", err},
		)

		return false, errors.Join(err, s.storer.Report(ctx, op.Chunk, storage.ChunkCouldNotSync))
	}

	switch receipt, err := s.pushSyncer.PushChunkToClosest(ctx, op.Chunk); {
	case errors.Is(err, topology.ErrWantSelf):
		// store the chunk
		loggerV1.Debug("chunk stays here, i'm the closest node", log.LogItem{"chunk_address", op.Chunk.Address()})
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
	case err == nil:
		if err := s.checkReceipt(receipt, loggerV1); err != nil {
			loggerV1.Error(err, "pusher: failed checking receipt", log.LogItem{"chunk_address", op.Chunk.Address()})
			return true, err
		}
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

	var (
		receipt *pushsync.Receipt
		err     error
	)

	defer func() {
		s.inflight.delete(op.Chunk)
		select {
		case op.Err <- err:
		default:
			loggerV1.Error(err, "pusher: failed to return error for direct upload")
		}
	}()

	_, err = s.validStamp(op.Chunk)
	if err != nil {
		logger.Warning(
			"stamp with is no longer valid, skipping direct upload for chunk",
			log.LogItem{"batch_id", hex.EncodeToString(op.Chunk.Stamp().BatchID())},
			log.LogItem{"chunk_address", op.Chunk.Address()},
			log.LogItem{"error", err},
		)
		return err
	}

	switch receipt, err = s.pushSyncer.PushChunkToClosest(ctx, op.Chunk); {
	case errors.Is(err, topology.ErrWantSelf):
		// store the chunk
		loggerV1.Debug("chunk stays here, i'm the closest node", log.LogItem{"chunk_address", op.Chunk.Address()})
		err = s.storer.ReservePutter().Put(ctx, op.Chunk)
		if err != nil {
			loggerV1.Error(err, "pusher: failed to store chunk")
		}
	case err == nil:
		err = s.checkReceipt(receipt, loggerV1)
		if err != nil {
			loggerV1.Error(err, "pusher: failed checking receipt", log.LogItem{"chunk_address", op.Chunk.Address()})
		}
	default:
		loggerV1.Error(err, "pusher: failed PushChunkToClosest")
	}
	return err
}

func (s *Service) checkReceipt(receipt *pushsync.Receipt, loggerV1 log.Logger) error {
	addr := receipt.Address
	publicKey, err := crypto.Recover(receipt.Signature, addr.Bytes())
	if err != nil {
		return fmt.Errorf("pusher: receipt recover: %w", err)
	}

	peer, err := crypto.NewOverlayAddress(*publicKey, s.networkID, receipt.Nonce)
	if err != nil {
		return fmt.Errorf("pusher: receipt storer address: %w", err)
	}

	po := swarm.Proximity(addr.Bytes(), peer.Bytes())

	d, err := s.radius()
	if err != nil {
		return fmt.Errorf("pusher: storage radius: %w", err)
	}

	// if the receipt po is out of depth AND the receipt has not yet hit the maximum retry limit, reject the receipt.
	if po < d && s.attempts.try(addr) {
		s.metrics.ShallowReceiptDepth.WithLabelValues(strconv.Itoa(int(po))).Inc()
		s.metrics.ShallowReceipt.Inc()
		return fmt.Errorf("pusher: shallow receipt depth %d, want at least %d, chunk_address %s: %w", po, d, addr, ErrShallowReceipt)
	}
	loggerV1.Debug("chunk pushed", log.LogItem{"chunk_address", addr}, log.LogItem{"peer_address", peer}, log.LogItem{"proximity_order", po})
	s.metrics.ReceiptDepth.WithLabelValues(strconv.Itoa(int(po))).Inc()
	s.attempts.delete(addr)
	return nil
}

func (s *Service) AddFeed(c <-chan *Op) {
	go func() {
		select {
		case s.smuggler <- c:
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
	case <-time.After(6 * time.Second):
	}
	return nil
}
