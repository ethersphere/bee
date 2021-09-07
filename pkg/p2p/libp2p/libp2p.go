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
	"runtime"
	"sync"
	"time"

	"github.com/ethersphere/bee"
	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/bzz"
	beecrypto "github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/blocklist"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/breaker"
	handshake "github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/libp2p/go-libp2p"
	autonat "github.com/libp2p/go-libp2p-autonat"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	protocol "github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	libp2pquic "github.com/libp2p/go-libp2p-quic-transport"
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	libp2pping "github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"github.com/libp2p/go-tcp-transport"
	ws "github.com/libp2p/go-ws-transport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multistream"
)

var (
	_ p2p.Service      = (*Service)(nil)
	_ p2p.DebugService = (*Service)(nil)
)

const defaultLightNodeLimit = 100

type Service struct {
	ctx               context.Context
	host              host.Host
	natManager        basichost.NATManager
	natAddrResolver   *staticAddressResolver
	autonatDialer     host.Host
	pingDialer        host.Host
	libp2pPeerstore   peerstore.Peerstore
	metrics           metrics
	networkID         uint64
	handshakeService  *handshake.Service
	addressbook       addressbook.Putter
	peers             *peerRegistry
	connectionBreaker breaker.Interface
	blocklist         *blocklist.Blocklist
	protocols         []p2p.ProtocolSpec
	notifier          p2p.PickyNotifier
	logger            logging.Logger
	tracer            *tracing.Tracer
	ready             chan struct{}
	halt              chan struct{}
	lightNodes        lightnodes
	lightNodeLimit    int
	protocolsmu       sync.RWMutex
}

type lightnodes interface {
	Connected(context.Context, p2p.Peer)
	Disconnected(p2p.Peer)
	Count() int
	RandomPeer(swarm.Address) (swarm.Address, error)
	EachPeer(pf topology.EachPeerFunc) error
}

type Options struct {
	PrivateKey     *ecdsa.PrivateKey
	NATAddr        string
	EnableWS       bool
	EnableQUIC     bool
	FullNode       bool
	LightNodeLimit int
	WelcomeMessage string
	Transaction    []byte
	hostFactory    func(context.Context, ...libp2p.Option) (host.Host, error)
}

