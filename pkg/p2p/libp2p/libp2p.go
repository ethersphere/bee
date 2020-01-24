// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/janos/bee/pkg/p2p"

	handshake "github.com/janos/bee/pkg/p2p/libp2p/internal/handshake"
	"github.com/libp2p/go-libp2p"
	autonat "github.com/libp2p/go-libp2p-autonat-svc"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
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
	overlayToPeerID  map[string]libp2ppeer.ID
	peerIDToOverlay  map[libp2ppeer.ID]string
}

type Options struct {
	PrivateKey       io.ReadWriteCloser
	Addr             string
	DisableWS        bool
	DisableQUIC      bool
	Bootnodes        []string
	NetworkID        int // TODO: to be used in the handshake protocol
	ConnectionsLow   int
	ConnectionsHigh  int
	ConnectionsGrace time.Duration
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

	if o.PrivateKey != nil {
		var privateKey crypto.PrivKey
		privateKeyData, err := ioutil.ReadAll(o.PrivateKey)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read private key: %w", err)
		}
		if len(privateKeyData) == 0 {
			var err error
			privateKey, _, err = crypto.GenerateSecp256k1Key(nil)
			if err != nil {
				return nil, fmt.Errorf("generate secp256k1 key: %w", err)
			}
			d, err := crypto.MarshalPrivateKey(privateKey)
			if err != nil {
				return nil, fmt.Errorf("encode private key: %w", err)
			}
			if _, err := io.Copy(o.PrivateKey, bytes.NewReader(d)); err != nil {
				return nil, fmt.Errorf("write private key: %w", err)
			}
		} else {
			var err error
			privateKey, err = crypto.UnmarshalPrivateKey(privateKeyData)
			if err != nil {
				return nil, fmt.Errorf("decode private key: %w", err)
			}
		}
		if err := o.PrivateKey.Close(); err != nil {
			return nil, fmt.Errorf("close private key: %w", err)
		}
		opts = append(opts,
			// Use the keypair we generated
			libp2p.Identity(privateKey),
		)
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

	overlay := strconv.Itoa(rand.Int())
	s := &Service{
		host:             h,
		metrics:          newMetrics(),
		overlayToPeerID:  make(map[string]libp2ppeer.ID),
		peerIDToOverlay:  make(map[libp2ppeer.ID]string),
		handshakeService: handshake.New(overlay),
	}

	// Construct protocols.

	id := protocol.ID(p2p.NewSwarmStreamName(handshake.ProtocolName, handshake.StreamName, handshake.StreamVersion))
	matcher, err := helpers.MultistreamSemverMatcher(id)
	if err != nil {
		return nil, fmt.Errorf("match semver %s: %w", id, err)
	}

	s.host.SetStreamHandlerMatch(id, matcher, func(stream network.Stream) {
		s.metrics.HandledStreamCount.Inc()
		overlay := s.handshakeService.Handler(stream)
		s.addAddresses(overlay, stream.Conn().RemotePeer())
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
			overlay, ok := s.peerIDToOverlay[stream.Conn().RemotePeer()]
			if !ok {
				// todo: handle better
				fmt.Printf("Could not fetch handshake for peerID %s\n", stream)
				return
			}

			s.metrics.HandledStreamCount.Inc()
			ss.Handler(p2p.Peer{Address: overlay}, stream)
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
		return err
	}

	if err := s.host.Connect(ctx, *info); err != nil {
		return err
	}

	stream, err := s.newStreamForPeerID(ctx, info.ID, handshake.ProtocolName, handshake.StreamName, handshake.StreamVersion)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	overlay, err := s.handshakeService.Handshake(stream)
	if err != nil {
		return err
	}

	s.addAddresses(overlay, info.ID)
	s.metrics.CreatedConnectionCount.Inc()
	fmt.Println("handshake handshake finished")
	return nil
}
func (s *Service) NewStream(ctx context.Context, overlay, protocolName, streamName, version string) (p2p.Stream, error) {
	peerID, ok := s.overlayToPeerID[overlay]
	if !ok {
		fmt.Printf("Could not fetch peerID for handshake %s\n", overlay)
		return nil, nil
	}

	return s.newStreamForPeerID(ctx, peerID, protocolName, streamName, version)
}

func (s *Service) newStreamForPeerID(ctx context.Context, peerID libp2ppeer.ID, protocolName, streamName, version string) (p2p.Stream, error) {
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

func (s *Service) addAddresses(overlay string, peerID libp2ppeer.ID) {
	s.overlayToPeerID[overlay] = peerID
	s.peerIDToOverlay[peerID] = overlay
}

func (s *Service) Close() error {
	return s.host.Close()
}
