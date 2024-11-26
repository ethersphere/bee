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
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	beecrypto "github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/blocklist"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/breaker"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/reacher"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/host/autonat"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	lp2pswarm "github.com/libp2p/go-libp2p/p2p/net/swarm"
	libp2pping "github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ws "github.com/libp2p/go-libp2p/p2p/transport/websocket"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multistream"
	"go.uber.org/atomic"

	ocprom "contrib.go.opencensus.io/exporter/prometheus"
	m2 "github.com/ethersphere/bee/v2/pkg/metrics"
	rcmgrObs "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/prometheus/client_golang/prometheus"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "libp2p"

var (
	_ p2p.Service      = (*Service)(nil)
	_ p2p.DebugService = (*Service)(nil)

	// reachabilityOverridePublic overrides autonat to simply report
	// public reachability status, it is set in the makefile.
	reachabilityOverridePublic = "false"
)

const (
	defaultLightNodeLimit = 100
	peerUserAgentTimeout  = time.Second

	defaultHeadersRWTimeout = 10 * time.Second

	IncomingStreamCountLimit = 5_000
	OutgoingStreamCountLimit = 10_000
)

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
	logger            log.Logger
	tracer            *tracing.Tracer
	ready             chan struct{}
	halt              chan struct{}
	lightNodes        lightnodes
	lightNodeLimit    int
	protocolsmu       sync.RWMutex
	reacher           p2p.Reacher
	networkStatus     atomic.Int32
	HeadersRWTimeout  time.Duration
	autoNAT           autonat.AutoNAT
}

type lightnodes interface {
	Connected(context.Context, p2p.Peer)
	Disconnected(p2p.Peer)
	Count() int
	RandomPeer(swarm.Address) (swarm.Address, error)
	EachPeer(pf topology.EachPeerFunc) error
}

type Options struct {
	PrivateKey       *ecdsa.PrivateKey
	NATAddr          string
	EnableWS         bool
	FullNode         bool
	LightNodeLimit   int
	WelcomeMessage   string
	Nonce            []byte
	ValidateOverlay  bool
	hostFactory      func(...libp2p.Option) (host.Host, error)
	HeadersRWTimeout time.Duration
	Registry         *prometheus.Registry
}

