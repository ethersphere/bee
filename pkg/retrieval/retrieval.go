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
	"strconv"
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
	"golang.org/x/sync/singleflight"
)

type requestSourceContextKey struct{}

const (
	protocolName    = "retrieval"
	protocolVersion = "1.0.0"
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
}

func New(addr swarm.Address, storer storage.Storer, streamer p2p.Streamer, chunkPeerer topology.EachPeerer, logger logging.Logger, accounting accounting.Interface, pricer pricer.Interface, tracer *tracing.Tracer) *Service {
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

	retrieveRetryIntervalDuration = 5 * time.Second
)

func (s *Service) RetrieveChunk(ctx context.Context, addr swarm.Address, origin bool) (swarm.Chunk, error) {
	s.metrics.RequestCounter.Inc()

	v, err, _ := s.singleflight.Do(addr.String(), func() (interface{}, error) {

		maxPeers := 1
		if origin {
			maxPeers = 8
		}

		span, logger, ctx := s.tracer.StartSpanFromContext(ctx, "retrieve-chunk", s.logger, opentracing.Tag{Key: "address", Value: addr.String()})
		defer span.Finish()

		sp := newSkipPeers()

		ticker := time.NewTicker(retrieveRetryIntervalDuration)
		defer ticker.Stop()

		var (
			peerAttempt  int
			peersResults int
			resultC      = make(chan retrievalResult, maxPeers)
		)

		requestAttempt := 0
		for requestAttempt < 3 {
			if peerAttempt < maxPeers {
				peerAttempt++

				s.metrics.PeerRequestCounter.Inc()

				go func() {
					chunk, peer, requested, err := s.retrieveChunk(ctx, addr, sp)
					resultC <- retrievalResult{
						chunk:     chunk,
						peer:      peer,
						err:       err,
						retrieved: requested,
					}
				}()
			} else {
				ticker.Stop()
			}

			select {
			case <-ticker.C:
				// break
			case res := <-resultC:
				if res.retrieved {
					if res.err != nil {
						if !res.peer.IsZero() {
							logger.Debugf("retrieval: failed to get chunk %s from peer %s: %v", addr, res.peer, res.err)
						}
						peersResults++
					} else {
						return res.chunk, nil
					}
				}
			case <-ctx.Done():
				logger.Tracef("retrieval: failed to get chunk %s: %v", addr, ctx.Err())
				return nil, fmt.Errorf("retrieval: %w", ctx.Err())
			}

			// all results received
			if peersResults >= maxPeers {
				logger.Tracef("retrieval: failed to get chunk %s", addr)
				return nil, storage.ErrNotFound
			}

			if peerAttempt >= maxPeers {
				requestAttempt++
				peerAttempt = 0
				sp = newSkipPeers()
			}
		}

		return nil, storage.ErrNotFound

	})
	if err != nil {
		return nil, err
	}

	return v.(swarm.Chunk), nil
}

