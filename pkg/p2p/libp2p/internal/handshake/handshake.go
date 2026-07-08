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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
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
	ProtocolVersion = "15.0.0"
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

// Service can perform initiate or handle a handshake between peers.
type Service struct {
	signer                crypto.Signer
	advertisableAddresser AdvertisableAddressResolver
	overlay               swarm.Address
	fullNode              bool
	nonce                 []byte
	networkID             uint64
	welcomeMessage        atomic.Value
	chequebookAddr        atomic.Pointer[common.Address] // set once the local chequebook is known; nil means absent.
	chequebookVerifier    chequebook.Verifier            // nil means verification disabled.
	addressbook           addressbook.Getter
	logger                log.Logger
	libp2pID              libp2ppeer.ID
	metrics               metrics
	picker                p2p.Picker
	mu                    sync.RWMutex
	hostAddresser         Addresser
	now                   func() time.Time
	addrCache             addressCache // session-stable signed address, keyed by chequebook + underlays
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

// New creates a new handshake Service. A nil chequebookVerifier disables the
// chequebook gate; otherwise handshake completion requires the peer's
// chequebook to pass verification.
func New(signer crypto.Signer, advertisableAddresser AdvertisableAddressResolver, overlay swarm.Address, networkID uint64, fullNode bool, nonce []byte, hostAddresser Addresser, welcomeMessage string, addrbook addressbook.Getter, ownPeerID libp2ppeer.ID, chequebookVerifier chequebook.Verifier, logger log.Logger) (*Service, error) {
	if len(welcomeMessage) > MaxWelcomeMessageLength {
		return nil, ErrWelcomeMessageLength
	}

	svc := &Service{
		signer:                signer,
		advertisableAddresser: advertisableAddresser,
		overlay:               overlay,
		networkID:             networkID,
		fullNode:              fullNode,
		nonce:                 nonce,
		libp2pID:              ownPeerID,
		logger:                logger.WithName(loggerName).Register(),
		metrics:               newMetrics(),
		hostAddresser:         hostAddresser,
		addressbook:           addrbook,
		chequebookVerifier:    chequebookVerifier,
		now:                   time.Now,
	}
	svc.welcomeMessage.Store(welcomeMessage)

	return svc, nil
}

// SetChequebookAddress sets the local chequebook address included in
// subsequent signed BzzAddress payloads; the zero value clears it. The
// chequebook is part of the signed-address cache key, so the next handshake
// misses the cache and re-signs with the new chequebook.
func (s *Service) SetChequebookAddress(addr common.Address) {
	if (addr == common.Address{}) {
		s.chequebookAddr.Store(nil)
	} else {
		s.chequebookAddr.Store(&addr)
	}
}

func (s *Service) chequebookAddress() common.Address {
	if v := s.chequebookAddr.Load(); v != nil {
		return *v
	}
	return common.Address{}
}

// signedAddress returns the session-stable signed BzzAddress advertising the
// given canonical underlay set. The record is minted on first use and reused
// byte-stable (same timestamp and signature) across handshakes, so receiving
// peers see it as unchanged and skip redundant addressbook writes and gossip
// updates. It is re-minted when the underlay set or the chequebook changes.
func (s *Service) signedAddress(underlays []ma.Multiaddr) (*bzz.Address, error) {
	underlaysBinary, err := bzz.SerializeUnderlays(underlays)
	if err != nil {
		return nil, fmt.Errorf("serialize underlays: %w", err)
	}

	chequebook := s.chequebookAddress()
	key := string(chequebook.Bytes()) + string(underlaysBinary)

	return s.addrCache.getOrMint(key, s.now().Unix(), func(timestamp int64) (*bzz.Address, error) {
		addr, err := bzz.NewAddress(s.signer, underlays, s.overlay, s.networkID, s.nonce, timestamp, chequebook)
		if err != nil {
			return nil, err
		}
		s.metrics.AddressMinted.Inc()
		return addr, nil
	})
}

func (s *Service) SetPicker(n p2p.Picker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.picker = n
}

// Handshake initiates a handshake with a peer.
func (s *Service) Handshake(ctx context.Context, stream p2p.Stream, peerMultiaddrs []ma.Multiaddr) (i *Info, err error) {
	loggerV1 := s.logger.V(1).Register()

	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	w, r := protobuf.NewWriterAndReader(stream)

	var observedTruncated bool
	peerMultiaddrs, observedTruncated = bzz.TruncateUnderlays(peerMultiaddrs)
	if observedTruncated {
		s.metrics.ObservedUnderlaysTruncated.Inc()
	}

	observedUnderlayBytes, err := bzz.SerializeUnderlays(peerMultiaddrs)
	if err != nil {
		return nil, fmt.Errorf("serialize observed underlays: %w", err)
	}

	if err := w.WriteMsgWithContext(ctx, &pb.Syn{
		ObservedUnderlay: observedUnderlayBytes,
	}); err != nil {
		return nil, fmt.Errorf("write syn message: %w", err)
	}

	var resp pb.SynAck
	if err := r.ReadMsgWithContext(ctx, &resp); err != nil {
		return nil, fmt.Errorf("read synack message: %w", err)
	}

	// Reject malformed SynAck messages where nested pointer fields are
	// absent. Proto3 generates these as optional pointers, so a peer can
	// send a decodable message that would otherwise panic on deref.
	if resp.Syn == nil {
		return nil, ErrInvalidSyn
	}
	if resp.Ack == nil || resp.Ack.Address == nil {
		return nil, ErrInvalidAck
	}

	observedUnderlays, err := bzz.DeserializeUnderlays(resp.Syn.ObservedUnderlay)
	if err != nil {
		return nil, fmt.Errorf("%w: observed underlay len=%d: %w", ErrInvalidSyn, len(resp.Syn.ObservedUnderlay), err)
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

	// Truncate to count and byte-size caps before signing.
	var advTruncated bool
	advertisableUnderlays, advTruncated = bzz.TruncateUnderlays(advertisableUnderlays)
	if advTruncated {
		s.metrics.AdvertisableUnderlaysTruncated.Inc()
	}

	bzzAddress, err := s.signedAddress(advertisableUnderlays)
	if err != nil {
		return nil, err
	}

	if resp.Ack.NetworkID != s.networkID {
		return nil, ErrNetworkIDIncompatible
	}

	remoteBzzAddress, err := s.parseCheckAck(ctx, resp.Ack)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidAck, err)
	}

	// Synced read:
	welcomeMessage := s.GetWelcomeMessage()

	ackUnderlayBytes, err := bzz.SerializeUnderlays(bzzAddress.Underlays)
	if err != nil {
		return nil, fmt.Errorf("serialize ack underlays: %w", err)
	}

	msg := &pb.Ack{
		Address: &pb.BzzAddress{
			Underlay:          ackUnderlayBytes,
			Overlay:           bzzAddress.Overlay.Bytes(),
			Signature:         bzzAddress.Signature,
			Nonce:             bzzAddress.Nonce,
			Timestamp:         bzzAddress.Timestamp,
			ChequebookAddress: bzzAddress.ChequebookAddress.Bytes(),
		},
		NetworkID:      s.networkID,
		FullNode:       s.fullNode,
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
func (s *Service) Handle(ctx context.Context, stream p2p.Stream, peerMultiaddrs []ma.Multiaddr) (i *Info, err error) {
	loggerV1 := s.logger.V(1).Register()

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
		return nil, fmt.Errorf("%w: observed underlay len=%d: %w", ErrInvalidSyn, len(syn.ObservedUnderlay), err)
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

	// Truncate to count and byte-size caps before signing.
	var handleAdvTruncated bool
	advertisableUnderlays, handleAdvTruncated = bzz.TruncateUnderlays(advertisableUnderlays)
	if handleAdvTruncated {
		s.metrics.AdvertisableUnderlaysTruncated.Inc()
	}

	bzzAddress, err := s.signedAddress(advertisableUnderlays)
	if err != nil {
		return nil, err
	}

	welcomeMessage := s.GetWelcomeMessage()

	var handleObsTruncated bool
	peerMultiaddrs, handleObsTruncated = bzz.TruncateUnderlays(peerMultiaddrs)
	if handleObsTruncated {
		s.metrics.ObservedUnderlaysTruncated.Inc()
	}

	synObservedBytes, err := bzz.SerializeUnderlays(peerMultiaddrs)
	if err != nil {
		return nil, fmt.Errorf("serialize syn observed underlays: %w", err)
	}

	synAckUnderlayBytes, err := bzz.SerializeUnderlays(bzzAddress.Underlays)
	if err != nil {
		return nil, fmt.Errorf("serialize synack underlays: %w", err)
	}

	if err := w.WriteMsgWithContext(ctx, &pb.SynAck{
		Syn: &pb.Syn{
			ObservedUnderlay: synObservedBytes,
		},
		Ack: &pb.Ack{
			Address: &pb.BzzAddress{
				Underlay:          synAckUnderlayBytes,
				Overlay:           bzzAddress.Overlay.Bytes(),
				Signature:         bzzAddress.Signature,
				Nonce:             bzzAddress.Nonce,
				Timestamp:         bzzAddress.Timestamp,
				ChequebookAddress: bzzAddress.ChequebookAddress.Bytes(),
			},
			NetworkID:      s.networkID,
			FullNode:       s.fullNode,
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

	// Reject malformed Ack messages with a nil nested BzzAddress — proto3
	// makes this a pointer field, so a peer can send one that would panic
	// on deref otherwise.
	if ack.Address == nil {
		return nil, ErrInvalidAck
	}

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

	remoteBzzAddress, err := s.parseCheckAck(ctx, &ack)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidAck, err)
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

func (s *Service) parseCheckAck(ctx context.Context, ack *pb.Ack) (*bzz.Address, error) {
	// Defence in depth: guard against nil nested fields so this helper is
	// safe independently of its callers.
	if ack == nil || ack.Address == nil {
		return nil, ErrInvalidAck
	}

	bzzAddress, err := bzz.ParseAddress(ack.Address.Underlay, ack.Address.Overlay, ack.Address.Signature, ack.Address.Nonce, ack.Address.Timestamp, s.networkID, ack.Address.ChequebookAddress)
	if err != nil {
		return nil, fmt.Errorf("parse address: %w", err)
	}

	existing, pastVerified, err := s.addressbook.Get(bzzAddress.Overlay)
	if err != nil && !errors.Is(err, addressbook.ErrNotFound) {
		return nil, fmt.Errorf("addressbook get: %w", err)
	}

	if err := bzz.CheckTimestamp(bzzAddress.Timestamp, existing, bzz.TimestampSourceHandshake, s.now()); err != nil {
		if reason, ok := bzz.TimestampErrorLabel(err); ok {
			s.metrics.TimestampRejected.WithLabelValues(reason).Inc()
		}
		return nil, fmt.Errorf("check timestamp: %w", err)
	}

	if s.chequebookVerifier != nil && ack.FullNode {
		if (bzzAddress.ChequebookAddress == common.Address{}) {
			s.metrics.ChequebookVerification.WithLabelValues("missing").Inc()
			return nil, chequebook.ErrChequebookAddressMissing
		}
		pairVerified := pastVerified && existing != nil && existing.ChequebookAddress == bzzAddress.ChequebookAddress
		peerEth := common.BytesToAddress(bzzAddress.EthereumAddress)
		if err := s.chequebookVerifier.Verify(ctx, bzzAddress.ChequebookAddress, peerEth, bzzAddress.Overlay, pairVerified); err != nil {
			s.metrics.ChequebookVerification.WithLabelValues(chequebook.VerifyErrorLabel(err)).Inc()
			return nil, fmt.Errorf("verify chequebook: %w", err)
		}
		s.metrics.ChequebookVerification.WithLabelValues("success").Inc()
	}

	return bzzAddress, nil
}
