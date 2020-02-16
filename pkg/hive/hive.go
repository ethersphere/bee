// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"context"
	"fmt"
	"time"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/discovery"
	"github.com/ethersphere/bee/pkg/hive/pb"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	protocolName    = "hive"
	protocolVersion = "1.0.0"
	peersStreamName = "peers"
	messageTimeout  = 5 * time.Second // maximum allowed time for a message to be read or written.
)

type Service struct {
	streamer    p2p.Streamer
	peerer      discovery.Peerer
	addressBook addressbook.GetterPutter
	logger      logging.Logger
}

type Options struct {
	Streamer    p2p.Streamer
	Peerer      discovery.Peerer
	AddressBook addressbook.GetterPutter
	Logger      logging.Logger
}

func New(o Options) *Service {
	return &Service{
		streamer:    o.Streamer,
		logger:      o.Logger,
		peerer:      o.Peerer,
		addressBook: o.AddressBook,
	}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		Init:    s.Init,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    peersStreamName,
				Handler: s.peersHandler,
			},
		},
	}
}

// Init is called when the new peer is being initialized.
// This should happen after overlay handshake is finished.
func (s *Service) Init(ctx context.Context, peer p2p.Peer) error {
	go func() {
		// todo: handle cancel, stop, errors, etc...
		for {
			// the assumption is that the peer suggester is taking care of the validity of suggested peers
			// peers call blocks until there is new peers to send
			peers := s.peerer.Peers(peer, 50)
			if err := s.sendPeers(ctx, peer, peers); err != nil {
				// todo: handle different errors differently
				s.logger.Error(err)
				continue
			}
		}
	}()

	return nil
}

func (s *Service) sendPeers(ctx context.Context, peer p2p.Peer, peers []p2p.Peer) error {
	stream, err := s.streamer.NewStream(ctx, peer.Address, protocolName, protocolVersion, peersStreamName)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}

	w, _ := protobuf.NewWriterAndReader(stream)
	var peersRequest pb.Peers
	for _, p := range peers {
		underlay, exists := s.addressBook.Get(p.Address)
		if !exists {
			// skip this peer
			// this might happen if there is a disconnect of the peer before the call to findAddress
			// or if there is an inconsistency between the suggested peer and our addresses bookkeeping
			s.logger.Warningf("Skipping peer in peers response: peer does not exists in the address book.", p)
			continue
		}

		peersRequest.Peers = append(peersRequest.Peers, &pb.BzzAddress{
			Overlay:  p.Address.Bytes(),
			Underlay: underlay.String(),
		})
	}

	if err := w.WriteMsg(&peersRequest); err != nil {
		return fmt.Errorf("write Peers message: %w", err)
	}
	// todo: await close from the receiver

	return nil
}

func (s *Service) peersHandler(peer p2p.Peer, stream p2p.Stream) error {
	defer stream.Close()
	_, r := protobuf.NewWriterAndReader(stream)
	var peersReq pb.Peers
	if err := r.ReadMsgWithTimeout(messageTimeout, &peersReq); err != nil {
		return fmt.Errorf("read requestPeers message: %w", err)
	}

	for _, newPeer := range peersReq.Peers {
		addr, err := ma.NewMultiaddr(newPeer.Underlay)
		if err != nil {
			s.logger.Infof("Skipping peer in response %s: %w", newPeer, err)
			continue
		}

		s.addressBook.Put(swarm.NewAddress(newPeer.Overlay), addr)
	}

	return nil
}
