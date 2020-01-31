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
	protocolName  = "hive"
	streamName    = "hive"
	streamVersion = "1.0.0"
)

// PeerSuggester suggests a peer to connect to
type PeerSuggester interface {
	Subscribe() (c <-chan swarm.Address, unsubscribe func())
}

type PeerTracker interface {
	AddPeer(address swarm.Address) error
	Peers() (peers []p2p.Peer, err error)
}

// SaturationTracker tracks weather the saturation has changed
type SaturationTracker interface {
	Subscribe() (c <-chan struct{}, unsubscribe func())
	Depth() (int, error)
}

type Service struct {
	streamer          p2p.Streamer
	logger            logging.Logger
	peerSuggester     PeerSuggester
	peerTracker       PeerTracker
	saturationTracker SaturationTracker
	quit              chan struct{}
}

type Options struct {
	Streamer          p2p.Streamer
	Logger            logging.Logger
	PeerSuggester     PeerSuggester
	PeerTracker       PeerTracker
	SaturationTracker SaturationTracker
	TickInterval      time.Duration
}

func New(o Options) *Service {
	return &Service{
		streamer:          o.Streamer,
		logger:            o.Logger,
		peerSuggester:     o.PeerSuggester,
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
				Name:    streamName,
				Version: streamVersion,
				Handler: s.Handler,
			},
		},
	}
}

// todo:
// Hive is a skeleton of a hive function that should be called when the general hive service is started
// this is an attempt to run these loops outside of the peer level, to avoid having them for every peer
func (s *Service) Start() {
	peerSuggestEvents, unsubPS := s.peerSuggester.Subscribe()
	saturationChangeSignal, unsubSC := s.saturationTracker.Subscribe()
	defer unsubPS()
	defer unsubSC()

	// todo: maybe refactor and listen separately on these events, this is just a skeleton for now
	for {
		select {
		// todo: handle channel close and similar stuff, depending on the implementation of publishers
		case ps := <-peerSuggestEvents:
			if err := s.peerTracker.AddPeer(ps); err != nil {
				// todo: handle err
				return
			}
		case <-saturationChangeSignal:
			depth, err := s.saturationTracker.Depth()
			if err != nil {
				// todo: handle err
				return
			}
			if err := s.notifyAllPeers(depth); err != nil {
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
	return s.notifyPeer(peer, depth)
}

func (s *Service) Handler(peer p2p.Peer, stream p2p.Stream) error {
	// todo:
	return nil
}

func (s *Service) notifyAllPeers(depth int) error {
	peers, err := s.peerTracker.Peers()
	if err != nil {
		return err
	}

	var eg errgroup.Group
	for _, peer := range peers {
		peer := peer
		eg.Go(func() error {
			return s.notifyPeer(peer, depth)
		})
	}

	return eg.Wait()
}

func (s *Service) notifyPeer(peer p2p.Peer, depth int) error {
	// todo: hive notify msg
	return nil
}
