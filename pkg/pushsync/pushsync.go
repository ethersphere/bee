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
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pricer"
	"github.com/ethersphere/bee/pkg/pushsync/pb"
	"github.com/ethersphere/bee/pkg/skippeers"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/tracing"
	opentracing "github.com/opentracing/opentracing-go"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "pushsync"

const (
	protocolName    = "pushsync"
	protocolVersion = "1.2.0"
	streamName      = "pushsync"
)

const (
	defaultTTL         = 30 * time.Second // request time to live
	preemptiveInterval = 5 * time.Second  // P90 request time to live
	sanctionWait       = 5 * time.Minute
	overDraftRefresh   = time.Second
)

const (
	nPeersToReplicate = 4 // number of peers to replicate to as receipt is sent upstream
	maxPushErrors     = 32
)

var (
	ErrNoPush            = errors.New("could not push chunk")
	ErrOutOfDepthStoring = errors.New("storing outside of the neighborhood")
	ErrWarmup            = errors.New("node warmup time not complete")
)

type PushSyncer interface {
	PushChunkToClosest(ctx context.Context, ch swarm.Chunk) (*Receipt, error)
}

type Receipt struct {
	Address   swarm.Address
	Signature []byte
	Nonce     []byte
}

type PushSync struct {
	address        swarm.Address
	nonce          []byte
	streamer       p2p.StreamerDisconnecter
	storer         storage.Putter
	topologyDriver topology.Driver
	radiusChecker  postage.Radius
	tagger         *tags.Tags
	unwrap         func(swarm.Chunk)
	logger         log.Logger
	accounting     accounting.Interface
	pricer         pricer.Interface
	metrics        metrics
	tracer         *tracing.Tracer
	validStamp     postage.ValidStampFn
	signer         crypto.Signer
	fullNode       bool
	warmupPeriod   time.Time
	skipList       *skippeers.List
}

type receiptResult struct {
	pushTime time.Time
	peer     swarm.Address
	receipt  *pb.Receipt
	err      error
}

