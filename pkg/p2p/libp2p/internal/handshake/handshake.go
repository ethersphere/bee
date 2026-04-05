// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

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

// loggerName is the tree path name of the logger for this package.
const loggerName = "handshake"

const (
	// ProtocolName is the text of the name of the handshake protocol.
	ProtocolName = "handshake"
	// ProtocolVersion is the current handshake protocol version.
	ProtocolVersion = "14.0.0"
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
	ErrPicker = errors.New("picker rejection")
)

// AdvertisableAddressResolver can Resolve a Multiaddress.
type AdvertisableAddressResolver interface {
	Resolve(observedAddress ma.Multiaddr) (ma.Multiaddr, error)
}

type Addresser interface {
	AdvertizableAddrs() ([]ma.Multiaddr, error)
}

type Option struct {
	bee260compatibility bool
}

// WithBee260Compatibility option ensures that only one underlay address is
// passed to the peer in p2p protocol messages, so that nodes with version 2.6.0
// and older can deserialize it. This option can be safely removed when bee
// version 2.6.0 is deprecated.
func WithBee260Compatibility(yes bool) func(*Option) {
	return func(o *Option) {
		o.bee260compatibility = yes
	}
}

// Service can perform initiate or handle a handshake between peers.
type Service struct {
	signer                crypto.Signer
	advertisableAddresser AdvertisableAddressResolver
	overlay               swarm.Address
	fullNode              bool
	nonce                 []byte
	networkID             uint64
	validateOverlay       bool
	welcomeMessage        atomic.Value
	logger                log.Logger
	libp2pID              libp2ppeer.ID
	metrics               metrics
	picker                p2p.Picker
	mu                    sync.RWMutex
	hostAddresser         Addresser
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
func New(signer crypto.Signer, advertisableAddresser AdvertisableAddressResolver, overlay swarm.Address, networkID uint64, fullNode bool, nonce []byte, hostAddresser Addresser, welcomeMessage string, validateOverlay bool, ownPeerID libp2ppeer.ID, logger log.Logger) (*Service, error) {
	if len(welcomeMessage) > MaxWelcomeMessageLength {
		return nil, ErrWelcomeMessageLength
	}

	svc := &Service{
		signer:                signer,
		advertisableAddresser: advertisableAddresser,
		overlay:               overlay,
		networkID:             networkID,
		fullNode:              fullNode,
		validateOverlay:       validateOverlay,
		nonce:                 nonce,
		libp2pID:              ownPeerID,
		logger:                logger.WithName(loggerName).Register(),
		metrics:               newMetrics(),
		hostAddresser:         hostAddresser,
	}
	svc.welcomeMessage.Store(welcomeMessage)

	return svc, nil
}

func (s *Service) SetPicker(n p2p.Picker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.picker = n
}

// Handshake initiates a handshake with a peer.
func (s *Service) Handshake(ctx context.Context, stream p2p.Stream, peerMultiaddrs []ma.Multiaddr, opts ...func(*Option)) (i *Info, err error) {
	loggerV1 := s.logger.V(1).Register()

	o := new(Option)
	for _, set := range opts {
		set(o)
	}

	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	w, r := protobuf.NewWriterAndReader(stream)

	peerMultiaddrs = p2p.FilterBee260CompatibleUnderlays(o.bee260compatibility, peerMultiaddrs)

	if err := w.WriteMsgWithContext(ctx, &pb.Syn{
		ObservedUnderlay: bzz.SerializeUnderlays(peerMultiaddrs),
	}); err != nil {
		return nil, fmt.Errorf("write syn message: %w", err)
	}

	var resp pb.SynAck
	if err := r.ReadMsgWithContext(ctx, &resp); err != nil {
		return nil, fmt.Errorf("read synack message: %w", err)
	}

	observedUnderlays, err := bzz.DeserializeUnderlays(resp.Syn.ObservedUnderlay)
	if err != nil {
		return nil, ErrInvalidSyn
	}

	advertisableUnderlays := make([]ma.Multiaddr, len(observedUnderlays))
	for i, observedUnderlay := range observedUnderlays {
		observedUnderlayAddrInfo, err := libp2ppeer.AddrInfoFromP2pAddr(observedUnderlay)
		if err != nil {
			return nil, fmt.Errorf("extract addr from P2P: %w", err)
		}

		if s.libp2pID != observedUnderlayAddrInfo.ID {
			return nil, fmt.Errorf("received peer ID %s does not match ours %s", observedUnderlayAddrInfo.ID.String(), s.libp2pID.String())
		}

		advertisableUnderlay, err := s.advertisableAddresser.Resolve(observedUnderlay)
		if err != nil {
			return nil, err
		}

		advertisableUnderlays[i] = advertisableUnderlay
	}

	if s.hostAddresser != nil {
		hostAddrs, err := s.hostAddresser.AdvertizableAddrs()
		if err != nil {
			return nil, fmt.Errorf("get host advertizable addresses: %w", err)
		}

		advertisableUnderlays = append(advertisableUnderlays, hostAddrs...)
	}

	// sort to remove potential duplicates
	slices.SortFunc(advertisableUnderlays, func(a, b ma.Multiaddr) int {
		return cmp.Compare(a.String(), b.String())
	})
	// remove duplicates
	advertisableUnderlays = slices.CompactFunc(advertisableUnderlays, func(a, b ma.Multiaddr) bool {
		return a.Equal(b)
	})

	advertisableUnderlays = p2p.FilterBee260CompatibleUnderlays(o.bee260compatibility, advertisableUnderlays)

	bzzAddress, err := bzz.NewAddress(s.signer, advertisableUnderlays, s.overlay, s.networkID, s.nonce)
	if err != nil {
		return nil, err
	}

	if resp.Ack.NetworkID != s.networkID {
		return nil, ErrNetworkIDIncompatible
	}

	remoteBzzAddress, err := s.parseCheckAck(resp.Ack)
	if err != nil {
		return nil, err
	}

	// Synced read:
	welcomeMessage := s.GetWelcomeMessage()
	msg := &pb.Ack{
		Address: &pb.BzzAddress{
			Underlay:  bzz.SerializeUnderlays(bzzAddress.Underlays),
			Overlay:   bzzAddress.Overlay.Bytes(),
			Signature: bzzAddress.Signature,
		},
		NetworkID:      s.networkID,
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

// Handle handles an incoming handshake from a peer.
func (s *Service) Handle(ctx context.Context, stream p2p.Stream, peerMultiaddrs []ma.Multiaddr, opts ...func(*Option)) (i *Info, err error) {
	loggerV1 := s.logger.V(1).Register()

	o := new(Option)
	for _, set := range opts {
		set(o)
	}

	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	w, r := protobuf.NewWriterAndReader(stream)

	var syn pb.Syn
	if err := r.ReadMsgWithContext(ctx, &syn); err != nil {
		s.metrics.SynRxFailed.Inc()
		return nil, fmt.Errorf("read syn message: %w", err)
	}
	s.metrics.SynRx.Inc()

	observedUnderlays, err := bzz.DeserializeUnderlays(syn.ObservedUnderlay)
	if err != nil {
		return nil, ErrInvalidSyn
	}

	advertisableUnderlays := make([]ma.Multiaddr, len(observedUnderlays))
	for i, observedUnderlay := range observedUnderlays {
		advertisableUnderlay, err := s.advertisableAddresser.Resolve(observedUnderlay)
		if err != nil {
			return nil, err
		}
		advertisableUnderlays[i] = advertisableUnderlay
	}

	if s.hostAddresser != nil {
		hostAddrs, err := s.hostAddresser.AdvertizableAddrs()
		if err != nil {
			return nil, fmt.Errorf("get host advertizable addresses: %w", err)
		}

		advertisableUnderlays = append(advertisableUnderlays, hostAddrs...)
	}

	// sort to remove potential duplicates
	slices.SortFunc(advertisableUnderlays, func(a, b ma.Multiaddr) int {
		return cmp.Compare(a.String(), b.String())
	})
	// remove duplicates
	advertisableUnderlays = slices.CompactFunc(advertisableUnderlays, func(a, b ma.Multiaddr) bool {
		return a.Equal(b)
	})

	advertisableUnderlays = p2p.FilterBee260CompatibleUnderlays(o.bee260compatibility, advertisableUnderlays)

	bzzAddress, err := bzz.NewAddress(s.signer, advertisableUnderlays, s.overlay, s.networkID, s.nonce)
	if err != nil {
		return nil, err
	}

	welcomeMessage := s.GetWelcomeMessage()

	peerMultiaddrs = p2p.FilterBee260CompatibleUnderlays(o.bee260compatibility, peerMultiaddrs)

	if err := w.WriteMsgWithContext(ctx, &pb.SynAck{
		Syn: &pb.Syn{
			ObservedUnderlay: bzz.SerializeUnderlays(peerMultiaddrs),
		},
		Ack: &pb.Ack{
			Address: &pb.BzzAddress{
				Underlay:  bzz.SerializeUnderlays(bzzAddress.Underlays),
				Overlay:   bzzAddress.Overlay.Bytes(),
				Signature: bzzAddress.Signature,
			},
			NetworkID:      s.networkID,
			FullNode:       s.fullNode,
			Nonce:          s.nonce,
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

	s.mu.RLock()
	picker := s.picker
	s.mu.RUnlock()

	if picker != nil {
		if !picker.Pick(p2p.Peer{Address: overlay, FullNode: ack.FullNode}) {
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

// SetWelcomeMessage sets the new handshake welcome message.
func (s *Service) SetWelcomeMessage(msg string) (err error) {
	if len(msg) > MaxWelcomeMessageLength {
		return ErrWelcomeMessageLength
	}
	s.welcomeMessage.Store(msg)
	return nil
}

// GetWelcomeMessage returns the current handshake welcome message.
func (s *Service) GetWelcomeMessage() string {
	return s.welcomeMessage.Load().(string)
}

func (s *Service) parseCheckAck(ack *pb.Ack) (*bzz.Address, error) {
	bzzAddress, err := bzz.ParseAddress(ack.Address.Underlay, ack.Address.Overlay, ack.Address.Signature, ack.Nonce, s.validateOverlay, s.networkID)
	if err != nil {
		return nil, ErrInvalidAck
	}

	return bzzAddress, nil
}
