// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/pushsync/pb"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

const (
	protocolName    = "pushsync"
	protocolVersion = "1.0.0"
	streamName      = "pushsync"
)

type PushSync struct {
	streamer      p2p.Streamer
	storer        storage.Storer
	peerSuggester topology.ClosestPeerer
	quit          chan struct{}

	logger  logging.Logger
	metrics metrics
}

type Options struct {
	Streamer      p2p.Streamer
	Storer        storage.Storer
	ClosestPeerer topology.ClosestPeerer

	Logger logging.Logger
}

var retryInterval = 10 * time.Second // time interval between retries

func New(o Options) *PushSync {
	ps := &PushSync{
		streamer:      o.Streamer,
		storer:        o.Storer,
		peerSuggester: o.ClosestPeerer,
		logger:        o.Logger,
		metrics:       newMetrics(),
		quit:          make(chan struct{}),
	}

	ctx := context.Background()
	go ps.chunksWorker(ctx)

	return ps
}

func (s *PushSync) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Handler: s.handler,
			},
		},
	}
}

func (ps *PushSync) Close() error {
	close(ps.quit)
	return nil
}

func (ps *PushSync) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	// handle chunk delivery from other node
	_, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()

	var ch pb.Delivery

	if err := r.ReadMsg(&ch); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}

	// create chunk and store it in the local store
	addr := swarm.NewAddress(ch.Address)
	chunk := swarm.NewChunk(addr, ch.Data)
	_, err := ps.storer.Put(ctx, storage.ModePutSync, chunk)
	if err != nil {
		return err
	}

	// push this to the closest node too
	peer, err := ps.peerSuggester.ClosestPeer(addr)
	if err != nil {
		if errors.Is(err, topology.ErrWantSelf) {
			// i'm the closest - nothing to do
			return nil
		}
		return err
	}
	if err := ps.sendChunkMsg(ctx, peer, chunk); err != nil {
		ps.metrics.SendChunkErrorCounter.Inc()
		return err
	}

	return ps.storer.Set(ctx, storage.ModeSetSyncPush, chunk.Address())
}

func (ps *PushSync) chunksWorker(ctx context.Context) {
	var chunks <-chan swarm.Chunk
	var unsubscribe func()

	// timer, initially set to 0 to fall through select case on timer.C for initialisation
	timer := time.NewTimer(0)
	defer timer.Stop()
	chunksInBatch := -1

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
			ps.metrics.SendChunkCounter.Inc()
			peer, err := ps.peerSuggester.ClosestPeer(ch.Address())
			if err != nil {
				if errors.Is(err, topology.ErrWantSelf) {
					if err := ps.storer.Set(ctx, storage.ModeSetSyncPush, ch.Address()); err != nil {
						ps.logger.Error("pushsync: error setting chunks to synced", "err", err)
					}
					continue
				}
			}

			if err := ps.sendChunkMsg(ctx, peer, ch); err != nil {
				ps.metrics.SendChunkErrorCounter.Inc()
				ps.logger.Errorf("error sending chunk", "addr", ch.Address().String(), "err", err)
			}

			// set chunk status to synced, insert to db GC index
			if err := ps.storer.Set(ctx, storage.ModeSetSyncPush, ch.Address()); err != nil {
				ps.logger.Error("pushsync: error setting chunks to synced", "err", err)
			}

			// retry interval timer triggers starting from new
		case <-timer.C:
			// initially timer is set to go off as well as every time we hit the end of push index
			startTime := time.Now()

			// if subscribe was running, stop it
			if unsubscribe != nil {
				unsubscribe()
			}

			// and start iterating on Push index from the beginning
			chunks, unsubscribe = ps.storer.SubscribePush(ctx)

			// reset timer to go off after retryInterval
			timer.Reset(retryInterval)

			timeSpent := float64(time.Since(startTime))
			ps.metrics.MarkAndSweepTimer.Add(timeSpent)

		case <-ps.quit:
			if unsubscribe != nil {
				unsubscribe()
			}
			return
		}
	}
}

// sendChunkMsg sends chunks to their destination
// by opening a stream to the closest peer
func (ps *PushSync) sendChunkMsg(ctx context.Context, peer swarm.Address, ch swarm.Chunk) error {
	startTimer := time.Now()
	streamer, err := ps.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}
	defer streamer.Close()

	w, _ := protobuf.NewWriterAndReader(streamer)

	if err := w.WriteMsg(&pb.Delivery{
		Address: ch.Address().Bytes(),
		Data:    ch.Data(),
	}); err != nil {
		return err
	}

	timeSpent := float64(time.Since(startTimer))
	ps.metrics.SendChunkTimer.Add(timeSpent)
	return err
}
