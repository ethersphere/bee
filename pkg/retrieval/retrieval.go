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

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/pricer"
	pb "github.com/ethersphere/bee/pkg/retrieval/pb"
	"github.com/ethersphere/bee/pkg/skippeers"
	"github.com/ethersphere/bee/pkg/soc"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/opentracing/opentracing-go"
	"resenje.org/singleflight"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "retrieval"

const (
	protocolName    = "retrieval"
	protocolVersion = "1.3.0"
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
	chunk             swarm.Chunk
	peer              swarm.Address
	err               error
	retrieveAttempted bool
}

type Storer interface {
	Cache() storage.Putter
	Lookup() storage.Getter
}

type Service struct {
	addr          swarm.Address
	streamer      p2p.Streamer
	peerSuggester topology.ClosestPeerer
	storer        Storer
	singleflight  singleflight.Group
	logger        log.Logger
	accounting    accounting.Interface
	metrics       metrics
	pricer        pricer.Interface
	tracer        *tracing.Tracer
	caching       bool
}

func New(
	addr swarm.Address,
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
		streamer:      streamer,
		peerSuggester: chunkPeerer,
		storer:        storer,
		logger:        logger.WithName(loggerName).Register(),
		accounting:    accounting,
		pricer:        pricer,
		metrics:       newMetrics(),
		tracer:        tracer,
		caching:       forwarderCaching,
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
	retrieveChunkTimeout = 10 * time.Second
	preemptiveInterval   = time.Second
	resetOverdraftDur    = time.Millisecond * 600
	maxRetrievedErrors   = 32
	originSuffix         = "_origin"
)

func (s *Service) RetrieveChunk(ctx context.Context, addr, sourcePeerAddr swarm.Address) (swarm.Chunk, error) {
	loggerV1 := s.logger.V(1).Register()

	s.metrics.RequestCounter.Inc()

	origin := sourcePeerAddr.IsZero()

	flightRoute := addr.String()
	if origin {
		flightRoute = addr.String() + originSuffix
	}

	totalRetrieveAttempts := 0
	requestStartTime := time.Now()
	defer func() {
		s.metrics.RequestDurationTime.Observe(time.Since(requestStartTime).Seconds())
		s.metrics.RequestAttempts.Observe(float64(totalRetrieveAttempts))
	}()

	// topCtx is passing the tracing span to the first singleflight call
	topCtx := ctx
	v, _, err := s.singleflight.Do(ctx, flightRoute, func(ctx context.Context) (interface{}, error) {

		sp := new(skippeers.List)

		if !sourcePeerAddr.IsZero() {
			sp.Add(sourcePeerAddr)
		}

		preemptiveTicker := time.NewTicker(preemptiveInterval)
		defer preemptiveTicker.Stop()

		retrievedErrorsLeft := 1
		if origin {
			retrievedErrorsLeft = maxRetrievedErrors
		}

		done := make(chan struct{})
		defer close(done)

		resultC := make(chan retrievalResult, 1)
		retryC := make(chan struct{})

		retry := func() {
			go func() {
				select {
				case retryC <- struct{}{}:
				case <-done:
				}
			}()
		}

		retry()

		inflight := 0

		for retrievedErrorsLeft > 0 {

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-preemptiveTicker.C:
				retry()
			case <-retryC:

				totalRetrieveAttempts++
				s.metrics.PeerRequestCounter.Inc()

				inflight++

				go func() {
					ctx := tracing.WithContext(context.Background(), tracing.FromContext(topCtx))
					span, _, ctx := s.tracer.StartSpanFromContext(ctx, "retrieve-chunk", s.logger, opentracing.Tag{Key: "address", Value: addr.String()})
					defer span.Finish()
					ctx, cancel := context.WithTimeout(ctx, retrieveChunkTimeout)
					defer cancel()

					s.retrieveChunk(ctx, done, resultC, addr, sp, origin)
				}()
			case res := <-resultC:

				inflight--

				if errors.Is(res.err, topology.ErrNotFound) {
					if sp.OverdraftListEmpty() { // no peer is available, and none skipped due to overdraft errors
						if inflight == 0 {
							loggerV1.Debug("no peers left to retry", "chunk_address", addr)
							return nil, storage.ErrNotFound
						}
						continue
					}

					// there are overdrafted peers, so we want to retry with them again
					loggerV1.Debug("peers are overdrafted, retrying", "chunk_address", addr)
					sp.ResetOverdraft()
					select {
					case <-time.After(resetOverdraftDur): // wait at least 600 milliseconds
						retry()
						continue
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}

				if res.retrieveAttempted {
					retrievedErrorsLeft--
				}

				if res.err != nil {
					loggerV1.Debug("failed to get chunk", "chunk_address", addr, "peer_address", res.peer, "error", res.err)
					retry()
				} else {
					loggerV1.Debug("retrieved chunk", "chunk_address", addr, "peer_address", res.peer)
					return res.chunk, nil
				}
			}
		}

		loggerV1.Debug("no peers left to retry", "chunk_address", addr)
		return nil, storage.ErrNotFound
	})
	if err != nil {
		s.metrics.RequestFailureCounter.Inc()

		s.logger.Debug("retrieval failed", "chunk_address", addr, "error", err)
		return nil, err
	}

	s.metrics.RequestSuccessCounter.Inc()

	return v.(swarm.Chunk), nil
}