func New(address swarm.Address, nonce []byte, streamer p2p.StreamerDisconnecter, storer storage.Putter, topology topology.Driver, rs postage.Radius, tagger *tags.Tags, fullNode bool, unwrap func(swarm.Chunk), validStamp postage.ValidStampFn, logger log.Logger, accounting accounting.Interface, pricer pricer.Interface, signer crypto.Signer, tracer *tracing.Tracer, warmupTime time.Duration) *PushSync {
	ps := &PushSync{
		address:        address,
		nonce:          nonce,
		streamer:       streamer,
		storer:         storer,
		topologyDriver: topology,
		radiusChecker:  rs,
		tagger:         tagger,
		fullNode:       fullNode,
		unwrap:         unwrap,
		logger:         logger.WithName(loggerName).Register(),
		accounting:     accounting,
		pricer:         pricer,
		metrics:        newMetrics(),
		tracer:         tracer,
		signer:         signer,
		skipList:       skippeers.NewList(),
		warmupPeriod:   time.Now().Add(warmupTime),
	}

	ps.validStamp = ps.validStampWrapper(validStamp)
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
	now := time.Now()

	w, r := protobuf.NewWriterAndReader(stream)

	ctx, cancel := context.WithTimeout(ctx, defaultTTL)
	defer cancel()

	defer func() {
		if err != nil {
			ps.metrics.TotalHandlerTime.WithLabelValues("failure").Observe(time.Since(now).Seconds())
			ps.metrics.TotalHandlerErrors.Inc()
			_ = stream.Reset()
		} else {
			ps.metrics.TotalHandlerTime.WithLabelValues("success").Observe(time.Since(now).Seconds())
			_ = stream.FullClose()
		}
	}()

	var ch pb.Delivery
	if err = r.ReadMsgWithContext(ctx, &ch); err != nil {
		return fmt.Errorf("pushsync read delivery: %w", err)
	}

	ps.metrics.TotalReceived.Inc()

	chunk := swarm.NewChunk(swarm.NewAddress(ch.Address), ch.Data)
	chunkAddress := chunk.Address()

	span, _, ctx := ps.tracer.StartSpanFromContext(ctx, "pushsync-handler", ps.logger, opentracing.Tag{Key: "address", Value: chunkAddress.String()})
	defer span.Finish()

	stamp := new(postage.Stamp)
	err = stamp.UnmarshalBinary(ch.Stamp)
	if err != nil {
		return fmt.Errorf("pushsync stamp unmarshall: %w", err)
	}
	chunk.WithStamp(stamp)

	if cac.Valid(chunk) {
		go ps.unwrap(chunk)
	} else if !soc.Valid(chunk) {
		return swarm.ErrInvalidChunk
	}

	price := ps.pricer.Price(chunkAddress)

	store := func(ctx context.Context) error {

		chunk, err := ps.validStamp(chunk, ch.Stamp)
		if err != nil {
			return fmt.Errorf("invalid stamp: %w", err)
		}

		_, err = ps.storer.Put(ctx, storage.ModePutSync, chunk)
		if err != nil {
			return fmt.Errorf("chunk store: %w", err)
		}

		signature, err := ps.signer.Sign(ch.Address)
		if err != nil {
			return fmt.Errorf("receipt signature: %w", err)
		}

		// return back receipt
		debit, err := ps.accounting.PrepareDebit(ctx, p.Address, price)
		if err != nil {
			return fmt.Errorf("prepare debit to peer %s before writeback: %w", p.Address.String(), err)
		}
		defer debit.Cleanup()

		receipt := pb.Receipt{Address: chunkAddress.Bytes(), Signature: signature, Nonce: ps.nonce}
		if err := w.WriteMsgWithContext(ctx, &receipt); err != nil {
			return fmt.Errorf("send receipt to peer %s: %w", p.Address.String(), err)
		}

		return debit.Apply()
	}

	if ch.Replicate {

		ps.metrics.HandlerReplication.Inc()
		defer func() {
			if err != nil {
				ps.metrics.HandlerReplicationErrors.Inc()
			}
		}()

		if !ps.radiusChecker.IsWithinStorageRadius(chunk.Address()) {
			return ErrOutOfDepthStoring
		}

		span, _, ctxd := ps.tracer.StartSpanFromContext(ctx, "pushsync-replication-storage", ps.logger, opentracing.Tag{Key: "address", Value: chunkAddress.String()})
		defer span.Finish()

		err = store(ctxd)
		if err != nil {
			err = fmt.Errorf("replication: %w", err)
		}

		return err
	}

	receipt, err := ps.pushToClosest(ctx, chunk, false)
	if err != nil {
		if errors.Is(err, topology.ErrWantSelf) {
			ps.metrics.Storer.Inc()
			return store(ctx)
		}

		ps.metrics.Forwarder.Inc()
		return fmt.Errorf("handler: push to closest: %w", err)
	}

	ps.metrics.Forwarder.Inc()

	debit, err := ps.accounting.PrepareDebit(ctx, p.Address, price)
	if err != nil {
		return fmt.Errorf("prepare debit to peer %s before writeback: %w", p.Address.String(), err)
	}
	defer debit.Cleanup()

	// pass back the receipt
	if err := w.WriteMsgWithContext(ctx, receipt); err != nil {
		return fmt.Errorf("send receipt to peer %s: %w", p.Address.String(), err)
	}

	return debit.Apply()
}

// PushChunkToClosest sends chunk to the closest peer by opening a stream. It then waits for
// a receipt from that peer and returns error or nil based on the receiving and
// the validity of the receipt.
func (ps *PushSync) PushChunkToClosest(ctx context.Context, ch swarm.Chunk) (*Receipt, error) {
	ps.metrics.TotalOutgoing.Inc()
	r, err := ps.pushToClosest(ctx, ch, true)
	if err != nil {
		ps.metrics.TotalOutgoingErrors.Inc()
		return nil, err
	}
	return &Receipt{
		Address:   swarm.NewAddress(r.Address),
		Signature: r.Signature,
		Nonce:     r.Nonce,
	}, nil
}

