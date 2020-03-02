// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"context"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/discovery"
	"github.com/ethersphere/bee/pkg/hive/pb"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"

	ma "github.com/multiformats/go-multiaddr"
)

const (
	protocolName    = "hive"
	protocolVersion = "1.0.0"
	peersStreamName = "peers"
	messageTimeout  = 5 * time.Second // maximum allowed time for a message to be read or written.
	maxBatchSize    = 50
)

type Service struct {
	streamer       p2p.Streamer
	addressBook    addressbook.Getter
	topologyDriver topology.Driver
	logger         logging.Logger
}

type Options struct {
	Streamer       p2p.Streamer
	AddressBook    addressbook.Getter
	TopologyDriver topology.Driver
	Logger         logging.Logger
}

func New(o Options) *Service {
	return &Service{
		streamer:       o.Streamer,
		logger:         o.Logger,
		addressBook:    o.AddressBook,
		topologyDriver: o.TopologyDriver,
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

func (s *Service) BroadcastPeers(ctx context.Context, addressee swarm.Address, peers ...discovery.BroadcastRecord) error {
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

func (s *Service) sendPeers(ctx context.Context, peer swarm.Address, peers []discovery.BroadcastRecord) error {
	stream, err := s.streamer.NewStream(ctx, peer, protocolName, protocolVersion, peersStreamName)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}

	defer stream.Close()

	w, _ := protobuf.NewWriterAndReader(stream)
	var peersRequest pb.Peers
	for _, p := range peers {
		peersRequest.Peers = append(peersRequest.Peers, &pb.BzzAddress{
			Overlay:  p.Overlay.Bytes(),
			Underlay: p.Addr.String(),
		})
	}

	if err := w.WriteMsg(&peersRequest); err != nil {
		return fmt.Errorf("write Peers message: %w", err)
	}

	return stream.FullClose()
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

		err = s.topologyDriver.AddPeer(swarm.NewAddress(newPeer.Overlay), addr)
		if err != nil {
			return err
		}
	}

	return nil
}
