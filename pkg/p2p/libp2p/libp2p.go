// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"net"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	handshake "github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/libp2p/go-libp2p"
	autonat "github.com/libp2p/go-libp2p-autonat-svc"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/helpers"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	protocol "github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	libp2pquic "github.com/libp2p/go-libp2p-quic-transport"
	"github.com/libp2p/go-tcp-transport"
	ws "github.com/libp2p/go-ws-transport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multistream"
)

var _ p2p.Service = (*Service)(nil)

type Service struct {
	host             host.Host
	libp2pPeerstore  peerstore.Peerstore
	metrics          metrics
	networkID        int32
	handshakeService *handshake.Service
	peers            *peerRegistry
	topologyDriver   topology.Driver
	addressBook      addressbook.Putter
	protocols        []p2p.ProtocolSpec
	logger           logging.Logger
}

type Options struct {
	PrivateKey     *ecdsa.PrivateKey
	Overlay        swarm.Address
	Addr           string
	DisableWS      bool
	DisableQUIC    bool
	Bootnodes      []string
	NetworkID      int32
	AddressBook    addressbook.GetterPutter
	TopologyDriver topology.Driver
	Logger         logging.Logger
}

func New(ctx context.Context, o Options) (*Service, error) {
	host, port, err := net.SplitHostPort(o.Addr)
	if err != nil {
		return nil, fmt.Errorf("address: %w", err)
	}

	ip4Addr := "0.0.0.0"
	ip6Addr := "::1"

	if host != "" {
		ip := net.ParseIP(host)
		if ip4 := ip.To4(); ip4 != nil {
			ip4Addr = ip4.String()
			ip6Addr = ""
		} else if ip6 := ip.To16(); ip6 != nil {
			ip6Addr = ip6.String()
			ip4Addr = ""
		}
	}

	var listenAddrs []string
	if ip4Addr != "" {
		listenAddrs = append(listenAddrs, fmt.Sprintf("/ip4/%s/tcp/%s", ip4Addr, port))
		if !o.DisableWS {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip4/%s/tcp/%s/ws", ip4Addr, port))
		}
		if !o.DisableQUIC {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip4/%s/udp/%s/quic", ip4Addr, port))
		}
	}

	if ip6Addr != "" {
		listenAddrs = append(listenAddrs, fmt.Sprintf("/ip6/%s/tcp/%s", ip6Addr, port))
		if !o.DisableWS {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip6/%s/tcp/%s/ws", ip6Addr, port))
		}
		if !o.DisableQUIC {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip6/%s/udp/%s/quic", ip6Addr, port))
		}
	}

	security := libp2p.DefaultSecurity
	libp2pPeerstore := pstoremem.NewPeerstore()

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(listenAddrs...),
		security,
		// Attempt to open ports using uPNP for NATed hosts.
		libp2p.NATPortMap(),
		// Use dedicated peerstore instead the global DefaultPeerstore
		libp2p.Peerstore(libp2pPeerstore),
	}

	if o.PrivateKey != nil {
		opts = append(opts,
			libp2p.Identity((*crypto.Secp256k1PrivateKey)(o.PrivateKey)),
		)
	}

	transports := []libp2p.Option{
		libp2p.Transport(tcp.NewTCPTransport),
	}

	if !o.DisableWS {
		transports = append(transports, libp2p.Transport(ws.New))
	}

	if !o.DisableQUIC {
		transports = append(transports, libp2p.Transport(libp2pquic.NewTransport))
	}

	opts = append(opts, transports...)

	h, err := libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	// If you want to help other peers to figure out if they are behind
	// NATs, you can launch the server-side of AutoNAT too (AutoRelay
	// already runs the client)
	if _, err = autonat.NewAutoNATService(ctx, h,
		// Support same non default security and transport options as
		// original host.
		append(transports, security)...,
	); err != nil {
		return nil, fmt.Errorf("autonat: %w", err)
	}

	peerRegistry := newPeerRegistry()
	s := &Service{
		host:             h,
		libp2pPeerstore:  libp2pPeerstore,
		metrics:          newMetrics(),
		networkID:        o.NetworkID,
		handshakeService: handshake.New(peerRegistry, o.Overlay, o.NetworkID, o.Logger),
		peers:            peerRegistry,
		addressBook:      o.AddressBook,
		topologyDriver:   o.TopologyDriver,
		logger:           o.Logger,
	}

	// Construct protocols.

	id := protocol.ID(p2p.NewSwarmStreamName(handshake.ProtocolName, handshake.ProtocolVersion, handshake.StreamName))
	matcher, err := s.protocolSemverMatcher(id)
	if err != nil {
		return nil, fmt.Errorf("protocol version match %s: %w", id, err)
	}

	s.host.SetStreamHandlerMatch(id, matcher, func(stream network.Stream) {
		peerID := stream.Conn().RemotePeer()
		i, err := s.handshakeService.Handle(newStream(stream))
		if err != nil {
			if err == handshake.ErrNetworkIDIncompatible {
				s.logger.Warningf("peer %s has a different network id.", peerID)
			}

			if err == handshake.ErrHandshakeDuplicate {
				s.logger.Warningf("handshake happened for already connected peer %s", peerID)
			}

			s.logger.Debugf("handshake: handle %s: %v", peerID, err)
			s.logger.Errorf("unable to handshake with peer %v", peerID)
			// todo: test connection close and refactor
			_ = s.disconnect(peerID)
			return
		}

		s.peers.add(stream.Conn(), i.Address)
		s.metrics.HandledStreamCount.Inc()
		s.logger.Infof("peer %s connected", i.Address)
	})

	// TODO: be more resilient on connection errors and connect in parallel
	for _, a := range o.Bootnodes {
		addr, err := ma.NewMultiaddr(a)
		if err != nil {
			return nil, fmt.Errorf("bootnode %s: %w", a, err)
		}

		overlay, err := s.Connect(ctx, addr)
		if err != nil {
			return nil, fmt.Errorf("connect to bootnode %s %s: %w", a, overlay, err)
		}
	}

	h.Network().SetConnHandler(func(_ network.Conn) {
		s.metrics.HandledConnectionCount.Inc()
	})

	h.Network().Notify(peerRegistry) // update peer registry on network events

	return s, nil
}