func (s *Service) retrieveChunk(ctx context.Context, addr swarm.Address, sp *skipPeers) (chunk swarm.Chunk, peer swarm.Address, requested bool, err error) {
	startTimer := time.Now()

	v := ctx.Value(requestSourceContextKey{})
	sourcePeerAddr := swarm.Address{}
	// allow upstream requests if this node is the source of the request
	// i.e. the request was not forwarded, to improve retrieval
	// if this node is the closest to he chunk but still does not contain it
	allowUpstream := true
	if src, ok := v.(string); ok {
		sourcePeerAddr, err = swarm.ParseHexAddress(src)
		if err == nil {
			sp.Add(sourcePeerAddr)
		}
		// do not allow upstream requests if the request was forwarded to this node
		// to avoid the request loops
		allowUpstream = false
	}

	ctx, cancel := context.WithTimeout(ctx, retrieveChunkTimeout)
	defer cancel()
	peer, err = s.closestPeer(addr, sp.All(), allowUpstream)
	if err != nil {
		return nil, peer, false, fmt.Errorf("get closest for address %s, allow upstream %v: %w", addr.String(), allowUpstream, err)
	}

	peerPO := swarm.Proximity(s.addr.Bytes(), peer.Bytes())

	if !sourcePeerAddr.IsZero() {
		// is forwarded request
		sourceAddrPO := swarm.Proximity(sourcePeerAddr.Bytes(), addr.Bytes())
		addrPO := swarm.Proximity(peer.Bytes(), addr.Bytes())

		poGain := int(addrPO) - int(sourceAddrPO)

		s.metrics.RetrieveChunkPOGainCounter.
			WithLabelValues(strconv.Itoa(poGain)).
			Inc()
	}

	sp.Add(peer)

	// compute the peer's price for this chunk for price header
	chunkPrice := s.pricer.PeerPrice(peer, addr)

	// Reserve to see whether we can request the chunk
	err = s.accounting.Reserve(ctx, peer, chunkPrice)
	if err != nil {
		return nil, peer, false, err
	}
	defer s.accounting.Release(peer, chunkPrice)

	s.logger.Tracef("retrieval: requesting chunk %s from peer %s", addr, peer)
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		s.metrics.TotalErrors.Inc()
		return nil, peer, false, fmt.Errorf("new stream: %w", err)
	}

	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()

	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsgWithContext(ctx, &pb.Request{
		Addr: addr.Bytes(),
	}); err != nil {
		s.metrics.TotalErrors.Inc()
		return nil, peer, false, fmt.Errorf("write request: %w peer %s", err, peer.String())
	}

	var d pb.Delivery
	if err := r.ReadMsgWithContext(ctx, &d); err != nil {
		s.metrics.TotalErrors.Inc()
		return nil, peer, true, fmt.Errorf("read delivery: %w peer %s", err, peer.String())
	}
	s.metrics.RetrieveChunkPeerPOTimer.
		WithLabelValues(strconv.Itoa(int(peerPO))).
		Observe(time.Since(startTimer).Seconds())
	s.metrics.TotalRetrieved.Inc()

	stamp := new(postage.Stamp)
	err = stamp.UnmarshalBinary(d.Stamp)
	if err != nil {
		return nil, peer, true, fmt.Errorf("stamp unmarshal: %w", err)
	}
	chunk = swarm.NewChunk(addr, d.Data).WithStamp(stamp)
	if !cac.Valid(chunk) {
		if !soc.Valid(chunk) {
			s.metrics.InvalidChunkRetrieved.Inc()
			s.metrics.TotalErrors.Inc()
			return nil, peer, true, swarm.ErrInvalidChunk
		}
	}

	// credit the peer after successful delivery
	err = s.accounting.Credit(peer, chunkPrice)
	if err != nil {
		return nil, peer, true, err
	}
	s.metrics.ChunkPrice.Observe(float64(chunkPrice))

	return chunk, peer, true, err
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
		dcmp, err := swarm.DistanceCmp(addr.Bytes(), closest.Bytes(), peer.Bytes())
		if err != nil {
			return false, false, fmt.Errorf("distance compare error. addr %s closest %s peer %s: %w", addr.String(), closest.String(), peer.String(), err)
		}
		switch dcmp {
		case 0:
			// do nothing
		case -1:
			// current peer is closer
			closest = peer
		case 1:
			// closest is already closer to chunk
			// do nothing
		}
		return false, false, nil
	})
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

	dcmp, err := swarm.DistanceCmp(addr.Bytes(), closest.Bytes(), s.addr.Bytes())
	if err != nil {
		return swarm.Address{}, fmt.Errorf("distance compare addr %s closest %s base address %s: %w", addr.String(), closest.String(), s.addr.String(), err)
	}
	if dcmp != 1 {
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
	chunk, err := s.storer.Get(ctx, storage.ModeGetRequest, addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// forward the request
			chunk, err = s.RetrieveChunk(ctx, addr, false)
			if err != nil {
				return fmt.Errorf("retrieve chunk: %w", err)
			}
		} else {
			return fmt.Errorf("get from store: %w", err)
		}
	}

	stamp, err := chunk.Stamp().MarshalBinary()
	if err != nil {
		return fmt.Errorf("stamp marshal: %w", err)
	}

	chunkPrice := s.pricer.Price(chunk.Address())
	debit := s.accounting.PrepareDebit(p.Address, chunkPrice)
	defer debit.Cleanup()

	if err := w.WriteMsgWithContext(ctx, &pb.Delivery{
		Data:  chunk.Data(),
		Stamp: stamp,
	}); err != nil {
		return fmt.Errorf("write delivery: %w peer %s", err, p.Address.String())
	}

	s.logger.Tracef("retrieval protocol debiting peer %s", p.Address.String())

	// debit price from p's balance
	return debit.Apply()
}