func New(ctx context.Context, signer beecrypto.Signer, networkID uint64, overlay swarm.Address, addr string, ab addressbook.Putter, storer storage.StateStorer, lightNodes *lightnode.Container, logger log.Logger, tracer *tracing.Tracer, o Options) (*Service, error) {
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
	}

	if ip6Addr != "" {
		listenAddrs = append(listenAddrs, fmt.Sprintf("/ip6/%s/tcp/%s", ip6Addr, port))
		if o.EnableWS {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip6/%s/tcp/%s/ws", ip6Addr, port))
		}
	}

	security := libp2p.DefaultSecurity
	libp2pPeerstore, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, err
	}

	if o.Registry != nil {
		rcmgrObs.MustRegisterWith(o.Registry)
	}

	_, err = ocprom.NewExporter(ocprom.Options{
		Namespace: m2.Namespace,
		Registry:  o.Registry,
	})
	if err != nil {
		return nil, err
	}

	// Tweak certain settings
	cfg := rcmgr.PartialLimitConfig{
		System: rcmgr.ResourceLimits{
			Streams:         IncomingStreamCountLimit + OutgoingStreamCountLimit,
			StreamsOutbound: OutgoingStreamCountLimit,
			StreamsInbound:  IncomingStreamCountLimit,
		},
	}

	// Create our limits by using our cfg and replacing the default values with values from `scaledDefaultLimits`
	limits := cfg.Build(rcmgr.InfiniteLimits)

	// The resource manager expects a limiter, se we create one from our limits.
	limiter := rcmgr.NewFixedLimiter(limits)

	str, err := rcmgrObs.NewStatsTraceReporter()
	if err != nil {
		return nil, err
	}

	rm, err := rcmgr.NewResourceManager(limiter, rcmgr.WithTraceReporter(str))
	if err != nil {
		return nil, err
	}

	var natManager basichost.NATManager

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(listenAddrs...),
		security,
		// Use dedicated peerstore instead the global DefaultPeerstore
		libp2p.Peerstore(libp2pPeerstore),
		libp2p.UserAgent(userAgent()),
		libp2p.ResourceManager(rm),
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
		myKey, _, err := crypto.ECDSAKeyPairFromKey(o.PrivateKey)
		if err != nil {
			return nil, err
		}
		opts = append(opts,
			libp2p.Identity(myKey),
		)
	}

	transports := []libp2p.Option{
		libp2p.Transport(tcp.NewTCPTransport, tcp.DisableReuseport()),
	}

	if o.EnableWS {
		transports = append(transports, libp2p.Transport(ws.New))
	}

	opts = append(opts, transports...)

	if o.hostFactory == nil {
		// Use the default libp2p host creation
		o.hostFactory = libp2p.New
	}

	h, err := o.hostFactory(opts...)
	if err != nil {
		return nil, err
	}

	// Support same non default security and transport options as
	// original host.
	dialer, err := o.hostFactory(append(transports, security)...)
	if err != nil {
		return nil, err
	}

	options := []autonat.Option{autonat.EnableService(dialer.Network())}

	val, err := strconv.ParseBool(reachabilityOverridePublic)
	if err != nil {
		return nil, err
	}
	if val {
		options = append(options, autonat.WithReachability(network.ReachabilityPublic))
	}

	// If you want to help other peers to figure out if they are behind
	// NATs, you can launch the server-side of AutoNAT too (AutoRelay
	// already runs the client)
	var autoNAT autonat.AutoNAT
	if autoNAT, err = autonat.New(h, options...); err != nil {
		return nil, fmt.Errorf("autonat: %w", err)
	}

	if o.HeadersRWTimeout == 0 {
		o.HeadersRWTimeout = defaultHeadersRWTimeout
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

	handshakeService, err := handshake.New(signer, advertisableAddresser, overlay, networkID, o.FullNode, o.Nonce, o.WelcomeMessage, o.ValidateOverlay, h.ID(), logger)
	if err != nil {
		return nil, fmt.Errorf("handshake service: %w", err)
	}

	// Create a new dialer for libp2p ping protocol. This ensures that the protocol
	// uses a different set of keys to do ping. It prevents inconsistencies in peerstore as
	// the addresses used are not dialable and hence should be cleaned up. We should create
	// this host with the same transports and security options to be able to dial to other
	// peers.
	pingDialer, err := o.hostFactory(append(transports, security, libp2p.NoListenAddrs)...)
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
		logger:            logger.WithName(loggerName).Register(),
		tracer:            tracer,
		connectionBreaker: breaker.NewBreaker(breaker.Options{}), // use default options
		ready:             make(chan struct{}),
		halt:              make(chan struct{}),
		lightNodes:        lightNodes,
		HeadersRWTimeout:  o.HeadersRWTimeout,
		autoNAT:           autoNAT,
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

	connMetricNotify := newConnMetricNotify(s.metrics)
	h.Network().Notify(peerRegistry) // update peer registry on network events
	h.Network().Notify(connMetricNotify)

	return s, nil
}

func (s *Service) reachabilityWorker() error {
	sub, err := s.host.EventBus().Subscribe([]interface{}{new(event.EvtLocalReachabilityChanged)})
	if err != nil {
		return fmt.Errorf("failed subscribing to reachability event %w", err)
	}

	go func() {
		defer sub.Close()
		for {
			select {
			case <-s.ctx.Done():
				return
			case e := <-sub.Out():
				if r, ok := e.(event.EvtLocalReachabilityChanged); ok {
					select {
					case <-s.ready:
					case <-s.halt:
						return
					}
					s.logger.Debug("reachability changed", "new_reachability", r.Reachability.String())
					s.notifier.UpdateReachability(p2p.ReachabilityStatus(r.Reachability))
				}
			}
		}
	}()
	return nil
}

