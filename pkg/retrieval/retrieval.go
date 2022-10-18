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
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pricer"
	pb "github.com/ethersphere/bee/pkg/retrieval/pb"
	"github.com/ethersphere/bee/pkg/skippeers"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
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
	protocolVersion = "1.2.0"
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

type Service struct {
	addr          swarm.Address
	streamer      p2p.Streamer
	peerSuggester topology.ClosestPeerer
	storer        storage.Storer
	singleflight  singleflight.Group
	logger        log.Logger
	accounting    accounting.Interface
	metrics       metrics
	pricer        pricer.Interface
	tracer        *tracing.Tracer
	caching       bool
	validStamp    postage.ValidStampFn
}

func New(addr swarm.Address, storer storage.Storer, streamer p2p.Streamer, chunkPeerer topology.ClosestPeerer, logger log.Logger, accounting accounting.Interface, pricer pricer.Interface, tracer *tracing.Tracer, forwarderCaching bool, validStamp postage.ValidStampFn) *Service {
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
		validStamp:    validStamp,
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

		retryC := make(chan struct{}, 1)

		retry := func() {
			select {
			case retryC <- struct{}{}:
			default:
			}
		}

		retryC <- struct{}{}
		resultC := make(chan retrievalResult, 1)

		for retrievedErrorsLeft > 0 {

			select {
			case <-ctx.Done():
				loggerV1.Debug("failed to get chunk", "chunk_address", addr, "error", ctx.Err())
				return nil, fmt.Errorf("retrieval: %w", ctx.Err())
			case <-preemptiveTicker.C:
				retry()
			case <-retryC:

				ctx := tracing.WithContext(context.Background(), tracing.FromContext(topCtx))

				// get the tracing span
				span, _, ctx := s.tracer.StartSpanFromContext(ctx, "retrieve-chunk", s.logger, opentracing.Tag{Key: "address", Value: addr.String()})
				defer span.Finish()

				s.metrics.PeerRequestCounter.Inc()
				go func() {
					ctx, cancel := context.WithTimeout(ctx, retrieveChunkTimeout)
					defer cancel()
					s.retrieveChunk(ctx, done, resultC, addr, sp, origin)
				}()
			case res := <-resultC:
				if errors.Is(res.err, topology.ErrNotFound) {
					if sp.OverdraftListEmpty() { // no peer is available, and none skipped due to overdraft errors
						loggerV1.Debug("no peers left to retry", "chunk_address", addr)
						return nil, storage.ErrNotFound
					}
					// there are overdrafted peers, so we want to retry with them again, let ticker trigger another retry
					sp.ResetOverdraft()
					select {
					case <-time.After(resetOverdraftDur): // wait at least 600 milliseconds
						retry()
						continue
					case <-ctx.Done():
						loggerV1.Debug("failed to get chunk", "chunk_address", addr, "error", ctx.Err())
						return nil, fmt.Errorf("retrieval: %w", ctx.Err())
					}
				}

				if res.retrieveAttempted {
					retrievedErrorsLeft--
					if res.err != nil {
						s.logger.Debug("failed to get chunk from peer", "peer_address", res.peer, "chunk_address", addr, "error", res.err)
					} else {
						s.logger.Debug("retrieved chunk from peer", "peer_address", res.peer, "chunk_address", addr)
						return res.chunk, nil
					}
				}

				retry()
			}
		}

		loggerV1.Debug("ran out of attempts", "chunk_address", addr)
		return nil, storage.ErrNotFound
	})

	if err != nil {
		return nil, err
	}

	return v.(swarm.Chunk), nil
}

func (s *Service) retrieveChunk(ctx context.Context, done chan struct{}, result chan retrievalResult, addr swarm.Address, sp *skippeers.List, isOrigin bool) {
	loggerV1 := s.logger.V(1).Build()

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
		select {
		case result <- retrievalResult{err: err, chunk: chunk, retrieveAttempted: retrieveAttempted, peer: peer}:
		case <-done:
			return
		}
	}()

	defer func() {
		if err != nil {
			s.metrics.TotalErrors.Inc()
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

	loggerV1.Debug("requesting chunk from peer", "chunk_address", addr, "peer_address", peer)

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

	stamp := new(postage.Stamp)
	err = stamp.UnmarshalBinary(d.Stamp)
	if err != nil {
		err = fmt.Errorf("stamp unmarshal: %w", err)
		return
	}
	chunk = swarm.NewChunk(addr, d.Data).WithStamp(stamp)
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
	chunk, err := s.storer.Get(ctx, storage.ModeGetRequest, addr)
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
	stamp, err := chunk.Stamp().MarshalBinary()
	if err != nil {
		return fmt.Errorf("stamp marshal: %w", err)
	}

	chunkPrice := s.pricer.Price(chunk.Address())
	debit, err := s.accounting.PrepareDebit(ctx, p.Address, chunkPrice)
	if err != nil {
		return fmt.Errorf("prepare debit to peer %s before writeback: %w", p.Address.String(), err)
	}
	defer debit.Cleanup()

	if err := w.WriteMsgWithContext(ctx, &pb.Delivery{
		Data:  chunk.Data(),
		Stamp: stamp,
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
		putMode := storage.ModePutRequest

		cch, err := s.validStamp(chunk, stamp)
		if err != nil {
			// if a chunk with an invalid postage stamp was received
			// we force it into the cache.
			putMode = storage.ModePutRequestCache
			cch = chunk
		}

		_, err = s.storer.Put(ctx, putMode, cch)
		if err != nil {
			return fmt.Errorf("retrieve cache put: %w", err)
		}
	}
	return nil
}
