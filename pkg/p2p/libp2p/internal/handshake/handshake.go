// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"bytes"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
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
	messageTimeout  = 10 * time.Second // maximum allowed time for a message to be read or written.
)

// PeerFinder has the information if the peer already exists in swarm.
type PeerFinder interface {
	Exists(overlay swarm.Address) (found bool)
}

type Service struct {
	overlay              swarm.Address
	underlay             []byte
	signature            []byte
	signer               crypto.SignRecoverer
	networkID            uint64
	receivedHandshakes   map[libp2ppeer.ID]struct{}
	receivedHandshakesMu sync.Mutex
	logger               logging.Logger

	network.Notifiee // handhsake service can be the receiver for network.Notify
}

func New(overlay swarm.Address, underlay []byte, signer crypto.SignRecoverer, networkID uint64, logger logging.Logger) (*Service, error) {
	toSign := append(underlay, strconv.FormatUint(networkID, 10)...)
	signature, err := signer.Sign(toSign)
	if err != nil {
		return nil, err
	}

	return &Service{
		overlay:            overlay,
		underlay:           underlay,
		signature:          signature,
		signer:             signer,
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
			Underlay:  s.underlay,
			Signature: s.signature,
			Overlay:   s.overlay.Bytes(),
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

	if err := s.checkSyn(resp.Syn); err != nil {
		return nil, err
	}

	if err := w.WriteMsgWithTimeout(messageTimeout, &pb.Ack{
		BzzAddress: resp.Syn.BzzAddress,
	}); err != nil {
		return nil, fmt.Errorf("write ack message: %w", err)
	}

	s.logger.Tracef("handshake finished for peer %s", resp.Syn.BzzAddress.Overlay)

	return &Info{
		Overlay:   swarm.NewAddress(resp.Syn.BzzAddress.Overlay),
		Underlay:  resp.Syn.BzzAddress.Underlay,
		NetworkID: resp.Syn.NetworkID,
		Light:     resp.Syn.Light,
	}, nil
}

func (s *Service) Handle(stream p2p.Stream, peerID libp2ppeer.ID) (i *Info, err error) {
	s.receivedHandshakesMu.Lock()
	if _, exists := s.receivedHandshakes[peerID]; exists {
		s.receivedHandshakesMu.Unlock()
		return nil, fmt.Errorf("handshake duplicate")
	}

	s.receivedHandshakes[peerID] = struct{}{}
	s.receivedHandshakesMu.Unlock()
	w, r := protobuf.NewWriterAndReader(stream)

	var req pb.Syn
	if err := r.ReadMsgWithTimeout(messageTimeout, &req); err != nil {
		return nil, fmt.Errorf("read syn message: %w", err)
	}

	if err := s.checkSyn(&req); err != nil {
		return nil, err
	}

	if err := w.WriteMsgWithTimeout(messageTimeout, &pb.SynAck{
		Syn: &pb.Syn{
			BzzAddress: &pb.BzzAddress{
				Underlay:  s.underlay,
				Signature: s.signature,
				Overlay:   s.overlay.Bytes(),
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

	s.logger.Tracef("handshake finished for peer %s", req.BzzAddress.Overlay)
	return &Info{
		Overlay:   swarm.NewAddress(req.BzzAddress.Overlay),
		Underlay:  req.BzzAddress.Underlay,
		NetworkID: req.NetworkID,
		Light:     req.Light,
	}, nil
}

func (s *Service) Disconnected(_ network.Network, c network.Conn) {
	s.receivedHandshakesMu.Lock()
	defer s.receivedHandshakesMu.Unlock()
	delete(s.receivedHandshakes, c.RemotePeer())
}

func (s *Service) checkSyn(syn *pb.Syn) error {
	if syn.NetworkID != s.networkID {
		return fmt.Errorf("incompatible network ID")
	}

	recoveredPK, err := s.signer.Recover(syn.BzzAddress.Signature, append(syn.BzzAddress.Underlay, strconv.FormatUint(syn.NetworkID, 10)...))
	if err != nil {
		return fmt.Errorf("could not recover public key from signature")
	}

	recoveredOverlay := crypto.NewOverlayAddress(*recoveredPK, syn.NetworkID)
	if !bytes.Equal(recoveredOverlay.Bytes(), syn.BzzAddress.Overlay) {
		return fmt.Errorf("invalid overlay from signature")
	}

	return nil
}

func (s *Service) checkAck(ack *pb.Ack) error {
	if !bytes.Equal(ack.BzzAddress.Overlay, s.overlay.Bytes()) ||
		!bytes.Equal(ack.BzzAddress.Underlay, s.underlay) ||
		!bytes.Equal(ack.BzzAddress.Signature, s.signature) {
		return fmt.Errorf("invalid ack received")
	}

	return nil
}

type Info struct {
	Overlay   swarm.Address
	Underlay  []byte
	NetworkID uint64
	Light     bool
}
