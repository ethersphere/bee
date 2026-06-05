// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/log"
	handshake "github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	libp2pm "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func (s *Service) HandshakeService() *handshake.Service {
	return s.handshakeService
}

func (s *Service) NewStreamForPeerID(peerID libp2ppeer.ID, protocolName, protocolVersion, streamName string) (network.Stream, error) {
	return s.newStreamForPeerID(context.Background(), peerID, protocolName, protocolVersion, streamName)
}

func (s *Service) WrapStream(ns network.Stream) *stream {
	return newStream(ns, s.metrics)
}

func (s *Service) Host() host.Host {
	return s.host
}

func (s *Service) SetHost(h host.Host) {
	s.host = h
}

type StaticAddressResolver = staticAddressResolver

var (
	NewStaticAddressResolver = newStaticAddressResolver
	UserAgent                = userAgent
)

func WithHostFactory(factory func(...libp2pm.Option) (host.Host, error)) Options {
	return Options{
		hostFactory: factory,
	}
}

func WithAutoTLSCertManager(m autoTLSCertManager) Options {
	return Options{
		autoTLSCertManager: m,
	}
}

func SetAutoTLSCertManager(o *Options, m autoTLSCertManager) {
	o.autoTLSCertManager = m
}

type AutoTLSCertManager = autoTLSCertManager

var NewCompositeAddressResolver = newCompositeAddressResolver

func (s *Service) FilterSupportedAddresses(addrs []ma.Multiaddr) []ma.Multiaddr {
	return s.filterSupportedAddresses(addrs)
}

func (s *Service) PeerMultiaddrs(ctx context.Context, peerID libp2ppeer.ID) ([]ma.Multiaddr, error) {
	return s.peerMultiaddrs(ctx, peerID)
}

func (s *Service) SetTransportFlags(hasTCP, hasWS, hasWSS bool) {
	s.enabledTransports = map[bzz.TransportType]bool{
		bzz.TransportTCP: hasTCP,
		bzz.TransportWS:  hasWS,
		bzz.TransportWSS: hasWSS,
	}
}

// Disconnected drives the unexported disconnect hook so tests can verify
// its side effects (e.g. chequebook registry eviction) end-to-end.
func (s *Service) Disconnected(address swarm.Address) {
	s.disconnected(address)
}

// NewDisconnectTestService constructs a minimal *Service wired only with the
// fields needed to drive the disconnect hook. Other fields are left
// zero-valued; callers must not invoke network-layer methods on it.
func NewDisconnectTestService(logger log.Logger, storer ChequebookStorer) *Service {
	return &Service{
		logger:           logger,
		metrics:          newMetrics(),
		peers:            newPeerRegistry(),
		chequebookStorer: storer,
	}
}

// PutHandshakeAddress drives the unexported addressbook-write path so tests
// can verify its branches end-to-end.
func (s *Service) PutHandshakeAddress(addr *bzz.Address) error {
	return s.putHandshakeAddress(addr)
}

// NewPutHandshakeAddressTestService constructs a minimal *Service wired only
// with the fields needed to drive putHandshakeAddress. Other fields are left
// zero-valued; callers must not invoke network-layer methods on it.
func NewPutHandshakeAddressTestService(logger log.Logger, book addressbook.GetPutter, storer ChequebookStorer) *Service {
	return &Service{
		logger:           logger,
		addressbook:      book,
		chequebookStorer: storer,
	}
}
