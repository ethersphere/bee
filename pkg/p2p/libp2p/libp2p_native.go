//go:build !js

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"time"

	ocprom "contrib.go.opencensus.io/exporter/prometheus"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	beecrypto "github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	m2 "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/blocklist"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/breaker"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake"
	libp2pmock "github.com/ethersphere/bee/v2/pkg/p2p/libp2p/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	p2pforge "github.com/ipshipyard/p2p-forge/client"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/host/autonat"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ws "github.com/libp2p/go-libp2p/p2p/transport/websocket"
	libp2prate "github.com/libp2p/go-libp2p/x/rate"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"
)

func New(ctx context.Context, signer beecrypto.Signer, networkID uint64, overlay swarm.Address, addr string, ab addressbook.GetPutter, storer storage.StateStorer, lightNodes *lightnode.Container, logger log.Logger, tracer *tracing.Tracer, o Options) (s *Service, returnErr error) {
	logger = logger.WithName(loggerName).Register()

	parsedAddr, err := parseAddress(addr)
	if err != nil {
		return nil, err
	}

	var listenAddrs []string

	if parsedAddr.IP4 != "" {
		listenAddrs = append(listenAddrs, fmt.Sprintf("/ip4/%s/tcp/%s", parsedAddr.IP4, parsedAddr.Port))
		if o.EnableWS {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip4/%s/tcp/%s/ws", parsedAddr.IP4, parsedAddr.Port))
		}
	}

	if parsedAddr.IP6 != "" {
		listenAddrs = append(listenAddrs, fmt.Sprintf("/ip6/%s/tcp/%s", parsedAddr.IP6, parsedAddr.Port))
		if o.EnableWS {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip6/%s/tcp/%s/ws", parsedAddr.IP6, parsedAddr.Port))
		}
	}

	if o.EnableWSS {
		parsedWssAddr, err := parseAddress(o.WSSAddr)
		if err != nil {
			return nil, err
		}

		if parsedWssAddr.IP4 != "" {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip4/%s/tcp/%s/tls/sni/*.%s/ws", parsedWssAddr.IP4, parsedWssAddr.Port, o.AutoTLSDomain))
		}

		if parsedWssAddr.IP6 != "" {
			listenAddrs = append(listenAddrs, fmt.Sprintf("/ip6/%s/tcp/%s/tls/sni/*.%s/ws", parsedWssAddr.IP6, parsedWssAddr.Port, o.AutoTLSDomain))
		}
	}

	security := libp2p.DefaultSecurity
	libp2pPeerstore, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, err
	}

	if o.Registry != nil {
		rcmgr.MustRegisterWith(o.Registry)
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

	str, err := rcmgr.NewStatsTraceReporter()
	if err != nil {
		return nil, err
	}

	limitPerIp := rcmgr.WithLimitPerSubnet(
		[]rcmgr.ConnLimitPerSubnet{{PrefixLength: 32, ConnCount: 200}}, // IPv4 /32 (Single IP) -> 200 conns
		[]rcmgr.ConnLimitPerSubnet{{PrefixLength: 56, ConnCount: 200}}, // IPv6 /56 subnet -> 200 conns
	)

	// Custom rate limiter for connection attempts
	// 20 peers cluster adaptation:
	// Allow bursts of connection attempts (e.g. restart) but prevent DDOS.
	connLimiter := &libp2prate.Limiter{
		// Allow unlimited local connections (same as default)
		NetworkPrefixLimits: []libp2prate.PrefixLimit{
			{Prefix: netip.MustParsePrefix("127.0.0.0/8"), Limit: libp2prate.Limit{}},
			{Prefix: netip.MustParsePrefix("::1/128"), Limit: libp2prate.Limit{}},
		},
		GlobalLimit: libp2prate.Limit{}, // Unlimited global
		SubnetRateLimiter: libp2prate.SubnetLimiter{
			IPv4SubnetLimits: []libp2prate.SubnetLimit{
				{
					PrefixLength: 32, // Apply limits per individual IPv4 address (/32)
					// Allow 10 connection attempts per second per IP, burst up to 40
					Limit: libp2prate.Limit{RPS: 10.0, Burst: 40},
				},
			},
			IPv6SubnetLimits: []libp2prate.SubnetLimit{
				{
					PrefixLength: 56, // Apply limits per /56 IPv6 subnet
					// Allow 10 connection attempts per second per IP, burst up to 40
					// Subnet-level limiting prevents flooding from multiple addresses in the same block.
					Limit: libp2prate.Limit{RPS: 10.0, Burst: 40},
				},
			},
			// Duration to retain state for an IP or subnet after it becomes inactive.
			GracePeriod: 10 * time.Second,
		},
	}

	rm, err := rcmgr.NewResourceManager(limiter, rcmgr.WithTraceReporter(str), limitPerIp, rcmgr.WithConnRateLimiters(connLimiter))
	if err != nil {
		return nil, err
	}

	var natManager basichost.NATManager

	var certManager autoTLSCertManager
	var zapLogger *zap.Logger

	// AutoTLS is only needed for WSS
	enableAutoTLS := o.EnableWSS

	if enableAutoTLS {
		if o.autoTLSCertManager != nil {
			certManager = o.autoTLSCertManager
		} else {
			forgeMgr, err := newP2PForgeCertManager(logger, P2PForgeOptions{
				Domain:               o.AutoTLSDomain,
				RegistrationEndpoint: o.AutoTLSRegistrationEndpoint,
				CAEndpoint:           o.AutoTLSCAEndpoint,
				StorageDir:           o.AutoTLSStorageDir,
			})
			if err != nil {
				return nil, err
			}

			certManager = forgeMgr.CertMgr()
			zapLogger = forgeMgr.ZapLogger()
		}

		defer func() {
			if returnErr != nil {
				// call if service is not constructed
				certManager.Stop()
				_ = zapLogger.Sync()
			}
		}()

		if err := certManager.Start(); err != nil {
			return nil, fmt.Errorf("start AutoTLS certificate manager: %w", err)
		}

		logger.Info("AutoTLS certificate manager initialized")
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(listenAddrs...),
		security,
		// Use dedicated peerstore instead the global DefaultPeerstore
		libp2p.Peerstore(libp2pPeerstore),
		libp2p.UserAgent(userAgent()),
		libp2p.ResourceManager(rm),
	}

	if o.NATAddr == "" && o.NATWSSAddr == "" {
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

	var tcpResolver handshake.AdvertisableAddressResolver
	if o.NATAddr != "" {
		r, err := newStaticAddressResolver(o.NATAddr, net.LookupIP)
		if err != nil {
			return nil, fmt.Errorf("static nat: %w", err)
		}
		tcpResolver = r
	}

	var wssResolver handshake.AdvertisableAddressResolver
	if o.EnableWSS && o.NATWSSAddr != "" {
		r, err := newStaticAddressResolver(o.NATWSSAddr, net.LookupIP)
		if err != nil {
			return nil, fmt.Errorf("static wss nat: %w", err)
		}
		wssResolver = r
	}

	if o.EnableWSS {
		wsOpt := ws.WithTLSConfig(certManager.TLSConfig())
		transports = append(transports, libp2p.Transport(ws.New, wsOpt))
	} else if o.EnableWS {
		transports = append(transports, libp2p.Transport(ws.New))
	}

	compositeResolver := newCompositeAddressResolver(tcpResolver, wssResolver)

	var addrFactory config.AddrsFactory
	if o.EnableWSS {
		certManagerFactory := certManager.AddressFactory()
		addrFactory = func(addrs []ma.Multiaddr) []ma.Multiaddr {
			addrs = includeNatResolvedAddresses(addrs, compositeResolver, logger)
			addrs = certManagerFactory(addrs)
			return addrs
		}
	} else {
		addrFactory = func(addrs []ma.Multiaddr) []ma.Multiaddr {
			return includeNatResolvedAddresses(addrs, compositeResolver, logger)
		}
	}

	opts = append(opts, libp2p.AddrsFactory(addrFactory))

	opts = append(opts, transports...)

	if o.hostFactory == nil {
		// Use the default libp2p host creation
		o.hostFactory = libp2p.New
	}

	h, err := o.hostFactory(opts...)
	if err != nil {
		return nil, err
	}

	if enableAutoTLS {
		switch cm := certManager.(type) {
		case *p2pforge.P2PForgeCertMgr:
			cm.ProvideHost(h)
		case *libp2pmock.MockP2PForgeCertMgr:
			if err := cm.ProvideHost(h); err != nil {
				return nil, fmt.Errorf("failed to provide host to MockP2PForgeCertMgr: %w", err)
			}
		default:
			return nil, fmt.Errorf("unknown cert manager type")
		}
	}
	// Support same non default security and transport options as
	// original host.
	dialer, err := o.hostFactory(append(transports, security)...)
	if err != nil {
		return nil, err
	}

	if o.HeadersRWTimeout == 0 {
		o.HeadersRWTimeout = defaultHeadersRWTimeout
	}

	options := []autonat.Option{
		autonat.EnableService(dialer.Network()),
	}

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

	handshakeService, err := handshake.New(signer, newCompositeAddressResolver(tcpResolver, wssResolver), overlay, networkID, o.FullNode, o.Nonce, newHostAddresser(h), o.WelcomeMessage, ab, h.ID(), o.ChequebookVerifier, logger)
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
	s = &Service{
		ctx:                ctx,
		host:               h,
		natManager:         natManager,
		autonatDialer:      dialer,
		pingDialer:         pingDialer,
		handshakeService:   handshakeService,
		libp2pPeerstore:    libp2pPeerstore,
		metrics:            newMetrics(),
		networkID:          networkID,
		peers:              peerRegistry,
		addressbook:        ab,
		blocklist:          blocklist.NewBlocklist(storer),
		logger:             logger,
		tracer:             tracer,
		connectionBreaker:  breaker.NewBreaker(breaker.Options{}), // use default options
		ready:              make(chan struct{}),
		halt:               make(chan struct{}),
		lightNodes:         lightNodes,
		HeadersRWTimeout:   o.HeadersRWTimeout,
		autoNAT:            autoNAT,
		autoTLSCertManager: certManager,
		zapLogger:          zapLogger,
		enabledTransports: map[bzz.TransportType]bool{
			bzz.TransportTCP: true, // TCP transport is always included
			bzz.TransportWS:  o.EnableWS,
			bzz.TransportWSS: o.EnableWSS,
		},
		allowPrivateCIDRs: o.AllowPrivateCIDRs,
		chequebookStorer:  o.ChequebookStorer,
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
