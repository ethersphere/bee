// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz

import (
	"sort"

	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

// TransportType represents the transport protocol of a multiaddress.
type TransportType int

const (
	// TransportUnknown indicates an unrecognized transport.
	TransportUnknown TransportType = iota
	// TransportTCP indicates plain TCP without WebSocket.
	TransportTCP
	// TransportWS indicates WebSocket without TLS.
	TransportWS
	// TransportWSS indicates WebSocket with TLS (secure).
	TransportWSS
)

// String returns a string representation of the transport type.
func (t TransportType) String() string {
	switch t {
	case TransportTCP:
		return "tcp"
	case TransportWS:
		return "ws"
	case TransportWSS:
		return "wss"
	default:
		return "unknown"
	}
}

// Priority returns the sorting priority for the transport type.
// Lower value = higher priority: TCP (0) > WS (1) > WSS (2) > Unknown (3)
func (t TransportType) Priority() int {
	switch t {
	case TransportTCP:
		return 0
	case TransportWS:
		return 1
	case TransportWSS:
		return 2
	default:
		return 3
	}
}

// ClassifyTransport returns the transport type of a multiaddress.
// It distinguishes between plain TCP, WebSocket (WS), and secure WebSocket (WSS).
func ClassifyTransport(addr ma.Multiaddr) TransportType {
	if addr == nil {
		return TransportUnknown
	}

	hasProtocol := func(p int) bool {
		_, err := addr.ValueForProtocol(p)
		return err == nil
	}

	hasWS := hasProtocol(ma.P_WS)
	hasTLS := hasProtocol(ma.P_TLS)
	hasTCP := hasProtocol(ma.P_TCP)

	switch {
	case hasWS && hasTLS:
		return TransportWSS
	case hasWS:
		return TransportWS
	case hasTCP:
		return TransportTCP
	default:
		return TransportUnknown
	}
}

func SelectBestAdvertisedAddress(addrs []ma.Multiaddr, fallback ma.Multiaddr) ma.Multiaddr {
	if len(addrs) == 0 {
		return fallback
	}

	// Sort addresses: first by transport priority (TCP > WS > WSS), preserving relative order
	sort.SliceStable(addrs, func(i, j int) bool {
		return ClassifyTransport(addrs[i]).Priority() < ClassifyTransport(addrs[j]).Priority()
	})

	for _, addr := range addrs {
		if manet.IsPublicAddr(addr) {
			return addr
		}
	}

	for _, addr := range addrs {
		if !manet.IsPrivateAddr(addr) {
			return addr
		}
	}

	return addrs[0]
}
