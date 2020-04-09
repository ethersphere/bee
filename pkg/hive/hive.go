// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"context"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/hive/pb"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/swarm"

	ma "github.com/multiformats/go-multiaddr"
)

const (
	protocolName    = "hive"
	protocolVersion = "1.0.0"
	peersStreamName = "peers"
	messageTimeout  = 1 * time.Minute // maximum allowed time for a message to be read or written.
	maxBatchSize    = 50
)

type Service struct {
	streamer    p2p.Streamer
	addressBook addressbook.GetPutter
	peerHandler func(context.Context, swarm.Address) error
	logger      logging.Logger
}

type Options struct {
	Streamer    p2p.Streamer
	AddressBook addressbook.GetPutter
	Logger      logging.Logger
}

func New(o Options) *Service {
	return &Service{
		streamer:    o.Streamer,
		logger:      o.Logger,
		addressBook: o.AddressBook,
	}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    peersStreamName,
				Handler: s.peersHandler,
			},
		},
	}
}

func (s *Service) BroadcastPeers(ctx context.Context, addressee swarm.Address, peers ...swarm.Address) error {
	max := maxBatchSize
	for len(peers) > 0 {
		if max > len(peers) {
			max = len(peers)
		}
		if err := s.sendPeers(ctx, addressee, peers[:max]); err != nil {
			return err
		}

		peers = peers[max:]
	}

	return nil
}

func (s *Service) SetPeerAddedHandler(h func(ctx context.Context, addr swarm.Address) error) {
	s.peerHandler = h
}

func (s *Service) sendPeers(ctx context.Context, peer swarm.Address, peers []swarm.Address) error {
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, peersStreamName)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	w, _ := protobuf.NewWriterAndReader(stream)
	var peersRequest pb.Peers
	for _, p := range peers {
		addr, found := s.addressBook.Get(p)
		if !found {
			s.logger.Debugf("Peer not found %s", peer, err)
			continue
		}

		peersRequest.Peers = append(peersRequest.Peers, &pb.BzzAddress{
			Overlay:  p.Bytes(),
			Underlay: addr.String(),
		})
	}

	if err := w.WriteMsg(&peersRequest); err != nil {
		return fmt.Errorf("write Peers message: %w", err)
	}

	return stream.FullClose()
}

func (s *Service) peersHandler(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
	_, r := protobuf.NewWriterAndReader(stream)
	var peersReq pb.Peers
	if err := r.ReadMsgWithTimeout(messageTimeout, &peersReq); err != nil {
		stream.Close()
		return fmt.Errorf("read requestPeers message: %w", err)
	}

	stream.Close()
	for _, newPeer := range peersReq.Peers {
		addr, err := ma.NewMultiaddr(newPeer.Underlay)
		if err != nil {
			s.logger.Infof("Skipping peer in response %s: %w", newPeer, err)
			continue
		}

		s.addressBook.Put(swarm.NewAddress(newPeer.Overlay), addr)
		if s.peerHandler != nil {
			if err := s.peerHandler(context.Background(), swarm.NewAddress(newPeer.Overlay)); err != nil {
				return err
			}
		}
	}

	return nil
}