func New(ctx context.Context, signer beecrypto.Signer, networkID uint64, overlay swarm.Address, addr string, ab addressbook.Putter, storer storage.StateStorer, lightNodes *lightnode.Container, swapBackend handshake.SenderMatcher, logger logging.Logger, tracer *tracing.Tracer, o Options) (*Service, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("address: %w", err)
	}

	ip4Addr := "0.0.0.0"
	ip6Addr := "::"

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
		libp2p.UserAgent(userAgent()),
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
		libp2p.Transport(func(u *tptu.Upgrader) *tcp.TcpTransport {
			t := tcp.NewTCPTransport(u)
			t.DisableReuseport = true
			return t
		}),
	}

	if o.EnableWS {
		transports = append(transports, libp2p.Transport(ws.New))
	}

	if o.EnableQUIC {
		transports = append(transports, libp2p.Transport(libp2pquic.NewTransport))
	}

	opts = append(opts, transports...)

	if o.hostFactory == nil {
		// Use the default libp2p host creation
		o.hostFactory = libp2p.New
	}

	h, err := o.hostFactory(ctx, opts...)
	if err != nil {
		return nil, err
	}

	// Support same non default security and transport options as
	// original host.
	dialer, err := o.hostFactory(ctx, append(transports, security)...)
	if err != nil {
		return nil, err
	}

	// If you want to help other peers to figure out if they are behind
	// NATs, you can launch the server-side of AutoNAT too (AutoRelay
	// already runs the client)
	if _, err = autonat.New(ctx, h, autonat.EnableService(dialer.Network())); err != nil {
		return nil, fmt.Errorf("autonat: %w", err)
	}

	var advertisableAddresser handshake.AdvertisableAddressResolver
	var natAddrResolver *staticAddressResolver
	if o.NATAddr == "" {
		advertisableAddresser = &UpnpAddressResolver{
			host: h,
		}
	} else {
		natAddrResolver, err = newStaticAddressResolver(o.NATAddr, net.LookupIP)
		if err != nil {
			return nil, fmt.Errorf("static nat: %w", err)
		}
		advertisableAddresser = natAddrResolver
	}

	handshakeService, err := handshake.New(signer, advertisableAddresser, swapBackend, overlay, networkID, o.FullNode, o.Transaction, o.WelcomeMessage, h.ID(), logger)
	if err != nil {
		return nil, fmt.Errorf("handshake service: %w", err)
	}

	// Create a new dialer for libp2p ping protocol. This ensures that the protocol
	// uses a different set of keys to do ping. It prevents inconsistencies in peerstore as
	// the addresses used are not dialable and hence should be cleaned up. We should create
	// this host with the same transports and security options to be able to dial to other
	// peers.
	pingDialer, err := o.hostFactory(ctx, append(transports, security, libp2p.NoListenAddrs)...)
	if err != nil {
		return nil, err
	}

	peerRegistry := newPeerRegistry()
	s := &Service{
		ctx:               ctx,
		host:              h,
		natManager:        natManager,
		natAddrResolver:   natAddrResolver,
		autonatDialer:     dialer,
		pingDialer:        pingDialer,
		handshakeService:  handshakeService,
		libp2pPeerstore:   libp2pPeerstore,
		metrics:           newMetrics(),
		networkID:         networkID,
		peers:             peerRegistry,
		addressbook:       ab,
		blocklist:         blocklist.NewBlocklist(storer),
		logger:            logger,
		tracer:            tracer,
		connectionBreaker: breaker.NewBreaker(breaker.Options{}), // use default options
		ready:             make(chan struct{}),
		halt:              make(chan struct{}),
		lightNodes:        lightNodes,
	}

	peerRegistry.setDisconnecter(s)

	s.lightNodeLimit = defaultLightNodeLimit
	if o.LightNodeLimit > 0 {
		s.lightNodeLimit = o.LightNodeLimit
	}

	// Construct protocols.
	id := protocol.ID(p2p.NewSwarmStreamName(handshake.ProtocolName, handshake.ProtocolVersion, handshake.StreamName))
	matcher, err := s.protocolSemverMatcher(id)
	if err != nil {
		return nil, fmt.Errorf("protocol version match %s: %w", id, err)
	}

	s.host.SetStreamHandlerMatch(id, matcher, s.handleIncoming)

	h.Network().SetConnHandler(func(_ network.Conn) {
		s.metrics.HandledConnectionCount.Inc()
	})

	h.Network().Notify(peerRegistry)       // update peer registry on network events
	h.Network().Notify(s.handshakeService) // update handshake service on network events
	return s, nil
}

