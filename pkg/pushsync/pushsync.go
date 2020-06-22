// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
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
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
)

const (
	protocolName    = "pushsync"
	protocolVersion = "1.0.0"
	streamName      = "pushsync"
)

type PushSyncer interface {
	PushChunkToClosest(ctx context.Context, ch swarm.Chunk) (*Receipt, error)
}

type Receipt struct {
	Address swarm.Address
}

type PushSync struct {
	streamer      p2p.Streamer
	storer        storage.Putter
	peerSuggester topology.ClosestPeerer
	tagger        *tags.Tags
	logger        logging.Logger
	metrics       metrics
}

type Options struct {
	Streamer      p2p.Streamer
	Storer        storage.Putter
	ClosestPeerer topology.ClosestPeerer
	Tagger        *tags.Tags
	Logger        logging.Logger
}

var timeToWaitForReceipt = 3 * time.Second // time to wait to get a receipt for a chunk

func New(o Options) *PushSync {
	ps := &PushSync{
		streamer:      o.Streamer,
		storer:        o.Storer,
		peerSuggester: o.ClosestPeerer,
		tagger:        o.Tagger,
		logger:        o.Logger,
		metrics:       newMetrics(),
	}
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

// handler handles chunk delivery from other node and forwards to its destination node.
// If the current node is the destination, it stores in the local store and sends a receipt.
func (ps *PushSync) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()

	// Get the delivery
	chunk, err := ps.getChunkDelivery(r)
	if err != nil {
		return fmt.Errorf("chunk delivery from peer %s: %w", p.Address.String(), err)
	}

	// Select the closest peer to forward the chunk
	peer, err := ps.peerSuggester.ClosestPeer(chunk.Address())
	if err != nil {
		// If i am the closest peer then store the chunk and send receipt
		if errors.Is(err, topology.ErrWantSelf) {

			// Store the chunk in the local store
			_, err := ps.storer.Put(ctx, storage.ModePutSync, chunk)
			if err != nil {
				return fmt.Errorf("chunk store: %w", err)
			}
			ps.metrics.TotalChunksStoredInDB.Inc()

			// Send a receipt immediately once the storage of the chunk is successfully
			receipt := &pb.Receipt{Address: chunk.Address().Bytes()}
			err = ps.sendReceipt(w, receipt)
			if err != nil {
				return fmt.Errorf("send receipt to peer %s: %w", p.Address.String(), err)
			}
			return nil
		}
		return err
	}

	// This is a special situation in that the other peer thinks thats we are the closest node
	// and we think that the sending peer
	if p.Address.Equal(peer) {

		// Store the chunk in the local store
		_, err := ps.storer.Put(ctx, storage.ModePutSync, chunk)
		if err != nil {
			return fmt.Errorf("chunk store: %w", err)
		}
		ps.metrics.TotalChunksStoredInDB.Inc()

		// Send a receipt immediately once the storage of the chunk is successfully
		receipt := &pb.Receipt{Address: chunk.Address().Bytes()}
		return ps.sendReceipt(w, receipt)
	}

	// Forward chunk to closest peer
	streamer, err := ps.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return fmt.Errorf("new stream peer %s: %w", peer.String(), err)
	}
	defer streamer.Close()

	wc, rc := protobuf.NewWriterAndReader(streamer)
	if err := ps.sendChunkDelivery(wc, chunk); err != nil {
		return fmt.Errorf("forward chunk to peer %s: %w", peer.String(), err)
	}
	receiptRTTTimer := time.Now()

	// Most of the times now you wont get a tag because
	// /bytes and /files API does not implement tags
	// SO dont print any logs or returning error, if you dont find them
	t, _ := ps.tagger.Get(chunk.TagID())

	receipt, err := ps.receiveReceipt(rc, t)
	if err != nil {
		return fmt.Errorf("receive receipt from peer %s: %w", peer.String(), err)
	}
	ps.metrics.ReceiptRTT.Observe(time.Since(receiptRTTTimer).Seconds())

	// Check if the receipt is valid
	if !chunk.Address().Equal(swarm.NewAddress(receipt.Address)) {
		ps.metrics.InvalidReceiptReceived.Inc()
		return fmt.Errorf("invalid receipt from peer %s", peer.String())
	}

	// pass back the received receipt in the previously received stream
	err = ps.sendReceipt(w, &receipt)
	if err != nil {
		return fmt.Errorf("send receipt to peer %s: %w", peer.String(), err)
	}
	ps.metrics.ReceiptsSentCounter.Inc()

	return nil
}