func (ps *PushSync) pushToClosest(ctx context.Context, ch swarm.Chunk, origin bool) (*pb.Receipt, error) {

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		sentErrorsLeft   = 1
		preemptiveTicker <-chan time.Time
		inflight         int
		parallelForwards = nPeersToReplicate
	)

	if origin {
		ticker := time.NewTicker(preemptiveInterval)
		defer ticker.Stop()
		preemptiveTicker = ticker.C
		sentErrorsLeft = maxPushErrors
	}

	resultChan := make(chan receiptResult)

	retryC := make(chan struct{}, parallelForwards)

	retry := func() {
		select {
		case retryC <- struct{}{}:
		case <-ctx.Done():
		default:
		}
	}

	nextPeer := func() (swarm.Address, error) {

		// we are a full, warmed up node and in the neighborhood of the chunk
		if ps.fullNode && ps.radiusChecker.IsWithinStorageRadius(ch.Address()) && ps.warmedUp() {
			return swarm.ZeroAddress, topology.ErrWantSelf
		}

		// find closer peer
		peer, err := ps.topologyDriver.ClosestPeer(ch.Address(), ps.fullNode, topology.Filter{Reachable: true}, ps.skipList.ChunkPeers(ch.Address())...)
		// only return ErrWantSelf if we are in the neighborhood of the chunk
		if errors.Is(err, topology.ErrWantSelf) {
			return swarm.ZeroAddress, topology.ErrNotFound
		}

		return peer, err
	}

	retry()

	for sentErrorsLeft > 0 {
		select {
		case <-ctx.Done():
			return nil, ErrNoPush
		case <-preemptiveTicker:
			retry()
		case <-retryC:

			peer, err := nextPeer()

			// no peers left
			if errors.Is(err, topology.ErrNotFound) {
				if ps.skipList.PruneExpiresAfter(ch.Address(), overDraftRefresh) == 0 { //no overdraft peers, we have depleted ALL peers
					if inflight == 0 {
						ps.logger.Debug("no peers left", "chunk_address", ch.Address(), "error", err)
						return nil, err
					} else {
						continue // there is still an inflight request, wait for it's result
					}
				}

				ps.logger.Debug("sleeping to refresh overdraft balance", "chunk_address", ch.Address())

				select {
				case <-time.After(overDraftRefresh):
					retry()
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}

			// we store the chunk
			if errors.Is(err, topology.ErrWantSelf) {
				go ps.replicateInNeighborhood(ctx, ch, origin)
				if origin && cac.Valid(ch) {
					go ps.unwrap(ch)
				}
				return nil, err
			}

			if err != nil {
				if inflight == 0 {
					return nil, fmt.Errorf("get closest for address %s, allow upstream %v: %w", ch, origin, err)
				} else {
					continue
				}
			}

			// since we can reach into the neighborhood of the chunk
			// act as the multiplexer and push the chunk in parallel to multiple peers
			if swarm.Proximity(peer.Bytes(), ch.Address().Bytes()) >= ps.radiusChecker.StorageRadius() {
				for ; parallelForwards > 0; parallelForwards-- {
					retry()
					sentErrorsLeft++
				}
			}

			action, err := ps.prepareCredit(ctx, peer, ch, origin)
			if err != nil {
				ps.skipList.Add(ch.Address(), peer, overDraftRefresh)
				retry()
				continue
			}
			ps.skipList.Add(ch.Address(), peer, sanctionWait)

			ps.metrics.TotalSendAttempts.Inc()
			inflight++

			go ps.push(ctx, resultChan, peer, ch, action)

		case result := <-resultChan:

			inflight--

			ps.measurePushPeer(result.pushTime, result.err, origin)

			if result.err == nil {
				return result.receipt, nil
			}

			ps.metrics.TotalFailedSendAttempts.Inc()
			ps.logger.Debug("could not push to peer", "peer_address", result.peer, "error", result.err)

			sentErrorsLeft--

			retry()
		}
	}

	return nil, ErrNoPush
}

func (ps *PushSync) push(parentCtx context.Context, resultChan chan<- receiptResult, peer swarm.Address, ch swarm.Chunk, action accounting.Action) {

	span := tracing.FromContext(parentCtx)

	ctx, cancel := context.WithTimeout(context.Background(), defaultTTL)
	defer cancel()

	spanInner, _, ctx := ps.tracer.StartSpanFromContext(tracing.WithContext(ctx, span), "push-closest", ps.logger, opentracing.Tag{Key: "address", Value: ch.Address().String()})
	defer spanInner.Finish()

	var (
		err     error
		receipt *pb.Receipt
		now     = time.Now()
	)

	defer func() {
		select {
		case resultChan <- receiptResult{pushTime: now, peer: peer, err: err, receipt: receipt}:
		case <-parentCtx.Done():
		}
	}()

	defer action.Cleanup()

	receipt, err = ps.pushChunkToPeer(ctx, peer, ch, false)
	if err != nil {
		return
	}

	ps.metrics.TotalSent.Inc()

	err = action.Apply()
}

// replicateInNeighborhood pushes the chunk to some peers in the neighborhood in parallel for replication.
func (ps *PushSync) replicateInNeighborhood(ctx context.Context, ch swarm.Chunk, origin bool) {
	count := 0
	_ = ps.topologyDriver.EachConnectedPeer(func(peer swarm.Address, po uint8) (bool, bool, error) {
		// stop here since the rest of the peers will also be further away
		if po < ps.radiusChecker.StorageRadius() || count == nPeersToReplicate {
			return true, false, nil
		}
		count++
		go ps.replicateWithPeer(ctx, peer, ch, origin)
		return false, false, nil
	}, topology.Filter{Reachable: true})
}