func (s *Service) handleIncoming(stream network.Stream) {
	select {
	case <-s.ready:
	case <-s.halt:
		go func() { _ = stream.Reset() }()
		return
	case <-s.ctx.Done():
		go func() { _ = stream.Reset() }()
		return
	}

	peerID := stream.Conn().RemotePeer()
	handshakeStream := NewStream(stream)
	i, err := s.handshakeService.Handle(s.ctx, handshakeStream, stream.Conn().RemoteMultiaddr(), peerID)
	if err != nil {
		s.logger.Debugf("stream handler: handshake: handle %s: %v", peerID, err)
		s.logger.Errorf("stream handler: handshake: unable to handshake with peer id %v", peerID)
		_ = handshakeStream.Reset()
		_ = s.host.Network().ClosePeer(peerID)
		return
	}

	overlay := i.BzzAddress.Overlay

	blocked, err := s.blocklist.Exists(overlay)
	if err != nil {
		s.logger.Debugf("stream handler: blocklisting: exists %s: %v", overlay, err)
		s.logger.Errorf("stream handler: internal error while connecting with peer %s", overlay)
		_ = handshakeStream.Reset()
		_ = s.host.Network().ClosePeer(peerID)
		return
	}

	if blocked {
		s.logger.Errorf("stream handler: blocked connection from blocklisted peer %s", overlay)
		_ = handshakeStream.Reset()
		_ = s.host.Network().ClosePeer(peerID)
		return
	}

	if s.notifier != nil {
		if !s.notifier.Pick(p2p.Peer{Address: overlay, FullNode: i.FullNode}) {
			s.logger.Warningf("stream handler: don't want incoming peer %s. disconnecting", overlay)
			_ = handshakeStream.Reset()
			_ = s.host.Network().ClosePeer(peerID)
			return
		}
	}

	if exists := s.peers.addIfNotExists(stream.Conn(), overlay, i.FullNode); exists {
		s.logger.Debugf("stream handler: peer %s already exists", overlay)
		if err = handshakeStream.FullClose(); err != nil {
			s.logger.Debugf("stream handler: could not close stream %s: %v", overlay, err)
			s.logger.Errorf("stream handler: unable to handshake with peer %v", overlay)
			_ = s.Disconnect(overlay, "unable to close handshake stream")
		}
		return
	}

	if err = handshakeStream.FullClose(); err != nil {
		s.logger.Debugf("stream handler: could not close stream %s: %v", overlay, err)
		s.logger.Errorf("stream handler: unable to handshake with peer %v", overlay)
		_ = s.Disconnect(overlay, "could not fully close stream on handshake")
		return
	}

	if i.FullNode {
		err = s.addressbook.Put(i.BzzAddress.Overlay, *i.BzzAddress)
		if err != nil {
			s.logger.Debugf("stream handler: addressbook put error %s: %v", peerID, err)
			s.logger.Errorf("stream handler: unable to persist peer %v", peerID)
			_ = s.Disconnect(i.BzzAddress.Overlay, "unable to persist peer in addressbook")
			return
		}
	}

	peer := p2p.Peer{Address: overlay, FullNode: i.FullNode, EthereumAddress: i.BzzAddress.EthereumAddress}

	s.protocolsmu.RLock()
	for _, tn := range s.protocols {
		if tn.ConnectIn != nil {
			if err := tn.ConnectIn(s.ctx, peer); err != nil {
				s.logger.Debugf("stream handler: connectIn: protocol: %s, version:%s, peer: %s: %v", tn.Name, tn.Version, overlay, err)
				_ = s.Disconnect(overlay, "failed to process inbound connection notifier")
				s.protocolsmu.RUnlock()
				return
			}
		}
	}
	s.protocolsmu.RUnlock()

	if s.notifier != nil {
		if !i.FullNode {
			s.lightNodes.Connected(s.ctx, peer)
			// light node announces explicitly
			if err := s.notifier.Announce(s.ctx, peer.Address, i.FullNode); err != nil {
				s.logger.Debugf("stream handler: notifier.Announce: %s: %v", peer.Address.String(), err)
			}

			if s.lightNodes.Count() > s.lightNodeLimit {
				// kick another node to fit this one in
				p, err := s.lightNodes.RandomPeer(peer.Address)
				if err != nil {
					s.logger.Debugf("stream handler: cant find a peer slot for light node: %v", err)
					_ = s.Disconnect(peer.Address, "unable to find peer slot for light node")
					return
				} else {
					s.logger.Tracef("stream handler: kicking away light node %s to make room for %s", p.String(), peer.Address.String())
					s.metrics.KickedOutPeersCount.Inc()
					_ = s.Disconnect(p, "kicking away light node to make room for peer")
					return
				}
			}
		} else {
			if err := s.notifier.Connected(s.ctx, peer, false); err != nil {
				s.logger.Debugf("stream handler: notifier.Connected: peer disconnected: %s: %v", i.BzzAddress.Overlay, err)
				// note: this cannot be unit tested since the node
				// waiting on handshakeStream.FullClose() on the other side
				// might actually get a stream reset when we disconnect here
				// resulting in a flaky response from the Connect method on
				// the other side.
				// that is why the Pick method has been added to the notifier
				// interface, in addition to the possibility of deciding whether
				// a peer connection is wanted prior to adding the peer to the
				// peer registry and starting the protocols.
				_ = s.Disconnect(overlay, "unable to signal connection notifier")
				return
			}
			// when a full node connects, we gossip about it to the
			// light nodes so that they can also have a chance at building
			// a solid topology.
			_ = s.lightNodes.EachPeer(func(addr swarm.Address, _ uint8) (bool, bool, error) {
				go func(addressee, peer swarm.Address, fullnode bool) {
					if err := s.notifier.AnnounceTo(s.ctx, addressee, peer, fullnode); err != nil {
						s.logger.Debugf("stream handler: notifier.Announce to light node %s %s: %v", addressee.String(), peer.String(), err)
					}
				}(addr, peer.Address, i.FullNode)
				return false, false, nil
			})
		}
	}

	s.metrics.HandledStreamCount.Inc()
	if !s.peers.Exists(overlay) {
		s.logger.Warningf("stream handler: inbound peer %s does not exist, disconnecting", overlay)
		_ = s.Disconnect(overlay, "unknown inbound peer")
		return
	}

	peerUserAgent, err := s.peerUserAgent(peerID)
	if err != nil {
		s.logger.Debugf("stream handler: inbound peer %s user agent: %w", err)
	}

	s.logger.Debugf("stream handler: successfully connected to peer %s%s%s (inbound)", i.BzzAddress.ShortString(), i.LightString(), appendSpace(peerUserAgent))
	s.logger.Infof("stream handler: successfully connected to peer %s%s%s (inbound)", i.BzzAddress.Overlay, i.LightString(), appendSpace(peerUserAgent))
}

