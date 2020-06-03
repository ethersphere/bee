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
	"github.com/ethersphere/bee/pkg/bzz"
	beecrypto "github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/breaker"
	handshake "github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/tracing"
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

var (
	_ p2p.Service = (*Service)(nil)
)

type Service struct {
	ctx              context.Context
	host             host.Host
	libp2pPeerstore  peerstore.Peerstore
	metrics          metrics
	networkID        uint64
	handshakeService *handshake.Service
	addressbook      addressbook.Putter
	peers            *peerRegistry
	topologyNotifier topology.Notifier
	conectionBreaker breaker.Interface
	logger           logging.Logger
	tracer           *tracing.Tracer
}

type Options struct {
	PrivateKey  *ecdsa.PrivateKey
	DisableWS   bool
	DisableQUIC bool
	Addressbook addressbook.Putter
	Logger      logging.Logger
	Tracer      *tracing.Tracer
}

func New(ctx context.Context, signer beecrypto.Signer, networkID uint64, overlay swarm.Address, addr string,
	o Options) (*Service, error) {
	host, port, err := net.SplitHostPort(addr)
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

	// todo: handle different underlays
	underlay, err := buildUnderlayAddress(h.Addrs()[1], h.ID())
	if err != nil {
		return nil, fmt.Errorf("build host multiaddress: %w", err)
	}

	handshakeService, err := handshake.New(overlay, underlay, signer, networkID, o.Logger)
	if err != nil {
		return nil, fmt.Errorf("handshake service: %w", err)
	}

	peerRegistry := newPeerRegistry()
	s := &Service{
		ctx:              ctx,
		host:             h,
		libp2pPeerstore:  libp2pPeerstore,
		metrics:          newMetrics(),
		networkID:        networkID,
		handshakeService: handshakeService,
		peers:            peerRegistry,
		addressbook:      o.Addressbook,
		logger:           o.Logger,
		tracer:           o.Tracer,
		conectionBreaker: breaker.NewBreaker(breaker.Options{}), // todo: fill non-default options
	}

	// Construct protocols.
	id := protocol.ID(p2p.NewSwarmStreamName(handshake.ProtocolName, handshake.ProtocolVersion, handshake.StreamName))
	matcher, err := s.protocolSemverMatcher(id)
	if err != nil {
		return nil, fmt.Errorf("protocol version match %s: %w", id, err)
	}

	// handshake
	s.host.SetStreamHandlerMatch(id, matcher, func(stream network.Stream) {
		peerID := stream.Conn().RemotePeer()
		i, err := s.handshakeService.Handle(NewStream(stream), peerID)
		if err != nil {
			s.logger.Debugf("handshake: handle %s: %v", peerID, err)
			s.logger.Errorf("unable to handshake with peer %v", peerID)
			_ = s.disconnect(peerID)
			return
		}

		if exists := s.peers.addIfNotExists(stream.Conn(), i.BzzAddress.Overlay); exists {
			_ = stream.Close()
			return
		}

		_ = stream.Close()

		err = s.addressbook.Put(i.BzzAddress.Overlay, *i.BzzAddress)
		if err != nil {
			s.logger.Debugf("handshake: addressbook put error %s: %v", peerID, err)
			s.logger.Errorf("unable to persist peer %v", peerID)
			_ = s.disconnect(peerID)
			return
		}

		if s.topologyNotifier != nil {
			if err := s.topologyNotifier.Connected(ctx, i.BzzAddress.Overlay); err != nil {
				s.logger.Debugf("topology notifier: %s: %v", peerID, err)
			}
		}

		s.metrics.HandledStreamCount.Inc()
		s.logger.Infof("peer %s connected", i.BzzAddress.Overlay)
	})

	h.Network().SetConnHandler(func(_ network.Conn) {
		s.metrics.HandledConnectionCount.Inc()
	})

	h.Network().Notify(peerRegistry)       // update peer registry on network events
	h.Network().Notify(s.handshakeService) // update handshake service on network events
	return s, nil
}

