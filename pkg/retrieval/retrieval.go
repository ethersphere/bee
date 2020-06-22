// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval

import (
	"context"
	"fmt"
	"time"

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
	RetrieveChunk(ctx context.Context, addr swarm.Address) (data []byte, err error)
}

type Service struct {
	streamer      p2p.Streamer
	peerSuggester topology.EachPeerer
	storer        storage.Storer
	singleflight  singleflight.Group
	logger        logging.Logger
}

type Options struct {
	Streamer    p2p.Streamer
	ChunkPeerer topology.EachPeerer
	Storer      storage.Storer
	Logger      logging.Logger
}

func New(o Options) *Service {
	return &Service{
		streamer:      o.Streamer,
		peerSuggester: o.ChunkPeerer,
		storer:        o.Storer,
		logger:        o.Logger,
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
	maxPeers             = 10
	retrieveChunkTimeout = 3 * time.Second
)

func (s *Service) RetrieveChunk(ctx context.Context, addr swarm.Address) (data []byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, maxPeers*retrieveChunkTimeout)
	defer cancel()
	v, err, _ := s.singleflight.Do(addr.String(), func() (v interface{}, err error) {
		var skipPeers []swarm.Address
		for i := 0; i < maxPeers; i++ {
			var peer swarm.Address
			data, peer, err = s.retrieveChunk(ctx, addr, skipPeers)
			if err != nil {
				if peer.IsZero() {
					return nil, err
				}
				s.logger.Debugf("retrieval: failed to get chunk %s from peer %s: %v", addr, peer, err)
				skipPeers = append(skipPeers, peer)
				continue
			}
			s.logger.Tracef("retrieval: got chunk %s from peer %s", addr, peer)
			return data, nil
		}
		return nil, err
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}

func (s *Service) retrieveChunk(ctx context.Context, addr swarm.Address, skipPeers []swarm.Address) (data []byte, peer swarm.Address, err error) {
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
	s.logger.Tracef("retrieval: get chunk %s from peer %s", addr, peer)
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, peer, fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	w, r := protobuf.NewWriterAndReader(stream)

	if err := w.WriteMsgWithContext(ctx, &pb.Request{
		Addr: addr.Bytes(),
	}); err != nil {
		return nil, peer, fmt.Errorf("write request: %w peer %s", err, peer.String())
	}

	var d pb.Delivery
	if err := r.ReadMsgWithContext(ctx, &d); err != nil {
		return nil, peer, fmt.Errorf("read delivery: %w peer %s", err, peer.String())
	}

	return d.Data, peer, nil
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

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()
	var req pb.Request
	if err := r.ReadMsg(&req); err != nil {
		return fmt.Errorf("read request: %w peer %s", err, p.Address.String())
	}
	ctx = context.WithValue(ctx, requestSourceContextKey{}, p.Address.String())
	chunk, err := s.storer.Get(ctx, storage.ModeGetRequest, swarm.NewAddress(req.Addr))
	if err != nil {
		return fmt.Errorf("get from store: %w peer %s", err, p.Address.String())
	}

	if err := w.WriteMsgWithContext(ctx, &pb.Delivery{
		Data: chunk.Data(),
	}); err != nil {
		return fmt.Errorf("write delivery: %w peer %s", err, p.Address.String())
	}

	return nil
}

// SetStorer sets the storer. This call is not goroutine safe.
func (s *Service) SetStorer(storer storage.Storer) {
	s.storer = storer
}
