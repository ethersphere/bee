// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"errors"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
)

const (
	protocolName             = "hive"
	peersStreamName          = "peers"
	peersBroadcastStreamName = "peers_broadcast"
)

type Service struct {
	streamer p2p.Streamer
	logger   logging.Logger
	quit     chan struct{}
}

type Options struct {
	Streamer p2p.Streamer
	Logger   logging.Logger
}

func New(o Options) *Service {
	return &Service{
		streamer: o.Streamer,
		logger:   o.Logger,
		quit:     make(chan struct{}),
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
	// todo: send get peers request for each bin and receive peers responses
	return errors.New("not implemented")
}

func (s *Service) BroadcastPeers(peer p2p.Peer, peers []p2p.Peer) error {
	// todo: create Peers request and broadcast over peers_broadcast stream
	return errors.New("not implemented")
}

func (s *Service) peersHandler(peer p2p.Peer, stream p2p.Stream) error {
	// todo: receive getPeers msg and send Peers response
	return errors.New("not implemented")
}

func (s *Service) peersBroadcastHandler(peer p2p.Peer, stream p2p.Stream) error {
	// todo: receive peers response close the stream and try to connect to each of them
	return errors.New("not implemented")
}
