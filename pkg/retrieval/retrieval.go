// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/content"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
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
	RetrieveChunk(ctx context.Context, addr swarm.Address) (chunk swarm.Chunk, err error)
}

type Service struct {
	addr          swarm.Address
	streamer      p2p.Streamer
	peerSuggester topology.EachPeerer
	storer        storage.Storer
	singleflight  singleflight.Group
	logger        logging.Logger
	accounting    accounting.Interface
	pricer        accounting.Pricer
	metrics       metrics
	tracer        *tracing.Tracer
}

func New(addr swarm.Address, storer storage.Storer, streamer p2p.Streamer, chunkPeerer topology.EachPeerer, logger logging.Logger, accounting accounting.Interface, pricer accounting.Pricer, tracer *tracing.Tracer) *Service {
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
	maxPeers             = 5
	retrieveChunkTimeout = 10 * time.Second

	retrieveRetryIntervalDuration = 5 * time.Second
)

type retrieveChunkResult struct {
	chunk swarm.Chunk
	err   error
}

func (s *Service) RetrieveChunk(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	ctx, cancel := context.WithTimeout(ctx, maxPeers*retrieveChunkTimeout)

	var wg sync.WaitGroup

	wg.Add(1)

	defer func() {
		wg.Done()
	}()

	go func() {
		wg.Wait()
		cancel()
	}()

	s.metrics.RequestCounter.Inc()

	v, err, _ := s.singleflight.Do(addr.String(), func() (interface{}, error) {
		span, logger, ctx := s.tracer.StartSpanFromContext(ctx, "retrieve-chunk", s.logger, opentracing.Tag{Key: "address", Value: addr.String()})
		defer span.Finish()

		sps := newSkipPeersService()

		ticker := time.NewTicker(retrieveRetryIntervalDuration)
		defer ticker.Stop()

		var (
			peerAttempt  int
			peersResults int
			resultC      = make(chan retrieveChunkResult)
		)

		wg.Add(maxPeers)

		defer func() {
			if peerAttempt != maxPeers {
				wg.Add(peerAttempt - maxPeers)
			}
		}()

		go func() {
			wg.Wait()
			close(resultC)
		}()

		for {
			if peerAttempt < maxPeers {
				peerAttempt++

				s.metrics.PeerRequestCounter.Inc()

				go func() {
					defer wg.Done()

					chunk, peer, err := s.retrieveChunk(ctx, addr, sps)
					if err != nil {
						if !peer.IsZero() {
							logger.Debugf("retrieval: failed to get chunk %s from peer %s: %v", addr, peer, err)
						}

						select {
						case resultC <- retrieveChunkResult{err: err}:
						default:
						}
						return
					}

					select {
					case resultC <- retrieveChunkResult{chunk: chunk}:
					default:
					}
				}()
			} else {
				ticker.Stop()
			}

			select {
			case <-ticker.C:
				// break
			case result := <-resultC:
				peersResults++

				if result.err != nil {
					break
				}

				if result.chunk == nil {
					logger.Tracef("retrieval: failed to get chunk %s", addr)
					return nil, storage.ErrNotFound
				}

				return result.chunk, nil
			case <-ctx.Done():
				logger.Tracef("retrieval: failed to get chunk %s: %v", addr, ctx.Err())
				return nil, storage.ErrNotFound
			}

			// all results received
			if peersResults >= maxPeers {
				logger.Tracef("retrieval: failed to get chunk %s", addr)
				return nil, storage.ErrNotFound
			}
		}
	})
	if err != nil {
		return nil, err
	}

	return v.(swarm.Chunk), nil
}

func (s *Service) retrieveChunk(ctx context.Context, addr swarm.Address, sps *skipPeersService) (chunk swarm.Chunk, peer swarm.Address, err error) {
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
			sps.AddAddressToSkip(sourcePeerAddr)
		}
		// do not allow upstream requests if the request was forwarded to this node
		// to avoid the request loops
		allowUpstream = false
	}

	ctx, cancel := context.WithTimeout(ctx, retrieveChunkTimeout)
	defer cancel()

	peer, err = s.closestPeer(addr, sps.Addresses(), allowUpstream)
	if err != nil {
		return nil, peer, fmt.Errorf("get closest for address %s, allow upstream %v: %w", addr.String(), allowUpstream, err)
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

	sps.AddAddressToSkip(peer)

	// compute the price we pay for this chunk and reserve it for the rest of this function
	chunkPrice := s.pricer.PeerPrice(peer, addr)
	err = s.accounting.Reserve(ctx, peer, chunkPrice)
	if err != nil {
		return nil, peer, err
	}
	defer s.accounting.Release(peer, chunkPrice)

	s.logger.Tracef("retrieval: requesting chunk %s from peer %s", addr, peer)
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		s.metrics.TotalErrors.Inc()
		return nil, peer, fmt.Errorf("new stream: %w", err)
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
		return nil, peer, fmt.Errorf("write request: %w peer %s", err, peer.String())
	}

	var d pb.Delivery
	if err := r.ReadMsgWithContext(ctx, &d); err != nil {
		s.metrics.TotalErrors.Inc()
		return nil, peer, fmt.Errorf("read delivery: %w peer %s", err, peer.String())
	}
	s.metrics.RetrieveChunkPeerPOTimer.
		WithLabelValues(strconv.Itoa(int(peerPO))).
		Observe(time.Since(startTimer).Seconds())
	s.metrics.TotalRetrieved.Inc()

	chunk = swarm.NewChunk(addr, d.Data)
	if !content.Valid(chunk) {
		if !soc.Valid(chunk) {
			s.metrics.InvalidChunkRetrieved.Inc()
			s.metrics.TotalErrors.Inc()
			return nil, peer, swarm.ErrInvalidChunk
		}
	}

	// credit the peer after successful delivery
	err = s.accounting.Credit(peer, chunkPrice)
	if err != nil {
		return nil, peer, err
	}
	s.metrics.ChunkPrice.Observe(float64(chunkPrice))

	return chunk, peer, err
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
			chunk, err = s.RetrieveChunk(ctx, addr)
			if err != nil {
				return fmt.Errorf("retrieve chunk: %w", err)
			}
		} else {
			return fmt.Errorf("get from store: %w", err)
		}
	}

	if err := w.WriteMsgWithContext(ctx, &pb.Delivery{
		Data: chunk.Data(),
	}); err != nil {
		return fmt.Errorf("write delivery: %w peer %s", err, p.Address.String())
	}

	s.logger.Tracef("retrieval protocol debiting peer %s", p.Address.String())

	// compute the price we charge for this chunk and debit it from p's balance
	chunkPrice := s.pricer.Price(chunk.Address())
	err = s.accounting.Debit(p.Address, chunkPrice)
	if err != nil {
		return err
	}

	return nil
}