func (s *Service) handleIncoming(stream network.Stream) {
	loggerV1 := s.logger.V(1).Register()

	select {
	case <-s.ready:
	case <-s.halt:
		_ = stream.Reset()
		return
	case <-s.ctx.Done():
		_ = stream.Reset()
		return
	}

	peerID := stream.Conn().RemotePeer()
	handshakeStream := newStream(stream, s.metrics)
	i, err := s.handshakeService.Handle(s.ctx, handshakeStream, stream.Conn().RemoteMultiaddr(), peerID)
	if err != nil {
		s.logger.Debug("stream handler: handshake: handle failed", "peer_id", peerID, "error", err)
		s.logger.Error(nil, "stream handler: handshake: handle failed", "peer_id", peerID)
		_ = handshakeStream.Reset()
		_ = s.host.Network().ClosePeer(peerID)
		return
	}

	overlay := i.BzzAddress.Overlay

	blocked, err := s.blocklist.Exists(overlay)
	if err != nil {
		s.logger.Debug("stream handler: blocklisting: exists failed", "peer_address", overlay, "error", err)
		s.logger.Error(nil, "stream handler: internal error while connecting with peer", "peer_address", overlay)
		_ = handshakeStream.Reset()
		_ = s.host.Network().ClosePeer(peerID)
		return
	}

	if blocked {
		s.logger.Error(nil, "stream handler: blocked connection from blocklisted peer", "peer_address", overlay)
		_ = handshakeStream.Reset()
		_ = s.host.Network().ClosePeer(peerID)
		return
	}

	if exists := s.peers.addIfNotExists(stream.Conn(), overlay, i.FullNode); exists {
		s.logger.Debug("stream handler: peer already exists", "peer_address", overlay)
		if err = handshakeStream.FullClose(); err != nil {
			s.logger.Debug("stream handler: could not close stream", "peer_address", overlay, "error", err)
			s.logger.Error(nil, "stream handler: unable to handshake with peer", "peer_address", overlay)
			_ = s.Disconnect(overlay, "unable to close handshake stream")
		}
		return
	}

	if err = handshakeStream.FullClose(); err != nil {
		s.logger.Debug("stream handler: could not close stream", "peer_address", overlay, "error", err)
		s.logger.Error(nil, "stream handler: unable to handshake with peer", "peer_address", overlay)
		_ = s.Disconnect(overlay, "could not fully close stream on handshake")
		return
	}

	if i.FullNode {
		err = s.addressbook.Put(i.BzzAddress.Overlay, *i.BzzAddress)
		if err != nil {
			s.logger.Debug("stream handler: addressbook put error", "peer_id", peerID, "error", err)
			s.logger.Error(nil, "stream handler: unable to persist peer", "peer_id", peerID)
			_ = s.Disconnect(i.BzzAddress.Overlay, "unable to persist peer in addressbook")
			return
		}
	}

	peer := p2p.Peer{Address: overlay, FullNode: i.FullNode, EthereumAddress: i.BzzAddress.EthereumAddress}

	s.protocolsmu.RLock()
	for _, tn := range s.protocols {
		if tn.ConnectIn != nil {
			if err := tn.ConnectIn(s.ctx, peer); err != nil {
				s.logger.Debug("stream handler: connectIn failed", "protocol", tn.Name, "version", tn.Version, "peer", overlay, "error", err)
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
				s.logger.Debug("stream handler: notifier.Announce failed", "peer", peer.Address, "error", err)
			}

			if s.lightNodes.Count() > s.lightNodeLimit {
				// kick another node to fit this one in
				p, err := s.lightNodes.RandomPeer(peer.Address)
				if err != nil {
					s.logger.Debug("stream handler: can't find a peer slot for light node", "error", err)
					_ = s.Disconnect(peer.Address, "unable to find peer slot for light node")
					return
				} else {
					loggerV1.Debug("stream handler: kicking away light node to make room for new node", "old_peer", p.String(), "new_peer", peer.Address)
					s.metrics.KickedOutPeersCount.Inc()
					_ = s.Disconnect(p, "kicking away light node to make room for peer")
					return
				}
			}
		} else {
			if err := s.notifier.Connected(s.ctx, peer, false); err != nil {
				s.logger.Debug("stream handler: notifier.Connected: peer disconnected", "peer", i.BzzAddress.Overlay, "error", err)
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
						s.logger.Debug("stream handler: notifier.AnnounceTo failed", "addressee", addressee, "peer", peer, "error", err)
					}
				}(addr, peer.Address, i.FullNode)
				return false, false, nil
			})
		}
	}

	s.metrics.HandledStreamCount.Inc()
	if !s.peers.Exists(overlay) {
		s.logger.Warning("stream handler: inbound peer does not exist, disconnecting", "peer", overlay)
		_ = s.Disconnect(overlay, "unknown inbound peer")
		return
	}

	if s.reacher != nil {
		s.reacher.Connected(overlay, i.BzzAddress.Underlay)
	}

	peerUserAgent := appendSpace(s.peerUserAgent(s.ctx, peerID))
	s.networkStatus.Store(int32(p2p.NetworkStatusAvailable))

	loggerV1.Debug("stream handler: successfully connected to peer (inbound)", "addresses", i.BzzAddress.ShortString(), "light", i.LightString(), "user_agent", peerUserAgent)
	s.logger.Debug("stream handler: successfully connected to peer (inbound)", "address", i.BzzAddress.Overlay, "light", i.LightString(), "user_agent", peerUserAgent)
}

