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
	"github.com/ethersphere/bee/pkg/hive/pb"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
)

const (
	protocolName    = "hive"
	protocolVersion = "1.0.0"
	peersStreamName = "peers"
	maxPO           = 16
	messageTimeout  = 5 * time.Second // maximum allowed time for a message to be read or written.
)

type Service struct {
	streamer        p2p.Streamer
	connecter       p2p.Connecter
	discoveryPeerer DiscoveryPeerer
	addressBook     addressbook.GetterPutter
	logger          logging.Logger
}

type Options struct {
	Streamer        p2p.Streamer
	Connecter       p2p.Connecter
	DiscoveryPeerer DiscoveryPeerer
	AddressBook     addressbook.GetterPutter
	Logger          logging.Logger
}

func New(o Options) *Service {
	return &Service{
		streamer:        o.Streamer,
		logger:          o.Logger,
		connecter:       o.Connecter,
		discoveryPeerer: o.DiscoveryPeerer,
		addressBook:     o.AddressBook,
	}
}

type DiscoveryPeerer interface {
	DiscoveryPeers(peer p2p.Peer, bin, limit int) (peers []p2p.Peer)
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
	for i := 0; i < maxPO; i++ {
		// todo: figure out the limit
		resp, err := s.requestPeers(ctx, peer, i, 10)
		if err != nil {
			return err
		}

		for _, newPeer := range resp {
			addr, err := ma.NewMultiaddr(newPeer)
			if err != nil {
				s.logger.Infof("Connect failed for %s: %w", newPeer, err)
				continue
			}

			if _, err := s.connecter.Connect(ctx, addr); err != nil {
				s.logger.Infof("Connect failed for %s: %w", addr.String(), err)
				continue
			}
		}
	}

	return nil
}

func (s *Service) requestPeers(ctx context.Context, peer p2p.Peer, bin, limit int) ([]string, error) {
	stream, err := s.streamer.NewStream(ctx, peer.Address, protocolName, protocolVersion, peersStreamName)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsg(&pb.GetPeers{
		Bin:   uint32(bin),
		Limit: uint32(limit),
	}); err != nil {
		return nil, fmt.Errorf("write requestPeers message: %w", err)
	}

	var peersResponse pb.Peers
	if err := r.ReadMsgWithTimeout(messageTimeout, &peersResponse); err != nil {
		return nil, fmt.Errorf("read requestPeers message: %w", err)
	}

	return peersResponse.Peers, nil
}

func (s *Service) peersHandler(peer p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	var peersReq pb.GetPeers
	if err := r.ReadMsgWithTimeout(messageTimeout, &peersReq); err != nil {
		return fmt.Errorf("read requestPeers message: %w", err)
	}

	// the assumption is that the peer suggester is taking care of the validity of suggested peers
	// todo: should we track peer sent in hive or leave it to the discoveryPeerer?
	peers := s.discoveryPeerer.DiscoveryPeers(peer, int(peersReq.Bin), int(peersReq.Limit))
	var peersResp pb.Peers
	for _, p := range peers {
		underlay, exists := s.addressBook.Get(p.Address)
		if !exists {
			// skip this peer
			// this might happen if there is a disconnect of the peer before the call to findAddress
			// or if there is an inconsistency between the suggested peer and our addresses bookkeeping
			s.logger.Warningf("Skipping peer in peers response, peer does not exists in address book.", p)
			continue
		}

		peersResp.Peers = append(peersResp.Peers, underlay.String())
	}

	if err := w.WriteMsg(&peersResp); err != nil {
		return fmt.Errorf("write Peers message: %w", err)
	}
	// todo: await close from the receiver
	return nil
}
