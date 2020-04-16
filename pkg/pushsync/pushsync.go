// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	"context"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/topology"
)

const (
	protocolName    = "pushsync"
	protocolVersion = "1.0.0"
	streamName      = "puhsync"
)

type Service struct {
	streamer      p2p.Streamer
	peerSuggester topology.SyncPeerer
	storer        storage.Storer
	logger        logging.Logger
}

type Options struct {
	Streamer    p2p.Streamer
	SyncPeerer 	topology.SyncPeerer
	Storer      storage.Storer
	Logger      logging.Logger
}

type Storer interface {
}

func New(o Options) *Service {
	return &Service{
		streamer:      o.Streamer,
		peerSuggester: o.SyncPeerer,
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


func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {

	// will get a delivery of chunk... we should store it in storer

	// push this to your closest node too


	//w, r := protobuf.NewWriterAndReader(stream)
	//defer stream.Close()
	//var req pb.Request
	//if err := r.ReadMsg(&req); err != nil {
	//	return err
	//}
	//
	//chunk, err := s.storer.Get(ctx, storage.ModeGetRequest, swarm.NewAddress(req.Addr))
	//if err != nil {
	//	return err
	//}
	//
	//if err := w.WriteMsgWithContext(ctx, &pb.Delivery{
	//	Data: chunk.Data(),
	//}); err != nil {
	//	return err
	//}
	//
	return nil
}


func (s *Service) pushChunksToClosestNode() {

	// listen to chunk chnanel and push then chunks to closest peer


}