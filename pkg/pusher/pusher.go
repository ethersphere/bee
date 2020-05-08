// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher

import (
	"context"
	"errors"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

type Service struct {
	storer            storage.Storer
	peerSuggester     topology.ClosestPeerer
	pushSyncer        pushsync.PushSyncer
	logger            logging.Logger
	metrics           metrics
	quit              chan struct{}
	chunksWorkerQuitC chan struct{}
}

type Options struct {
	Storer        storage.Storer
	PeerSuggester topology.ClosestPeerer
	PushSyncer    pushsync.PushSyncer
	Logger        logging.Logger
}

var retryInterval = 10 * time.Second // time interval between retries

func New(o Options) *Service {
	service := &Service{
		storer:            o.Storer,
		peerSuggester:     o.PeerSuggester,
		pushSyncer:        o.PushSyncer,
		logger:            o.Logger,
		metrics:           newMetrics(),
		quit:              make(chan struct{}),
		chunksWorkerQuitC: make(chan struct{}),
	}
	go service.chunksWorker()
	return service
}

// chunksWorker is a loop that keeps looking for chunks that are locally uploaded ( by monitoring pushIndex )
// and pushes them to the closest peer and get a receipt.
func (s *Service) chunksWorker() {
	var chunks <-chan swarm.Chunk
	var unsubscribe func()
	// timer, initially set to 0 to fall through select case on timer.C for initialisation
	timer := time.NewTimer(0)
	defer timer.Stop()
	defer close(s.chunksWorkerQuitC)
	chunksInBatch := -1
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-s.quit
		cancel()
	}()
	for {
		select {
		// handle incoming chunks
		case ch, more := <-chunks:
			// if no more, set to nil, reset timer to 0 to finalise batch immediately
			if !more {
				chunks = nil
				var dur time.Duration
				if chunksInBatch == 0 {
					dur = 500 * time.Millisecond
				}
				timer.Reset(dur)
				break
			}

			chunksInBatch++
			s.metrics.TotalChunksToBeSentCounter.Inc()
			peer, err := s.peerSuggester.ClosestPeer(ch.Address())
			if err != nil {
				if errors.Is(err, topology.ErrWantSelf) {
					// set chunk status to synced
					s.setChunkAsSynced(ctx, ch.Address())
					continue
				}
			}

			_, err = s.pushSyncer.SendChunkAndReceiveReceipt(ctx, peer, ch)
			if err != nil {
				s.logger.Errorf("pusher: error while sending chunk or receiving receipt: %v", err)
				continue
			}

			// set chunk status to synced
			s.setChunkAsSynced(ctx, ch.Address())
			continue

			// retry interval timer triggers starting from new
		case <-timer.C:
			// initially timer is set to go off as well as every time we hit the end of push index
			startTime := time.Now()

			// if subscribe was running, stop it
			if unsubscribe != nil {
				unsubscribe()
			}

			// and start iterating on Push index from the beginning
			chunks, unsubscribe = s.storer.SubscribePush(ctx)

			// reset timer to go off after retryInterval
			timer.Reset(retryInterval)
			s.metrics.MarkAndSweepTimer.Observe(time.Since(startTime).Seconds())

		case <-s.quit:
			if unsubscribe != nil {
				unsubscribe()
			}
			return
		}
	}
}

func (s *Service) setChunkAsSynced(ctx context.Context, addr swarm.Address) {
	if err := s.storer.Set(ctx, storage.ModeSetSyncPush, addr); err != nil {
		s.logger.Errorf("pusher: error setting chunk as synced: %v", err)
		s.metrics.ErrorSettingChunkToSynced.Inc()
	} else {
		s.metrics.TotalChunksSynced.Inc()
	}
}

func (s *Service) Close() error {
	close(s.quit)

	// Wait for chunks worker to finish
	select {
	case <-s.chunksWorkerQuitC:
	case <-time.After(3 * time.Second):
	}
	return nil
}
