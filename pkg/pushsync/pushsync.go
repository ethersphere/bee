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
	"github.com/ethersphere/bee/pkg/crypto"
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

var (
	ErrOutOfDepthReplication = errors.New("replication outside of the neighborhood")
)

type PushSyncer interface {
	PushChunkToClosest(ctx context.Context, ch swarm.Chunk) (*Receipt, error)
}

type Receipt struct {
	Address   swarm.Address
	Signature []byte
}

type PushSync struct {
	address        swarm.Address
	streamer       p2p.StreamerDisconnecter
	storer         storage.Putter
	topologyDriver topology.Driver
	tagger         *tags.Tags
	unwrap         func(swarm.Chunk)
	logger         logging.Logger
	accounting     accounting.Interface
	pricer         pricer.Interface
	metrics        metrics
	tracer         *tracing.Tracer
	validStamp     func(swarm.Chunk, []byte) (swarm.Chunk, error)
	signer         crypto.Signer
	isFullNode     bool
}

var timeToLive = 5 * time.Second                      // request time to live
var timeToWaitForPushsyncToNeighbor = 3 * time.Second // time to wait to get a receipt for a chunk
var nPeersToPushsync = 3                              // number of peers to replicate to as receipt is sent upstream

func New(address swarm.Address, streamer p2p.StreamerDisconnecter, storer storage.Putter, topology topology.Driver, tagger *tags.Tags, isFullNode bool, unwrap func(swarm.Chunk), validStamp func(swarm.Chunk, []byte) (swarm.Chunk, error), logger logging.Logger, accounting accounting.Interface, pricer pricer.Interface, signer crypto.Signer, tracer *tracing.Tracer) *PushSync {
	ps := &PushSync{
		address:        address,
		streamer:       streamer,
		storer:         storer,
		topologyDriver: topology,
		tagger:         tagger,
		isFullNode:     isFullNode,
		unwrap:         unwrap,
		logger:         logger,
		accounting:     accounting,
		pricer:         pricer,
		metrics:        newMetrics(),
		tracer:         tracer,
		validStamp:     validStamp,
		signer:         signer,
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
	if chunk, err = ps.validStamp(chunk, ch.Stamp); err != nil {
		return fmt.Errorf("pushsync valid stamp: %w", err)
	}

	if cac.Valid(chunk) {
		if ps.unwrap != nil {
			go ps.unwrap(chunk)
		}
	} else if !soc.Valid(chunk) {
		return swarm.ErrInvalidChunk
	}

	price := ps.pricer.Price(chunk.Address())

	// if the peer is closer to the chunk, we were selected for replication. Return early.
	if dcmp, _ := swarm.DistanceCmp(chunk.Address().Bytes(), p.Address.Bytes(), ps.address.Bytes()); dcmp == 1 {
		if ps.topologyDriver.IsWithinDepth(chunk.Address()) {
			ctxd, _ := context.WithTimeout(context.Background(), timeToWaitForPushsyncToNeighbor)

			_, err = ps.storer.Put(ctxd, storage.ModePutSync, chunk)
			if err != nil {
				ps.logger.Errorf("pushsync: chunk store: %v", err)
			}
			signature, err := ps.signer.Sign(ch.Address)
			if err != nil {
				return fmt.Errorf("receipt signature: %w", err)
			}

			// return back receipt
			receipt := pb.Receipt{Address: chunk.Address().Bytes(), Signature: signature}
			if err := w.WriteMsgWithContext(ctxd, &receipt); err != nil {
				return fmt.Errorf("send receipt to peer %s: %w", p.Address.String(), err)
			}

			return ps.accounting.Debit(p.Address, price)
		}

		return ErrOutOfDepthReplication
	}

	// forwarding replication
	if ps.topologyDriver.IsWithinDepth(chunk.Address()) {
		_, err = ps.storer.Put(ctx, storage.ModePutSync, chunk)
		if err != nil {
			ps.logger.Warningf("pushsync: within depth peer's attempt to store chunk failed: %v", err)
		}
	}

	span, _, ctx := ps.tracer.StartSpanFromContext(ctx, "pushsync-handler", ps.logger, opentracing.Tag{Key: "address", Value: chunk.Address().String()})
	defer span.Finish()

	receipt, err := ps.pushToClosest(ctx, chunk)
	if err != nil {
		if errors.Is(err, topology.ErrWantSelf) {
			_, err = ps.storer.Put(ctx, storage.ModePutSync, chunk)
			if err != nil {
				return fmt.Errorf("chunk store: %w", err)
			}

			count := 0
			// Push the chunk to some peers in the neighborhood in parallel for replication.
			// Any errors here should NOT impact the rest of the handler.
			err = ps.topologyDriver.EachNeighbor(func(peer swarm.Address, po uint8) (bool, bool, error) {

				// skip forwarding peer
				if peer.Equal(p.Address) {
					return false, false, nil
				}

				if count == nPeersToPushsync {
					return true, false, nil
				}
				count++

				go func(peer swarm.Address) {

					var err error
					defer func() {
						if err != nil {
							ps.logger.Tracef("pushsync replication: %v", err)
							ps.metrics.TotalReplicatedError.Inc()
						} else {
							ps.metrics.TotalReplicated.Inc()
						}
					}()

					// price for neighborhood replication
					receiptPrice := ps.pricer.PeerPrice(peer, chunk.Address())

					ctx, cancel := context.WithTimeout(context.Background(), timeToWaitForPushsyncToNeighbor)
					defer cancel()

					err = ps.accounting.Reserve(ctx, peer, receiptPrice)
					if err != nil {
						err = fmt.Errorf("reserve balance for peer %s: %w", peer.String(), err)
						return
					}
					defer ps.accounting.Release(peer, receiptPrice)

					streamer, err := ps.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
					if err != nil {
						err = fmt.Errorf("new stream for peer %s: %w", peer.String(), err)
						return
					}

					defer func() {
						if err != nil {
							ps.metrics.TotalErrors.Inc()
							_ = streamer.Reset()
						} else {
							_ = streamer.FullClose()
						}
					}()

					w := protobuf.NewWriter(streamer)
					ctx, cancel = context.WithTimeout(ctx, timeToWaitForPushsyncToNeighbor)
					defer cancel()
					stamp, err := chunk.Stamp().MarshalBinary()
					if err != nil {
						return
					}
					err = w.WriteMsgWithContext(ctx, &pb.Delivery{
						Address: chunk.Address().Bytes(),
						Data:    chunk.Data(),
						Stamp:   stamp,
					})
					if err != nil {
						return
					}

					var receipt pb.Receipt
					if err = r.ReadMsgWithContext(ctx, &receipt); err != nil {
						return
					}

					if !chunk.Address().Equal(swarm.NewAddress(receipt.Address)) {
						// if the receipt is invalid, give up
						return
					}

					err = ps.accounting.Credit(peer, receiptPrice)

				}(peer)

				return false, false, nil
			})
			if err != nil {
				ps.logger.Tracef("pushsync replication closest peer: %w", err)
			}

			signature, err := ps.signer.Sign(ch.Address)
			if err != nil {
				return fmt.Errorf("receipt signature: %w", err)
			}

			// return back receipt
			receipt := pb.Receipt{Address: chunk.Address().Bytes(), Signature: signature}
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
	return &Receipt{
		Address:   swarm.NewAddress(r.Address),
		Signature: r.Signature}, nil
}

func (ps *PushSync) pushToClosest(ctx context.Context, ch swarm.Chunk) (rr *pb.Receipt, reterr error) {
	span, logger, ctx := ps.tracer.StartSpanFromContext(ctx, "push-closest", ps.logger, opentracing.Tag{Key: "address", Value: ch.Address().String()})
	defer span.Finish()
	var (
		skipPeers []swarm.Address
		lastErr   error
	)

	stamp, err := ch.Stamp().MarshalBinary()
	if err != nil {
		return nil, err
	}

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

		includeSelf := ps.isFullNode

		// find the next closest peer
		peer, err := ps.topologyDriver.ClosestPeer(ch.Address(), includeSelf, skipPeers...)
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

		streamer, err := ps.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
		if err != nil {
			lastErr = fmt.Errorf("new stream for peer %s: %w", peer.String(), err)
			continue
		}
		deferFuncs = append(deferFuncs, func() { go streamer.FullClose() })

		// Reserve to see whether we can make the request
		err = ps.accounting.Reserve(ctx, peer, receiptPrice)
		if err != nil {
			return nil, fmt.Errorf("reserve balance for peer %s: %w", peer.String(), err)
		}
		deferFuncs = append(deferFuncs, func() { ps.accounting.Release(peer, receiptPrice) })

		w, r := protobuf.NewWriterAndReader(streamer)
		ctxd, canceld := context.WithTimeout(ctx, timeToLive)
		deferFuncs = append(deferFuncs, func() { canceld() })
		if err := w.WriteMsgWithContext(ctxd, &pb.Delivery{
			Address: ch.Address().Bytes(),
			Data:    ch.Data(),
			Stamp:   stamp,
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
