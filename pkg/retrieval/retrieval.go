// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	pb "github.com/ethersphere/bee/pkg/retrieval/pb"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
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
	streamer      p2p.StreamerDisconnecter
	peerSuggester topology.EachPeerer
	storer        storage.Storer
	singleflight  singleflight.Group
	logger        logging.Logger
	accounting    accounting.Interface
	pricer        accounting.Pricer
	validator     swarm.Validator
}

func New(streamer p2p.StreamerDisconnecter, storer storage.Storer, chunkPeerer topology.EachPeerer, logger logging.Logger, accounting accounting.Interface, pricer accounting.Pricer, validator swarm.Validator) *Service {
	return &Service{
		streamer:      streamer,
		peerSuggester: chunkPeerer,
		storer:        storer,
		logger:        logger,
		accounting:    accounting,
		pricer:        pricer,
		validator:     validator,
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
	blocklistDuration    = time.Minute
)

func (s *Service) RetrieveChunk(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {

	v, err, _ := s.singleflight.Do(addr.String(), func() (interface{}, error) {
		var skipPeers []swarm.Address
		for i := 0; i < maxPeers; i++ {
			var peer swarm.Address
			chunk, peer, err := s.retrieveChunk(ctx, addr, skipPeers)
			if err != nil {
				if peer.IsZero() {
					return nil, err
				}
				s.logger.Debugf("retrieval: failed to get chunk %s from peer %s: %v", addr, peer, err)
				skipPeers = append(skipPeers, peer)
				if errors.Is(err, context.DeadlineExceeded) {
					if err := s.streamer.Blocklist(peer, blocklistDuration); err != nil {
						s.logger.Errorf("retrieval: unable to block peer %s", peer)
						s.logger.Debugf("retrieval: blocking peer %s: %v", peer, err)
					} else {
						s.logger.Warningf("retrieval: peer %s blocked as unresponsive", peer)
					}
				}
				continue
			}
			s.logger.Tracef("retrieval: got chunk %s from peer %s", addr, peer)
			return chunk, nil
		}
		s.logger.Tracef("retrieval: failed to get chunk %s: reached max peers of %v", addr, maxPeers)
		return nil, storage.ErrNotFound
	})
	if err != nil {
		return nil, err
	}

	return v.(swarm.Chunk), nil
}

func (s *Service) retrieveChunk(ctx context.Context, addr swarm.Address, skipPeers []swarm.Address) (chunk swarm.Chunk, peer swarm.Address, err error) {
	v := ctx.Value(requestSourceContextKey{})
	if src, ok := v.(string); ok {
		skipAddr, err := swarm.ParseHexAddress(src)
		if err == nil {
			skipPeers = append(skipPeers, skipAddr)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, retrieveChunkTimeout)
	defer cancel()

	peer, err = s.closestPeer(addr, skipPeers)
	if err != nil {
		return nil, peer, fmt.Errorf("get closest: %w", err)
	}

	// compute the price we pay for this chunk and reserve it for the rest of this function
	chunkPrice := s.pricer.PeerPrice(peer, addr)
	err = s.accounting.Reserve(peer, chunkPrice)
	if err != nil {
		return nil, peer, fmt.Errorf("accounting retrieve: %w", err)
	}
	defer s.accounting.Release(peer, chunkPrice)

	s.logger.Tracef("retrieval: requesting chunk %s from peer %s", addr, peer)
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
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
		return nil, peer, fmt.Errorf("write request: %w", err)
	}

	var d pb.Delivery
	if err := r.ReadMsgWithContext(ctx, &d); err != nil {
		return nil, peer, fmt.Errorf("read delivery: %w", err)
	}

	// credit the peer after successful delivery
	chunk = swarm.NewChunk(addr, d.Data)
	if !s.validator.Validate(chunk) {
		return nil, peer, fmt.Errorf("new chunk: %w", err)
	}

	err = s.accounting.Credit(peer, chunkPrice)
	if err != nil {
		return nil, peer, fmt.Errorf("accounting credit: %w", err)
	}

	return chunk, peer, nil
}

func (s *Service) closestPeer(addr swarm.Address, skipPeers []swarm.Address) (swarm.Address, error) {
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
	if err := r.ReadMsg(&req); err != nil {
		return fmt.Errorf("read request: %w", err)
	}
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
		return fmt.Errorf("write delivery: %w", err)
	}

	// compute the price we charge for this chunk and debit it from p's balance
	chunkPrice := s.pricer.Price(chunk.Address())
	err = s.accounting.Debit(p.Address, chunkPrice)
	if err != nil {
		return err
	}

	return nil
}
