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
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/tracing"

	"github.com/sirupsen/logrus"
)

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
	depther           topology.NeighborhoodDepther
	logger            logging.Logger
	tag               *tags.Tags
	metrics           metrics
	quit              chan struct{}
	chunksWorkerQuitC chan struct{}
	inflight          *inflight
	attempts          *attempts
	sem               chan struct{}
	smugler           chan OpChan
}

var (
	retryInterval    = 5 * time.Second  // time interval between retries
	traceDuration    = 30 * time.Second // duration for every root tracing span
	concurrentPushes = 50               // how many chunks to push simultaneously
	retryCount       = 6
)

var (
	ErrInvalidAddress = errors.New("invalid address")
	ErrShallowReceipt = errors.New("shallow recipt")
)

func New(networkID uint64, storer storage.Storer, depther topology.NeighborhoodDepther, pushSyncer pushsync.PushSyncer, validStamp postage.ValidStampFn, tagger *tags.Tags, logger logging.Logger, tracer *tracing.Tracer, warmupTime time.Duration) *Service {
	p := &Service{
		networkID:         networkID,
		storer:            storer,
		pushSyncer:        pushSyncer,
		validStamp:        validStamp,
		depther:           depther,
		tag:               tagger,
		logger:            logger,
		metrics:           newMetrics(),
		quit:              make(chan struct{}),
		chunksWorkerQuitC: make(chan struct{}),
		inflight:          newInflight(),
		attempts:          &attempts{attempts: make(map[string]int)},
		sem:               make(chan struct{}, concurrentPushes),
		smugler:           make(chan OpChan),
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
		timer             = time.NewTimer(traceDuration)
	)

	// inflight.set handles the backpressure for the maximum amount of inflight chunks
	// and duplicate handling.
	chunks, repeat, unsubscribe := s.storer.SubscribePush(ctx, s.inflight.set)
	go func() {
		<-s.quit
		unsubscribe()
		cancel()
		if !timer.Stop() {
			<-timer.C
		}
	}()

	ctxLogger := func() (context.Context, *logrus.Entry) {
		mtx.Lock()
		defer mtx.Unlock()
		return ctx, logger
	}

	push := func(op *Op) {
		s.metrics.TotalToPush.Inc()
		ctx, logger := ctxLogger()
		startTime := time.Now()
		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()
				<-s.sem
			}()
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
				logger.Tracef("pusher: cannot push chunk %s: %v", op.Chunk.Address().String(), err)
				return
			}
			if op.Err != nil {
				op.Err <- nil
			}
			s.metrics.TotalSynced.Inc()
		}()
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

	// fan-in channel
	cc := make(chan *Op)

	go func() {
		for ch := range chunks {
			// If the stamp is invalid, the chunk is not synced with the network
			// since other nodes would reject the chunk, so the chunk is marked as
			// synced which makes it available to the node but not to the network
			if err := s.valid(ch); err != nil {
				logger.Warningf("pusher: stamp with batch ID %x is no longer valid, skipping syncing for chunk %s: %v", ch.Stamp().BatchID(), ch.Address().String(), err)
				if err = s.storer.Set(ctx, storage.ModeSetSync, ch.Address()); err != nil {
					s.logger.Errorf("pusher: set sync: %v", err)
				}
			}
			cc <- &Op{Chunk: ch, Direct: false}
		}
	}()

	defer wg.Wait()

	for {
		select {
		case apiC := <-s.smugler:
			go func() {
				for op := range apiC {
					select {
					case cc <- op:
					case <-s.quit:
						return
					}
				}
			}()
		case op, ok := <-cc:
			if !ok {
				chunks = nil
				continue
			}

			select {
			case s.sem <- struct{}{}:
			case <-s.quit:
				return
			}

			push(op)
		case <-s.quit:
			return
		}
	}
}

func (s *Service) pushChunk(ctx context.Context, ch swarm.Chunk, logger *logrus.Entry, directUpload bool) error {
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
		logger.Tracef("pusher: chunk %s stays here, i'm the closest node", ch.Address().String())
	} else if err = s.checkReceipt(receipt); err != nil {
		return err
	}
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
				logger.Debugf("pusher: increment synced: %v", err)
				return nil // tag error is non-fatal
			}
			if wantSelf {
				err = t.Inc(tags.StateSent)
				if err != nil {
					logger.Debugf("pusher: increment sent: %v", err)
					return nil // tag error is non-fatal
				}
			}
		}
	}
	return nil
}

func (s *Service) checkReceipt(receipt *pushsync.Receipt) error {
	addr := receipt.Address
	publicKey, err := crypto.Recover(receipt.Signature, addr.Bytes())
	if err != nil {
		return fmt.Errorf("pusher: receipt recover: %w", err)
	}

	peer, err := crypto.NewOverlayAddress(*publicKey, s.networkID, receipt.BlockHash)
	if err != nil {
		return fmt.Errorf("pusher: receipt storer address: %w", err)
	}

	po := swarm.Proximity(addr.Bytes(), peer.Bytes())
	d := s.depther.NeighborhoodDepth()
	if po < d && s.attempts.try(addr) {
		s.metrics.ShallowReceiptDepth.WithLabelValues(strconv.Itoa(int(po))).Inc()
		return fmt.Errorf("pusher: shallow receipt depth %d, want at least %d", po, d)
	}
	s.logger.Tracef("pusher: pushed chunk %s to node %s, receipt depth %d", addr, peer, po)
	s.metrics.ReceiptDepth.WithLabelValues(strconv.Itoa(int(po))).Inc()
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
	s.smugler <- c
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
