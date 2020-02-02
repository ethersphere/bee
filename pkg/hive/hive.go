// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	protocolName       = "hive"
	depthStreamName    = "hive_depth"
	depthStreamVersion = "1.0.0"
	peersStreamName    = "hive_peers"
	peersStreamVersion = "1.0.0"
)

type PeerTracker interface {
	AddPeer(address swarm.Address) error
	SuggestPeers() (peers []p2p.Peer, err error)
	Subscribe() (c <-chan p2p.Peer, unsubscribe func())
}

// SaturationTracker tracks weather the saturation has changed
type SaturationTracker interface {
	Subscribe() (c <-chan struct{}, unsubscribe func())
	Depth() (int, error)
}

type Service struct {
	streamer          p2p.Streamer
	logger            logging.Logger
	peerTracker       PeerTracker
	saturationTracker SaturationTracker
	quit              chan struct{}
}

type Options struct {
	Streamer          p2p.Streamer
	Logger            logging.Logger
	PeerTracker       PeerTracker
	SaturationTracker SaturationTracker
	TickInterval      time.Duration
}

func New(o Options) *Service {
	return &Service{
		streamer:          o.Streamer,
		logger:            o.Logger,
		saturationTracker: o.SaturationTracker,
		peerTracker:       o.PeerTracker,
		quit:              make(chan struct{}),
	}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name: protocolName,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    depthStreamName,
				Version: depthStreamVersion,
				Handler: s.DepthHandler,
			},
			{
				Name:    peersStreamName,
				Version: peersStreamVersion,
				Handler: s.PeersHandler,
			},
		},
	}
}

// todo:
// Hive is a skeleton of a hive function that should be called when the general hive service is started
// this is an attempt to run these loops outside of the peer level, to avoid having them for every peer
func (s *Service) Start() {
	peerAddedEvents, unsubPS := s.peerTracker.Subscribe()
	saturationChangeSignal, unsubSC := s.saturationTracker.Subscribe()
	defer unsubPS()
	defer unsubSC()

	// todo: maybe refactor and listen separately on these events, this is just a skeleton for now
	for {
		select {
		// todo: handle channel close and similar stuff, depending on the implementation of publishers
		case peer := <-peerAddedEvents:
			if err := s.notifyPeerAdded(peer); err != nil {
				// todo: handle err
				return
			}
		case <-saturationChangeSignal:
			depth, err := s.saturationTracker.Depth()
			if err != nil {
				// todo: handle err
				return
			}
			if err := s.notifyDepthChanged(depth); err != nil {
				// todo: handle err
				return
			}

		// todo: cancel, quit, etc...
		case <-s.quit:
			return

		}
	}
}

// Init is called when the new peer is being initialized.
// This should happen after overlay handshake is finished.
func (s *Service) Init(peer p2p.Peer) error {
	depth, err := s.saturationTracker.Depth()
	if err != nil {
		// todo: handle err
		return err
	}

	// todo: handle err
	return s.sendDepthMsg(peer, depth)
}

func (s *Service) DepthHandler(peer p2p.Peer, stream p2p.Stream) error {
	// todo:
	return nil
}

func (s *Service) PeersHandler(peer p2p.Peer, stream p2p.Stream) error {
	// todo:
	return nil
}

// notify new depth to all suggested peers
func (s *Service) notifyDepthChanged(depth int) error {
	peers, err := s.peerTracker.SuggestPeers()
	if err != nil {
		return err
	}

	var eg errgroup.Group
	for _, peer := range peers {
		peer := peer
		eg.Go(func() error {
			return s.sendDepthMsg(peer, depth)
		})
	}

	// todo: add timeout
	return eg.Wait()
}

func (s *Service) sendDepthMsg(peer p2p.Peer, depth int) error {
	// todo:
	return nil
}

// notify to suggested peers that the new peer has been added
func (s *Service) notifyPeerAdded(peer p2p.Peer) error {
	peers, err := s.peerTracker.SuggestPeers()
	if err != nil {
		return err
	}

	var eg errgroup.Group
	for _, peer := range peers {
		peer := peer
		eg.Go(func() error {
			return s.sendPeers(peer, []p2p.Peer{peer})
		})
	}

	// todo: add timeout
	return eg.Wait()
}

func (s *Service) sendPeers(peer p2p.Peer, peers []p2p.Peer) error {
	// todo:
	return nil
}
