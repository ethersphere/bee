// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/swarm"

	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	// ProtocolName is the text of the name of the handshake protocol.
	ProtocolName = "handshake"
	// ProtocolVersion is the current handshake protocol version.
	ProtocolVersion = "5.0.0"
	// StreamName is the name of the stream used for handshake purposes.
	StreamName = "handshake"
	// MaxWelcomeMessageLength is maximum number of characters allowed in the welcome message.
	MaxWelcomeMessageLength = 140
	handshakeTimeout        = 15 * time.Second
)

var (
	// ErrNetworkIDIncompatible is returned if response from the other peer does not have valid networkID.
	ErrNetworkIDIncompatible = errors.New("incompatible network ID")

	// ErrInvalidAck is returned if data in received in ack is not valid (invalid signature for example).
	ErrInvalidAck = errors.New("invalid ack")

	// ErrInvalidSyn is returned if observable address in ack is not a valid..
	ErrInvalidSyn = errors.New("invalid syn")

	// ErrWelcomeMessageLength is returned if the welcome message is longer than the maximum length
	ErrWelcomeMessageLength = fmt.Errorf("handshake welcome message longer than maximum of %d characters", MaxWelcomeMessageLength)

	// ErrPicker is returned if the picker (kademlia) rejects the peer
	ErrPicker = fmt.Errorf("picker rejection")
)

// AdvertisableAddressResolver can Resolve a Multiaddress.
type AdvertisableAddressResolver interface {
	Resolve(observedAddress ma.Multiaddr) (ma.Multiaddr, error)
}

type SenderMatcher interface {
	Matches(ctx context.Context, tx []byte, networkID uint64, senderOverlay swarm.Address, ignoreGreylist bool) ([]byte, error)
}

// Service can perform initiate or handle a handshake between peers.
type Service struct {
	signer                crypto.Signer
	advertisableAddresser AdvertisableAddressResolver
	senderMatcher         SenderMatcher
	overlay               swarm.Address
	fullNode              bool
	transaction           []byte
	networkID             uint64
	welcomeMessage        atomic.Value
	logger                logging.Logger
	libp2pID              libp2ppeer.ID
	metrics               metrics
	picker                p2p.Picker
}

// Info contains the information received from the handshake.
type Info struct {
	BzzAddress *bzz.Address
	FullNode   bool
}

func (i *Info) LightString() string {
	if !i.FullNode {
		return " (light)"
	}

	return ""
}

// New creates a new handshake Service.
func New(signer crypto.Signer, advertisableAddresser AdvertisableAddressResolver, isSender SenderMatcher, overlay swarm.Address, networkID uint64, fullNode bool, transaction []byte, welcomeMessage string, ownPeerID libp2ppeer.ID, logger logging.Logger) (*Service, error) {
	if len(welcomeMessage) > MaxWelcomeMessageLength {
		return nil, ErrWelcomeMessageLength
	}

	svc := &Service{
		signer:                signer,
		advertisableAddresser: advertisableAddresser,
		overlay:               overlay,
		networkID:             networkID,
		fullNode:              fullNode,
		transaction:           transaction,
		senderMatcher:         isSender,
		libp2pID:              ownPeerID,
		logger:                logger,
		metrics:               newMetrics(),
	}
	svc.welcomeMessage.Store(welcomeMessage)

	return svc, nil
}

func (s *Service) SetPicker(n p2p.Picker) {
	s.picker = n
}

