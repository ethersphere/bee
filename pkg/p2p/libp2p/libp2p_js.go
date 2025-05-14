//go:build js
// +build js

package libp2p

import (
	"context"
	"fmt"
	"net"
	"strconv"

	ocprom "contrib.go.opencensus.io/exporter/prometheus"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	beecrypto "github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	m2 "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/blocklist"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/breaker"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/host/autonat"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	rcmgrObs "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	wasmws "github.com/talentlessguy/go-libp2p-wasmws"
)

func New(ctx context.Context, signer beecrypto.Signer, networkID uint64, overlay swarm.Address, addr string, ab addressbook.Putter, storer storage.StateStorer, lightNodes *lightnode.Container, logger log.Logger, tracer *tracing.Tracer, o Options) (*Service, error) {

	var listenAddrs []string

	var security libp2p.Option = libp2p.Security(noise.ID, noise.New)

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
		libp2p.ShareTCPListener(),
		libp2p.ListenAddrStrings(listenAddrs...),
		security,
		// Use dedicated peerstore instead the global DefaultPeerstore
		libp2p.Peerstore(libp2pPeerstore),
		libp2p.UserAgent(userAgent()),
		libp2p.ResourceManager(rm),
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
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

	transports := []libp2p.Option{}

	if o.EnableWS {
		transports = append(transports, libp2p.Transport(wasmws.New))
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
