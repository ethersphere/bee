// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"context"
	"fmt"
	"time"

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
	maxPO           = 7
	messageTimeout  = 5 * time.Second // maximum allowed time for a message to be read or written.
)

type Service struct {
	streamer          p2p.Streamer
	connectionManager ConnectionManager
	peerSuggester     PeerSuggester
	addressFinder     AddressFinder
	logger            logging.Logger
}

type Options struct {
	Streamer          p2p.Streamer
	ConnectionManager ConnectionManager
	PeerSuggester     PeerSuggester
	AddressFinder     AddressFinder
	Logger            logging.Logger
}

func New(o Options) *Service {
	return &Service{
		streamer:          o.Streamer,
		logger:            o.Logger,
		connectionManager: o.ConnectionManager,
		peerSuggester:     o.PeerSuggester,
		addressFinder:     o.AddressFinder,
	}
}

type bzzAddress struct {
	Overlay, Underlay []byte
}

type ConnectionManager interface {
	// todo: this can be the libp2p.Connect or something else
	Connect(ctx context.Context, underlay []byte) error
}

type PeerSuggester interface {
	SuggestPeers(peer p2p.Peer, bin, limit int) (peers []p2p.Peer)
}

type AddressFinder interface {
	Underlay(overlay swarm.Address) (underlay []byte, err error)
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
		resp, err := s.getPeers(ctx, peer, i, 10)
		if err != nil {
			return err
		}

		for _, newPeer := range resp {
			if err := s.connectionManager.Connect(ctx, newPeer.Underlay); err != nil {
				s.logger.Infof("Connect failed for %s: %w", string(newPeer.Underlay), err)
				continue
			}
		}
	}

	return nil
}

func (s *Service) getPeers(ctx context.Context, peer p2p.Peer, bin, limit int) ([]bzzAddress, error) {
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
		return nil, fmt.Errorf("write getPeers message: %w", err)
	}

	var peersResponse pb.Peers
	if err := r.ReadMsgWithTimeout(messageTimeout, &peersResponse); err != nil {
		return nil, fmt.Errorf("read getPeers message: %w", err)
	}

	var res []bzzAddress
	for _, peer := range peersResponse.Peers {
		res = append(res, bzzAddress{
			Overlay:  peer.Overlay,
			Underlay: []byte(peer.Underlay),
		})
	}

	return res, nil
}

func (s *Service) peersHandler(peer p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	var peersReq pb.GetPeers
	if err := r.ReadMsgWithTimeout(messageTimeout, &peersReq); err != nil {
		return fmt.Errorf("read getPeers message: %w", err)
	}

	// the assumption is that the peer suggester is taking care of the validity of suggested peers
	// todo: should we track peer sent in hive or leave it to the peerSuggester?
	peers := s.peerSuggester.SuggestPeers(peer, int(peersReq.Bin), int(peersReq.Limit))
	var peersResp pb.Peers
	for _, p := range peers {
		underlay, err := s.addressFinder.Underlay(p.Address)
		if err != nil {
			// skip this peer
			continue
		}

		peersResp.Peers = append(peersResp.Peers, &pb.BzzAddress{
			Overlay:  p.Address.Bytes(),
			Underlay: string(underlay),
		})
	}

	if err := w.WriteMsg(&peersResp); err != nil {
		return fmt.Errorf("write Peers message: %w", err)
	}

	// todo: await close from the receiver
	return nil
}
