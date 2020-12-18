// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pushsync provides the pushsync protocol
// implementation.
package pushsync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/pricer"
	"github.com/ethersphere/bee/pkg/pushsync/pb"
	"github.com/ethersphere/bee/pkg/soc"
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

const (
	maxPeers = 5
)

type PushSyncer interface {
	PushChunkToClosest(ctx context.Context, ch swarm.Chunk) (*Receipt, error)
}

type Receipt struct {
	Address swarm.Address
}

type PushSync struct {
	streamer      p2p.StreamerDisconnecter
	storer        storage.Putter
	peerSuggester topology.ClosestPeerer
	tagger        *tags.Tags
	unwrap        func(swarm.Chunk)
	logger        logging.Logger
	accounting    accounting.Interface
	pricer        pricer.Interface
	metrics       metrics
	tracer        *tracing.Tracer
}

var timeToLive = 5 * time.Second // request time to live

func New(streamer p2p.StreamerDisconnecter, storer storage.Putter, closestPeerer topology.ClosestPeerer, tagger *tags.Tags, unwrap func(swarm.Chunk), logger logging.Logger, accounting accounting.Interface, pricer pricer.Interface, tracer *tracing.Tracer) *PushSync {

	ps := &PushSync{
		streamer:      streamer,
		storer:        storer,
		peerSuggester: closestPeerer,
		tagger:        tagger,
		unwrap:        unwrap,
		logger:        logger,
		accounting:    accounting,
		pricer:        pricer,
		metrics:       newMetrics(),
		tracer:        tracer,
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
	ctx, cancel := context.WithTimeout(ctx, timeToLive)
	defer cancel()
	defer func() {
		if err != nil {
			ps.metrics.TotalErrors.Inc()
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()
	var ch pb.Delivery
	if err = r.ReadMsgWithContext(ctx, &ch); err != nil {
		return fmt.Errorf("pushsync read delivery: %w", err)
	}
	ps.metrics.TotalReceived.Inc()

	chunk := swarm.NewChunk(swarm.NewAddress(ch.Address), ch.Data)

	if cac.Valid(chunk) {
		if ps.unwrap != nil {
			go ps.unwrap(chunk)
		}
	} else if !soc.Valid(chunk) {
		return swarm.ErrInvalidChunk
	}

	span, _, ctx := ps.tracer.StartSpanFromContext(ctx, "pushsync-handler", ps.logger, opentracing.Tag{Key: "address", Value: chunk.Address().String()})
	defer span.Finish()

	price := ps.pricer.PriceForPeer(p.Address, chunk.Address())

	receipt, err := ps.pushToClosest(ctx, chunk)
	if err != nil {
		if errors.Is(err, topology.ErrWantSelf) {
			_, err = ps.storer.Put(ctx, storage.ModePutSync, chunk)
			if err != nil {
				return fmt.Errorf("chunk store: %w", err)
			}

			receipt := pb.Receipt{Address: chunk.Address().Bytes()}
			if err := w.WriteMsgWithContext(ctx, &receipt); err != nil {
				return fmt.Errorf("send receipt to peer %s: %w", p.Address.String(), err)
			}

			return ps.accounting.Debit(p.Address, price)
		}
		return fmt.Errorf("handler: push to closest: %w", err)
	}

	// pass back the receipt
	if err := w.WriteMsgWithContext(ctx, receipt); err != nil {
		return fmt.Errorf("send receipt to peer %s: %w", p.Address.String(), err)
	}

	return ps.accounting.Debit(p.Address, price)
}

// PushChunkToClosest sends chunk to the closest peer by opening a stream. It then waits for
// a receipt from that peer and returns error or nil based on the receiving and
// the validity of the receipt.
func (ps *PushSync) PushChunkToClosest(ctx context.Context, ch swarm.Chunk) (*Receipt, error) {
	r, err := ps.pushToClosest(ctx, ch)
	if err != nil {
		return nil, err
	}
	return &Receipt{Address: swarm.NewAddress(r.Address)}, nil
}

func (ps *PushSync) pushToClosest(ctx context.Context, ch swarm.Chunk) (rr *pb.Receipt, reterr error) {
	span, logger, ctx := ps.tracer.StartSpanFromContext(ctx, "push-closest", ps.logger, opentracing.Tag{Key: "address", Value: ch.Address().String()})
	defer span.Finish()
	var (
		skipPeers []swarm.Address
		lastErr   error
	)

	deferFuncs := make([]func(), 0)
	defersFn := func() {
		if len(deferFuncs) > 0 {
			for _, deferFn := range deferFuncs {
				deferFn()
			}
			deferFuncs = deferFuncs[:0]
		}
	}
	defer defersFn()

	for i := 0; i < maxPeers; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		defersFn()

		// find next closest peer
		peer, err := ps.peerSuggester.ClosestPeer(ch.Address(), skipPeers...)
		if err != nil {
			// ClosestPeer can return ErrNotFound in case we are not connected to any peers
			// in which case we should return immediately.
			// if ErrWantSelf is returned, it means we are the closest peer.
			return nil, fmt.Errorf("closest peer: %w", err)
		}

		// save found peer (to be skipped if there is some error with him)
		skipPeers = append(skipPeers, peer)

		deferFuncs = append(deferFuncs, func() {
			if lastErr != nil {
				ps.metrics.TotalErrors.Inc()
				logger.Errorf("pushsync: %v", lastErr)
			}
		})

		// compute the price we pay for this receipt and reserve it for the rest of this function
		receiptPrice := ps.pricer.PeerPrice(peer, ch.Address())
		err = ps.accounting.Reserve(ctx, peer, receiptPrice)
		if err != nil {
			return nil, fmt.Errorf("reserve balance for peer %s: %w", peer.String(), err)
		}
		deferFuncs = append(deferFuncs, func() { ps.accounting.Release(peer, receiptPrice) })

		streamer, err := ps.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
		if err != nil {
			lastErr = fmt.Errorf("new stream for peer %s: %w", peer.String(), err)
			continue
		}
		deferFuncs = append(deferFuncs, func() { go streamer.FullClose() })

		w, r := protobuf.NewWriterAndReader(streamer)
		ctxd, canceld := context.WithTimeout(ctx, timeToLive)
		deferFuncs = append(deferFuncs, func() { canceld() })
		if err := w.WriteMsgWithContext(ctxd, &pb.Delivery{
			Address: ch.Address().Bytes(),
			Data:    ch.Data(),
		}); err != nil {
			_ = streamer.Reset()
			lastErr = fmt.Errorf("chunk %s deliver to peer %s: %w", ch.Address().String(), peer.String(), err)
			continue
		}

		ps.metrics.TotalSent.Inc()

		// if you manage to get a tag, just increment the respective counter
		t, err := ps.tagger.Get(ch.TagID())
		if err == nil && t != nil {
			err = t.Inc(tags.StateSent)
			if err != nil {
				lastErr = fmt.Errorf("tag %d increment: %v", ch.TagID(), err)
				err = lastErr
				return nil, err
			}
		}

		var receipt pb.Receipt
		if err := r.ReadMsgWithContext(ctxd, &receipt); err != nil {
			_ = streamer.Reset()
			lastErr = fmt.Errorf("chunk %s receive receipt from peer %s: %w", ch.Address().String(), peer.String(), err)
			continue
		}

		if !ch.Address().Equal(swarm.NewAddress(receipt.Address)) {
			// if the receipt is invalid, try to push to the next peer
			lastErr = fmt.Errorf("invalid receipt. chunk %s, peer %s", ch.Address().String(), peer.String())
			continue
		}

		err = ps.accounting.Credit(peer, receiptPrice)
		if err != nil {
			return nil, err
		}

		return &receipt, nil
	}

	logger.Tracef("pushsync: chunk %s: reached %v peers", ch.Address(), maxPeers)

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, topology.ErrNotFound
}
