// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
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

	// ErrInvalidAck is returned if data in received in ack is not valid (invalid signature for example).
	ErrInvalidAck = errors.New("invalid ack")

	// ErrInvalidSyn is returned if observable address in ack is not a valid..
	ErrInvalidSyn = errors.New("invalid syn")
)

type Service struct {
	signer               crypto.Signer
	overlay              swarm.Address
	lightNode            bool
	networkID            uint64
	receivedHandshakes   map[libp2ppeer.ID]struct{}
	receivedHandshakesMu sync.Mutex
	logger               logging.Logger

	network.Notifiee // handshake service can be the receiver for network.Notify
}

func New(overlay swarm.Address, signer crypto.Signer, networkID uint64, lighNode bool, logger logging.Logger) (*Service, error) {
	return &Service{
		signer:             signer,
		overlay:            overlay,
		networkID:          networkID,
		lightNode:          lighNode,
		receivedHandshakes: make(map[libp2ppeer.ID]struct{}),
		logger:             logger,
		Notifiee:           new(network.NoopNotifiee),
	}, nil
}

func (s *Service) Handshake(stream p2p.Stream, peerMultiaddr ma.Multiaddr, peerID libp2ppeer.ID) (i *Info, err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	fullRemoteMA, err := buildFullMA(peerMultiaddr, peerID)
	if err != nil {
		return nil, err
	}

	fullRemoteMABytes, err := fullRemoteMA.MarshalBinary()
	if err != nil {
		return nil, err
	}

	if err := w.WriteMsgWithTimeout(messageTimeout, &pb.Syn{
		ObservedUnderlay: fullRemoteMABytes,
	}); err != nil {
		return nil, fmt.Errorf("write syn message: %w", err)
	}

	var resp pb.SynAck
	if err := r.ReadMsgWithTimeout(messageTimeout, &resp); err != nil {
		return nil, fmt.Errorf("read synack message: %w", err)
	}

	remoteBzzAddress, err := s.parseCheckAck(resp.Ack, fullRemoteMABytes)
	if err != nil {
		return nil, err
	}

	addr, err := ma.NewMultiaddrBytes(resp.Syn.ObservedUnderlay)
	if err != nil {
		return nil, ErrInvalidSyn
	}

	bzzAddress, err := bzz.NewAddress(s.signer, addr, s.overlay, s.networkID)
	if err != nil {
		return nil, err
	}

	if err := w.WriteMsgWithTimeout(messageTimeout, &pb.Ack{
		Overlay:   bzzAddress.Overlay.Bytes(),
		Signature: bzzAddress.Signature,
		NetworkID: s.networkID,
		Light:     s.lightNode,
	}); err != nil {
		return nil, fmt.Errorf("write ack message: %w", err)
	}

	s.logger.Tracef("handshake finished for peer %s", remoteBzzAddress.Overlay.String())
	return &Info{
		BzzAddress: remoteBzzAddress,
		Light:      resp.Ack.Light,
	}, nil
}

func (s *Service) Handle(stream p2p.Stream, remoteMultiaddr ma.Multiaddr, remotePeerID libp2ppeer.ID) (i *Info, err error) {
	s.receivedHandshakesMu.Lock()
	if _, exists := s.receivedHandshakes[remotePeerID]; exists {
		s.receivedHandshakesMu.Unlock()
		return nil, ErrHandshakeDuplicate
	}

	s.receivedHandshakes[remotePeerID] = struct{}{}
	s.receivedHandshakesMu.Unlock()
	w, r := protobuf.NewWriterAndReader(stream)
	fullRemoteMA, err := buildFullMA(remoteMultiaddr, remotePeerID)
	if err != nil {
		return nil, err
	}

	fullRemoteMABytes, err := fullRemoteMA.MarshalBinary()
	if err != nil {
		return nil, err
	}

	var syn pb.Syn
	if err := r.ReadMsgWithTimeout(messageTimeout, &syn); err != nil {
		return nil, fmt.Errorf("read syn message: %w", err)
	}

	addr, err := ma.NewMultiaddrBytes(syn.ObservedUnderlay)
	if err != nil {
		return nil, ErrInvalidSyn
	}

	bzzAddress, err := bzz.NewAddress(s.signer, addr, s.overlay, s.networkID)
	if err != nil {
		return nil, err
	}

	if err := w.WriteMsgWithTimeout(messageTimeout, &pb.SynAck{
		Syn: &pb.Syn{
			ObservedUnderlay: fullRemoteMABytes,
		},
		Ack: &pb.Ack{
			Overlay:   bzzAddress.Overlay.Bytes(),
			Signature: bzzAddress.Signature,
			NetworkID: s.networkID,
			Light:     s.lightNode,
		},
	}); err != nil {
		return nil, fmt.Errorf("write synack message: %w", err)
	}

	var ack pb.Ack
	if err := r.ReadMsgWithTimeout(messageTimeout, &ack); err != nil {
		return nil, fmt.Errorf("read ack message: %w", err)
	}

	remoteBzzAddress, err := s.parseCheckAck(&ack, fullRemoteMABytes)
	if err != nil {
		return nil, err
	}

	s.logger.Tracef("handshake finished for peer %s", remoteBzzAddress.Overlay.String())
	return &Info{
		BzzAddress: remoteBzzAddress,
		Light:      ack.Light,
	}, nil
}

func (s *Service) Disconnected(_ network.Network, c network.Conn) {
	s.receivedHandshakesMu.Lock()
	defer s.receivedHandshakesMu.Unlock()
	delete(s.receivedHandshakes, c.RemotePeer())
}

func buildFullMA(addr ma.Multiaddr, peerID libp2ppeer.ID) (ma.Multiaddr, error) {
	return ma.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", addr.String(), peerID.Pretty()))
}

func (s *Service) parseCheckAck(ack *pb.Ack, remoteMA []byte) (*bzz.Address, error) {
	if ack.NetworkID != s.networkID {
		return nil, ErrNetworkIDIncompatible
	}

	bzzAddress, err := bzz.ParseAddress(remoteMA, ack.Overlay, ack.Signature, s.networkID)
	if err != nil {
		return nil, ErrInvalidAck
	}

	return bzzAddress, nil
}

type Info struct {
	BzzAddress *bzz.Address
	Light      bool
}
