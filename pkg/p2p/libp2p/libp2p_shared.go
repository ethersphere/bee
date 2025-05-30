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
	"strings"
	"time"

	"github.com/ethersphere/bee/v2"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/reacher"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"

	lp2pswarm "github.com/libp2p/go-libp2p/p2p/net/swarm"
	libp2pping "github.com/libp2p/go-libp2p/p2p/protocol/ping"

	ma "github.com/multiformats/go-multiaddr"

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

func (s *Service) SetPickyNotifier(n p2p.PickyNotifier) {
	s.handshakeService.SetPicker(n)
	s.notifier = n
	s.reacher = reacher.New(s, n, nil)
}

func (s *Service) Protocols() []protocol.ID {
	return s.host.Mux().Protocols()
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