func (s *Service) SetPickyNotifier(n p2p.PickyNotifier) {
	s.handshakeService.SetPicker(n)
	s.notifier = n
	s.reacher = reacher.New(s, n, nil)
}

func (s *Service) AddProtocol(p p2p.ProtocolSpec) (err error) {
	for _, ss := range p.StreamSpecs {
		id := protocol.ID(p2p.NewSwarmStreamName(p.Name, p.Version, ss.Name))
		matcher, err := s.protocolSemverMatcher(id)
		if err != nil {
			return fmt.Errorf("protocol version match %s: %w", id, err)
		}

		s.host.SetStreamHandlerMatch(id, matcher, func(streamlibp2p network.Stream) {
			start := time.Now()
			peerID := streamlibp2p.Conn().RemotePeer()
			overlay, found := s.peers.overlay(peerID)
			if !found {
				_ = streamlibp2p.Reset()
				s.logger.Debug("overlay address for peer not found", "peer_id", peerID)
				return
			}
			full, found := s.peers.fullnode(peerID)
			if !found {
				_ = streamlibp2p.Reset()
				s.logger.Debug("fullnode info for peer not found", "peer_id", peerID)
				return
			}

			stream := newStream(streamlibp2p, s.metrics)

			// exchange headers
			ctx, cancel := context.WithTimeout(s.ctx, s.HeadersRWTimeout)
			defer cancel()
			if err := handleHeaders(ctx, ss.Headler, stream, overlay); err != nil {
				s.logger.Debug("handle protocol: handle headers failed", "protocol", p.Name, "version", p.Version, "stream", ss.Name, "peer", overlay, "error", err)
				_ = stream.Reset()
				return
			}
			s.metrics.HeadersExchangeDuration.Observe(time.Since(start).Seconds())

			ctx, cancel = context.WithCancel(s.ctx)

			s.peers.addStream(peerID, streamlibp2p, cancel)
			defer s.peers.removeStream(peerID, streamlibp2p)

			// tracing: get span tracing context and add it to the context
			// silently ignore if the peer is not providing tracing
			ctx, err := s.tracer.WithContextFromHeaders(ctx, stream.Headers())
			if err != nil && !errors.Is(err, tracing.ErrContextNotFound) {
				s.logger.Debug("handle protocol: get tracing context failed", "protocol", p.Name, "version", p.Version, "stream", ss.Name, "peer", overlay, "error", err)
				_ = stream.Reset()
				return
			}

			logger := tracing.NewLoggerWithTraceID(ctx, s.logger)
			loggerV1 := logger.V(1).Build()

			s.metrics.HandledStreamCount.Inc()
			if err := ss.Handler(ctx, p2p.Peer{Address: overlay, FullNode: full}, stream); err != nil {
				var de *p2p.DisconnectError
				if errors.As(err, &de) {
					loggerV1.Debug("libp2p handler: disconnecting due to disconnect error", "protocol", p.Name, "address", overlay)
					_ = stream.Reset()
					_ = s.Disconnect(overlay, de.Error())
				}

				var bpe *p2p.BlockPeerError
				if errors.As(err, &bpe) {
					_ = stream.Reset()
					if err := s.Blocklist(overlay, bpe.Duration(), bpe.Error()); err != nil {
						logger.Debug("blocklist: could not blocklist peer", "peer_id", peerID, "error", err)
						logger.Error(nil, "unable to blocklist peer", "peer_id", peerID)
					}
					loggerV1.Debug("handler: peer blocklisted", "protocol", p.Name, "peer_address", overlay)
				}
				// count unexpected requests
				if errors.Is(err, p2p.ErrUnexpected) {
					s.metrics.UnexpectedProtocolReqCount.Inc()
				}
				if errors.Is(err, network.ErrReset) {
					s.metrics.StreamHandlerErrResetCount.Inc()
				}
				logger.Debug("handle protocol failed", "protocol", p.Name, "version", p.Version, "stream", ss.Name, "peer", overlay, "error", err)
				return
			}
		})
	}

	s.protocolsmu.Lock()
	s.protocols = append(s.protocols, p)
	s.protocolsmu.Unlock()
	return nil
}

