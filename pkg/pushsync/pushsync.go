// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/pushsync/pb"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/tracing"
	opentracing "github.com/opentracing/opentracing-go"
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
	streamer         p2p.Streamer
	storer           storage.Putter
	peerSuggester    topology.ClosestPeerer
	tagg             *tags.Tags
	deliveryCallback func(context.Context, swarm.Chunk) // callback func to be invoked to deliver chunks to PSS
	logger           logging.Logger
	accounting       accounting.Interface
	pricer           accounting.Pricer
	metrics          metrics
	tracer           *tracing.Tracer
}

var timeToWaitForReceipt = 3 * time.Second // time to wait to get a receipt for a chunk

func New(streamer p2p.Streamer, storer storage.Putter, closestPeerer topology.ClosestPeerer, tagger *tags.Tags, deliveryCallback func(context.Context, swarm.Chunk), logger logging.Logger, accounting accounting.Interface, pricer accounting.Pricer, tracer *tracing.Tracer) *PushSync {
	ps := &PushSync{
		streamer:         streamer,
		storer:           storer,
		peerSuggester:    closestPeerer,
		tagg:             tagger,
		deliveryCallback: deliveryCallback,
		logger:           logger,
		accounting:       accounting,
		pricer:           pricer,
		metrics:          newMetrics(),
		tracer:           tracer,
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
func (ps *PushSync) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()

	var ch pb.Delivery
	if err = r.ReadMsgWithContext(ctx, &ch); err != nil {
		ps.metrics.ReceivedChunkErrorCounter.Inc()
		return fmt.Errorf("pushsync read delivery: %w", err)
	}
	ps.metrics.ChunksReceivedCounter.Inc()

	chunk := swarm.NewChunk(swarm.NewAddress(ch.Address), ch.Data)
	span, _, ctx := ps.tracer.StartSpanFromContext(ctx, "pushsync-handler", ps.logger, opentracing.Tag{Key: "address", Value: chunk.Address().String()})
	defer span.Finish()

	// Select the closest peer to forward the chunk
	peer, err := ps.peerSuggester.ClosestPeer(chunk.Address())
	if err != nil {
		// If i am the closest peer then store the chunk and send receipt
		if errors.Is(err, topology.ErrWantSelf) {
			return ps.handleDeliveryResponse(ctx, w, p, chunk)
		}
		return err
	}

	// This is a special situation in that the other peer thinks thats we are the closest node
	// and we think that the sending peer is the closest
	if p.Address.Equal(peer) {
		return ps.handleDeliveryResponse(ctx, w, p, chunk)
	}

	// compute the price we pay for this receipt and reserve it for the rest of this function
	receiptPrice := ps.pricer.PeerPrice(peer, chunk.Address())
	err = ps.accounting.Reserve(ctx, peer, receiptPrice)
	if err != nil {
		return fmt.Errorf("reserve balance for peer %s: %w", peer.String(), err)
	}
	defer ps.accounting.Release(peer, receiptPrice)

	// Forward chunk to closest peer
	streamer, err := ps.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return fmt.Errorf("new stream peer %s: %w", peer.String(), err)
	}
	defer func() {
		if err != nil {
			_ = streamer.Reset()
		} else {
			go streamer.FullClose()
		}
	}()

	wc, rc := protobuf.NewWriterAndReader(streamer)
	if err := ps.sendChunkDelivery(ctx, wc, chunk); err != nil {
		return fmt.Errorf("forward chunk to peer %s: %w", peer.String(), err)
	}
	receiptRTTTimer := time.Now()

	receipt, err := ps.receiveReceipt(ctx, rc)
	if err != nil {
		return fmt.Errorf("receive receipt from peer %s: %w", peer.String(), err)
	}
	ps.metrics.ReceiptRTT.Observe(time.Since(receiptRTTTimer).Seconds())

	// Check if the receipt is valid
	if !chunk.Address().Equal(swarm.NewAddress(receipt.Address)) {
		ps.metrics.InvalidReceiptReceived.Inc()
		return fmt.Errorf("invalid receipt from peer %s", peer.String())
	}

	err = ps.accounting.Credit(peer, receiptPrice)
	if err != nil {
		return err
	}

	// pass back the received receipt in the previously received stream
	err = ps.sendReceipt(ctx, w, &receipt)
	if err != nil {
		return fmt.Errorf("send receipt to peer %s: %w", peer.String(), err)
	}
	ps.metrics.ReceiptsSentCounter.Inc()

	return ps.accounting.Debit(p.Address, ps.pricer.Price(chunk.Address()))
}

func (ps *PushSync) sendChunkDelivery(ctx context.Context, w protobuf.Writer, chunk swarm.Chunk) (err error) {
	ctx, cancel := context.WithTimeout(ctx, timeToWaitForReceipt)
	defer cancel()
	startTimer := time.Now()
	if err = w.WriteMsgWithContext(ctx, &pb.Delivery{
		Address: chunk.Address().Bytes(),
		Data:    chunk.Data(),
	}); err != nil {
		ps.metrics.SendChunkErrorCounter.Inc()
		return err
	}
	ps.metrics.SendChunkTimer.Observe(time.Since(startTimer).Seconds())
	ps.metrics.ChunksSentCounter.Inc()
	return nil
}

