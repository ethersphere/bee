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
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pricer"
	pb "github.com/ethersphere/bee/pkg/retrieval/pb"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/opentracing/opentracing-go"
	"resenje.org/singleflight"
)

type requestSourceContextKey struct{}

const (
	protocolName    = "retrieval"
	protocolVersion = "1.1.0"
	streamName      = "retrieval"
)

var _ Interface = (*Service)(nil)

type Interface interface {
	RetrieveChunk(ctx context.Context, addr swarm.Address, origin bool) (chunk swarm.Chunk, err error)
}

type retrievalResult struct {
	chunk     swarm.Chunk
	peer      swarm.Address
	err       error
	retrieved bool
}

type Service struct {
	addr          swarm.Address
	streamer      p2p.Streamer
	peerSuggester topology.EachPeerer
	storer        storage.Storer
	singleflight  singleflight.Group
	logger        logging.Logger
	accounting    accounting.Interface
	metrics       metrics
	pricer        pricer.Interface
	tracer        *tracing.Tracer
	caching       bool
	validStamp    postage.ValidStampFn
}

func New(addr swarm.Address, storer storage.Storer, streamer p2p.Streamer, chunkPeerer topology.EachPeerer, logger logging.Logger, accounting accounting.Interface, pricer pricer.Interface, tracer *tracing.Tracer, forwarderCaching bool, validStamp postage.ValidStampFn) *Service {
	return &Service{
		addr:          addr,
		streamer:      streamer,
		peerSuggester: chunkPeerer,
		storer:        storer,
		logger:        logger,
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
	defaultTTL   = 10 * time.Second
	p90TTL       = time.Second
	maxRetries   = 32
	maxRetrieved = 8
	originSuffix = "_origin"
)

func (s *Service) RetrieveChunk(ctx context.Context, addr swarm.Address, origin bool) (swarm.Chunk, error) {
	s.metrics.RequestCounter.Inc()

	flightRoute := addr.String()
	if origin {
		flightRoute = addr.String() + originSuffix
	}

	v, _, err := s.singleflight.Do(ctx, flightRoute, func(_ context.Context) (interface{}, error) {

		sp := newSkipPeers()

		var (
			allowedRetries   = maxRetries
			allowedRetrieves = 1
		)

		if origin {
			allowedRetrieves = maxRetrieved
		}

		timer := time.NewTimer(0)
		defer timer.Stop()

		resultChan := make(chan retrievalResult)
		doneChan := make(chan struct{})
		defer close(doneChan)

		for {

			select {
			case <-ctx.Done():
				return nil, storage.ErrNotFound
			case <-timer.C:

				allowedRetries--
				allowedRetrieves--

				s.metrics.PeerRequestCounter.Inc()

				// create a new context without cancelation but
				// set the tracing span to the new context from the context of the first caller
				ctx := tracing.WithContext(context.Background(), tracing.FromContext(ctx))

				// get the tracing span
				span, _, ctx := s.tracer.StartSpanFromContext(ctx, "retrieve-chunk", s.logger, opentracing.Tag{Key: "address", Value: addr.String()})
				defer span.Finish()

				go func() {
					// cancel the goroutine just with the timeout
					ctx, cancel := context.WithTimeout(ctx, defaultTTL)
					defer cancel()
					s.retrieveChunk(ctx, resultChan, doneChan, addr, sp, origin)
				}()

				if allowedRetries <= 0 || allowedRetrieves <= 0 {
					continue
				}

				timer.Reset(p90TTL)

			case res := <-resultChan:

				if errors.Is(res.err, topology.ErrNotFound) {
					if sp.Saturated() {
						// if no peer is available, and none skipped temporarily
						s.logger.Tracef("retrieval: failed to get chunk %s", addr)
						return nil, storage.ErrNotFound
					}
				}

				if res.retrieved {
					if res.err == nil {
						return res.chunk, nil
					}
					if !res.peer.IsZero() {
						s.logger.Debugf("retrieval: failed to get chunk %s from peer %s: %v", addr, res.peer, res.err)
					}
				} else {
					allowedRetrieves++
				}

				if allowedRetries <= 0 || allowedRetrieves <= 0 {
					return nil, storage.ErrNotFound
				}

				// retry immediately
				timer.Reset(0)
			}
		}
	})
	if err != nil {
		return nil, err
	}

	return v.(swarm.Chunk), nil
}

func (s *Service) retrieveChunk(ctx context.Context, resultChan chan retrievalResult, doneChan chan struct{}, addr swarm.Address, sp *skipPeers, originated bool) {

	var (
		peer       swarm.Address
		err        error
		chunk      swarm.Chunk
		retrieved  bool
		startTimer = time.Now()
	)

	defer func() {
		select {
		case resultChan <- retrievalResult{peer: peer, err: err, retrieved: retrieved, chunk: chunk}:
		case <-doneChan:
		}
	}()

	v := ctx.Value(requestSourceContextKey{})
	// allow upstream requests if this node is the source of the request
	// i.e. the request was not forwarded, to improve retrieval
	// if this node is the closest to he chunk but still does not contain it
	allowUpstream := true
	if src, ok := v.(string); ok {
		sourcePeerAddr, err := swarm.ParseHexAddress(src)
		if err == nil {
			sp.Add(sourcePeerAddr)
		}
		// do not allow upstream requests if the request was forwarded to this node
		// to avoid the request loops
		allowUpstream = false
	}

	peer, err = s.closestPeer(addr, sp.All(), allowUpstream)
	if err != nil {
		err = fmt.Errorf("get closest for address %s, allow upstream %v: %w", addr.String(), allowUpstream, err)
		return
	}

	// compute the peer's price for this chunk for price header
	chunkPrice := s.pricer.PeerPrice(peer, addr)

	// Reserve to see whether we can request the chunk
	creditAction, err := s.accounting.PrepareCredit(peer, chunkPrice, originated)
	if err != nil {
		sp.AddOverdraft(peer)
	}
	defer creditAction.Cleanup()

	sp.Add(peer)

	s.logger.Tracef("retrieval: requesting chunk %s from peer %s", addr, peer)

	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		s.metrics.TotalErrors.Inc()
		err = fmt.Errorf("new stream: %w", err)
		return
	}

	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()

	w, r := protobuf.NewWriterAndReader(stream)
	err = w.WriteMsgWithContext(ctx, &pb.Request{
		Addr: addr.Bytes(),
	})
	if err != nil {
		s.metrics.TotalErrors.Inc()
		err = fmt.Errorf("write request: %w peer %s", err, peer.String())
		return
	}

	retrieved = true

	var d pb.Delivery
	err = r.ReadMsgWithContext(ctx, &d)
	if err != nil {
		s.metrics.TotalErrors.Inc()
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
			s.metrics.TotalErrors.Inc()
			err = swarm.ErrInvalidChunk
			chunk = nil
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
	closest := swarm.Address{}
	err := s.peerSuggester.EachPeerRev(func(peer swarm.Address, po uint8) (bool, bool, error) {
		for _, a := range skipPeers {
			if a.Equal(peer) {
				return false, false, nil
			}
		}
		if closest.IsZero() {
			closest = peer
			return false, false, nil
		}
		closer, err := peer.Closer(addr, closest)
		if err != nil {
			return false, false, fmt.Errorf("distance compare error. addr %s closest %s peer %s: %w", addr.String(), closest.String(), peer.String(), err)
		}
		if closer {
			closest = peer
		}
		return false, false, nil
	}, topology.Filter{Reachable: true})
	if err != nil {
		return swarm.Address{}, err
	}

	// check if found
	if closest.IsZero() {
		return swarm.Address{}, topology.ErrNotFound
	}
	if allowUpstream {
		return closest, nil
	}

	closer, err := closest.Closer(addr, s.addr)
	if err != nil {
		return swarm.Address{}, fmt.Errorf("distance compare addr %s closest %s base address %s: %w", addr.String(), closest.String(), s.addr.String(), err)
	}
	if closer {
		return swarm.Address{}, topology.ErrNotFound
	}

	return closest, nil
}

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
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

	ctx = context.WithValue(ctx, requestSourceContextKey{}, p.Address.String())
	addr := swarm.NewAddress(req.Addr)

	forwarded := false
	chunk, err := s.storer.Get(ctx, storage.ModeGetRequest, addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// forward the request
			chunk, err = s.RetrieveChunk(ctx, addr, false)
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
	debit, err := s.accounting.PrepareDebit(p.Address, chunkPrice)
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

	s.logger.Tracef("retrieval protocol debiting peer %s", p.Address.String())

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