func (s *Service) Addresses() (addresses []ma.Multiaddr, err error) {
	for _, addr := range s.host.Addrs() {
		a, err := buildUnderlayAddress(addr, s.host.ID())
		if err != nil {
			return nil, err
		}

		addresses = append(addresses, a)
	}
	if s.natAddrResolver != nil && len(addresses) > 0 {
		a, err := s.natAddrResolver.Resolve(addresses[0])
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, a)
	}

	return addresses, nil
}

func (s *Service) NATManager() basichost.NATManager {
	return s.natManager
}

func (s *Service) Blocklist(overlay swarm.Address, duration time.Duration, reason string) error {
	loggerV1 := s.logger.V(1).Register()

	if s.NetworkStatus() != p2p.NetworkStatusAvailable {
		return errors.New("cannot blocklist peer when network not available")
	}

	id, ok := s.peers.peerID(overlay)
	if !ok {
		return p2p.ErrPeerNotFound
	}

	full, _ := s.peers.fullnode(id)

	loggerV1.Debug("libp2p blocklisting peer", "peer_address", overlay.String(), "duration", duration, "reason", reason)
	if err := s.blocklist.Add(overlay, duration, reason, full); err != nil {
		s.metrics.BlocklistedPeerErrCount.Inc()
		_ = s.Disconnect(overlay, "failed blocklisting peer")
		return fmt.Errorf("blocklist peer %s: %w", overlay, err)
	}
	s.metrics.BlocklistedPeerCount.Inc()

	_ = s.Disconnect(overlay, reason)
	return nil
}

