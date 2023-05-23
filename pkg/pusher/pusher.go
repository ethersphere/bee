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
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
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

type Service struct {
	networkID         uint64
	storer            storage.Storer
	pushSyncer        pushsync.PushSyncer
	validStamp        postage.ValidStampFn
	radius            func() uint8
	logger            log.Logger
	tag               *tags.Tags
	metrics           metrics
	quit              chan struct{}
	chunksWorkerQuitC chan struct{}
	inflight          *inflight
	attempts          *attempts
	smuggler          chan OpChan
}

const (
	traceDuration     = 30 * time.Second // duration for every root tracing span
	concurrentPushes  = 100              // how many chunks to push simultaneously
	DefaultRetryCount = 6
)

var (
	ErrInvalidAddress = errors.New("invalid address")
	ErrShallowReceipt = errors.New("shallow recipt")
)

const chunkStoreTimeout = 2 * time.Second

func New(networkID uint64, storer storage.Storer, pushSyncer pushsync.PushSyncer, validStamp postage.ValidStampFn, tagger *tags.Tags, radius func() uint8, logger log.Logger, tracer *tracing.Tracer, warmupTime time.Duration, retryCount int) *Service {
	p := &Service{
		networkID:         networkID,
		storer:            storer,
		pushSyncer:        pushSyncer,
		validStamp:        validStamp,
		radius:            radius,
		tag:               tagger,
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
		s.logger.Info("pusher: warmup period complete, worker starting.")
	case <-s.quit:
		return
	}

	var (
		cctx, cancel      = context.WithCancel(context.Background())
		mtx               sync.Mutex
		wg                sync.WaitGroup
		span, logger, ctx = tracer.StartSpanFromContext(cctx, "pusher-sync-batch", s.logger)
		loggerV1          = logger.V(1).Build()
		timer             = time.NewTimer(traceDuration)
		sem               = make(chan struct{}, concurrentPushes)
		cc                = make(chan *Op)
	)

	// inflight.set handles the backpressure for the maximum amount of inflight chunks
	// and duplicate handling.
	chunks, repeat, unsubscribe := s.storer.SubscribePush(ctx, s.inflight.set)
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
		defer func() {
			wg.Done()
			<-sem
		}()

		s.metrics.TotalToPush.Inc()
		ctx, logger := ctxLogger()
		startTime := time.Now()

		if err := s.valid(op.Chunk); err != nil {
			logger.Warning("stamp with is no longer valid, skipping syncing for chunk", "batch_id", hex.EncodeToString(op.Chunk.Stamp().BatchID()), "direct_upload", op.Direct, "chunk_address", op.Chunk.Address(), "error", err)
			if op.Direct {
				if op.Err != nil {
					op.Err <- err
				}
			} else {
				ctx, cancel := context.WithTimeout(ctx, chunkStoreTimeout)
				defer cancel()
				if err = s.storer.Set(ctx, storage.ModeSetSync, op.Chunk.Address()); err != nil {
					s.logger.Error(err, "set sync failed")
				}
			}
			return
		}

		if err := s.pushChunk(ctx, op.Chunk, logger, op.Direct); err != nil {
			// warning: ugly flow control
			// if errc is set it means we are in a direct push,
			// we therefore communicate the error into the channel
			// otherwise we assume this is a buffered upload and
			// therefore we repeat().
			if op.Err != nil {
				op.Err <- err
			}
			repeat()
			s.metrics.TotalErrors.Inc()
			s.metrics.ErrorTime.Observe(time.Since(startTime).Seconds())
			loggerV1.Debug("cannot push chunk", "chunk_address", op.Chunk.Address(), "error", err)
			return
		}
		if op.Err != nil {
			op.Err <- nil
		}
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
				loggerV1 = logger.V(1).Build()
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

func (s *Service) pushChunk(ctx context.Context, ch swarm.Chunk, logger log.Logger, directUpload bool) error {
	loggerV1 := logger.V(1).Build()

	defer s.inflight.delete(ch)
	var wantSelf bool
	// Later when we process receipt, get the receipt and process it
	// for now ignoring the receipt and checking only for error
	receipt, err := s.pushSyncer.PushChunkToClosest(ctx, ch)
	if err != nil {
		// when doing a direct upload from a light node this will never happen because the light node
		// never includes self in kademlia iterator. This is only hit when doing a direct upload from a full node
		if directUpload && errors.Is(err, topology.ErrWantSelf) {
			return err
		}
		if !errors.Is(err, topology.ErrWantSelf) {
			return err
		}
		// we are the closest ones - this is fine
		// this is to make sure that the sent number does not diverge from the synced counter
		// the edge case is on the uploader node, in the case where the uploader node is
		// connected to other nodes, but is the closest one to the chunk.
		wantSelf = true
		loggerV1.Debug("chunk stays here, i'm the closest node", "chunk_address", ch.Address())
		if _, err = s.storer.Put(ctx, storage.ModePutSync, ch); err != nil {
			return fmt.Errorf("pusher: put sync: %w", err)
		}
	} else if err = s.checkReceipt(receipt); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err = s.storer.Set(ctx, storage.ModeSetSync, ch.Address()); err != nil {
		return fmt.Errorf("pusher: set sync: %w", err)
	}
	if ch.TagID() > 0 {
		// for individual chunks uploaded using the
		// /chunks api endpoint the tag will be missing
		// by default, unless the api consumer specifies one
		t, err := s.tag.Get(ch.TagID())
		if err == nil && t != nil {
			err = t.Inc(tags.StateSynced)
			if err != nil {
				logger.Debug("increment synced failed", "error", err)
				return nil // tag error is non-fatal
			}
			if wantSelf {
				err = t.Inc(tags.StateSent)
				if err != nil {
					logger.Debug("increment sent failed", "error", err)
					return nil // tag error is non-fatal
				}
			}
		}
	}
	return nil
}

func (s *Service) checkReceipt(receipt *pushsync.Receipt) error {
	loggerV1 := s.logger.V(1).Register()

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

	d := s.radius()

	// if the receipt po is out of depth AND the receipt has not yet hit the maximum retry limit, reject the receipt.
	if po < d && s.attempts.try(addr) {
		s.metrics.ShallowReceiptDepth.WithLabelValues(strconv.Itoa(int(po))).Inc()
		s.metrics.ShallowReceipt.Inc()
		return fmt.Errorf("pusher: shallow receipt depth %d, want at least %d", po, d)
	}
	loggerV1.Debug("chunk pushed", "chunk_address", addr, "peer_address", peer, "proximity_order", po)
	s.metrics.ReceiptDepth.WithLabelValues(strconv.Itoa(int(po))).Inc()
	s.attempts.delete(addr)
	return nil
}

// valid checks whether the stamp for a chunk is valid before sending
// it out on the network.
func (s *Service) valid(ch swarm.Chunk) error {
	stampBytes, err := ch.Stamp().MarshalBinary()
	if err != nil {
		return fmt.Errorf("pusher: valid stamp marshal: %w", err)
	}
	_, err = s.validStamp(ch, stampBytes)
	if err != nil {
		return fmt.Errorf("pusher: valid stamp: %w", err)
	}
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
