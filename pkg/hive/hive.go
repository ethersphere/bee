// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/hive/pb"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	protocolName             = "hive"
	protocolVersion          = "1.0.0"
	peersStreamName          = "peers"
	peersBroadcastStreamName = "peers_broadcast"
	maxPO                    = 7
	messageTimeout           = 5 * time.Second // maximum allowed time for a message to be read or written.
)

type Service struct {
	streamer          p2p.Streamer
	connectionManager ConnectionManager
	logger            logging.Logger
	quit              chan struct{}
}

type Options struct {
	Streamer          p2p.Streamer
	ConnectionManager ConnectionManager
	Logger            logging.Logger
}

type BzzAddress struct {
	Overlay, Underlay swarm.Address
}

type ConnectionManager interface {
	// todo: this can be the libp2p.Connect
	Connect(underlay swarm.Address) error
}

func New(o Options) *Service {
	return &Service{
		streamer:          o.Streamer,
		logger:            o.Logger,
		connectionManager: o.ConnectionManager,
		quit:              make(chan struct{}),
	}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name: protocolName,
		Init: s.Init,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    peersStreamName,
				Handler: s.peersHandler,
			},
			{
				Name:    peersBroadcastStreamName,
				Handler: s.peersBroadcastHandler,
			},
		},
	}
}

// Init is called when the new peer is being initialized.
// This should happen after overlay handshake is finished.
func (s *Service) Init(peer p2p.Peer) error {
	for i := 0; i < maxPO; i++ {
		// todo: figure out the limit
		resp, err := s.getPeers(peer, i, 10)
		if err != nil {
			return err
		}

		for _, newPeer := range resp {
			if err := s.connectionManager.Connect(newPeer.Underlay); err != nil {
				continue
			}
		}
	}

	return nil
}

// BroadcastPeers broadcasts the provided list of peers to the provided peer.
func (s *Service) BroadcastPeers(peer p2p.Peer, peers []p2p.Peer) error {
	// todo: create Peers request and broadcast over peers_broadcast stream
	return errors.New("not implemented")
}

func (s *Service) getPeers(peer p2p.Peer, bin, limit int) ([]BzzAddress, error) {
	stream, err := s.streamer.NewStream(context.Background(), peer.Address, protocolName, protocolVersion, peersStreamName)
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

	var res []BzzAddress
	for _, peer := range peersResponse.Peers {
		res = append(res, BzzAddress{
			Overlay:  swarm.NewAddress(peer.Overlay),
			Underlay: swarm.NewAddress(peer.Underlay),
		})
	}

	return res, nil
}

func (s *Service) peersHandler(peer p2p.Peer, stream p2p.Stream) error {
	// todo: receive getPeers msg and send Peers response
	return errors.New("not implemented")
}

func (s *Service) peersBroadcastHandler(peer p2p.Peer, stream p2p.Stream) error {
	// todo: receive peers response close the stream and try to connect to each of them
	return errors.New("not implemented")
}
