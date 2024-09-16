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

	"github.com/ethersphere/bee/v2/pkg/accounting"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/pricer"
	pb "github.com/ethersphere/bee/v2/pkg/retrieval/pb"
	"github.com/ethersphere/bee/v2/pkg/skippeers"
	"github.com/ethersphere/bee/v2/pkg/soc"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	olog "github.com/opentracing/opentracing-go/log"
	"resenje.org/singleflight"
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

type Service struct {
	addr          swarm.Address
	radiusFunc    func() (uint8, error)
	streamer      p2p.Streamer
	peerSuggester topology.ClosestPeerer
	storer        Storer
	singleflight  singleflight.Group[string, swarm.Chunk]
	logger        log.Logger
	accounting    accounting.Interface
	metrics       metrics
	pricer        pricer.Interface
	tracer        *tracing.Tracer
	caching       bool
	errSkip       *skippeers.List
}

func New(
	addr swarm.Address,
	radiusFunc func() (uint8, error),
	storer Storer,
	streamer p2p.Streamer,
	chunkPeerer topology.ClosestPeerer,
	logger log.Logger,
	accounting accounting.Interface,
	pricer pricer.Interface,
	tracer *tracing.Tracer,
	forwarderCaching bool,
) *Service {
	return &Service{
		addr:          addr,
		radiusFunc:    radiusFunc,
		streamer:      streamer,
		peerSuggester: chunkPeerer,
		storer:        storer,
		logger:        logger.WithName(loggerName).Register(),
		accounting:    accounting,
		pricer:        pricer,
		metrics:       newMetrics(),
		tracer:        tracer,
		caching:       forwarderCaching,
		errSkip:       skippeers.NewList(),
	}
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

func (s *Service) RetrieveChunk(ctx context.Context, chunkAddr, sourcePeerAddr swarm.Address) (swarm.Chunk, error) {
	loggerV1 := s.logger

	s.metrics.RequestCounter.Inc()

	origin := sourcePeerAddr.IsZero()

	if chunkAddr.IsZero() || chunkAddr.IsEmpty() || !chunkAddr.IsValidLength() {
		return nil, fmt.Errorf("invalid address queried")
	}

	flightRoute := chunkAddr.String()
	if origin {
		flightRoute = chunkAddr.String() + originSuffix
	}

	totalRetrieveAttempts := 0
	requestStartTime := time.Now()
	defer func() {
		s.metrics.RequestDurationTime.Observe(time.Since(requestStartTime).Seconds())
		s.metrics.RequestAttempts.Observe(float64(totalRetrieveAttempts))
	}()

	spanCtx := context.WithoutCancel(ctx)

	v, _, err := s.singleflight.Do(ctx, flightRoute, func(ctx context.Context) (swarm.Chunk, error) {

		skip := skippeers.NewList()
		defer skip.Close()

		var preemptiveTicker <-chan time.Time

		if !sourcePeerAddr.IsZero() {
			skip.Forever(chunkAddr, sourcePeerAddr)
		}

		quit := make(chan struct{})
		defer close(quit)

		var forwards = maxMultiplexForwards

		// if we are the origin node, allow many preemptive retries to speed up the retrieval of the chunk.
		errorsLeft := 1
		if origin {
			ticker := time.NewTicker(preemptiveInterval)
			defer ticker.Stop()
			preemptiveTicker = ticker.C
			errorsLeft = maxOriginErrors
		}

		resultC := make(chan retrievalResult, 1)
		retryC := make(chan struct{}, forwards+1)

		retry := func() {
			select {
			case retryC <- struct{}{}:
			case <-ctx.Done():
			default:
			}
		}

		retry()

		inflight := 0

		for errorsLeft > 0 {

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-preemptiveTicker:
				retry()
			case <-retryC:

				totalRetrieveAttempts++
				s.metrics.PeerRequestCounter.Inc()

				fullSkip := append(skip.ChunkPeers(chunkAddr), s.errSkip.ChunkPeers(chunkAddr)...)
				peer, err := s.closestPeer(chunkAddr, fullSkip, origin)

				if errors.Is(err, topology.ErrNotFound) {
					if skip.PruneExpiresAfter(chunkAddr, overDraftRefresh) == 0 { //no overdraft peers, we have depleted ALL peers
						if inflight == 0 {
							loggerV1.Debug("no peers left", "chunk_address", chunkAddr, "errors_left", errorsLeft, "isOrigin", origin, "own_proximity", swarm.Proximity(s.addr.Bytes(), chunkAddr.Bytes()), "error", err)
							return nil, err
						}
						continue // there is still an inflight request, wait for it's result
					}

					loggerV1.Debug("sleeping to refresh overdraft balance", "chunk_address", chunkAddr)

					select {
					case <-time.After(overDraftRefresh):
						retry()
						continue
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}

				if err != nil {
					if inflight == 0 {
						loggerV1.Debug("peer selection", "chunk_address", chunkAddr, "error", err)
						return nil, err
					}
					continue
				}

				// since we can reach into the neighborhood of the chunk
				// act as the multiplexer and push the chunk in parallel to multiple peers.
				// neighbor peers will also have multiple retries, which means almost the entire neighborhood
				// will be scanned for the chunk, starting from the closest to the furthest peer in the neighborhood.
				if radius, err := s.radiusFunc(); err == nil && swarm.Proximity(peer.Bytes(), chunkAddr.Bytes()) >= radius {
					for ; forwards > 0; forwards-- {
						retry()
						errorsLeft++
					}
				}

				action, err := s.prepareCredit(ctx, peer, chunkAddr, origin)
				if err != nil {
					skip.Add(chunkAddr, peer, overDraftRefresh)
					retry()
					continue
				}
				skip.Forever(chunkAddr, peer)

				inflight++

				go func() {
					span, _, ctx := s.tracer.FollowSpanFromContext(spanCtx, "retrieve-chunk", s.logger, opentracing.Tag{Key: "address", Value: chunkAddr.String()})
					defer span.Finish()
					s.retrieveChunk(ctx, quit, chunkAddr, peer, resultC, action, span)
				}()

			case res := <-resultC:

				inflight--

				if res.err == nil {
					loggerV1.Debug("retrieved chunk", "chunk_address", chunkAddr, "peer_address", res.peer, "peer_proximity", swarm.Proximity(res.peer.Bytes(), chunkAddr.Bytes()))
					return res.chunk, nil
				}

				loggerV1.Debug("failed to get chunk", "chunk_address", chunkAddr, "peer_address", res.peer,
					"peer_proximity", swarm.Proximity(res.peer.Bytes(), chunkAddr.Bytes()), "error", res.err)

				errorsLeft--
				s.errSkip.Add(chunkAddr, res.peer, skiplistDur)
				retry()
			}
		}

		return nil, storage.ErrNotFound
	})
	if err != nil {
		s.metrics.RequestFailureCounter.Inc()
		s.logger.Debug("retrieval failed", "chunk_address", chunkAddr, "error", err)
		return nil, err
	}

	s.metrics.RequestSuccessCounter.Inc()

	return v, nil
}

func (s *Service) retrieveChunk(ctx context.Context, quit chan struct{}, chunkAddr, peer swarm.Address, result chan retrievalResult, action accounting.Action, span opentracing.Span) {

	var (
		startTime = time.Now()
		err       error
		chunk     swarm.Chunk
	)

	defer func() {
		action.Cleanup()
		if err != nil {
			ext.LogError(span, err)
			s.metrics.TotalErrors.Inc()
		} else {
			span.LogFields(olog.Bool("success", true))
		}
		select {
		case result <- retrievalResult{err: err, chunk: chunk, peer: peer}:
		case <-quit:
			return
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, RetrieveChunkTimeout)
	defer cancel()

	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		err = fmt.Errorf("new stream: %w", err)
		return
	}

	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()

	w, r := protobuf.NewWriterAndReader(stream)
	err = w.WriteMsgWithContext(ctx, &pb.Request{Addr: chunkAddr.Bytes()})
	if err != nil {
		err = fmt.Errorf("write request: %w peer %s", err, peer.String())
		return
	}

	var d pb.Delivery
	if err = r.ReadMsgWithContext(ctx, &d); err != nil {
		err = fmt.Errorf("read delivery: %w peer %s", err, peer.String())
		return
	}
	if d.Err != "" {
		err = p2p.NewChunkDeliveryError(d.Err)
		return
	}

	s.metrics.ChunkRetrieveTime.Observe(time.Since(startTime).Seconds())
	s.metrics.TotalRetrieved.Inc()

	chunk = swarm.NewChunk(chunkAddr, d.Data)
	if !cac.Valid(chunk) {
		if !soc.Valid(chunk) {
			s.metrics.InvalidChunkRetrieved.Inc()
			err = swarm.ErrInvalidChunk
			return
		}
	}

	err = action.Apply()
}

func (s *Service) prepareCredit(ctx context.Context, peer, chunk swarm.Address, origin bool) (accounting.Action, error) {

	price := s.pricer.PeerPrice(peer, chunk)
	s.metrics.ChunkPrice.Observe(float64(price))

	creditAction, err := s.accounting.PrepareCredit(ctx, peer, price, origin)
	if err != nil {
		return nil, err
	}

	return creditAction, nil
}

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