func (ps *PushSync) getChunkDelivery(r protobuf.Reader) (chunk swarm.Chunk, err error) {
	var ch pb.Delivery
	if err = r.ReadMsg(&ch); err != nil {
		ps.metrics.ReceivedChunkErrorCounter.Inc()
		return nil, err
	}
	ps.metrics.ChunksSentCounter.Inc()

	// create chunk
	addr := swarm.NewAddress(ch.Address)
	chunk = swarm.NewChunk(addr, ch.Data)
	return chunk, nil
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
	ps.metrics.SendChunkTimer.Observe(time.Since(startTimer).Seconds())
	ps.metrics.ChunksSentCounter.Inc()

	//  if you manage to get a tag, just increment the respective counter
	t, err := ps.tagger.Get(chunk.TagID())
	if err == nil && t != nil {
		// most of the times now you wont get a tag because
		// /bytes and /files API does not implement tags
		// SO dont print any logs or returning error, if you dont find them
		// if you find a tag, increment the respective counter
		t.Inc(tags.StateSent)
	}
	return nil
}

func (ps *PushSync) sendReceipt(w protobuf.Writer, receipt *pb.Receipt) (err error) {
	if err := w.WriteMsg(receipt); err != nil {
		ps.metrics.SendReceiptErrorCounter.Inc()
		return err
	}
	ps.metrics.ReceiptsSentCounter.Inc()
	return nil
}

func (ps *PushSync) receiveReceipt(r protobuf.Reader, t *tags.Tag) (receipt pb.Receipt, err error) {
	if err := r.ReadMsg(&receipt); err != nil {
		ps.metrics.ReceiveReceiptErrorCounter.Inc()
		return receipt, err
	}
	ps.metrics.ReceiptsReceivedCounter.Inc()

	// increment the tag only if it is valid
	if t != nil {
		t.Inc(tags.StateSynced)
	}
	return receipt, nil
}

// PushChunkToClosest sends chunk to the closest peer by opening a stream. It then waits for
// a receipt from that peer and returns error or nil based on the receiving and
// the validity of the receipt.
func (ps *PushSync) PushChunkToClosest(ctx context.Context, ch swarm.Chunk) (*Receipt, error) {
	peer, err := ps.peerSuggester.ClosestPeer(ch.Address())
	if err != nil {
		if errors.Is(err, topology.ErrWantSelf) {
			// if you are the closest node return a receipt immediately
			return &Receipt{
				Address: ch.Address(),
			}, nil
		}
		return nil, fmt.Errorf("closest peer: %w", err)
	}

	streamer, err := ps.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, fmt.Errorf("new stream for peer %s: %w", peer.String(), err)
	}
	defer streamer.Close()

	w, r := protobuf.NewWriterAndReader(streamer)
	if err := ps.sendChunkDelivery(w, ch); err != nil {
		return nil, fmt.Errorf("chunk deliver to peer %s: %w", peer.String(), err)
	}
	receiptRTTTimer := time.Now()

	// most of the times now you wont get a tag because
	// /bytes and /files API does not implement tags
	// SO dont print any logs or returning error, if you dont find them
	t, _ := ps.tagger.Get(ch.TagID())

	receipt, err := ps.receiveReceipt(r, t)
	if err != nil {
		return nil, fmt.Errorf("receive receipt from peer %s: %w", peer.String(), err)
	}
	ps.metrics.ReceiptRTT.Observe(time.Since(receiptRTTTimer).Seconds())

	// Check if the receipt is valid
	if !ch.Address().Equal(swarm.NewAddress(receipt.Address)) {
		ps.metrics.InvalidReceiptReceived.Inc()
		return nil, fmt.Errorf("invalid receipt. peer %s", peer.String())
	}

	rec := &Receipt{
		Address: swarm.NewAddress(receipt.Address),
	}

	return rec, nil
}