func (s *Service) SetPickyNotifier(n p2p.PickyNotifier) {
	s.notifier = n
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
				_ = streamlibp2p.Reset()
				s.logger.Debugf("overlay address for peer %q not found", peerID)
				return
			}
			full, found := s.peers.fullnode(peerID)
			if !found {
				_ = streamlibp2p.Reset()
				s.logger.Debugf("fullnode info for peer %q not found", peerID)
				return
			}

			stream := newStream(streamlibp2p)

			// exchange headers
			if err := handleHeaders(ss.Headler, stream, overlay); err != nil {
				s.logger.Debugf("handle protocol %s/%s: stream %s: peer %s: handle headers: %v", p.Name, p.Version, ss.Name, overlay, err)
				_ = stream.Reset()
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
				_ = stream.Reset()
				return
			}

			logger := tracing.NewLoggerWithTraceID(ctx, s.logger)

			s.metrics.HandledStreamCount.Inc()
			if err := ss.Handler(ctx, p2p.Peer{Address: overlay, FullNode: full}, stream); err != nil {
				var de *p2p.DisconnectError
				if errors.As(err, &de) {
					logger.Tracef("libp2p handler(%s): disconnecting %s", p.Name, overlay.String())
					_ = stream.Reset()
					_ = s.Disconnect(overlay, de.Error())
					logger.Tracef("handler(%s): disconnecting %s due to disconnect error", p.Name, overlay.String())
				}

				var bpe *p2p.BlockPeerError
				if errors.As(err, &bpe) {
					_ = stream.Reset()
					if err := s.Blocklist(overlay, bpe.Duration(), bpe.Error()); err != nil {
						logger.Debugf("blocklist: could not blocklist peer %s: %v", peerID, err)
						logger.Errorf("unable to blocklist peer %v", peerID)
					}
					logger.Tracef("handler(%s): blocklisted %s", p.Name, overlay.String())
				}
				// count unexpected requests
				if errors.Is(err, p2p.ErrUnexpected) {
					s.metrics.UnexpectedProtocolReqCount.Inc()
				}
				logger.Debugf("could not handle protocol %s/%s: stream %s: peer %s: error: %v", p.Name, p.Version, ss.Name, overlay, err)
				return
			}
		})
	}

	s.protocolsmu.Lock()
	s.protocols = append(s.protocols, p)
	s.protocolsmu.Unlock()
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
	if s.natAddrResolver != nil && len(addreses) > 0 {
		a, err := s.natAddrResolver.Resolve(addreses[0])
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

func (s *Service) Blocklist(overlay swarm.Address, duration time.Duration, reason string) error {
	s.logger.Tracef("libp2p blocklist: peer %s for %v reason: %s", overlay.String(), duration, reason)
	if err := s.blocklist.Add(overlay, duration); err != nil {
		s.metrics.BlocklistedPeerErrCount.Inc()
		_ = s.Disconnect(overlay, "failed blocklisting peer")
		return fmt.Errorf("blocklist peer %s: %v", overlay, err)
	}
	s.metrics.BlocklistedPeerCount.Inc()

	_ = s.Disconnect(overlay, "blocklisting peer")
	return nil
}

func buildHostAddress(peerID libp2ppeer.ID) (ma.Multiaddr, error) {
	return ma.NewMultiaddr(fmt.Sprintf("/p2p/%s", peerID.Pretty()))
}

func buildUnderlayAddress(addr ma.Multiaddr, peerID libp2ppeer.ID) (ma.Multiaddr, error) {
	// Build host multiaddress
	hostAddr, err := buildHostAddress(peerID)
	if err != nil {
		return nil, err
	}

	return addr.Encapsulate(hostAddr), nil
}

func (s *Service) Connect(ctx context.Context, addr ma.Multiaddr) (address *bzz.Address, err error) {
	// Extract the peer ID from the multiaddr.
	info, err := libp2ppeer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return nil, fmt.Errorf("addr from p2p: %w", err)
	}

	hostAddr, err := buildHostAddress(info.ID)
	if err != nil {
		return nil, fmt.Errorf("build host address: %w", err)
	}

	remoteAddr := addr.Decapsulate(hostAddr)

	if overlay, found := s.peers.isConnected(info.ID, remoteAddr); found {
		address = &bzz.Address{
			Overlay:  overlay,
			Underlay: addr,
		}
		return address, p2p.ErrAlreadyConnected
	}

	if err := s.connectionBreaker.Execute(func() error { return s.host.Connect(ctx, *info) }); err != nil {
		if errors.Is(err, breaker.ErrClosed) {
			s.metrics.ConnectBreakerCount.Inc()
			return nil, p2p.NewConnectionBackoffError(err, s.connectionBreaker.ClosedUntil())
		}
		return nil, err
	}

	stream, err := s.newStreamForPeerID(ctx, info.ID, handshake.ProtocolName, handshake.ProtocolVersion, handshake.StreamName)
	if err != nil {
		_ = s.host.Network().ClosePeer(info.ID)
		return nil, fmt.Errorf("connect new stream: %w", err)
	}

	handshakeStream := NewStream(stream)
	i, err := s.handshakeService.Handshake(ctx, handshakeStream, stream.Conn().RemoteMultiaddr(), stream.Conn().RemotePeer())
	if err != nil {
		_ = handshakeStream.Reset()
		_ = s.host.Network().ClosePeer(info.ID)
		return nil, fmt.Errorf("handshake: %w", err)
	}

	if !i.FullNode {
		_ = handshakeStream.Reset()
		_ = s.host.Network().ClosePeer(info.ID)
		return nil, p2p.ErrDialLightNode
	}

	overlay := i.BzzAddress.Overlay

	blocked, err := s.blocklist.Exists(overlay)
	if err != nil {
		s.logger.Debugf("blocklisting: exists %s: %v", info.ID, err)
		s.logger.Errorf("internal error while connecting with peer %s", info.ID)
		_ = handshakeStream.Reset()
		_ = s.host.Network().ClosePeer(info.ID)
		return nil, fmt.Errorf("peer blocklisted")
	}

	if blocked {
		s.logger.Errorf("blocked connection to blocklisted peer %s", info.ID)
		_ = handshakeStream.Reset()
		_ = s.host.Network().ClosePeer(info.ID)
		return nil, fmt.Errorf("peer blocklisted")
	}

	if exists := s.peers.addIfNotExists(stream.Conn(), overlay, i.FullNode); exists {
		if err := handshakeStream.FullClose(); err != nil {
			_ = s.Disconnect(overlay, "failed closing handshake stream after connect")
			return nil, fmt.Errorf("peer exists, full close: %w", err)
		}

		return i.BzzAddress, nil
	}

	if err := handshakeStream.FullClose(); err != nil {
		_ = s.Disconnect(overlay, "could not fully close handshake stream after connect")
		return nil, fmt.Errorf("connect full close %w", err)
	}

	if i.FullNode {
		err = s.addressbook.Put(overlay, *i.BzzAddress)
		if err != nil {
			_ = s.Disconnect(overlay, "failed storing peer in addressbook")
			return nil, fmt.Errorf("storing bzz address: %w", err)
		}
	}

	s.protocolsmu.RLock()
	for _, tn := range s.protocols {
		if tn.ConnectOut != nil {
			if err := tn.ConnectOut(ctx, p2p.Peer{Address: overlay, FullNode: i.FullNode, EthereumAddress: i.BzzAddress.EthereumAddress}); err != nil {
				s.logger.Debugf("connectOut: protocol: %s, version:%s, peer: %s: %v", tn.Name, tn.Version, overlay, err)
				_ = s.Disconnect(overlay, "failed to process outbound connection notifier")
				s.protocolsmu.RUnlock()
				return nil, fmt.Errorf("connectOut: protocol: %s, version:%s: %w", tn.Name, tn.Version, err)
			}
		}
	}
	s.protocolsmu.RUnlock()

	if !s.peers.Exists(overlay) {
		_ = s.Disconnect(overlay, "outbound peer does not exist")
		return nil, fmt.Errorf("libp2p connect: peer %s does not exist %w", overlay, p2p.ErrPeerNotFound)
	}

	s.metrics.CreatedConnectionCount.Inc()

	peerUserAgent, err := s.peerUserAgent(info.ID)
	if err != nil {
		return nil, fmt.Errorf("peer user agent: %w", err)
	}

	s.logger.Debugf("successfully connected to peer %s%s%s (outbound)", i.BzzAddress.ShortString(), i.LightString(), appendSpace(peerUserAgent))
	s.logger.Infof("successfully connected to peer %s%s%s (outbound)", overlay, i.LightString(), appendSpace(peerUserAgent))
	return i.BzzAddress, nil
}

