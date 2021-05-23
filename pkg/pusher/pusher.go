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
	"crypto/ecdsa"
	"errors"
	"fmt"
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
	"github.com/opentracing/opentracing-go"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

type Service struct {
	networkID         uint64
	storer            storage.Storer
	pushSyncer        pushsync.PushSyncer
	logger            logging.Logger
	tag               *tags.Tags
	tracer            *tracing.Tracer
	metrics           metrics
	quit              chan struct{}
	chunksWorkerQuitC chan struct{}
	retryMap          map[string]time.Time
	retryMux          sync.Mutex
}

var (
	retryInterval  = 5 * time.Second // time interval between retries
	expiresAfter   = 5 * time.Minute // time interval between retries
	concurrentJobs = 10              // how many chunks to push simultaneously
)

var ErrInvalidAddress = errors.New("invalid address")

func New(networkID uint64, storer storage.Storer, peerSuggester topology.ClosestPeerer, pushSyncer pushsync.PushSyncer, tagger *tags.Tags, logger logging.Logger, tracer *tracing.Tracer) *Service {
	service := &Service{
		networkID:         networkID,
		storer:            storer,
		pushSyncer:        pushSyncer,
		tag:               tagger,
		logger:            logger,
		tracer:            tracer,
		metrics:           newMetrics(),
		quit:              make(chan struct{}),
		chunksWorkerQuitC: make(chan struct{}),
		retryMap:          make(map[string]time.Time),
	}
	go service.chunksWorker()
	return service
}

// chunksWorker is a loop that keeps looking for chunks that are locally uploaded ( by monitoring pushIndex )
// and pushes them to the closest peer and get a receipt.
func (s *Service) chunksWorker() {
	var (
		chunks        <-chan swarm.Chunk
		unsubscribe   func()
		timer         = time.NewTimer(0) // timer, initially set to 0 to fall through select case on timer.C for initialisation
		chunksInBatch = -1
		cctx, cancel  = context.WithCancel(context.Background())
		ctx           = cctx
		sem           = make(chan struct{}, concurrentJobs)
		span          opentracing.Span
		logger        *logrus.Entry
		requestGroup  singleflight.Group
	)
	defer timer.Stop()
	defer close(s.chunksWorkerQuitC)
	go func() {
		<-s.quit
		cancel()
	}()

LOOP:

	for {
		select {
		// handle incoming chunks
		case ch, more := <-chunks:
			// if no more, set to nil, reset timer to finalise batch
			if !more {
				chunks = nil
				var dur time.Duration
				if chunksInBatch == 0 {
					dur = 500 * time.Millisecond
				}
				timer.Reset(dur)
				break
			}

			if span == nil {
				span, logger, ctx = s.tracer.StartSpanFromContext(cctx, "pusher-sync-batch", s.logger)
			}

			// postpone a retry only after we've finished processing everything in index
			timer.Reset(retryInterval)
			chunksInBatch++
			s.metrics.TotalToPush.Inc()

			select {
			case sem <- struct{}{}:
			case <-s.quit:
				if unsubscribe != nil {
					unsubscribe()
				}
				if span != nil {
					span.Finish()
				}

				return
			}

			func(ctx context.Context, ch swarm.Chunk) {
				_ = requestGroup.DoChan(ch.Address().String(), func() (_ interface{}, _ error) {
					var (
						err        error
						startTime  = time.Now()
						t          *tags.Tag
						wantSelf   bool
						storerPeer swarm.Address
					)
					defer func() {
						if err == nil {
							s.metrics.TotalSynced.Inc()
							s.metrics.SyncTime.Observe(time.Since(startTime).Seconds())
							// only print this if there was no error while sending the chunk
							logger.Tracef("pusher: pushed chunk %s to node %s", ch.Address().String(), storerPeer.String())
						} else {
							s.metrics.TotalErrors.Inc()
							s.metrics.ErrorTime.Observe(time.Since(startTime).Seconds())
							logger.Tracef("pusher: cannot push chunk %s: %v", ch.Address().String(), err)
						}
						<-sem
					}()

					// Later when we process receipt, get the receipt and process it
					// for now ignoring the receipt and checking only for error
					receipt, err := s.pushSyncer.PushChunkToClosest(ctx, ch)
					if err != nil {
						if errors.Is(err, topology.ErrWantSelf) {
							// we are the closest ones - this is fine
							// this is to make sure that the sent number does not diverge from the synced counter
							// the edge case is on the uploader node, in the case where the uploader node is
							// connected to other nodes, but is the closest one to the chunk.
							wantSelf = true
						} else {
							if retry, clean := s.retry(ch.Address()); retry {
								return
							} else {
								clean()
								_ = s.storer.Set(ctx, storage.ModeSetRemove, ch.Address())
							}
						}
					}

					if receipt != nil {
						var publicKey *ecdsa.PublicKey
						publicKey, err = crypto.Recover(receipt.Signature, receipt.Address.Bytes())
						if err != nil {
							err = fmt.Errorf("pusher: receipt recover: %w", err)
							return

						}

						storerPeer, err = crypto.NewOverlayAddress(*publicKey, s.networkID)
						if err != nil {
							err = fmt.Errorf("pusher: receipt storer address: %w", err)
							return

						}
					}

					if err = s.storer.Set(ctx, storage.ModeSetSync, ch.Address()); err != nil {
						err = fmt.Errorf("pusher: set sync: %w", err)
						return

					}

					t, err = s.tag.Get(ch.TagID())
					if err == nil && t != nil {
						err = t.Inc(tags.StateSynced)
						if err != nil {
							err = fmt.Errorf("pusher: increment synced: %v", err)
							return
						}
						if wantSelf {
							err = t.Inc(tags.StateSent)
							if err != nil {
								err = fmt.Errorf("pusher: increment sent: %w", err)
								return

							}
						}
					}
					return
				})

			}(ctx, ch)
		case <-timer.C:
			// initially timer is set to go off as well as every time we hit the end of push index
			startTime := time.Now()

			// if subscribe was running, stop it
			if unsubscribe != nil {
				unsubscribe()
			}

			chunksInBatch = 0

			// and start iterating on Push index from the beginning
			chunks, unsubscribe = s.storer.SubscribePush(ctx)

			// reset timer to go off after retryInterval
			timer.Reset(retryInterval)
			s.metrics.MarkAndSweepTime.Observe(time.Since(startTime).Seconds())

			if span != nil {
				span.Finish()
				span = nil
			}

		case <-s.quit:
			if unsubscribe != nil {
				unsubscribe()
			}
			if span != nil {
				span.Finish()
			}

			break LOOP
		}
	}

	// wait for all pending push operations to terminate
	closeC := make(chan struct{})
	go func() {
		defer func() { close(closeC) }()
		for i := 0; i < cap(sem); i++ {
			sem <- struct{}{}
		}
	}()

	select {
	case <-closeC:
	case <-time.After(5 * time.Second):
		s.logger.Warning("pusher shutting down with pending operations")
	}
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

func (s *Service) retry(addr swarm.Address) (bool, func()) {

	s.retryMux.Lock()
	defer s.retryMux.Unlock()

	if expiration, ok := s.retryMap[addr.String()]; !ok {
		s.retryMap[addr.String()] = time.Now().Add(expiresAfter)
		return true, nil
	} else if time.Now().Before(expiration) {
		return true, nil
	}

	return false, func() {
		s.retryMux.Lock()
		defer s.retryMux.Unlock()
		delete(s.retryMap, addr.String())
	}
}
