//go:build js
// +build js

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

type Service struct {
	addr          swarm.Address
	radiusFunc    func() (uint8, error)
	streamer      p2p.Streamer
	peerSuggester topology.ClosestPeerer
	storer        Storer
	singleflight  singleflight.Group[string, swarm.Chunk]
	logger        log.Logger
	accounting    accounting.Interface
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
		tracer:        tracer,
		caching:       forwarderCaching,
		errSkip:       skippeers.NewList(time.Minute),
	}
}

func (s *Service) RetrieveChunk(ctx context.Context, chunkAddr, sourcePeerAddr swarm.Address) (swarm.Chunk, error) {
	loggerV1 := s.logger

	origin := sourcePeerAddr.IsZero()

	if chunkAddr.IsZero() || chunkAddr.IsEmpty() || !chunkAddr.IsValidLength() {
		return nil, fmt.Errorf("invalid address queried")
	}

	flightRoute := chunkAddr.String()
	if origin {
		flightRoute = chunkAddr.String() + originSuffix
	}

	totalRetrieveAttempts := 0

	spanCtx := context.WithoutCancel(ctx)

	v, _, err := s.singleflight.Do(ctx, flightRoute, func(ctx context.Context) (swarm.Chunk, error) {

		skip := skippeers.NewList(0)
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
		s.logger.Debug("retrieval failed", "chunk_address", chunkAddr, "error", err)
		return nil, err
	}

	return v, nil
}

func (s *Service) retrieveChunk(ctx context.Context, quit chan struct{}, chunkAddr, peer swarm.Address, result chan retrievalResult, action accounting.Action, span opentracing.Span) {

	var (
		err   error
		chunk swarm.Chunk
	)

	defer func() {
		action.Cleanup()
		if err != nil {
			ext.LogError(span, err)
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

	chunk = swarm.NewChunk(chunkAddr, d.Data)
	if !cac.Valid(chunk) {
		if !soc.Valid(chunk) {
			err = swarm.ErrInvalidChunk
			return
		}
	}

	err = action.Apply()
}

func (s *Service) prepareCredit(ctx context.Context, peer, chunk swarm.Address, origin bool) (accounting.Action, error) {

	price := s.pricer.PeerPrice(peer, chunk)

	creditAction, err := s.accounting.PrepareCredit(ctx, peer, price, origin)
	if err != nil {
		return nil, err
	}

	return creditAction, nil
}
