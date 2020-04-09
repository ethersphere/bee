// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/swarm"

	"github.com/libp2p/go-libp2p-core/network"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

const (
	ProtocolName    = "handshake"
	ProtocolVersion = "1.0.0"
	StreamName      = "handshake"
	messageTimeout  = 5 * time.Second // maximum allowed time for a message to be read or written.
)

// ErrNetworkIDIncompatible should be returned by handshake handlers if
// response from the other peer does not have valid networkID.
var ErrNetworkIDIncompatible = errors.New("incompatible network ID")

// ErrHandshakeDuplicate should be returned by handshake handlers if
// the handshake response has been received by an already processed peer.
var ErrHandshakeDuplicate = errors.New("duplicate handshake")

// PeerFinder has the information if the peer already exists in swarm.
type PeerFinder interface {
	Exists(overlay swarm.Address) (found bool)
}

type Service struct {
	overlay              swarm.Address
	networkID            int32
	receivedHandshakes   map[libp2ppeer.ID]struct{}
	receivedHandshakesMu sync.Mutex
	logger               logging.Logger

	network.Notifiee // handhsake service can be the receiver for network.Notify
}

func New(overlay swarm.Address, networkID int32, logger logging.Logger) *Service {
	return &Service{
		overlay:            overlay,
		networkID:          networkID,
		receivedHandshakes: make(map[libp2ppeer.ID]struct{}),
		logger:             logger,
		Notifiee:           new(network.NoopNotifiee),
	}
}

func (s *Service) Handshake(stream p2p.Stream) (i *Info, err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsgWithTimeout(messageTimeout, &pb.Syn{
		Address:   s.overlay.Bytes(),
		NetworkID: s.networkID,
	}); err != nil {
		return nil, fmt.Errorf("write syn message: %w", err)
	}

	var resp pb.SynAck
	if err := r.ReadMsgWithTimeout(messageTimeout, &resp); err != nil {
		return nil, fmt.Errorf("read synack message: %w", err)
	}

	address := swarm.NewAddress(resp.Syn.Address)
	if resp.Syn.NetworkID != s.networkID {
		return nil, ErrNetworkIDIncompatible
	}

	if err := w.WriteMsgWithTimeout(messageTimeout, &pb.Ack{
		Address: resp.Syn.Address,
	}); err != nil {
		return nil, fmt.Errorf("write ack message: %w", err)
	}

	s.logger.Tracef("handshake finished for peer %s", address)

	return &Info{
		Address:   address,
		NetworkID: resp.Syn.NetworkID,
		Light:     resp.Syn.Light,
	}, nil
}

func (s *Service) Handle(stream p2p.Stream, peerID libp2ppeer.ID) (i *Info, err error) {
	defer stream.Close()
	s.receivedHandshakesMu.Lock()
	if _, exists := s.receivedHandshakes[peerID]; exists {
		s.receivedHandshakesMu.Unlock()
		return nil, ErrHandshakeDuplicate
	}

	s.receivedHandshakes[peerID] = struct{}{}
	s.receivedHandshakesMu.Unlock()
	w, r := protobuf.NewWriterAndReader(stream)

	var req pb.Syn
	if err := r.ReadMsgWithTimeout(messageTimeout, &req); err != nil {
		return nil, fmt.Errorf("read syn message: %w", err)
	}

	address := swarm.NewAddress(req.Address)
	if req.NetworkID != s.networkID {
		return nil, ErrNetworkIDIncompatible
	}

	if err := w.WriteMsgWithTimeout(messageTimeout, &pb.SynAck{
		Syn: &pb.Syn{
			Address:   s.overlay.Bytes(),
			NetworkID: s.networkID,
		},
		Ack: &pb.Ack{Address: req.Address},
	}); err != nil {
		return nil, fmt.Errorf("write synack message: %w", err)
	}

	var ack pb.Ack
	if err := r.ReadMsgWithTimeout(messageTimeout, &ack); err != nil {
		return nil, fmt.Errorf("read ack message: %w", err)
	}

	s.logger.Tracef("handshake finished for peer %s", address)
	return &Info{
		Address:   address,
		NetworkID: req.NetworkID,
		Light:     req.Light,
	}, nil
}

func (s *Service) Disconnected(_ network.Network, c network.Conn) {
	s.receivedHandshakesMu.Lock()
	defer s.receivedHandshakesMu.Unlock()
	delete(s.receivedHandshakes, c.RemotePeer())
}

type Info struct {
	Address   swarm.Address
	NetworkID int32
	Light     bool
}
