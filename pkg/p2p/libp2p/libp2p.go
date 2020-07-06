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
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	protocol "github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	libp2pquic "github.com/libp2p/go-libp2p-quic-transport"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	"github.com/libp2p/go-tcp-transport"
	ws "github.com/libp2p/go-ws-transport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multistream"
)

var (
	_ p2p.Service = (*Service)(nil)
)

type Service struct {
	ctx               context.Context
	host              host.Host
	natManager        basichost.NATManager
	libp2pPeerstore   peerstore.Peerstore
	metrics           metrics
	networkID         uint64
	handshakeService  *handshake.Service
	addressbook       addressbook.Putter
	peers             *peerRegistry
	topologyNotifier  topology.Notifier
	connectionBreaker breaker.Interface
	logger            logging.Logger
	tracer            *tracing.Tracer
}

type Options struct {
	PrivateKey     *ecdsa.PrivateKey
	NATAddr        string
	EnableWS       bool
	EnableQUIC     bool
	LightNode      bool
	WelcomeMessage string
	Addressbook    addressbook.Putter
	Logger         logging.Logger
	Tracer         *tracing.Tracer
}

func New(ctx context.Context, signer beecrypto.Signer, networkID uint64, overlay swarm.Address, addr string, o Options) (*Service, error) {
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
		if o.EnableWS {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip4/%s/tcp/%s/ws", ip4Addr, port))
		}
		if o.EnableQUIC {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip4/%s/udp/%s/quic", ip4Addr, port))
		}
	}

	if ip6Addr != "" {
		listenAddrs = append(listenAddrs, fmt.Sprintf("/ip6/%s/tcp/%s", ip6Addr, port))
		if o.EnableWS {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip6/%s/tcp/%s/ws", ip6Addr, port))
		}
		if o.EnableQUIC {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip6/%s/udp/%s/quic", ip6Addr, port))
		}
	}

	security := libp2p.DefaultSecurity
	libp2pPeerstore := pstoremem.NewPeerstore()

	var natManager basichost.NATManager

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(listenAddrs...),
		security,
		// Use dedicated peerstore instead the global DefaultPeerstore
		libp2p.Peerstore(libp2pPeerstore),
	}

	if o.NATAddr == "" {
		opts = append(opts,
			libp2p.NATManager(func(n network.Network) basichost.NATManager {
				natManager = basichost.NewNATManager(n)
				return natManager
			}),
		)
	}

	if o.PrivateKey != nil {
		opts = append(opts,
			libp2p.Identity((*crypto.Secp256k1PrivateKey)(o.PrivateKey)),
		)
	}

	transports := []libp2p.Option{
		libp2p.Transport(tcp.NewTCPTransport),
	}

	if o.EnableWS {
		transports = append(transports, libp2p.Transport(ws.New))
	}

	if o.EnableQUIC {
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

	var advertisableAddresser handshake.AdvertisableAddressResolver
	if o.NATAddr == "" {
		advertisableAddresser = &UpnpAddressResolver{
			host: h,
		}
	} else {
		advertisableAddresser, err = newStaticAddressResolver(o.NATAddr)
		if err != nil {
			return nil, fmt.Errorf("static nat: %w", err)
		}
	}

	handshakeService, err := handshake.New(signer, advertisableAddresser, overlay, networkID, o.LightNode, o.WelcomeMessage, o.Logger)
	if err != nil {
		return nil, fmt.Errorf("handshake service: %w", err)
	}

	peerRegistry := newPeerRegistry()
	s := &Service{
		ctx:               ctx,
		host:              h,
		natManager:        natManager,
		handshakeService:  handshakeService,
		libp2pPeerstore:   libp2pPeerstore,
		metrics:           newMetrics(),
		networkID:         networkID,
		peers:             peerRegistry,
		addressbook:       o.Addressbook,
		logger:            o.Logger,
		tracer:            o.Tracer,
		connectionBreaker: breaker.NewBreaker(breaker.Options{}), // use default options
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
		handshakeStream := NewStream(stream)
		i, err := s.handshakeService.Handle(handshakeStream, stream.Conn().RemoteMultiaddr(), peerID)
		if err != nil {
			s.logger.Debugf("handshake: handle %s: %v", peerID, err)
			s.logger.Errorf("unable to handshake with peer %v", peerID)
			_ = handshakeStream.Reset()
			_ = s.disconnect(peerID)
			return
		}

		if exists := s.peers.addIfNotExists(stream.Conn(), i.BzzAddress.Overlay); exists {
			if err = handshakeStream.FullClose(); err != nil {
				s.logger.Debugf("handshake: could not close stream %s: %v", peerID, err)
				s.logger.Errorf("unable to handshake with peer %v", peerID)
				_ = s.disconnect(peerID)
			}
			return
		}

		if err = handshakeStream.FullClose(); err != nil {
			s.logger.Debugf("handshake: could not close stream %s: %v", peerID, err)
			s.logger.Errorf("unable to handshake with peer %v", peerID)
			_ = s.disconnect(peerID)
		}

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
		s.logger.Infof("successfully connected to peer (inbound) %s", i.BzzAddress.ShortString())
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
		ss := ss
		id := protocol.ID(p2p.NewSwarmStreamName(p.Name, p.Version, ss.Name))
		matcher, err := s.protocolSemverMatcher(id)
		if err != nil {
			return fmt.Errorf("protocol version match %s: %w", id, err)
		}

		s.host.SetStreamHandlerMatch(id, matcher, func(streamlibp2p network.Stream) {
			peerID := streamlibp2p.Conn().RemotePeer()
			overlay, found := s.peers.overlay(peerID)
			if !found {
				_ = s.disconnect(peerID)
				s.logger.Debugf("overlay address for peer %q not found", peerID)
				return
			}

			stream := newStream(streamlibp2p)

			// exchange headers
			if err := handleHeaders(ss.Headler, stream); err != nil {
				s.logger.Debugf("handle protocol %s/%s: stream %s: peer %s: handle headers: %v", p.Name, p.Version, ss.Name, overlay, err)
				if err := stream.Close(); err != nil {
					s.logger.Debugf("handle protocol %s/%s: stream %s: peer %s: handle headers close stream: %v", p.Name, p.Version, ss.Name, overlay, err)
				}
				return
			}

			ctx, cancel := context.WithCancel(s.ctx)

			s.peers.addStream(peerID, streamlibp2p, cancel)
			defer s.peers.removeStream(peerID, streamlibp2p)

			// tracing: get span tracing context and add it to the context
			// silently ignore if the peer is not providing tracing
			ctx, err := s.tracer.WithContextFromHeaders(ctx, stream.Headers())
			if err != nil && !errors.Is(err, tracing.ErrContextNotFound) {
				s.logger.Debugf("handle protocol %s/%s: stream %s: peer %s: get tracing context: %v", p.Name, p.Version, ss.Name, overlay, err)
				return
			}

			logger := tracing.NewLoggerWithTraceID(ctx, s.logger)

			s.metrics.HandledStreamCount.Inc()
			if err := ss.Handler(ctx, p2p.Peer{Address: overlay}, stream); err != nil {
				var e *p2p.DisconnectError
				if errors.As(err, &e) {
					_ = s.Disconnect(overlay)
				}

				logger.Debugf("error handle protocol %s/%s: stream %s: peer %s: error: %v", p.Name, p.Version, ss.Name, overlay, err)
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

func (s *Service) NATManager() basichost.NATManager {
	return s.natManager
}

func buildUnderlayAddress(addr ma.Multiaddr, peerID libp2ppeer.ID) (ma.Multiaddr, error) {
	// Build host multiaddress
	hostAddr, err := ma.NewMultiaddr(fmt.Sprintf("/p2p/%s", peerID.Pretty()))
	if err != nil {
		return nil, err
	}

	return addr.Encapsulate(hostAddr), nil
}

func (s *Service) ConnectNotify(ctx context.Context, addr ma.Multiaddr) (address *bzz.Address, err error) {
	info, err := libp2ppeer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return nil, fmt.Errorf("addr from p2p: %w", err)
	}

	address, err = s.Connect(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("connect notify: %w", err)
	}
	if s.topologyNotifier != nil {
		if err := s.topologyNotifier.Connected(ctx, address.Overlay); err != nil {
			_ = s.disconnect(info.ID)
			return nil, fmt.Errorf("notify topology: %w", err)
		}
	}
	return address, nil
}

func (s *Service) Connect(ctx context.Context, addr ma.Multiaddr) (address *bzz.Address, err error) {
	// Extract the peer ID from the multiaddr.
	info, err := libp2ppeer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return nil, fmt.Errorf("addr from p2p: %w", err)
	}

	if _, found := s.peers.overlay(info.ID); found {
		return nil, p2p.ErrAlreadyConnected
	}

	if err := s.connectionBreaker.Execute(func() error { return s.host.Connect(ctx, *info) }); err != nil {
		if errors.Is(err, breaker.ErrClosed) {
			return nil, p2p.NewConnectionBackoffError(err, s.connectionBreaker.ClosedUntil())
		}
		return nil, err
	}

	stream, err := s.newStreamForPeerID(ctx, info.ID, handshake.ProtocolName, handshake.ProtocolVersion, handshake.StreamName)
	if err != nil {
		_ = s.disconnect(info.ID)
		return nil, fmt.Errorf("connect new stream: %w", err)
	}

	handshakeStream := NewStream(stream)
	i, err := s.handshakeService.Handshake(handshakeStream, stream.Conn().RemoteMultiaddr(), stream.Conn().RemotePeer())
	if err != nil {
		_ = handshakeStream.Reset()
		_ = s.disconnect(info.ID)
		return nil, fmt.Errorf("handshake: %w", err)
	}

	if exists := s.peers.addIfNotExists(stream.Conn(), i.BzzAddress.Overlay); exists {
		if err := handshakeStream.FullClose(); err != nil {
			_ = s.disconnect(info.ID)
			return nil, fmt.Errorf("peer exists, full close: %w", err)
		}

		return i.BzzAddress, nil
	}

	if err := handshakeStream.FullClose(); err != nil {
		_ = s.disconnect(info.ID)
		return nil, fmt.Errorf("connect full close %w", err)
	}

	err = s.addressbook.Put(i.BzzAddress.Overlay, *i.BzzAddress)
	if err != nil {
		_ = s.disconnect(info.ID)
		return nil, fmt.Errorf("storing bzz address: %w", err)
	}

	s.metrics.CreatedConnectionCount.Inc()
	s.logger.Infof("successfully connected to peer (outbound) %s", i.BzzAddress.ShortString())
	return i.BzzAddress, nil
}

func (s *Service) Disconnect(overlay swarm.Address) error {
	peerID, found := s.peers.peerID(overlay)
	if !found {
		s.peers.disconnecter.Disconnected(overlay)
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
		s.peers.disconnecter.Disconnected(overlay)
		return nil, p2p.ErrPeerNotFound
	}

	streamlibp2p, err := s.newStreamForPeerID(ctx, peerID, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, fmt.Errorf("new stream for peerid: %w", err)
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
		if err := stream.Close(); err != nil {
			s.logger.Debugf("send headers %s: close stream %v", peerID, err)
		}
		return nil, fmt.Errorf("send headers: %w", err)
	}

	return stream, nil
}

func (s *Service) newStreamForPeerID(ctx context.Context, peerID libp2ppeer.ID, protocolName, protocolVersion, streamName string) (network.Stream, error) {
	swarmStreamName := p2p.NewSwarmStreamName(protocolName, protocolVersion, streamName)
	st, err := s.host.NewStream(ctx, peerID, protocol.ID(swarmStreamName))
	if err != nil {
		if st != nil {
			s.logger.Debug("stream experienced unexpected early close")
			_ = st.Close()
		}
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
