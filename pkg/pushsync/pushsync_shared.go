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

	"github.com/ethersphere/bee/v2/pkg/accounting"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/pushsync/pb"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "pushsync"

const (
	protocolName    = "pushsync"
	protocolVersion = "1.3.1"
	streamName      = "pushsync"
)

const (
	defaultTTL         = 30 * time.Second // request time to live
	preemptiveInterval = 5 * time.Second  // P90 request time to live
	skiplistDur        = 5 * time.Minute
	overDraftRefresh   = time.Millisecond * 600
)

const (
	maxMultiplexForwards = 2 // number of extra peers to forward the request from the multiplex node
	maxPushErrors        = 32
)

var (
	ErrNoPush            = errors.New("could not push chunk")
	ErrOutOfDepthStoring = errors.New("storing outside of the neighborhood")
	ErrWarmup            = errors.New("node warmup time not complete")
	ErrShallowReceipt    = errors.New("shallow receipt")
)

type PushSyncer interface {
	PushChunkToClosest(ctx context.Context, ch swarm.Chunk) (*Receipt, error)
}

type Receipt struct {
	Address   swarm.Address
	Signature []byte
	Nonce     []byte
}

type Storer interface {
	storage.PushReporter
	ReservePutter() storage.Putter
}

type receiptResult struct {
	pushTime time.Time
	peer     swarm.Address
	receipt  *pb.Receipt
	err      error
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

func (ps *PushSync) closestPeer(chunkAddress swarm.Address, origin bool, skipList []swarm.Address) (swarm.Address, error) {
	includeSelf := ps.fullNode && !origin

	peer, err := ps.topologyDriver.ClosestPeer(chunkAddress, includeSelf, topology.Select{Reachable: true, Healthy: true}, skipList...)
	if errors.Is(err, topology.ErrNotFound) {
		peer, err := ps.topologyDriver.ClosestPeer(chunkAddress, includeSelf, topology.Select{Reachable: true}, skipList...)
		if errors.Is(err, topology.ErrNotFound) {
			return ps.topologyDriver.ClosestPeer(chunkAddress, includeSelf, topology.Select{}, skipList...)
		}
		return peer, err
	}

	return peer, err
}

func (ps *PushSync) pushChunkToPeer(ctx context.Context, peer swarm.Address, ch swarm.Chunk) (receipt *pb.Receipt, err error) {
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
		Address: ch.Address().Bytes(),
		Data:    ch.Data(),
		Stamp:   stamp,
	})
	if err != nil {
		return nil, err
	}

	// if the chunk has a tag, then it's from a local deferred upload
	if ch.TagID() != 0 {
		err = ps.store.Report(ctx, ch, storage.ChunkSent)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			err = fmt.Errorf("tag %d increment: %w", ch.TagID(), err)
			return
		}
	}

	var rec pb.Receipt
	if err = r.ReadMsgWithContext(ctx, &rec); err != nil {
		return nil, err
	}
	if rec.Err != "" {
		return nil, p2p.NewChunkDeliveryError(rec.Err)
	}

	if !ch.Address().Equal(swarm.NewAddress(rec.Address)) {
		return nil, fmt.Errorf("invalid receipt. chunk %s, peer %s", ch.Address(), peer)
	}

	return &rec, nil
}

func (ps *PushSync) prepareCredit(ctx context.Context, peer swarm.Address, ch swarm.Chunk, origin bool) (accounting.Action, error) {
	creditAction, err := ps.accounting.PrepareCredit(ctx, peer, ps.pricer.PeerPrice(peer, ch.Address()), origin)
	if err != nil {
		return nil, err
	}

	return creditAction, nil
}

func (s *PushSync) Close() error {
	return s.errSkip.Close()
}