func buildHostAddress(peerID libp2ppeer.ID) (ma.Multiaddr, error) {
	return ma.NewMultiaddr(fmt.Sprintf("/p2p/%s", peerID.String()))
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
	loggerV1 := s.logger.V(1).Register()

	defer func() {
		err = s.determineCurrentNetworkStatus(err)
	}()

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

	handshakeStream := newStream(stream, s.metrics)
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
		s.logger.Debug("blocklisting: exists failed", "peer_id", info.ID, "error", err)
		s.logger.Error(nil, "internal error while connecting with peer", "peer_id", info.ID)
		_ = handshakeStream.Reset()
		_ = s.host.Network().ClosePeer(info.ID)
		return nil, err
	}

	if blocked {
		s.logger.Error(nil, "blocked connection to blocklisted peer", "peer_id", info.ID)
		_ = handshakeStream.Reset()
		_ = s.host.Network().ClosePeer(info.ID)
		return nil, p2p.ErrPeerBlocklisted
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
				s.logger.Debug("connectOut: failed to connect", "protocol", tn.Name, "version", tn.Version, "peer", overlay, "error", err)
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

	if s.reacher != nil {
		s.reacher.Connected(overlay, i.BzzAddress.Underlay)
	}

	peerUserAgent := appendSpace(s.peerUserAgent(ctx, info.ID))

	loggerV1.Debug("successfully connected to peer (outbound)", "addresses", i.BzzAddress.ShortString(), "light", i.LightString(), "user_agent", peerUserAgent)
	s.logger.Debug("successfully connected to peer (outbound)", "address", i.BzzAddress.Overlay, "light", i.LightString(), "user_agent", peerUserAgent)
	return i.BzzAddress, nil
}

