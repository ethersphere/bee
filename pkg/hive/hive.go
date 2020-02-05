// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"context"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/hive/pb"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"

	"golang.org/x/sync/errgroup"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
)

const (
	protocolName           = "hive"
	depthStreamName        = "hive_depth"
	depthStreamVersion     = "1.0.0"
	peersStreamName        = "hive_peers"
	peersStreamVersion     = "1.0.0"
	subscribeStreamName    = "hive_subscribe"
	subscribeStreamVersion = "1.0.0"
	maxPeersCount          = 50
)

type PeerTracker interface {
	AddPeer(overlay, underlay []byte) error // todo: introduce bzz address if needed
	SuggestPeers() (peers []p2p.Peer, err error)
	Subscribe() (c <-chan p2p.Peer, unsubscribe func())
	ConnectedPeers() (peers []Peer)
}

// SaturationTracker tracks weather the saturation has changed
type SaturationTracker interface {
	Subscribe() (c <-chan struct{}, unsubscribe func())
	Depth() (uint32, error)
}

type Peer struct {
	po       uint32
	overlay  []byte
	underlay []byte
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
			{
				Name:    peersStreamName,
				Version: peersStreamVersion,
				Handler: s.SubscribeHandler,
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
func (s *Service) Init(ctx context.Context, peer p2p.Peer) error {
	// todo: handle err
	depth, err := s.saturationTracker.Depth()
	if err != nil {
		return err
	}
	stream, err := s.streamer.NewStream(ctx, peer.Address, protocolName, subscribeStreamName, subscribeStreamVersion)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}

	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsg(&pb.Subscribe{
		Depth: depth,
	}); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	var resp pb.SubscribeResponse
	if err := r.ReadMsg(&resp); err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	for _, address := range resp.Peers.BzzAddress {
		if err := s.peerTracker.AddPeer(address.Overlay, address.Underlay); err != nil {
			return err
		}
	}
	//todo optional: return peers message here to avoid init on both sides, similar to handshake protocol
	return nil
}

func (s *Service) SubscribeHandler(peer p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	var subscribe pb.Subscribe
	if err := r.ReadMsg(&subscribe); err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	connPeers := s.peerTracker.ConnectedPeers()
	bzzAddresses := make([]*pb.BzzAddress, 1)
	for _, peer := range connPeers {
		if peer.po < subscribe.Depth {
			// todo: should we also check seen peers here?
			bzzAddresses = append(bzzAddresses, &pb.BzzAddress{
				Overlay:  peer.overlay,
				Underlay: peer.underlay, // todo: correct underlay
			})
		}
	}

	// todo: filter peers per depth criteria if needed
	// todo: populate bzzAddresses
	var peers pb.Peers
	peers.BzzAddress = bzzAddresses[:maxPeersCount]
	if err := w.WriteMsg(&pb.SubscribeResponse{
		Peers: &peers,
	}); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	return nil
}

func (s *Service) DepthHandler(peer p2p.Peer, stream p2p.Stream) error {
	// todo: update depth for peer
	return nil
}

func (s *Service) PeersHandler(peer p2p.Peer, stream p2p.Stream) error {
	_, r := protobuf.NewWriterAndReader(stream)
	var peers pb.Peers
	if err := r.ReadMsg(&peers); err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	for _, address := range peers.BzzAddress {
		if err := s.peerTracker.AddPeer(address.Overlay, address.Underlay); err != nil {
			return err
		}
	}

	return nil
}

// notify new depth to all suggested peers
func (s *Service) notifyDepthChanged(depth uint32) error {
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

func (s *Service) sendDepthMsg(peer p2p.Peer, depth uint32) error {
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
	for _, p := range peers {
		peer := peer
		eg.Go(func() error {
			return s.sendPeer(p, peer)
		})
	}

	// todo: add timeout
	return eg.Wait()
}

func (s *Service) sendPeer(peer p2p.Peer, peerUpdate p2p.Peer) error {
	// todo: send peer request
	return nil
}
