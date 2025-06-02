// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package retrieval provides the retrieval protocol
// implementation. The protocol is used to retrieve
// chunks over the network using forwarding-kademlia
// routing.
package retrieval

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	pb "github.com/ethersphere/bee/v2/pkg/retrieval/pb"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	olog "github.com/opentracing/opentracing-go/log"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "retrieval"

const (
	protocolName    = "retrieval"
	protocolVersion = "1.4.0"
	streamName      = "retrieval"
)

var _ Interface = (*Service)(nil)

type Interface interface {
	// RetrieveChunk retrieves a chunk from the network using the retrieval protocol.
	// it takes as parameters a context, a chunk address to retrieve (content-addressed or single-owner) and
	// a source peer address, for the case that we are requesting the chunk for another peer. In case the request
	// originates at the current node (i.e. no forwarding involved), the caller should use swarm.ZeroAddress
	// as the value for sourcePeerAddress.
	RetrieveChunk(ctx context.Context, address, sourcePeerAddr swarm.Address) (chunk swarm.Chunk, err error)
}

type retrievalResult struct {
	chunk swarm.Chunk
	peer  swarm.Address
	err   error
}

type Storer interface {
	Cache() storage.Putter
	Lookup() storage.Getter
}

func (s *Service) Protocol() p2p.ProtocolSpec {
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

const (
	RetrieveChunkTimeout = time.Second * 30
	preemptiveInterval   = time.Second
	overDraftRefresh     = time.Millisecond * 600
	skiplistDur          = time.Minute
	originSuffix         = "_origin"
	maxOriginErrors      = 32
	maxMultiplexForwards = 2
)

// closestPeer returns address of the peer that is closest to the chunk with
// provided address addr. This function will ignore peers with addresses
// provided in skipPeers and if allowUpstream is true, peers that are further of
// the chunk than this node is, could also be returned, allowing the upstream
// retrieve request.
func (s *Service) closestPeer(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {

	var (
		closest swarm.Address
		err     error
	)

	closest, err = s.peerSuggester.ClosestPeer(addr, false, topology.Select{Reachable: true, Healthy: true}, skipPeers...)
	if errors.Is(err, topology.ErrNotFound) {
		closest, err = s.peerSuggester.ClosestPeer(addr, false, topology.Select{Reachable: true}, skipPeers...)
		if errors.Is(err, topology.ErrNotFound) {
			closest, err = s.peerSuggester.ClosestPeer(addr, false, topology.Select{}, skipPeers...)
		}
	}

	if err != nil {
		return swarm.Address{}, err
	}

	if allowUpstream {
		return closest, nil
	}

	closer, err := closest.Closer(addr, s.addr)
	if err != nil {
		return swarm.Address{}, fmt.Errorf("distance compare addr %s closest %s base address %s: %w", addr.String(), closest.String(), s.addr.String(), err)
	}
	if !closer {
		return swarm.Address{}, topology.ErrNotFound
	}

	return closest, nil
}

func (s *Service) handler(p2pctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	ctx, cancel := context.WithTimeout(p2pctx, RetrieveChunkTimeout)
	defer cancel()

	w, r := protobuf.NewWriterAndReader(stream)
	var attemptedWrite bool

	defer func() {
		if err != nil {
			if !attemptedWrite {
				_ = w.WriteMsgWithContext(ctx, &pb.Delivery{Err: err.Error()})
			}
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()
	var req pb.Request
	if err := r.ReadMsgWithContext(ctx, &req); err != nil {
		return fmt.Errorf("read request: %w peer %s", err, p.Address.String())
	}

	addr := swarm.NewAddress(req.Addr)

	if addr.IsZero() || addr.IsEmpty() || !addr.IsValidLength() {
		return fmt.Errorf("invalid address queried by peer %s", p.Address.String())
	}

	var forwarded bool

	span, _, ctx := s.tracer.StartSpanFromContext(ctx, "handle-retrieve-chunk", s.logger, opentracing.Tag{Key: "address", Value: addr.String()})
	defer func() {
		if err != nil {
			ext.LogError(span, err)
		} else {
			span.LogFields(olog.Bool("success", true))
		}
		span.LogFields(olog.Bool("forwarded", forwarded))
		span.Finish()
	}()

	chunk, err := s.storer.Lookup().Get(ctx, addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// forward the request
			chunk, err = s.RetrieveChunk(ctx, addr, p.Address)
			if err != nil {
				return fmt.Errorf("retrieve chunk: %w", err)
			}
			forwarded = true
		} else {
			return fmt.Errorf("get from store: %w", err)
		}
	}

	chunkPrice := s.pricer.Price(chunk.Address())
	debit, err := s.accounting.PrepareDebit(ctx, p.Address, chunkPrice)
	if err != nil {
		return fmt.Errorf("prepare debit to peer %s before writeback: %w", p.Address.String(), err)
	}
	defer debit.Cleanup()

	attemptedWrite = true

	if err := w.WriteMsgWithContext(ctx, &pb.Delivery{
		Data: chunk.Data(),
	}); err != nil {
		return fmt.Errorf("write delivery: %w peer %s", err, p.Address.String())
	}

	// debit price from p's balance
	if err := debit.Apply(); err != nil {
		return fmt.Errorf("apply debit: %w", err)
	}

	// cache the request last, so that putting to the localstore does not slow down the request flow
	if s.caching && forwarded {
		if err := s.storer.Cache().Put(p2pctx, chunk); err != nil {
			s.logger.Debug("retrieve cache put", "error", err)
		}
	}

	return nil
}

func (s *Service) Close() error {
	return s.errSkip.Close()
}