func (s *Service) Disconnect(overlay swarm.Address, reason string) (err error) {
	s.metrics.DisconnectCount.Inc()

	s.logger.Debug("libp2p disconnect: disconnecting peer", "peer_address", overlay, "reason", reason)

	// found is checked at the bottom of the function
	found, full, peerID := s.peers.remove(overlay)

	_ = s.host.Network().ClosePeer(peerID)

	peer := p2p.Peer{Address: overlay, FullNode: full}

	s.protocolsmu.RLock()
	for _, tn := range s.protocols {
		if tn.DisconnectOut != nil {
			if err := tn.DisconnectOut(peer); err != nil {
				s.logger.Debug("disconnectOut failed", "protocol", tn.Name, "version", tn.Version, "peer", overlay, "error", err)
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
	if s.reacher != nil {
		s.reacher.Disconnected(overlay)
	}

	if !found {
		s.logger.Debug("libp2p disconnect: peer not found", "peer_address", overlay)
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
				s.logger.Debug("disconnectIn failed", tn.Name, "version", tn.Version, "peer", address, "error", err)
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
	if s.reacher != nil {
		s.reacher.Disconnected(address)
	}
}

func (s *Service) Peers() []p2p.Peer {
	return s.peers.peers()
}

func (s *Service) Blocklisted(overlay swarm.Address) (bool, error) {
	return s.blocklist.Exists(overlay)
}

func (s *Service) BlocklistedPeers() ([]p2p.BlockListedPeer, error) {
	return s.blocklist.Peers()
}

func (s *Service) NewStream(ctx context.Context, overlay swarm.Address, headers p2p.Headers, protocolName, protocolVersion, streamName string) (p2p.Stream, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	peerID, found := s.peers.peerID(overlay)
	if !found {
		return nil, p2p.ErrPeerNotFound
	}

	streamlibp2p, err := s.newStreamForPeerID(ctx, peerID, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, fmt.Errorf("new stream for peerid: %w", err)
	}

	stream := newStream(streamlibp2p, s.metrics)

	// tracing: add span context header
	if headers == nil {
		headers = make(p2p.Headers)
	}
	if err := s.tracer.AddContextHeader(ctx, headers); err != nil && !errors.Is(err, tracing.ErrContextNotFound) {
		_ = stream.Reset()
		return nil, fmt.Errorf("new stream add context header fail: %w", err)
	}

	// exchange headers
	ctx, cancel := context.WithTimeout(ctx, s.HeadersRWTimeout)
	defer cancel()
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
		var errNotSupported multistream.ErrNotSupported[protocol.ID]
		if errors.As(err, &errNotSupported) {
			return nil, p2p.NewIncompatibleStreamError(err)
		}
		if errors.Is(err, multistream.ErrIncorrectVersion) {
			return nil, p2p.NewIncompatibleStreamError(err)
		}
		return nil, fmt.Errorf("create stream %s to %s: %w", swarmStreamName, peerID, err)
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
	if s.reacher != nil {
		if err := s.reacher.Close(); err != nil {
			return err
		}
	}
	if s.autoNAT != nil {
		if err := s.autoNAT.Close(); err != nil {
			return err
		}
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

func (s *Service) Ready() error {
	if err := s.reachabilityWorker(); err != nil {
		return fmt.Errorf("reachability worker: %w", err)
	}

	close(s.ready)
	return nil
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
func (s *Service) peerUserAgent(ctx context.Context, peerID libp2ppeer.ID) string {
	ctx, cancel := context.WithTimeout(ctx, peerUserAgentTimeout)
	defer cancel()
	var (
		v   interface{}
		err error
	)
	// Peerstore may not contain all keys and values right after the connections is created.
	// This retry mechanism ensures more reliable user agent propagation.
	for iterate := true; iterate; {
		v, err = s.host.Peerstore().Get(peerID, "AgentVersion")
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			iterate = false
		case <-time.After(50 * time.Millisecond):
		}
	}
	if err != nil {
		// error is ignored as user agent is informative only
		return ""
	}
	ua, ok := v.(string)
	if !ok {
		return ""
	}
	// Ignore the default user agent.
	if ua == "github.com/libp2p/go-libp2p" {
		return ""
	}
	return ua
}

// NetworkStatus implements the p2p.NetworkStatuser interface.
func (s *Service) NetworkStatus() p2p.NetworkStatus {
	return p2p.NetworkStatus(s.networkStatus.Load())
}

// determineCurrentNetworkStatus determines if the network
// is available/unavailable based on the given error, and
// returns ErrNetworkUnavailable if unavailable.
// The result of this operation is stored and can be reflected
// in the results of future NetworkStatus method calls.
func (s *Service) determineCurrentNetworkStatus(err error) error {
	switch {
	case err == nil:
		s.networkStatus.Store(int32(p2p.NetworkStatusAvailable))
	case errors.Is(err, lp2pswarm.ErrDialBackoff):
		if s.NetworkStatus() == p2p.NetworkStatusUnavailable {
			err = errors.Join(err, p2p.ErrNetworkUnavailable)
		}
	case isNetworkOrHostUnreachableError(err):
		s.networkStatus.Store(int32(p2p.NetworkStatusUnavailable))
		err = errors.Join(err, p2p.ErrNetworkUnavailable)
	default:
		err = fmt.Errorf("network status unknown: %w", err)
	}
	return err
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

func newConnMetricNotify(m metrics) *connectionNotifier {
	return &connectionNotifier{
		metrics:  m,
		Notifiee: new(network.NoopNotifiee),
	}
}

type connectionNotifier struct {
	metrics metrics
	network.Notifiee
}

func (c *connectionNotifier) Connected(_ network.Network, _ network.Conn) {
	c.metrics.HandledConnectionCount.Inc()
}

// isNetworkOrHostUnreachableError determines based on the
// given error whether the host or network is reachable.
func isNetworkOrHostUnreachableError(err error) bool {
	var de *lp2pswarm.DialError
	if !errors.As(err, &de) {
		return false
	}

	// Since TransportError doesn't implement the Unwrap
	// method we need to inspect the errors manually.
	for i := range de.DialErrors {
		var te *lp2pswarm.TransportError
		if !errors.As(&de.DialErrors[i], &te) {
			continue
		}

		var ne *net.OpError
		if !errors.As(te.Cause, &ne) || ne.Op != "dial" {
			continue
		}

		var se *os.SyscallError
		if errors.As(ne, &se) && strings.HasPrefix(se.Syscall, "connect") &&
			(errors.Is(se.Err, errHostUnreachable) || errors.Is(se.Err, errNetworkUnreachable)) {
			return true
		}
	}
	return false
}