// replicateWithPeer handles in-neighborhood replication for a single peer.
func (ps *PushSync) replicateWithPeer(parentCtx context.Context, peer swarm.Address, ch swarm.Chunk, origin bool) {
	var err error
	ps.metrics.TotalReplicatedAttempts.Inc()

	span := tracing.FromContext(parentCtx)

	ctx, cancel := context.WithTimeout(context.Background(), defaultTTL)
	defer cancel()

	spanInner, logger, ctx := ps.tracer.StartSpanFromContext(tracing.WithContext(ctx, span), "pushsync-replication", ps.logger, opentracing.Tag{Key: "address", Value: ch.Address().String()})
	loggerV1 := logger.V(1).Build()
	defer spanInner.Finish()
	defer func() {
		if err != nil {
			loggerV1.Debug("pushsync replication failed", "error", err)
			ps.metrics.TotalReplicatedError.Inc()
		}
	}()

	action, err := ps.prepareCredit(ctx, peer, ch, origin)
	if err != nil {
		return
	}
	defer action.Cleanup()

	_, err = ps.pushChunkToPeer(ctx, peer, ch, true)
	if err != nil {
		return
	}

	err = action.Apply()
}

func (ps *PushSync) pushChunkToPeer(ctx context.Context, peer swarm.Address, ch swarm.Chunk, replicate bool) (*pb.Receipt, error) {

	streamer, err := ps.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, fmt.Errorf("new stream for peer %s: %w", peer.String(), err)
	}

	defer func() {
		if err != nil {
			_ = streamer.Reset()
		} else {
			_ = streamer.FullClose()
		}
	}()

	w, r := protobuf.NewWriterAndReader(streamer)
	stamp, err := ch.Stamp().MarshalBinary()
	if err != nil {
		return nil, err
	}
	err = w.WriteMsgWithContext(ctx, &pb.Delivery{
		Address:   ch.Address().Bytes(),
		Data:      ch.Data(),
		Stamp:     stamp,
		Replicate: replicate,
	})
	if err != nil {
		return nil, err
	}

	// if you manage to get a tag, just increment the respective counter
	t, err := ps.tagger.Get(ch.TagID())
	if err == nil && t != nil {
		err = t.Inc(tags.StateSent)
		if err != nil {
			return nil, fmt.Errorf("tag %d increment: %w", ch.TagID(), err)
		}
	}

	var receipt pb.Receipt
	if err = r.ReadMsgWithContext(ctx, &receipt); err != nil {
		return nil, err
	}

	if !ch.Address().Equal(swarm.NewAddress(receipt.Address)) {
		return nil, fmt.Errorf("invalid receipt. chunk %s, peer %s", ch.Address(), peer)
	}

	return &receipt, nil

}

func (ps *PushSync) prepareCredit(ctx context.Context, peer swarm.Address, ch swarm.Chunk, origin bool) (accounting.Action, error) {

	creditCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	creditAction, err := ps.accounting.PrepareCredit(creditCtx, peer, ps.pricer.PeerPrice(peer, ch.Address()), origin)
	if err != nil {
		return nil, err
	}

	return creditAction, nil
}

func (ps *PushSync) measurePushPeer(t time.Time, err error, origin bool) {
	var status string
	if err != nil {
		status = "failure"
	} else {
		status = "success"
	}
	ps.metrics.PushToPeerTime.WithLabelValues(status).Observe(time.Since(t).Seconds())
}

func (ps *PushSync) validStampWrapper(f postage.ValidStampFn) postage.ValidStampFn {
	return func(c swarm.Chunk, s []byte) (swarm.Chunk, error) {
		t := time.Now()
		chunk, err := f(c, s)
		if err != nil {
			ps.metrics.InvalidStampErrors.Inc()
			ps.metrics.StampValidationTime.WithLabelValues("failure").Observe(time.Since(t).Seconds())
		} else {
			ps.metrics.StampValidationTime.WithLabelValues("success").Observe(time.Since(t).Seconds())
		}
		return chunk, err
	}
}

func (s *PushSync) Close() error {
	return s.skipList.Close()
}

func (ps *PushSync) warmedUp() bool {
	return time.Now().After(ps.warmupPeriod)
}
