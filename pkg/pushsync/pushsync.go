// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	logger        logging.Logger
	metrics       metrics
}

type Options struct {
	Streamer      p2p.Streamer
	Storer        storage.Storer
	ClosestPeerer topology.ClosestPeerer
	Logger        logging.Logger
}

var (
	retryInterval        = 10 * time.Second // time interval between retries
	timeToWaitForReceipt = 3 * time.Second  // time to wait to get a receipt for a chunk
)

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

// handler handles chunk delivery from other node and forwards to its destination node.
// If the current node is the destination, it stores in the local store and sends a receipt
func (ps *PushSync) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()

	// Get the delivery
	ch, err := ps.getChunkDelivery(r)
	if err != nil {
		return err
	}

	// create chunk
	addr := swarm.NewAddress(ch.Address)
	chunk := swarm.NewChunk(addr, ch.Data)

	// Select the closest peer to forward the chunk
	peer, err := ps.peerSuggester.ClosestPeer(chunk.Address())
	if err != nil {
		// If i am the closest peer then store the chunk and send receipt
		if errors.Is(err, topology.ErrWantSelf) {

			// Store the chunk in the local store
			_, err := ps.storer.Put(ctx, storage.ModePutSync, chunk)
			if err != nil {
				return err
			}
			ps.metrics.TotalChunksStoredInDB.Inc()

			// Send a receipt immediately once the storage of the chunk is successfull
			err = ps.sendReceipt(w, chunk.Address().Bytes())
			if err != nil {
				return err
			}
			return nil
		}
		ps.logger.Debugf("Error finding the closest peer. Error : %s", err.Error())
		return err
	}

	//Dont forward this chunk to the same peer as the received peer
	if bytes.Equal(peer.Bytes(), p.Address.Bytes()) {
		ps.logger.Debugf("Could not forward delivery to the same peer as the received one.")
		return err
	}

	// Forward chunk to closest peer
	streamer, err := ps.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}
	defer streamer.Close()

	wc, rc := protobuf.NewWriterAndReader(streamer)

	if err := ps.sendChunkDelivery(wc, chunk); err != nil {
		return err
	}
	receiptRTTTimer := time.Now()

	receipt, err := ps.receiveReceipt(rc)
	if err != nil {
		return nil
	}
	timeSpent := float64(time.Since(receiptRTTTimer))
	ps.metrics.ReceiptRTT.Add(timeSpent)

	// Check if the receipt is valid
	if !bytes.Equal(chunk.Address().Bytes(), receipt.Address) {
		ps.metrics.InvalidReceiptReceived.Inc()
		return err
	}

	// forward the receipt to the received stream
	err = ps.sendReceipt(w, chunk.Address().Bytes())
	if err != nil {
		return err
	}

	return nil
}

func (ps *PushSync) getChunkDelivery(r protobuf.Reader) (ch pb.Delivery, err error) {
	if err = r.ReadMsg(&ch); err != nil {
		ps.metrics.ReceivedChunkErrorCounter.Inc()
		return ch, err
	}
	ps.metrics.ChunksSentCounter.Inc()
	return ch, nil
}

func (ps *PushSync) sendChunkDelivery(w protobuf.Writer, chunk swarm.Chunk) (err error) {
	startTimer := time.Now()
	if err = w.WriteMsgWithTimeout(timeToWaitForReceipt, &pb.Delivery{
		Address: chunk.Address().Bytes(),
		Data:    chunk.Data(),
	}); err != nil {
		ps.metrics.SendChunkErrorCounter.Inc()
		return err
	}

	timeSpent := float64(time.Since(startTimer))
	ps.metrics.SendChunkTimer.Add(timeSpent)
	ps.metrics.ChunksSentCounter.Inc()
	return nil
}

func (ps *PushSync) sendReceipt(w protobuf.Writer, addr []byte) (err error) {
	receipt := &pb.Receipt{
		Address: addr,
	}
	if err := w.WriteMsg(receipt); err != nil {
		ps.metrics.SendReceiptErrorCounter.Inc()
		return err
	}
	ps.metrics.ReceiptsSentCounter.Inc()
	return nil
}

func (ps *PushSync) receiveReceipt(r protobuf.Reader) (receipt pb.Receipt, err error) {
	if err := r.ReadMsg(&receipt); err != nil {
		ps.metrics.ReceiveReceiptErrorCounter.Inc()
		return receipt, err
	}
	ps.metrics.ReceiptsReceivedCounter.Inc()
	return receipt, nil
}

// chunksWorker is a loop that keeps looking for chunks that are locally uploaded ( by monitoring pushIndex )
// and pushes them to the closest peer and get a receipt.
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
			ps.metrics.TotalChunksToBeSentCounter.Inc()
			peer, err := ps.peerSuggester.ClosestPeer(ch.Address())
			if err != nil {
				if errors.Is(err, topology.ErrWantSelf) {
					if err := ps.storer.Set(ctx, storage.ModeSetSyncPush, ch.Address()); err != nil {
						ps.logger.Error("pushsync: error setting chunks to synced", "err", err)
					}
					continue
				}
			}

			// TODO: make this function as a go routine and process several chunks in parallel
			if err := ps.SendChunkAndReceiveReceipt(ctx, peer, ch); err != nil {
				ps.logger.Errorf("error while sending chunk or receiving receipt", "addr", ch.Address().String(), "err", err)
				continue
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

// sendChunkAndReceiveReceipt sends chunk to a given peer
// by opening a stream. It then waits for a receipt from that peer.
// Once the receipt is received within a given time frame it marks that this chunk
// as synced in the localstore.
func (ps *PushSync) SendChunkAndReceiveReceipt(ctx context.Context, peer swarm.Address, ch swarm.Chunk) error {
	streamer, err := ps.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}
	defer streamer.Close()

	w, r := protobuf.NewWriterAndReader(streamer)
	if err := ps.sendChunkDelivery(w, ch); err != nil {
		return err
	}
	receiptRTTTimer := time.Now()

	receipt, err := ps.receiveReceipt(r)
	if err != nil {
		return nil
	}
	timeSpent := float64(time.Since(receiptRTTTimer))
	ps.metrics.ReceiptRTT.Add(timeSpent)

	// Check if the receipt is valid
	if !bytes.Equal(ch.Address().Bytes(), receipt.Address) {
		ps.metrics.InvalidReceiptReceived.Inc()
		return err
	}

	// set chunk status to synced, insert to db GC index
	if err := ps.storer.Set(ctx, storage.ModeSetSyncPush, ch.Address()); err != nil {
		ps.metrics.ErrorSettingChunkToSynced.Inc()
		return err
	}

	ps.metrics.TotalChunksSynced.Inc()
	return nil
}
