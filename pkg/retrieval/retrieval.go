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
)

const (
	protocolName  = "retrieval"
	streamName    = "retrieval"
	streamVersion = "1.0.0"
)

type Service struct {
	streamer      p2p.Streamer
	peerSuggester p2p.PeerSuggester
	storer        storage.Storer
	logger        logging.Logger
}

type Options struct {
	Streamer      p2p.Streamer
	PeerSuggester p2p.PeerSuggester
	Storer        storage.Storer
	Logger        logging.Logger
}

type Storer interface {
}

func New(o Options) *Service {
	return &Service{
		streamer:      o.Streamer,
		peerSuggester: o.PeerSuggester,
		storer:        o.Storer,
		logger:        o.Logger,
	}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name: protocolName,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Version: streamVersion,
				Handler: s.Handler,
			},
		},
	}
}

func (s *Service) RetrieveChunk(ctx context.Context, addr swarm.Address) (data []byte, err error) {
	peerID, err := s.peerSuggester.SuggestPeer(addr)
	if err != nil {
		return nil, err
	}
	stream, err := s.streamer.NewStream(ctx, peerID.String(), protocolName, streamName, streamVersion)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	w, r := protobuf.NewWriterAndReader(stream)

	if err := w.WriteMsg(&pb.Request{
		Addr: addr[:],
	}); err != nil {
		return nil, fmt.Errorf("stream write: %w", err)
	}

	var d pb.Delivery
	if err := r.ReadMsg(&d); err != nil {
		return nil, err
	}

	return d.Data, nil
}

func (s *Service) Handler(p p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()
	var req pb.Request
	if err := r.ReadMsg(&req); err != nil {
		return err
	}

	data, err := s.storer.Get(context.TODO(), swarm.Address(req.Addr))
	if err != nil {
		return err
	}

	if err := w.WriteMsg(&pb.Delivery{
		Data: data,
	}); err != nil {
		return err
	}

	return nil
}