func (s *Service) retrieveChunk(ctx context.Context, done chan struct{}, result chan retrievalResult, addr swarm.Address, sp *skippeers.List, isOrigin bool) {

	startTimer := time.Now()
	// allow upstream requests if this node is the source of the request
	// i.e. the request was not forwarded, to improve retrieval
	// if this node is the closest to he chunk but still does not contain it
	allowUpstream := isOrigin

	var (
		err               error
		peer              swarm.Address
		retrieveAttempted bool
		chunk             swarm.Chunk
	)

	defer func() {
		if err != nil {
			s.metrics.TotalErrors.Inc()
		}
		select {
		case result <- retrievalResult{err: err, chunk: chunk, retrieveAttempted: retrieveAttempted, peer: peer}:
		case <-done:
			return
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, retrieveChunkTimeout)
	defer cancel()
	peer, err = s.closestPeer(addr, sp.All(), allowUpstream)
	if err != nil {
		err = fmt.Errorf("get closest for address %s, allow upstream %v: %w", addr.String(), allowUpstream, err)
		return
	}

	// compute the peer's price for this chunk for price header
	chunkPrice := s.pricer.PeerPrice(peer, addr)

	creditCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Reserve to see whether we can request the chunk
	creditAction, err := s.accounting.PrepareCredit(creditCtx, peer, chunkPrice, isOrigin)
	if err != nil {
		sp.AddOverdraft(peer)
		return
	}
	defer creditAction.Cleanup()

	sp.Add(peer)

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
	err = w.WriteMsgWithContext(ctx, &pb.Request{Addr: addr.Bytes()})
	if err != nil {
		err = fmt.Errorf("write request: %w peer %s", err, peer.String())
		return
	}

	retrieveAttempted = true

	var d pb.Delivery
	err = r.ReadMsgWithContext(ctx, &d)
	if err != nil {
		err = fmt.Errorf("read delivery: %w peer %s", err, peer.String())
		return
	}
	s.metrics.ChunkRetrieveTime.Observe(time.Since(startTimer).Seconds())
	s.metrics.TotalRetrieved.Inc()

	chunk = swarm.NewChunk(addr, d.Data)
	if !cac.Valid(chunk) {
		if !soc.Valid(chunk) {
			s.metrics.InvalidChunkRetrieved.Inc()
			err = swarm.ErrInvalidChunk
			return
		}
	}

	// credit the peer after successful delivery
	err = creditAction.Apply()
	if err != nil {
		return
	}
	s.metrics.ChunkPrice.Observe(float64(chunkPrice))
}

// closestPeer returns address of the peer that is closest to the chunk with
// provided address addr. This function will ignore peers with addresses
// provided in skipPeers and if allowUpstream is true, peers that are further of
// the chunk than this node is, could also be returned, allowing the upstream
// retrieve request.
func (s *Service) closestPeer(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {

	closest, err := s.peerSuggester.ClosestPeer(addr, false, topology.Filter{Reachable: true}, skipPeers...)
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

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	loggerV1 := s.logger.V(1).Register()

	ctx, cancel := context.WithTimeout(ctx, retrieveChunkTimeout)
	defer cancel()

	w, r := protobuf.NewWriterAndReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()
	var req pb.Request
	if err := r.ReadMsgWithContext(ctx, &req); err != nil {
		return fmt.Errorf("read request: %w peer %s", err, p.Address.String())
	}

	span, _, ctx := s.tracer.StartSpanFromContext(ctx, "handle-retrieve-chunk", s.logger, opentracing.Tag{Key: "address", Value: swarm.NewAddress(req.Addr).String()})
	defer span.Finish()

	addr := swarm.NewAddress(req.Addr)

	forwarded := false
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

	if err := w.WriteMsgWithContext(ctx, &pb.Delivery{
		Data: chunk.Data(),
	}); err != nil {
		return fmt.Errorf("write delivery: %w peer %s", err, p.Address.String())
	}

	loggerV1.Debug("retrieval protocol debiting peer", "peer_address", p.Address)

	// debit price from p's balance
	if err := debit.Apply(); err != nil {
		return fmt.Errorf("apply debit: %w", err)
	}

	// cache the request last, so that putting to the localstore does not slow down the request flow
	if s.caching && forwarded {
		err = s.storer.Cache().Put(ctx, chunk)
		if err != nil {
			return fmt.Errorf("retrieve cache put: %w", err)
		}
	}
	return nil
}
