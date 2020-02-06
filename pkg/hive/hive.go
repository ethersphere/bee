// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/ethersphere/bee/pkg/hive/pb"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
)

const (
	protocolName           = "hive"
	initStreamName         = "hive_init"
	initStreamVersion      = "1.0.0"
	subscribeStreamName    = "hive_init"
	subscribeStreamVersion = "1.0.0"
	peersStreamName        = "hive_init"
	peersStreamVersion     = "1.0.0"
	//maxPeersCount     = 50
)

type AddressBook interface {
	// Add peer to knows peers (address book)
	AddPeer(overlay, underlay []byte) error // todo: introduce bzz address if needed
	ConnectedPeers() (peers []p2p.Peer)
}

// SaturationTracker tracks weather the saturation has changed
type SaturationTracker interface {
	Subscribe() (c <-chan struct{}, unsubscribe func())
	Depth() (uint8, error)
}

type Service struct {
	streamer          p2p.Streamer
	logger            logging.Logger
	addressBook       AddressBook
	saturationTracker SaturationTracker
	quit              chan struct{}
}

type Options struct {
	Streamer          p2p.Streamer
	Logger            logging.Logger
	PeerTracker       AddressBook
	SaturationTracker SaturationTracker
}

func New(o Options) *Service {
	return &Service{
		streamer:          o.Streamer,
		logger:            o.Logger,
		saturationTracker: o.SaturationTracker,
		addressBook:       o.PeerTracker,
		quit:              make(chan struct{}),
	}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name: protocolName,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    initStreamName,
				Version: initStreamVersion,
				Handler: s.InitHandler,
			},
		},
	}
}

// todo:
// Hive is a skeleton of a hive function that should be called when the general hive service is started
// this is an attempt to run these loops outside of the peer level, to avoid having them for every peer
func (s *Service) Start() {
	saturationChangeSignal, unsubSC := s.saturationTracker.Subscribe()
	defer unsubSC()

	ctx := context.Background()

	// todo: maybe refactor and listen separately on these events, this is just a skeleton for now
	for {
		select {
		// todo: handle channel close and similar stuff, depending on the implementation of publishers
		case <-saturationChangeSignal:
			depth, err := s.saturationTracker.Depth()
			if err != nil {
				// todo: handle err
				return
			}
			if err := s.notifyDepthChanged(ctx, depth); err != nil {
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
func (s *Service) Init(ctx context.Context, peer p2p.Peer) error {
	depth, err := s.saturationTracker.Depth()
	if err != nil {
		return err
	}

	stream, err := s.streamer.NewStream(ctx, peer.Address, protocolName, initStreamName, initStreamVersion)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}

	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsg(&pb.Subscribe{
		Depth:         uint32(depth),
		PeersResponse: true,
	}); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	var peers pb.Peers
	if err := r.ReadMsg(&peers); err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	for _, address := range peers.BzzAddress {
		// todo: maybe add and notify peers in a single batch
		if err := s.addressBook.AddPeer(address.Overlay, address.Underlay); err != nil {
			return err
		}

		if err := s.notifyPeerAdded(ctx, address.Overlay, address.Underlay); err != nil {
			return err
		}
	}

	// todo: maybe do a 3-way style handshake here to make sure that both sides subscribed, similar to handshake protocol.
	return nil
}

func (s *Service) InitHandler(peer p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	var subscribe pb.Subscribe
	if err := r.ReadMsg(&subscribe); err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	var peers pb.Peers
	// todo: populate peers response
	if err := w.WriteMsg(&peers); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	return nil
}

// notify new depth to all suggested peers
func (s *Service) notifyDepthChanged(ctx context.Context, depth uint8) error {
	peers := s.addressBook.ConnectedPeers()

	var eg errgroup.Group
	for _, peer := range peers {
		peer := peer
		eg.Go(func() error {
			stream, err := s.streamer.NewStream(ctx, peer.Address, protocolName, subscribeStreamName, subscribeStreamVersion)
			if err != nil {
				return fmt.Errorf("new stream: %w", err)
			}

			w, _ := protobuf.NewWriterAndReader(stream)
			if err := w.WriteMsg(&pb.Subscribe{
				Depth:         uint32(depth),
				PeersResponse: false,
			}); err != nil {
				return fmt.Errorf("write message: %w", err)
			}
			return nil
		})
	}

	// todo: add timeout
	return eg.Wait()
}

// notify to suggested peers that the new peer has been added
func (s *Service) notifyPeerAdded(ctx context.Context, overlay, underlay []byte) error {
	peers := s.addressBook.ConnectedPeers()

	var eg errgroup.Group
	for _, p := range peers {
		// todo: filter peers based on po & pot function
		peer := p
		eg.Go(func() error {
			stream, err := s.streamer.NewStream(ctx, peer.Address, protocolName, peersStreamName, peersStreamVersion)
			if err != nil {
				return fmt.Errorf("new stream: %w", err)
			}
			w, _ := protobuf.NewWriterAndReader(stream)

			if err := w.WriteMsg(&pb.Peers{BzzAddress: []*pb.BzzAddress{{
				Overlay:  overlay,
				Underlay: underlay,
			}}}); err != nil {
				return fmt.Errorf("write message: %w", err)
			}

			return nil
		})
	}

	// todo: add timeout
	return eg.Wait()
}