func (s *Service) AddProtocol(p p2p.ProtocolSpec) (err error) {
	for _, ss := range p.StreamSpecs {
		id := protocol.ID(p2p.NewSwarmStreamName(p.Name, p.Version, ss.Name))
		matcher, err := s.protocolSemverMatcher(id)
		if err != nil {
			return fmt.Errorf("protocol version match %s: %w", id, err)
		}

		s.host.SetStreamHandlerMatch(id, matcher, func(stream network.Stream) {
			peerID := stream.Conn().RemotePeer()
			overlay, found := s.peers.overlay(peerID)
			if !found {
				// todo: this should never happen, should we disconnect in this case?
				// todo: test connection close and refactor
				_ = s.disconnect(peerID)
				s.logger.Errorf("overlay address for peer %q not found", peerID)
				return
			}

			s.metrics.HandledStreamCount.Inc()
			if err := ss.Handler(p2p.Peer{Address: overlay}, newStream(stream)); err != nil {
				var e *p2p.DisconnectError
				if errors.Is(err, e) {
					// todo: test connection close and refactor
					_ = s.Disconnect(overlay)
				}

				s.logger.Debugf("handle protocol %s/%s: stream %s: peer %s: %w", p.Name, p.Version, ss.Name, overlay, err)
			}
		})
	}

	s.protocols = append(s.protocols, p)
	return nil
}

func (s *Service) Addresses() (addrs []ma.Multiaddr, err error) {
	// Build host multiaddress
	hostAddr, err := ma.NewMultiaddr(fmt.Sprintf("/p2p/%s", s.host.ID().Pretty()))
	if err != nil {
		return nil, err
	}

	// Now we can build a full multiaddress to reach this host
	// by encapsulating both addresses:
	for _, addr := range s.host.Addrs() {
		addrs = append(addrs, addr.Encapsulate(hostAddr))
	}
	return addrs, nil
}

func (s *Service) Connect(ctx context.Context, addr ma.Multiaddr) (overlay swarm.Address, err error) {
	// Extract the peer ID from the multiaddr.
	info, err := libp2ppeer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return swarm.Address{}, err
	}

	if err := s.host.Connect(ctx, *info); err != nil {
		return swarm.Address{}, err
	}

	stream, err := s.newStreamForPeerID(ctx, info.ID, handshake.ProtocolName, handshake.ProtocolVersion, handshake.StreamName)
	if err != nil {
		_ = s.disconnect(info.ID)
		return swarm.Address{}, err
	}

	i, err := s.handshakeService.Handshake(newStream(stream))
	if err != nil {
		_ = s.disconnect(info.ID)
		return swarm.Address{}, fmt.Errorf("handshake: %w", err)
	}

	if err := helpers.FullClose(stream); err != nil {
		return swarm.Address{}, err
	}

	s.peers.add(stream.Conn(), i.Address)
	s.addressBook.Put(i.Address, addr)

	err = s.topologyDriver.AddPeer(i.Address)
	if err != nil {
		return swarm.Address{}, fmt.Errorf("topology addpeer: %w", err)
	}

	s.metrics.CreatedConnectionCount.Inc()
	s.logger.Infof("peer %s connected", i.Address)
	return i.Address, nil
}

func (s *Service) Disconnect(overlay swarm.Address) error {
	peerID, found := s.peers.peerID(overlay)
	if !found {
		return p2p.ErrPeerNotFound
	}
	return s.disconnect(peerID)
}

func (s *Service) disconnect(peerID libp2ppeer.ID) error {
	if err := s.host.Network().ClosePeer(peerID); err != nil {
		return err
	}
	s.peers.remove(peerID)
	return nil
}

func (s *Service) Peers() []p2p.Peer {
	return s.peers.peers()
}

func (s *Service) NewStream(ctx context.Context, overlay swarm.Address, protocolName, protocolVersion, streamName string) (p2p.Stream, error) {
	peerID, found := s.peers.peerID(overlay)
	if !found {
		return nil, p2p.ErrPeerNotFound
	}

	stream, err := s.newStreamForPeerID(ctx, peerID, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, err
	}

	return newStream(stream), nil
}

func (s *Service) newStreamForPeerID(ctx context.Context, peerID libp2ppeer.ID, protocolName, protocolVersion, streamName string) (network.Stream, error) {
	swarmStreamName := p2p.NewSwarmStreamName(protocolName, protocolVersion, streamName)
	st, err := s.host.NewStream(ctx, peerID, protocol.ID(swarmStreamName))
	if err != nil {
		if err == multistream.ErrNotSupported || err == multistream.ErrIncorrectVersion {
			return nil, p2p.NewIncompatibleStreamError(err)
		}
		return nil, fmt.Errorf("create stream %q to %q: %w", swarmStreamName, peerID, err)
	}
	s.metrics.CreatedStreamCount.Inc()
	return st, nil
}

func (s *Service) Close() error {
	if err := s.libp2pPeerstore.Close(); err != nil {
		return err
	}
	return s.host.Close()
}
