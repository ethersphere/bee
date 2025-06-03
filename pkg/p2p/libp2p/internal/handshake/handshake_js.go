//go:build js
// +build js

package handshake

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// Service can perform initiate or handle a handshake between peers.
type Service struct {
	signer                crypto.Signer
	advertisableAddresser AdvertisableAddressResolver
	overlay               swarm.Address
	fullNode              bool
	nonce                 []byte
	validateOverlay       bool
	welcomeMessage        atomic.Value
	logger                log.Logger
	libp2pID              libp2ppeer.ID
	picker                p2p.Picker
}

// New creates a new handshake Service.
func New(signer crypto.Signer, advertisableAddresser AdvertisableAddressResolver, overlay swarm.Address, networkID uint64, fullNode bool, nonce []byte, welcomeMessage string, validateOverlay bool, ownPeerID libp2ppeer.ID, logger log.Logger) (*Service, error) {
	if len(welcomeMessage) > MaxWelcomeMessageLength {
		return nil, ErrWelcomeMessageLength
	}

	svc := &Service{
		signer:                signer,
		advertisableAddresser: advertisableAddresser,
		overlay:               overlay,
		fullNode:              fullNode,
		validateOverlay:       validateOverlay,
		nonce:                 nonce,
		libp2pID:              ownPeerID,
		logger:                logger.WithName(loggerName).Register(),
	}
	svc.welcomeMessage.Store(welcomeMessage)

	return svc, nil
}

// Handle handles an incoming handshake from a peer.
func (s *Service) Handle(ctx context.Context, stream p2p.Stream, remoteMultiaddr ma.Multiaddr, remotePeerID libp2ppeer.ID) (i *Info, err error) {
	loggerV1 := s.logger.V(1).Register()

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
		return nil, fmt.Errorf("read syn message: %w", err)
	}
	observedUnderlay, err := ma.NewMultiaddrBytes(syn.ObservedUnderlay)
	if err != nil {
		return nil, ErrInvalidSyn
	}

	advertisableUnderlay, err := s.advertisableAddresser.Resolve(observedUnderlay)
	if err != nil {
		return nil, err
	}

	bzzAddress, err := bzz.NewAddress(s.signer, advertisableUnderlay, s.overlay, 0, s.nonce)
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
			FullNode:       s.fullNode,
			Nonce:          s.nonce,
			WelcomeMessage: welcomeMessage,
		},
	}); err != nil {
		return nil, fmt.Errorf("write synack message: %w", err)
	}

	var ack pb.Ack
	if err := r.ReadMsgWithContext(ctx, &ack); err != nil {
		return nil, fmt.Errorf("read ack message: %w", err)
	}

	overlay := swarm.NewAddress(ack.Address.Overlay)

	if s.picker != nil {
		if !s.picker.Pick(p2p.Peer{Address: overlay, FullNode: ack.FullNode}) {
			return nil, ErrPicker
		}
	}

	remoteBzzAddress, err := s.parseCheckAck(&ack)
	if err != nil {
		return nil, err
	}

	loggerV1.Debug("handshake finished for peer (inbound)", "peer_address", remoteBzzAddress.Overlay)
	if len(ack.WelcomeMessage) > 0 {
		loggerV1.Debug("greeting message from peer", "peer_address", remoteBzzAddress.Overlay, "message", ack.WelcomeMessage)
	}

	return &Info{
		BzzAddress: remoteBzzAddress,
		FullNode:   ack.FullNode,
	}, nil
}

// Handshake initiates a handshake with a peer.
func (s *Service) Handshake(ctx context.Context, stream p2p.Stream, peerMultiaddr ma.Multiaddr, peerID libp2ppeer.ID) (i *Info, err error) {
	loggerV1 := s.logger.V(1).Register()

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
		// NOTE eventually we will return error here, but for now we want to gather some statistics
		s.logger.Warning("received peer ID does not match ours", "their", observedUnderlayAddrInfo.ID, "ours", s.libp2pID)
	}

	advertisableUnderlay, err := s.advertisableAddresser.Resolve(observedUnderlay)
	if err != nil {
		return nil, err
	}

	bzzAddress, err := bzz.NewAddress(s.signer, advertisableUnderlay, s.overlay, 0, s.nonce)
	if err != nil {
		return nil, err
	}

	advertisableUnderlayBytes, err := bzzAddress.Underlay.MarshalBinary()
	if err != nil {
		return nil, err
	}

	remoteBzzAddress, err := s.parseCheckAck(resp.Ack)
	if err != nil {
		return nil, err
	}

	// Synced read:
	welcomeMessage := s.GetWelcomeMessage()
	msg := &pb.Ack{
		Address: &pb.BzzAddress{
			Underlay:  advertisableUnderlayBytes,
			Overlay:   bzzAddress.Overlay.Bytes(),
			Signature: bzzAddress.Signature,
		},
		FullNode:       s.fullNode,
		Nonce:          s.nonce,
		WelcomeMessage: welcomeMessage,
	}

	if err := w.WriteMsgWithContext(ctx, msg); err != nil {
		return nil, fmt.Errorf("write ack message: %w", err)
	}

	loggerV1.Debug("handshake finished for peer (outbound)", "peer_address", remoteBzzAddress.Overlay)
	if len(resp.Ack.WelcomeMessage) > 0 {
		s.logger.Debug("greeting message from peer", "peer_address", remoteBzzAddress.Overlay, "message", resp.Ack.WelcomeMessage)
	}

	return &Info{
		BzzAddress: remoteBzzAddress,
		FullNode:   resp.Ack.FullNode,
	}, nil
}

func (s *Service) parseCheckAck(ack *pb.Ack) (*bzz.Address, error) {
	bzzAddress, err := bzz.ParseAddress(ack.Address.Underlay, ack.Address.Overlay, ack.Address.Signature, ack.Nonce, s.validateOverlay, 0)
	if err != nil {
		return nil, ErrInvalidAck
	}

	return bzzAddress, nil
}