func (s *Service) AddProtocol(p p2p.ProtocolSpec) (err error) {
	for _, ss := range p.StreamSpecs {
		id := protocol.ID(p2p.NewSwarmStreamName(p.Name, p.Version, ss.Name))
		matcher, err := s.protocolSemverMatcher(id)
		if err != nil {
			return fmt.Errorf("protocol version match %s: %w", id, err)
		}

		s.host.SetStreamHandlerMatch(id, matcher, func(streamlibp2p network.Stream) {
			peerID := streamlibp2p.Conn().RemotePeer()
			overlay, found := s.peers.overlay(peerID)
			if !found {
				// todo: this should never happen
				_ = s.disconnect(peerID)
				s.logger.Errorf("overlay address for peer %q not found", peerID)
				return
			}

			stream := newStream(streamlibp2p)

			// exchange headers
			if err := handleHeaders(ss.Headler, stream); err != nil {
				s.logger.Debugf("handle protocol %s/%s: stream %s: peer %s: handle headers: %v", p.Name, p.Version, ss.Name, overlay, err)
				return
			}

			// tracing: get span tracing context and add it to the context
			// silently ignore if the peer is not providing tracing
			ctx, err := s.tracer.WithContextFromHeaders(s.ctx, stream.Headers())
			if err != nil && !errors.Is(err, tracing.ErrContextNotFound) {
				s.logger.Debugf("handle protocol %s/%s: stream %s: peer %s: get tracing context: %v", p.Name, p.Version, ss.Name, overlay, err)
				return
			}

			logger := tracing.NewLoggerWithTraceID(ctx, s.logger)

			logger.Tracef("handle protocol %s/%s: stream %s: peer %s", p.Name, p.Version, ss.Name, overlay)

			s.metrics.HandledStreamCount.Inc()
			if err := ss.Handler(ctx, p2p.Peer{Address: overlay}, stream); err != nil {
				var e *p2p.DisconnectError
				if errors.As(err, &e) {
					_ = s.Disconnect(overlay)
				}

				logger.Debugf("handle protocol %s/%s: stream %s: peer %s: %v", p.Name, p.Version, ss.Name, overlay, err)
				return
			}
		})
	}
	return nil
}

func (s *Service) Addresses() (addreses []ma.Multiaddr, err error) {
	for _, addr := range s.host.Addrs() {
		a, err := buildUnderlayAddress(addr, s.host.ID())
		if err != nil {
			return nil, err
		}

		addreses = append(addreses, a)
	}

	return addreses, nil
}

func buildUnderlayAddress(addr ma.Multiaddr, peerID libp2ppeer.ID) (ma.Multiaddr, error) {
	// Build host multiaddress
	hostAddr, err := ma.NewMultiaddr(fmt.Sprintf("/p2p/%s", peerID.Pretty()))
	if err != nil {
		return nil, err
	}

	return addr.Encapsulate(hostAddr), nil
}

func (s *Service) Connect(ctx context.Context, addr ma.Multiaddr) (address *bzz.Address, err error) {
	// Extract the peer ID from the multiaddr.
	info, err := libp2ppeer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return nil, err
	}

	if _, found := s.peers.overlay(info.ID); found {
		return nil, p2p.ErrAlreadyConnected
	}

	if err := s.conectionBreaker.Execute(func() error { return s.host.Connect(ctx, *info) }); err != nil {
		if errors.Is(err, breaker.ErrClosed) {
			return nil, p2p.NewConnectionBackoffError(err, s.conectionBreaker.ClosedUntil())
		}
		return nil, err
	}

	stream, err := s.newStreamForPeerID(ctx, info.ID, handshake.ProtocolName, handshake.ProtocolVersion, handshake.StreamName)
	if err != nil {
		_ = s.disconnect(info.ID)
		return nil, err
	}

	i, err := s.handshakeService.Handshake(NewStream(stream))
	if err != nil {
		_ = s.disconnect(info.ID)
		return nil, fmt.Errorf("handshake: %w", err)
	}

	if exists := s.peers.addIfNotExists(stream.Conn(), i.BzzAddress.Overlay); exists {
		if err := helpers.FullClose(stream); err != nil {
			return nil, err
		}

		return i.BzzAddress, nil
	}

	if err := helpers.FullClose(stream); err != nil {
		return nil, err
	}

	s.metrics.CreatedConnectionCount.Inc()
	s.logger.Infof("peer %s connected", i.BzzAddress.Overlay)
	return i.BzzAddress, nil
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

func (s *Service) SetNotifier(n topology.Notifier) {
	s.topologyNotifier = n
	s.peers.setDisconnecter(n)
}

func (s *Service) NewStream(ctx context.Context, overlay swarm.Address, headers p2p.Headers, protocolName, protocolVersion, streamName string) (p2p.Stream, error) {
	peerID, found := s.peers.peerID(overlay)
	if !found {
		return nil, p2p.ErrPeerNotFound
	}

	streamlibp2p, err := s.newStreamForPeerID(ctx, peerID, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, err
	}

	stream := newStream(streamlibp2p)

	// tracing: add span context header
	if headers == nil {
		headers = make(p2p.Headers)
	}
	if err := s.tracer.AddContextHeader(ctx, headers); err != nil && !errors.Is(err, tracing.ErrContextNotFound) {
		return nil, err
	}

	// exchange headers
	if err := sendHeaders(ctx, headers, stream); err != nil {
		return nil, fmt.Errorf("send headers: %w", err)
	}

	return stream, nil
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