func (s *Service) Disconnect(overlay swarm.Address, reason string) error {
	s.metrics.DisconnectCount.Inc()

	s.logger.Debugf("libp2p disconnect: disconnecting peer %s reason: %s", overlay, reason)

	// found is checked at the bottom of the function
	found, full, peerID := s.peers.remove(overlay)

	_ = s.host.Network().ClosePeer(peerID)

	peer := p2p.Peer{Address: overlay, FullNode: full}

	s.protocolsmu.RLock()
	for _, tn := range s.protocols {
		if tn.DisconnectOut != nil {
			if err := tn.DisconnectOut(peer); err != nil {
				s.logger.Debugf("disconnectOut: protocol: %s, version:%s, peer: %s: %v", tn.Name, tn.Version, overlay, err)
			}
		}
	}
	s.protocolsmu.RUnlock()

	if s.notifier != nil {
		s.notifier.Disconnected(peer)
	}
	if s.lightNodes != nil {
		s.lightNodes.Disconnected(peer)
	}

	if !found {
		s.logger.Debugf("libp2p disconnect: peer %s not found", overlay)
		return p2p.ErrPeerNotFound
	}

	return nil
}

// disconnected is a registered peer registry event
func (s *Service) disconnected(address swarm.Address) {
	peer := p2p.Peer{Address: address}
	peerID, found := s.peers.peerID(address)
	if found {
		// peerID might not always be found on shutdown
		full, found := s.peers.fullnode(peerID)
		if found {
			peer.FullNode = full
		}
	}
	s.protocolsmu.RLock()
	for _, tn := range s.protocols {
		if tn.DisconnectIn != nil {
			if err := tn.DisconnectIn(peer); err != nil {
				s.logger.Debugf("disconnectIn: protocol: %s, version:%s, peer: %s: %v", tn.Name, tn.Version, address.String(), err)
			}
		}
	}

	s.protocolsmu.RUnlock()

	if s.notifier != nil {
		s.notifier.Disconnected(peer)
	}
	if s.lightNodes != nil {
		s.lightNodes.Disconnected(peer)
	}
}

