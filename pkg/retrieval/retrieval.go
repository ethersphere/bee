// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	pb "github.com/ethersphere/bee/pkg/retrieval/pb"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

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
	peerSuggester topology.ClosestPeerer
	storer        storage.Storer
	logger        logging.Logger
}

type Options struct {
	Streamer    p2p.Streamer
	ChunkPeerer topology.ClosestPeerer
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

func (s *Service) RetrieveChunk(ctx context.Context, addr swarm.Address) (data []byte, err error) {
	peer, err := s.peerSuggester.ClosestPeer(addr)
	if err != nil {
		return nil, fmt.Errorf("get closest: %w", err)
	}
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	w, r := protobuf.NewWriterAndReader(stream)

	if err := w.WriteMsg(&pb.Request{
		Addr: addr.Bytes(),
	}); err != nil {
		return nil, fmt.Errorf("write request: %w peer %s", err, peer.String())
	}

	var d pb.Delivery
	if err := r.ReadMsg(&d); err != nil {
		return nil, fmt.Errorf("read delivery: %w peer %s", err, peer.String())
	}

	return d.Data, nil
}

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()
	var req pb.Request
	if err := r.ReadMsg(&req); err != nil {
		return fmt.Errorf("read request: %w peer %s", err, p.Address.String())
	}

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
