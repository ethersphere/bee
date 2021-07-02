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
	"sync"
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
	maxPeers           = 16
	maxAttempts        = 16
	skipPeerExpiration = time.Minute
)

var (
	ErrOutOfDepthReplication = errors.New("replication outside of the neighborhood")
	ErrNoPush                = errors.New("could not push chunk")
	ErrWarmup                = errors.New("node warmup time not complete")
)

type PushSyncer interface {
	PushChunkToClosest(ctx context.Context, ch swarm.Chunk) (*Receipt, error)
}

type Receipt struct {
	Address   swarm.Address
	Signature []byte
	BlockHash []byte
}

type PushSync struct {
	address        swarm.Address
	blockHash      []byte
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
	warmupPeriod   time.Time
	skipList       *peerSkipList
}

var defaultTTL = 15 * time.Second                     // request time to live
var sendReceiptDelay = 9 * time.Second                // Time to wait for in-neighborhood chunks before attempting to establish storage as self
var timeToWaitForPushsyncToNeighbor = 3 * time.Second // time to wait to get a receipt for a chunk
var nPeersToPushsync = 3                              // number of peers to replicate to as receipt is sent upstream

func New(address swarm.Address, blockHash []byte, streamer p2p.StreamerDisconnecter, storer storage.Putter, topology topology.Driver, tagger *tags.Tags, isFullNode bool, unwrap func(swarm.Chunk), validStamp func(swarm.Chunk, []byte) (swarm.Chunk, error), logger logging.Logger, accounting accounting.Interface, pricer pricer.Interface, signer crypto.Signer, tracer *tracing.Tracer, warmupTime time.Duration) *PushSync {
	ps := &PushSync{
		address:        address,
		blockHash:      blockHash,
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
		skipList:       newPeerSkipList(),
		warmupPeriod:   time.Now().Add(warmupTime),
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
	ctx, cancel := context.WithTimeout(ctx, defaultTTL)
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

	// if the peer is closer to the chunk, AND it's a full node, we were selected for replication. Return early.
	if p.FullNode {
		bytes := chunk.Address().Bytes()
		if dcmp, _ := swarm.DistanceCmp(bytes, p.Address.Bytes(), ps.address.Bytes()); dcmp == 1 {
			if ps.topologyDriver.IsWithinDepth(chunk.Address()) {
				ctxd, canceld := context.WithTimeout(context.Background(), timeToWaitForPushsyncToNeighbor)
				defer canceld()

				_, err = ps.storer.Put(ctxd, storage.ModePutSync, chunk)
				if err != nil {
					return fmt.Errorf("chunk store: %w", err)
				}

				debit, err := ps.accounting.PrepareDebit(p.Address, price)
				if err != nil {
					return fmt.Errorf("prepare debit to peer %s before writeback: %w", p.Address.String(), err)
				}
				defer debit.Cleanup()

				// return back receipt
				signature, err := ps.signer.Sign(bytes)
				if err != nil {
					return fmt.Errorf("receipt signature: %w", err)
				}
				receipt := pb.Receipt{Address: bytes, Signature: signature, BlockHash: ps.blockHash}
				if err := w.WriteMsgWithContext(ctxd, &receipt); err != nil {
					return fmt.Errorf("send receipt to peer %s: %w", p.Address.String(), err)
				}

				return debit.Apply()
			}

			return ErrOutOfDepthReplication
		}
	}

	// forwarding replication
	storedChunk := false
	inNeighborhood := ps.topologyDriver.IsWithinDepth(chunk.Address())
	if inNeighborhood {
		_, err = ps.storer.Put(ctx, storage.ModePutSync, chunk)
		if err != nil {
			ps.logger.Warningf("pushsync: within depth peer's attempt to store chunk failed: %v", err)
		} else {
			storedChunk = true
		}
	}

	span, _, ctx := ps.tracer.StartSpanFromContext(ctx, "pushsync-handler", ps.logger, opentracing.Tag{Key: "address", Value: chunk.Address().String()})
	defer span.Finish()

	receipt, err := ps.pushToClosest(ctx, chunk, p.Address)
	if err != nil {
		if errors.Is(err, topology.ErrWantSelf) {
			if !storedChunk {
				_, err = ps.storer.Put(ctx, storage.ModePutSync, chunk)
				if err != nil {
					return fmt.Errorf("chunk store: %w", err)
				}
			}

			signature, err := ps.signer.Sign(ch.Address)
			if err != nil {
				return fmt.Errorf("receipt signature: %w", err)
			}

			// return back receipt
			debit, err := ps.accounting.PrepareDebit(p.Address, price)
			if err != nil {
				return fmt.Errorf("prepare debit to peer %s before writeback: %w", p.Address.String(), err)
			}
			defer debit.Cleanup()

			receipt := pb.Receipt{Address: chunk.Address().Bytes(), Signature: signature, BlockHash: ps.blockHash}
			if err := w.WriteMsgWithContext(ctx, &receipt); err != nil {
				return fmt.Errorf("send receipt to peer %s: %w", p.Address.String(), err)
			}

			return debit.Apply()
		}
		return fmt.Errorf("handler: push to closest: %w", err)

	}

	debit, err := ps.accounting.PrepareDebit(p.Address, price)
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
	r, err := ps.pushToClosest(ctx, ch, swarm.ZeroAddress)
	if err != nil {
		return nil, err
	}
	return &Receipt{
		Address:   swarm.NewAddress(r.Address),
		Signature: r.Signature,
		BlockHash: r.BlockHash}, nil
}

type pushResult struct {
	receipt   *pb.Receipt
	err       error
	attempted bool
}

func (ps *PushSync) pushToClosest(ctx context.Context, ch swarm.Chunk, origin swarm.Address) (*pb.Receipt, error) {
	span, logger, ctx := ps.tracer.StartSpanFromContext(ctx, "push-closest", ps.logger, opentracing.Tag{Key: "address", Value: ch.Address().String()})
	defer span.Finish()
	defer ps.skipList.PruneExpired()

	originated := false
	if origin.IsZero() {
		originated = true
	}

	var sendReceipt <-chan time.Time
	chunkInNeighbourhood := ps.topologyDriver.IsWithinDepth(ch.Address())
	if chunkInNeighbourhood {
		sendReceipt = time.After(sendReceiptDelay)
	}

	var (
		skipPeers      []swarm.Address
		allowedRetries = 1
		resultC        = make(chan *pushResult, 1)
		includeSelf    = ps.isFullNode
		signInProgress = false
		attempts       = 0
		results        = 0
	)

	if originated || chunkInNeighbourhood {
		// only originator and peers in neighborhood of chunk retry
		allowedRetries = maxPeers
	}

	for i := maxAttempts; allowedRetries > 0 && i > 0; i-- {
		// find the next closest peer
		peer, err := ps.topologyDriver.ClosestPeer(ch.Address(), includeSelf, skipPeers...)
		if err != nil {
			// ClosestPeer can return ErrNotFound in case we are not connected to any peers
			// in which case we should return immediately.
			// if ErrWantSelf is returned, it means we are the closest peer.
			if errors.Is(err, topology.ErrWantSelf) && !signInProgress {
				if time.Now().Before(ps.warmupPeriod) {
					return nil, ErrWarmup
				}

				if !chunkInNeighbourhood {
					return nil, ErrNoPush
				}

				signInProgress = true
				go ps.establishStorage(ch, origin, resultC)
			}
			if !errors.Is(err, topology.ErrWantSelf) {
				return nil, fmt.Errorf("closest peer: %w", err)
			}
		}

		if !peer.Equal(swarm.Address{}) {
			skipPeers = append(skipPeers, peer)

			if ps.skipList.ShouldSkip(peer) {
				ps.metrics.TotalSkippedPeers.Inc()
				continue
			}

			ps.metrics.TotalSendAttempts.Inc()
			attempts++

			go func(peer swarm.Address, ch swarm.Chunk, chunkInNeighbourhood bool) {
				ctxd, canceld := context.WithTimeout(context.Background(), defaultTTL)
				defer canceld()

				r, attempted, err := ps.pushPeer(ctxd, peer, ch, originated)
				// attempted is true if we get past accounting and actually attempt
				// to send the request to the peer. If we dont get past accounting, we
				// should not count the retry and try with a different peer again
				if err != nil {

					if time.Now().After(ps.warmupPeriod) && !ps.skipList.HasChunk(ch.Address()) && chunkInNeighbourhood && errors.Is(err, context.DeadlineExceeded) {
						ps.skipList.Add(peer, ch.Address(), skipPeerExpiration)
					}

					logger.Debugf("could not push to peer %s: %v", peer, err)
					resultC <- &pushResult{err: err, attempted: attempted}
					return
				}
				select {
				case resultC <- &pushResult{receipt: r}:
				case <-ctx.Done():
				}
			}(peer, ch, chunkInNeighbourhood)
		}

		select {
		case r := <-resultC:
			results++

			if r.attempted {
				allowedRetries--
			}

			// receipt received for chunk
			if r.receipt != nil {
				ps.skipList.PruneChunk(ch.Address())
				return r.receipt, nil
			}
			if r.err != nil && r.attempted {
				ps.metrics.TotalFailedSendAttempts.Inc()

				if results > attempts {
					return nil, ErrNoPush
				}
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-sendReceipt:
			if !signInProgress {
				signInProgress = true
				allowedRetries++
				go ps.establishStorage(ch, origin, resultC)
			}
		}
	}

	return nil, ErrNoPush
}

func (ps *PushSync) pushPeer(ctx context.Context, peer swarm.Address, ch swarm.Chunk, originated bool) (*pb.Receipt, bool, error) {
	// compute the price we pay for this receipt and reserve it for the rest of this function
	receiptPrice := ps.pricer.PeerPrice(peer, ch.Address())

	// Reserve to see whether we can make the request
	err := ps.accounting.Reserve(ctx, peer, receiptPrice)
	if err != nil {
		return nil, false, fmt.Errorf("reserve balance for peer %s: %w", peer, err)
	}
	defer ps.accounting.Release(peer, receiptPrice)

	stamp, err := ch.Stamp().MarshalBinary()
	if err != nil {
		return nil, false, err
	}

	streamer, err := ps.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, false, fmt.Errorf("new stream for peer %s: %w", peer, err)
	}
	defer streamer.Close()

	w, r := protobuf.NewWriterAndReader(streamer)
	if err := w.WriteMsgWithContext(ctx, &pb.Delivery{
		Address: ch.Address().Bytes(),
		Data:    ch.Data(),
		Stamp:   stamp,
	}); err != nil {
		_ = streamer.Reset()
		return nil, false, fmt.Errorf("chunk %s deliver to peer %s: %w", ch.Address(), peer, err)
	}

	ps.metrics.TotalSent.Inc()

	// if you manage to get a tag, just increment the respective counter
	t, err := ps.tagger.Get(ch.TagID())
	if err == nil && t != nil {
		err = t.Inc(tags.StateSent)
		if err != nil {
			return nil, true, fmt.Errorf("tag %d increment: %v", ch.TagID(), err)
		}
	}

	var receipt pb.Receipt
	if err := r.ReadMsgWithContext(ctx, &receipt); err != nil {
		_ = streamer.Reset()
		return nil, true, fmt.Errorf("chunk %s receive receipt from peer %s: %w", ch.Address(), peer, err)
	}

	if !ch.Address().Equal(swarm.NewAddress(receipt.Address)) {
		// if the receipt is invalid, try to push to the next peer
		return nil, true, fmt.Errorf("invalid receipt. chunk %s, peer %s", ch.Address(), peer)
	}

	err = ps.accounting.Credit(peer, receiptPrice, originated)
	if err != nil {
		return nil, true, err
	}

	return &receipt, true, nil
}

// pushToNeighbour handles in-neighborhood replication for a single peer.
func (ps *PushSync) pushToNeighbour(peer swarm.Address, ch swarm.Chunk, origin bool) {
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
	receiptPrice := ps.pricer.PeerPrice(peer, ch.Address())

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

	w, r := protobuf.NewWriterAndReader(streamer)
	stamp, err := ch.Stamp().MarshalBinary()
	if err != nil {
		return
	}
	err = w.WriteMsgWithContext(ctx, &pb.Delivery{
		Address: ch.Address().Bytes(),
		Data:    ch.Data(),
		Stamp:   stamp,
	})
	if err != nil {
		return
	}

	var receipt pb.Receipt
	if err = r.ReadMsgWithContext(ctx, &receipt); err != nil {
		return
	}

	if !ch.Address().Equal(swarm.NewAddress(receipt.Address)) {
		// if the receipt is invalid, give up
		return
	}

	err = ps.accounting.Credit(peer, receiptPrice, origin)
}

type peerSkipList struct {
	sync.Mutex
	chunks         map[string]struct{}
	skipExpiration map[string]time.Time
}

func newPeerSkipList() *peerSkipList {
	return &peerSkipList{
		chunks:         make(map[string]struct{}),
		skipExpiration: make(map[string]time.Time),
	}
}

func (l *peerSkipList) Add(peer, chunk swarm.Address, expire time.Duration) {
	l.Lock()
	defer l.Unlock()

	l.skipExpiration[peer.ByteString()] = time.Now().Add(expire)
	l.chunks[chunk.ByteString()] = struct{}{}
}

func (l *peerSkipList) ShouldSkip(peer swarm.Address) bool {
	l.Lock()
	defer l.Unlock()

	peerStr := peer.ByteString()

	if exp, has := l.skipExpiration[peerStr]; has {
		// entry is expired
		if exp.Before(time.Now()) {
			delete(l.skipExpiration, peerStr)
			return false
		} else {
			return true
		}
	}

	return false
}

func (l *peerSkipList) HasChunk(chunk swarm.Address) bool {
	l.Lock()
	defer l.Unlock()

	_, has := l.chunks[chunk.ByteString()]
	return has
}

func (l *peerSkipList) PruneChunk(chunk swarm.Address) {
	l.Lock()
	defer l.Unlock()
	delete(l.chunks, chunk.ByteString())
}

func (l *peerSkipList) PruneExpired() {
	l.Lock()
	defer l.Unlock()

	now := time.Now()

	for k, v := range l.skipExpiration {
		if v.Before(now) {
			delete(l.skipExpiration, k)
		}
	}
}

func (ps *PushSync) establishStorage(ch swarm.Chunk, origin swarm.Address, resultC chan<- *pushResult) {

	originated := false
	if origin.IsZero() {
		originated = true
	}

	count := 0
	// Push the chunk to some peers in the neighborhood in parallel for replication.
	// Any errors here should NOT impact the rest of the handler.
	_ = ps.topologyDriver.EachNeighbor(func(peer swarm.Address, po uint8) (bool, bool, error) {
		// skip forwarding peer
		if peer.Equal(origin) {
			return false, false, nil
		}

		// here we skip the peer if the peer is closer to the chunk than us
		// we replicate with peers that are further away than us because we are the storer
		if dcmp, _ := swarm.DistanceCmp(ch.Address().Bytes(), peer.Bytes(), ps.address.Bytes()); dcmp == 1 {
			return false, false, nil
		}

		if count == nPeersToPushsync {
			return true, false, nil
		}

		count++

		go ps.pushToNeighbour(peer, ch, originated)
		return false, false, nil
	})

	bytes := ch.Address().Bytes()

	signature, err := ps.signer.Sign(bytes)
	if err != nil {
		resultC <- &pushResult{
			receipt:   nil,
			err:       err,
			attempted: true,
		}
		return
	}

	receipt := pb.Receipt{Address: bytes, Signature: signature, BlockHash: ps.blockHash}
	resultC <- &pushResult{
		receipt:   &receipt,
		err:       err,
		attempted: true,
	}

}