func (s *Service) Peers() []p2p.Peer {
	return s.peers.peers()
}

func (s *Service) BlocklistedPeers() ([]p2p.Peer, error) {
	return s.blocklist.Peers()
}

func (s *Service) NewStream(ctx context.Context, overlay swarm.Address, headers p2p.Headers, protocolName, protocolVersion, streamName string) (p2p.Stream, error) {
	peerID, found := s.peers.peerID(overlay)
	if !found {
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
		_ = stream.Reset()
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
	if s.natManager != nil {
		if err := s.natManager.Close(); err != nil {
			return err
		}
	}
	if err := s.autonatDialer.Close(); err != nil {
		return err
	}
	if err := s.pingDialer.Close(); err != nil {
		return err
	}

	return s.host.Close()
}

// SetWelcomeMessage sets the welcome message for the handshake protocol.
func (s *Service) SetWelcomeMessage(val string) error {
	return s.handshakeService.SetWelcomeMessage(val)
}

// GetWelcomeMessage returns the value of the welcome message.
func (s *Service) GetWelcomeMessage() string {
	return s.handshakeService.GetWelcomeMessage()
}

func (s *Service) Ready() {
	close(s.ready)
}

func (s *Service) Halt() {
	close(s.halt)
}

func (s *Service) Ping(ctx context.Context, addr ma.Multiaddr) (rtt time.Duration, err error) {
	info, err := libp2ppeer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return rtt, fmt.Errorf("unable to parse underlay address: %w", err)
	}

	// Add the address to libp2p peerstore for it to be dialable
	s.pingDialer.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.TempAddrTTL)

	// Cleanup connection after ping is done
	defer func() {
		_ = s.pingDialer.Network().ClosePeer(info.ID)
	}()

	select {
	case <-ctx.Done():
		return rtt, ctx.Err()
	case res := <-libp2pping.Ping(ctx, s.pingDialer, info.ID):
		return res.RTT, res.Error
	}
}

// peerUserAgent returns User Agent string of the connected peer if the peer
// provides it. It ignores the default libp2p user agent string
// "github.com/libp2p/go-libp2p" and returns empty string in that case.
func (s *Service) peerUserAgent(peerID libp2ppeer.ID) (string, error) {
	v, err := s.host.Peerstore().Get(peerID, "AgentVersion")
	if err != nil {
		// error is ignored as user agent is informative only
		return "", fmt.Errorf("peerstore get AgentVersion: %w", err)
	}
	ua, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("user agent %v is not a string", v)
	}
	// Ignore the default user agent.
	// if ua == "github.com/libp2p/go-libp2p" {
	// 	return ""
	// }
	return ua, nil
}

// appendSpace adds a leading space character if the string is not empty.
// It is useful for constructing log messages with conditional substrings.
func appendSpace(s string) string {
	if s == "" {
		return ""
	}
	return " " + s
}

// userAgent returns a User Agent string passed to the libp2p host to identify peer node.
func userAgent() string {
	return fmt.Sprintf("bee/%s %s %s/%s", bee.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