// Handshake initiates a handshake with a peer.
func (s *Service) Handshake(ctx context.Context, stream p2p.Stream, peerMultiaddr ma.Multiaddr, peerID libp2ppeer.ID) (i *Info, err error) {
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	w, r := protobuf.NewWriterAndReader(stream)
	fullRemoteMA, err := buildFullMA(peerMultiaddr, peerID)
	if err != nil {
		return nil, err
	}

	fullRemoteMABytes, err := fullRemoteMA.MarshalBinary()
	if err != nil {
		return nil, err
	}

	if err := w.WriteMsgWithContext(ctx, &pb.Syn{
		ObservedUnderlay: fullRemoteMABytes,
	}); err != nil {
		return nil, fmt.Errorf("write syn message: %w", err)
	}

	var resp pb.SynAck
	if err := r.ReadMsgWithContext(ctx, &resp); err != nil {
		return nil, fmt.Errorf("read synack message: %w", err)
	}

	observedUnderlay, err := ma.NewMultiaddrBytes(resp.Syn.ObservedUnderlay)
	if err != nil {
		return nil, ErrInvalidSyn
	}

	observedUnderlayAddrInfo, err := libp2ppeer.AddrInfoFromP2pAddr(observedUnderlay)
	if err != nil {
		return nil, fmt.Errorf("extract addr from P2P: %w", err)
	}

	if s.libp2pID != observedUnderlayAddrInfo.ID {
		//NOTE eventually we will return error here, but for now we want to gather some statistics
		s.logger.Warningf("received peer ID %s does not match ours: %s", observedUnderlayAddrInfo.ID, s.libp2pID)
	}

	advertisableUnderlay, err := s.advertisableAddresser.Resolve(observedUnderlay)
	if err != nil {
		return nil, err
	}

	bzzAddress, err := bzz.NewAddress(s.signer, advertisableUnderlay, s.overlay, s.networkID, s.transaction)
	if err != nil {
		return nil, err
	}

	advertisableUnderlayBytes, err := bzzAddress.Underlay.MarshalBinary()
	if err != nil {
		return nil, err
	}

	overlay := swarm.NewAddress(resp.Ack.Address.Overlay)

	if resp.Ack.NetworkID != s.networkID {
		return nil, ErrNetworkIDIncompatible
	}

	blockHash, err := s.senderMatcher.Matches(ctx, resp.Ack.Transaction, s.networkID, overlay, false)
	if err != nil {
		return nil, fmt.Errorf("overlay %v verification failed: %w", overlay, err)
	}

	remoteBzzAddress, err := s.parseCheckAck(resp.Ack, blockHash)
	if err != nil {
		return nil, err
	}

	// Synced read:
	welcomeMessage := s.GetWelcomeMessage()
	if err := w.WriteMsgWithContext(ctx, &pb.Ack{
		Address: &pb.BzzAddress{
			Underlay:  advertisableUnderlayBytes,
			Overlay:   bzzAddress.Overlay.Bytes(),
			Signature: bzzAddress.Signature,
		},
		NetworkID:      s.networkID,
		FullNode:       s.fullNode,
		Transaction:    s.transaction,
		WelcomeMessage: welcomeMessage,
	}); err != nil {
		return nil, fmt.Errorf("write ack message: %w", err)
	}

	s.logger.Tracef("handshake finished for peer (outbound) %s", remoteBzzAddress.Overlay.String())
	if len(resp.Ack.WelcomeMessage) > 0 {
		s.logger.Infof("greeting \"%s\" from peer: %s", resp.Ack.WelcomeMessage, remoteBzzAddress.Overlay.String())
	}

	return &Info{
		BzzAddress: remoteBzzAddress,
		FullNode:   resp.Ack.FullNode,
	}, nil
}

