// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/swarm"

	"github.com/libp2p/go-libp2p-core/network"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	ProtocolName    = "handshake"
	ProtocolVersion = "1.0.0"
	StreamName      = "handshake"
	messageTimeout  = 5 * time.Second // maximum allowed time for a message to be read or written.
)

var (
	// ErrNetworkIDIncompatible is returned if response from the other peer does not have valid networkID.
	ErrNetworkIDIncompatible = errors.New("incompatible network ID")

	// ErrHandshakeDuplicate is returned  if the handshake response has been received by an already processed peer.
	ErrHandshakeDuplicate = errors.New("duplicate handshake")

	// ErrInvalidBzzAddress is returned if peer info was received with invalid bzz address
	ErrInvalidBzzAddress = errors.New("invalid bzz address")

	// ErrInvalidAck is returned if ack does not match the syn provided
	ErrInvalidAck = errors.New("invalid ack")
)

// PeerFinder has the information if the peer already exists in swarm.
type PeerFinder interface {
	Exists(overlay swarm.Address) (found bool)
}

type Service struct {
	bzzAddress           bzz.Address
	networkID            uint64
	receivedHandshakes   map[libp2ppeer.ID]struct{}
	receivedHandshakesMu sync.Mutex
	logger               logging.Logger

	network.Notifiee // handshake service can be the receiver for network.Notify
}

func New(overlay swarm.Address, underlay ma.Multiaddr, signer crypto.Signer, networkID uint64, logger logging.Logger) (*Service, error) {
	bzzAddress, err := bzz.NewAddress(signer, underlay, overlay, networkID)
	if err != nil {
		return nil, err
	}

	return &Service{
		bzzAddress:         *bzzAddress,
		networkID:          networkID,
		receivedHandshakes: make(map[libp2ppeer.ID]struct{}),
		logger:             logger,
		Notifiee:           new(network.NoopNotifiee),
	}, nil
}

func (s *Service) Handshake(stream p2p.Stream) (i *Info, err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsgWithTimeout(messageTimeout, &pb.Syn{
		BzzAddress: &pb.BzzAddress{
			Underlay:  s.bzzAddress.Underlay.Bytes(),
			Signature: s.bzzAddress.Signature,
			Overlay:   s.bzzAddress.Overlay.Bytes(),
		},
		NetworkID: s.networkID,
	}); err != nil {
		return nil, fmt.Errorf("write syn message: %w", err)
	}

	var resp pb.SynAck
	if err := r.ReadMsgWithTimeout(messageTimeout, &resp); err != nil {
		return nil, fmt.Errorf("read synack message: %w", err)
	}

	if err := s.checkAck(resp.Ack); err != nil {
		return nil, err
	}

	if resp.Syn.NetworkID != s.networkID {
		return nil, ErrNetworkIDIncompatible
	}

	bzzAddress, err := bzz.ParseAddress(resp.Syn.BzzAddress.Underlay, resp.Syn.BzzAddress.Overlay, resp.Syn.BzzAddress.Signature, resp.Syn.NetworkID)
	if err != nil {
		return nil, ErrInvalidBzzAddress
	}

	if err := w.WriteMsgWithTimeout(messageTimeout, &pb.Ack{
		BzzAddress: resp.Syn.BzzAddress,
	}); err != nil {
		return nil, fmt.Errorf("write ack message: %w", err)
	}

	s.logger.Tracef("handshake finished for peer %s", swarm.NewAddress(resp.Syn.BzzAddress.Overlay).String())
	return &Info{
		BzzAddress: bzzAddress,
		Light:      resp.Syn.Light,
	}, nil
}

func (s *Service) Handle(stream p2p.Stream, peerID libp2ppeer.ID) (i *Info, err error) {
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

	if req.NetworkID != s.networkID {
		return nil, ErrNetworkIDIncompatible
	}

	bzzAddress, err := bzz.ParseAddress(req.BzzAddress.Underlay, req.BzzAddress.Overlay, req.BzzAddress.Signature, req.NetworkID)
	if err != nil {
		return nil, ErrInvalidBzzAddress
	}

	if err := w.WriteMsgWithTimeout(messageTimeout, &pb.SynAck{
		Syn: &pb.Syn{
			BzzAddress: &pb.BzzAddress{
				Underlay:  s.bzzAddress.Underlay.Bytes(),
				Signature: s.bzzAddress.Signature,
				Overlay:   s.bzzAddress.Overlay.Bytes(),
			},
			NetworkID: s.networkID,
		},
		Ack: &pb.Ack{BzzAddress: req.BzzAddress},
	}); err != nil {
		return nil, fmt.Errorf("write synack message: %w", err)
	}

	var ack pb.Ack
	if err := r.ReadMsgWithTimeout(messageTimeout, &ack); err != nil {
		return nil, fmt.Errorf("read ack message: %w", err)
	}

	if err := s.checkAck(&ack); err != nil {
		return nil, err
	}

	s.logger.Tracef("handshake finished for peer %s", swarm.NewAddress(req.BzzAddress.Overlay).String())
	return &Info{
		BzzAddress: bzzAddress,
		Light:      req.Light,
	}, nil
}

func (s *Service) Disconnected(_ network.Network, c network.Conn) {
	s.receivedHandshakesMu.Lock()
	defer s.receivedHandshakesMu.Unlock()
	delete(s.receivedHandshakes, c.RemotePeer())
}

func (s *Service) checkAck(ack *pb.Ack) error {
	if !bytes.Equal(ack.BzzAddress.Overlay, s.bzzAddress.Overlay.Bytes()) ||
		!bytes.Equal(ack.BzzAddress.Underlay, s.bzzAddress.Underlay.Bytes()) ||
		!bytes.Equal(ack.BzzAddress.Signature, s.bzzAddress.Signature) {
		return ErrInvalidAck
	}

	return nil
}

type Info struct {
	BzzAddress *bzz.Address
	Light      bool
}
