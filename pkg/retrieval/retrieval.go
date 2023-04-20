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
	chunk swarm.Chunk
	peer  swarm.Address
	err   error
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
	errSkip       *skippeers.List
}

func New(addr swarm.Address, storer storage.Storer, streamer p2p.Streamer, chunkPeerer topology.ClosestPeerer, logger log.Logger, accounting accounting.Interface, pricer pricer.Interface, tracer *tracing.Tracer, forwarderCaching bool, validStamp postage.ValidStampFn) *Service {
	s := &Service{
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
		errSkip:       skippeers.NewList(),
	}

	return s
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
	retrieveChunkTimeout = time.Second * 10
	preemptiveInterval   = time.Second
	overDraftRefresh     = time.Second
	skiplistDur          = time.Minute
	maxRetrievedErrors   = 32
	originSuffix         = "_origin"
)

func (s *Service) RetrieveChunk(ctx context.Context, chunkAddr, sourcePeerAddr swarm.Address) (swarm.Chunk, error) {
	loggerV1 := s.logger.V(1).Register()

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

	// topCtx is passing the tracing span to the first singleflight call
	topCtx := ctx

	v, _, err := s.singleflight.Do(topCtx, flightRoute, func(ctx context.Context) (interface{}, error) {

		skip := skippeers.NewList()
		defer skip.Close()

		var preemptiveTicker <-chan time.Time

		if !sourcePeerAddr.IsZero() {
			skip.Add(chunkAddr, sourcePeerAddr, skippeers.MaxDuration)
		}

		errorsLeft := 1
		if origin {
			ticker := time.NewTicker(preemptiveInterval)
			defer ticker.Stop()
			preemptiveTicker = ticker.C
			errorsLeft = maxRetrievedErrors
		}

		done := make(chan struct{})
		defer close(done)

		resultC := make(chan retrievalResult, 1)
		retryC := make(chan struct{}, 1)

		retry := func() {
			select {
			case <-ctx.Done():
			case retryC <- struct{}{}:
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

				inflight++

				go func() {
					ctx := tracing.WithContext(context.Background(), tracing.FromContext(topCtx))
					span, _, ctx := s.tracer.StartSpanFromContext(ctx, "retrieve-chunk", s.logger, opentracing.Tag{Key: "address", Value: chunkAddr.String()})
					defer span.Finish()
					s.retrieveChunk(ctx, chunkAddr, skip, done, resultC, origin)
				}()

			case res := <-resultC:

				inflight--

				if res.err == nil {
					loggerV1.Debug("retrieved chunk", "chunk_address", chunkAddr, "peer_address", res.peer)
					return res.chunk, nil
				}

				loggerV1.Debug("failed to get chunk", "chunk_address", chunkAddr, "peer_address", res.peer, "error", res.err)

				// peer is overdrafted, skip to next result
				if errors.Is(res.err, accounting.ErrOverdraft) {
					retry()
					continue
				}

				// no peers left
				if errors.Is(res.err, topology.ErrNotFound) {
					if skip.PruneExpiresAfter(chunkAddr, overDraftRefresh) == 0 { //no overdraft peers, we have depleted ALL peers
						if inflight == 0 {
							loggerV1.Debug("no peers left", "chunk_address", chunkAddr, "error", res.err)
							return nil, res.err
						} else {
							continue // there is still an inflight request, wait for it's result
						}
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

				// the above errors are the ONLY cases that we consider as non-fatal

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

	return v.(swarm.Chunk), nil
}

func (s *Service) retrieveChunk(ctx context.Context, addr swarm.Address, skip *skippeers.List, done chan struct{}, result chan retrievalResult, isOrigin bool) {

	var (
		startTime = time.Now()
		err       error
		chunk     swarm.Chunk
		peer      swarm.Address
	)

	defer func() {
		if err != nil {
			s.metrics.TotalErrors.Inc()
		}
		select {
		case result <- retrievalResult{err: err, chunk: chunk, peer: peer}:
		case <-done:
			return
		}
	}()

	fullSkip := append(skip.ChunkPeers(addr), s.errSkip.ChunkPeers(addr)...)

	peer, err = s.closestPeer(addr, fullSkip, isOrigin)
	if err != nil {
		err = fmt.Errorf("get closest for address %s, allow upstream %v: %w", addr, isOrigin, err)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, retrieveChunkTimeout)
	defer cancel()

	// compute the peer's price for this chunk for price header
	chunkPrice := s.pricer.PeerPrice(peer, addr)

	creditCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Reserve to see whether we can request the chunk
	creditAction, err := s.accounting.PrepareCredit(creditCtx, peer, chunkPrice, isOrigin)
	if err != nil {
		skip.Add(addr, peer, overDraftRefresh)
		return
	}
	defer creditAction.Cleanup()

	skip.Add(addr, peer, skippeers.MaxDuration)

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

	var d pb.Delivery
	err = r.ReadMsgWithContext(ctx, &d)
	if err != nil {
		err = fmt.Errorf("read delivery: %w peer %s", err, peer.String())
		return
	}
	s.metrics.ChunkRetrieveTime.Observe(time.Since(startTime).Seconds())
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

	addr := swarm.NewAddress(req.Addr)

	if addr.IsZero() || addr.IsEmpty() || !addr.IsValidLength() {
		return fmt.Errorf("invalid address queried by peer %s", p.Address.String())
	}

	span, _, ctx := s.tracer.StartSpanFromContext(ctx, "handle-retrieve-chunk", s.logger, opentracing.Tag{Key: "address", Value: addr.String()})
	defer span.Finish()

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

func (s *Service) Close() error {
	return s.errSkip.Close()
}
