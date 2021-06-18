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
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/tracing"
)

var (
	concurrentPushes = 1000 // how many chunks to push simultaneously
	retryCount       = 3
)

type Service struct {
	networkID  uint64
	storer     storage.Storer
	pushSyncer pushsync.PushSyncer
	depther    topology.NeighborhoodDepther
	logger     logging.Logger
	tag        *tags.Tags
	tracer     *tracing.Tracer
	metrics    metrics
	quit       chan struct{}
	inflight   *inflight
	attempts   *attempts
}

func New(networkID uint64, storer storage.Storer, depther topology.NeighborhoodDepther, pushSyncer pushsync.PushSyncer, tagger *tags.Tags, logger logging.Logger, tracer *tracing.Tracer, warmupTime time.Duration) *Service {
	service := &Service{
		networkID:  networkID,
		storer:     storer,
		pushSyncer: pushSyncer,
		depther:    depther,
		tag:        tagger,
		logger:     logger,
		tracer:     tracer,
		metrics:    newMetrics(),
		quit:       make(chan struct{}),
		inflight:   newInflight(),
		attempts:   &attempts{attempts: make(map[string]int)},
	}
	go service.start(warmupTime)
	return service
}

// start is a loop that keeps looking for chunks that are locally uploaded ( by monitoring pushIndex )
// and pushes them to the closest peer and get a receipt.
func (s *Service) start(warmupTime time.Duration) {
	// retryCounter := make(map[string]int)

	select {
	case <-time.After(warmupTime):
		s.logger.Info("pusher: warmup period complete, worker starting.")
	case <-s.quit:
		return
	}
	ctx := context.Background()
	chunks, repeat, unsubscribe := s.storer.SubscribePush(ctx, s.inflight.set)
	go func() {
		<-s.quit
		unsubscribe()
	}()

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for ch := range chunks {
		s.metrics.TotalToPush.Inc()
		go func(cctx context.Context, ch swarm.Chunk) {
			startTime := time.Now()
			if err := s.pushChunk(cctx, ch); err != nil {
				repeat()
				s.metrics.TotalErrors.Inc()
				s.metrics.ErrorTime.Observe(time.Since(startTime).Seconds())
				s.logger.Tracef("pusher: cannot push chunk %s: %v", ch.Address().String(), err)
				return
			}
			s.metrics.TotalSynced.Inc()
			s.metrics.SyncTime.Observe(time.Since(startTime).Seconds())
		}(cctx, ch)
	}
}

func (s *Service) Close() error {
	s.logger.Info("pusher shutting down")
	close(s.quit)
	return nil
}

func (s *Service) pushChunk(ctx context.Context, ch swarm.Chunk) error {
	defer s.inflight.delete(ch)
	var wantSelf bool
	// Later when we process receipt, get the receipt and process it
	// for now ignoring the receipt and checking only for error
	receipt, err := s.pushSyncer.PushChunkToClosest(ctx, ch)
	if err != nil {
		if !errors.Is(err, topology.ErrWantSelf) {
			return err
		}
		// we are the closest ones - this is fine
		// this is to make sure that the sent number does not diverge from the synced counter
		// the edge case is on the uploader node, in the case where the uploader node is
		// connected to other nodes, but is the closest one to the chunk.
		wantSelf = true
	} else if err = s.checkReceipt(receipt); err != nil {
		return err
	}
	// receipt is nil only if wantSelf is true so better in else clause
	// if receipt != nil {

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
				return fmt.Errorf("pusher: increment synced: %v", err)
			}
			if wantSelf {
				err = t.Inc(tags.StateSent)
				if err != nil {
					return fmt.Errorf("pusher: increment sent: %w", err)
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

type inflight struct {
	mtx      sync.Mutex
	inflight map[string]struct{}
	slots    chan struct{}
}

func newInflight() *inflight {
	return &inflight{
		inflight: make(map[string]struct{}),
		slots:    make(chan struct{}, concurrentPushes),
	}
}

func (i *inflight) delete(ch swarm.Chunk) {
	i.mtx.Lock()
	delete(i.inflight, ch.Address().ByteString())
	i.mtx.Unlock()
	<-i.slots
}

func (i *inflight) set(addr []byte) bool {
	i.mtx.Lock()
	key := string(addr)
	if _, ok := i.inflight[key]; ok {
		i.mtx.Unlock()
		return true
	}
	i.inflight[key] = struct{}{}
	i.mtx.Unlock()
	i.slots <- struct{}{}
	return false
}

type attempts struct {
	mtx      sync.Mutex
	attempts map[string]int
}

func (a *attempts) try(ch swarm.Address) bool {
	key := ch.ByteString()
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.attempts[key]++
	if a.attempts[key] == retryCount {
		delete(a.attempts, key)
		return false
	}
	return true
}