func (ps *PushSync) sendReceipt(ctx context.Context, w protobuf.Writer, receipt *pb.Receipt) (err error) {
	ctx, cancel := context.WithTimeout(ctx, timeToWaitForReceipt)
	defer cancel()
	if err := w.WriteMsgWithContext(ctx, receipt); err != nil {
		ps.metrics.SendReceiptErrorCounter.Inc()
		return err
	}
	ps.metrics.ReceiptsSentCounter.Inc()
	return nil
}

func (ps *PushSync) receiveReceipt(ctx context.Context, r protobuf.Reader) (receipt pb.Receipt, err error) {
	ctx, cancel := context.WithTimeout(ctx, timeToWaitForReceipt)
	defer cancel()
	if err := r.ReadMsgWithContext(ctx, &receipt); err != nil {
		ps.metrics.ReceiveReceiptErrorCounter.Inc()
		return receipt, err
	}
	ps.metrics.ReceiptsReceivedCounter.Inc()
	return receipt, nil
}

// PushChunkToClosest sends chunk to the closest peer by opening a stream. It then waits for
// a receipt from that peer and returns error or nil based on the receiving and
// the validity of the receipt.
func (ps *PushSync) PushChunkToClosest(ctx context.Context, ch swarm.Chunk) (*Receipt, error) {
	span, _, ctx := ps.tracer.StartSpanFromContext(ctx, "pushsync-push", ps.logger, opentracing.Tag{Key: "address", Value: ch.Address().String()})
	defer span.Finish()

	peer, err := ps.peerSuggester.ClosestPeer(ch.Address())
	if err != nil {
		if errors.Is(err, topology.ErrWantSelf) {
			// this is to make sure that the sent number does not diverge from the synced counter
			t, err := ps.tagg.Get(ch.TagID())
			if err == nil && t != nil {
				err = t.Inc(tags.StateSent)
				if err != nil {
					return nil, err
				}
			}

			// if you are the closest node return a receipt immediately
			return &Receipt{
				Address: ch.Address(),
			}, nil
		}
		return nil, fmt.Errorf("closest peer: %w", err)
	}

	// compute the price we pay for this receipt and reserve it for the rest of this function
	receiptPrice := ps.pricer.PeerPrice(peer, ch.Address())
	err = ps.accounting.Reserve(ctx, peer, receiptPrice)
	if err != nil {
		return nil, fmt.Errorf("reserve balance for peer %s: %w", peer.String(), err)
	}
	defer ps.accounting.Release(peer, receiptPrice)

	streamer, err := ps.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, fmt.Errorf("new stream for peer %s: %w", peer.String(), err)
	}
	defer func() { go streamer.FullClose() }()

	w, r := protobuf.NewWriterAndReader(streamer)
	if err := ps.sendChunkDelivery(ctx, w, ch); err != nil {
		_ = streamer.Reset()
		return nil, fmt.Errorf("chunk deliver to peer %s: %w", peer.String(), err)
	}

	//  if you manage to get a tag, just increment the respective counter
	t, err := ps.tagg.Get(ch.TagID())
	if err == nil && t != nil {
		err = t.Inc(tags.StateSent)
		if err != nil {
			return nil, err
		}
	}

	receiptRTTTimer := time.Now()
	receipt, err := ps.receiveReceipt(ctx, r)
	if err != nil {
		_ = streamer.Reset()
		return nil, fmt.Errorf("receive receipt from peer %s: %w", peer.String(), err)
	}
	ps.metrics.ReceiptRTT.Observe(time.Since(receiptRTTTimer).Seconds())

	// Check if the receipt is valid
	if !ch.Address().Equal(swarm.NewAddress(receipt.Address)) {
		ps.metrics.InvalidReceiptReceived.Inc()
		_ = streamer.Reset()
		return nil, fmt.Errorf("invalid receipt. peer %s", peer.String())
	}

	err = ps.accounting.Credit(peer, receiptPrice)
	if err != nil {
		return nil, err
	}

	rec := &Receipt{
		Address: swarm.NewAddress(receipt.Address),
	}

	return rec, nil
}

func (ps *PushSync) handleDeliveryResponse(ctx context.Context, w protobuf.Writer, p p2p.Peer, chunk swarm.Chunk) error {
	// Store the chunk in the local store
	_, err := ps.storer.Put(ctx, storage.ModePutSync, chunk)
	if err != nil {
		return fmt.Errorf("chunk store: %w", err)
	}
	ps.metrics.TotalChunksStoredInDB.Inc()

	// Send a receipt immediately once the storage of the chunk is successfully
	receipt := &pb.Receipt{Address: chunk.Address().Bytes()}
	err = ps.sendReceipt(ctx, w, receipt)
	if err != nil {
		return fmt.Errorf("send receipt to peer %s: %w", p.Address.String(), err)
	}

	err = ps.accounting.Debit(p.Address, ps.pricer.Price(chunk.Address()))
	if err != nil {
		return err
	}

	if ps.deliveryCallback != nil {
		ps.deliveryCallback(ctx, chunk)
	}

	return nil
}