// Handle handles an incoming handshake from a peer.
func (s *Service) Handle(ctx context.Context, stream p2p.Stream, remoteMultiaddr ma.Multiaddr, remotePeerID libp2ppeer.ID) (i *Info, err error) {
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

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
	if err := r.ReadMsgWithContext(ctx, &syn); err != nil {
		s.metrics.SynRxFailed.Inc()
		return nil, fmt.Errorf("read syn message: %w", err)
	}
	s.metrics.SynRx.Inc()

	observedUnderlay, err := ma.NewMultiaddrBytes(syn.ObservedUnderlay)
	if err != nil {
		return nil, ErrInvalidSyn
	}

	advertisableUnderlay, err := s.advertisableAddresser.Resolve(observedUnderlay)
	if err != nil {
		return nil, err
	}

	bzzAddress, err := bzz.NewAddress(s.signer, advertisableUnderlay, s.overlay, s.networkID, s.transaction)
	if err != nil {
		return nil, err
	}

	advertisableUnderlayBytes, err := bzzAddress.Underlay.MarshalBinary()
	if err != nil {
		return nil, err
	}

	welcomeMessage := s.GetWelcomeMessage()

	if err := w.WriteMsgWithContext(ctx, &pb.SynAck{
		Syn: &pb.Syn{
			ObservedUnderlay: fullRemoteMABytes,
		},
		Ack: &pb.Ack{
			Address: &pb.BzzAddress{
				Underlay:  advertisableUnderlayBytes,
				Overlay:   bzzAddress.Overlay.Bytes(),
				Signature: bzzAddress.Signature,
			},
			NetworkID:      s.networkID,
			FullNode:       s.fullNode,
			Transaction:    s.transaction,
			WelcomeMessage: welcomeMessage,
		},
	}); err != nil {
		s.metrics.SynAckTxFailed.Inc()
		return nil, fmt.Errorf("write synack message: %w", err)
	}
	s.metrics.SynAckTx.Inc()

	var ack pb.Ack
	if err := r.ReadMsgWithContext(ctx, &ack); err != nil {
		s.metrics.AckRxFailed.Inc()
		return nil, fmt.Errorf("read ack message: %w", err)
	}
	s.metrics.AckRx.Inc()

	if ack.NetworkID != s.networkID {
		return nil, ErrNetworkIDIncompatible
	}

	overlay := swarm.NewAddress(ack.Address.Overlay)

	if s.picker != nil {
		if !s.picker.Pick(p2p.Peer{Address: overlay, FullNode: ack.FullNode}) {
			return nil, ErrPicker
		}
	}

	blockHash, err := s.senderMatcher.Matches(ctx, ack.Transaction, s.networkID, overlay, false)
	if err != nil {
		return nil, fmt.Errorf("overlay %v verification failed: %w", overlay, err)
	}

	remoteBzzAddress, err := s.parseCheckAck(&ack, blockHash)
	if err != nil {
		return nil, err
	}

	s.logger.Tracef("handshake finished for peer (inbound) %s", remoteBzzAddress.Overlay.String())
	if len(ack.WelcomeMessage) > 0 {
		s.logger.Infof("greeting \"%s\" from peer: %s", ack.WelcomeMessage, remoteBzzAddress.Overlay.String())
	}

	return &Info{
		BzzAddress: remoteBzzAddress,
		FullNode:   ack.FullNode,
	}, nil
}

// SetWelcomeMessage sets the new handshake welcome message.
func (s *Service) SetWelcomeMessage(msg string) (err error) {
	if len(msg) > MaxWelcomeMessageLength {
		return ErrWelcomeMessageLength
	}
	s.welcomeMessage.Store(msg)
	return nil
}

// GetWelcomeMessage returns the the current handshake welcome message.
func (s *Service) GetWelcomeMessage() string {
	return s.welcomeMessage.Load().(string)
}

func buildFullMA(addr ma.Multiaddr, peerID libp2ppeer.ID) (ma.Multiaddr, error) {
	return ma.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", addr.String(), peerID.Pretty()))
}

func (s *Service) parseCheckAck(ack *pb.Ack, blockHash []byte) (*bzz.Address, error) {
	bzzAddress, err := bzz.ParseAddress(ack.Address.Underlay, ack.Address.Overlay, ack.Address.Signature, ack.Transaction, blockHash, s.networkID)
	if err != nil {
		return nil, ErrInvalidAck
	}

	return bzzAddress, nil
}
