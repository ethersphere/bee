// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/janos/bee/pkg/p2p"

	handshake "github.com/janos/bee/pkg/p2p/libp2p/internal/overlay"
	"github.com/libp2p/go-libp2p"
	autonat "github.com/libp2p/go-libp2p-autonat-svc"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/helpers"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"
	libp2pquic "github.com/libp2p/go-libp2p-quic-transport"
	secio "github.com/libp2p/go-libp2p-secio"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multistream"
)

var _ p2p.Service = new(Service)

type Service struct {
	host             host.Host
	metrics          metrics
	handshakeService *handshake.Service
	peers            map[string]*Peer // overlay -> peer
	addresses        map[libp2ppeer.ID]string // peerID -> overlay
	overlay			string
}

type Options struct {
	Addr             string
	DisableWS        bool
	DisableQUIC      bool
	Bootnodes        []string
	ConnectionsLow   int
	ConnectionsHigh  int
	ConnectionsGrace time.Duration
}

type Handshake interface {

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

	opts := []libp2p.Option{
		// Use the keypair we generated
		//libp2p.Identity(priv),
		// Multiple listen addresses
		libp2p.ListenAddrStrings(listenAddrs...),
		// support TLS connections
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		// support secio connections
		libp2p.Security(secio.ID, secio.New),
		// support any other default transports (TCP)
		libp2p.DefaultTransports,
		// Let's prevent our peer from having too many
		// connections by attaching a connection manager.
		libp2p.ConnectionManager(connmgr.NewConnManager(
			o.ConnectionsLow,
			o.ConnectionsHigh,
			o.ConnectionsGrace,
		)),
		// Attempt to open ports using uPNP for NATed hosts.
		libp2p.NATPortMap(),
	}

	if !o.DisableQUIC {
		opts = append(opts,
			// support QUIC - experimental
			libp2p.Transport(libp2pquic.NewTransport),
		)
	}

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
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Security(secio.ID, secio.New),
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.DefaultTransports,
	); err != nil {
		return nil, fmt.Errorf("autonat: %w", err)
	}

	s := &Service{
		host:    h,
		metrics: newMetrics(),
		peers: make(map[string]*Peer),
		addresses: make(map[libp2ppeer.ID]string),
		overlay : strconv.Itoa(rand.Int()),
	}

	handshakeService := handshake.New(s)
	s.handshakeService = handshakeService

	// Construct protocols.

	id := protocol.ID(p2p.NewSwarmStreamName(handshake.ProtocolName, handshake.StreamName, handshake.StreamVersion))
	matcher, err := helpers.MultistreamSemverMatcher(id)
	if err != nil {
		return nil, fmt.Errorf("match semver %s: %w", id, err)
	}

	s.host.SetStreamHandlerMatch(id, matcher, func(stream network.Stream) {
		s.metrics.HandledStreamCount.Inc()
		handshakeService.Handler(stream, stream.Conn().RemotePeer())
	})

	// TODO: be more resilient on connection errors and connect in parallel
	for _, a := range o.Bootnodes {
		addr, err := ma.NewMultiaddr(a)
		if err != nil {
			return nil, fmt.Errorf("bootnode %s: %w", a, err)
		}

		err = s.Connect(ctx, addr)
		if err != nil {
			return nil, fmt.Errorf("connect to bootnode %s: %w", a, err)
		}
	}

	h.Network().SetConnHandler(func(_ network.Conn) {
		s.metrics.HandledConnectionCount.Inc()
	})

	return s, nil
}

func (s *Service) AddProtocol(p p2p.ProtocolSpec) (err error) {
	for _, ss := range p.StreamSpecs {
		id := protocol.ID(p2p.NewSwarmStreamName(p.Name, ss.Name, ss.Version))
		matcher, err := helpers.MultistreamSemverMatcher(id)
		if err != nil {
			return fmt.Errorf("match semver %s: %w", id, err)
		}

		s.host.SetStreamHandlerMatch(id, matcher, func(stream network.Stream) {
			 overlay, ok := s.addresses[stream.Conn().RemotePeer()]
			 if !ok {
			 	// todo: handle better
				fmt.Printf("Could not fetch overlay for peerID %s\n", stream)
				return
			}

			peer, ok := s.peers[overlay]
			if !ok {
				fmt.Printf("Could not fetch peer for overlay %s\n", overlay)
				return
			}

			s.metrics.HandledStreamCount.Inc()
			ss.Handler(peer,stream)
		})
	}
	return nil
}

func (s *Service) Addresses() (addrs []string, err error) {
	// Build host multiaddress
	hostAddr, err := ma.NewMultiaddr(fmt.Sprintf("/p2p/%s", s.host.ID().Pretty()))
	if err != nil {
		return nil, err
	}

	// Now we can build a full multiaddress to reach this host
	// by encapsulating both addresses:
	for _, addr := range s.host.Addrs() {
		addrs = append(addrs, addr.Encapsulate(hostAddr).String())
	}
	return addrs, nil
}

func (s *Service) Connect(ctx context.Context, addr ma.Multiaddr) (err error) {
	// Extract the peer ID from the multiaddr.
	info, err := libp2ppeer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return  err
	}

	if err := s.host.Connect(ctx, *info); err != nil {
		return  err
	}

	overlay, err := s.handshakeService.Overlay(ctx, info.ID)
	if err != nil {
		return  err
	}

	s.InitPeer(overlay, info.ID)
	s.metrics.CreatedConnectionCount.Inc()
	fmt.Println("overlay handshake finished")
	return nil
}

func (s *Service) NewStreamForPeerID(ctx context.Context, peerID libp2ppeer.ID, protocolName, streamName, version string) (p2p.Stream, error) {
	swarmStreamName := p2p.NewSwarmStreamName(protocolName, streamName, version)
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


func (s *Service) NewStream(ctx context.Context, overlay, protocolName, streamName, version string) (p2p.Stream, error) {
	peer, ok := s.peers[overlay]
	if !ok {
		fmt.Printf("Could not fetch peer for overlay %s\n", overlay)
		return nil, nil
	}

	return s.NewStreamForPeerID(ctx, peer.peerID, protocolName, streamName, version)
}

func (s *Service) InitPeer(overlay string, peerID libp2ppeer.ID) {
	s.peers[overlay] = &Peer{
		overlay: overlay,
		peerID:peerID,
	}

	s.addresses[peerID] = overlay
}

func (s *Service) Overlay() string {
	return s.overlay
}

func (s *Service) Close() error {
	return s.host.Close()
}
