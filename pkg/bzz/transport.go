// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz

import (
	"sort"

	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/multiformats/go-varint"
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

// underlayScore returns a priority score for a multiaddr address.
// Lower score = higher priority:
//   - IPv4 over non-IPv4 (+0 vs +100) — rewards IPv4 explicitly so that
//     DNS-based and IPv6 addresses are deprioritized equally.
//   - Public over private over loopback (+0 vs +10 vs +20)
//   - Transport priority: TCP(0) > WS(1) > WSS(2) > Unknown(3)
func underlayScore(addr ma.Multiaddr) int {
	score := 0

	// Non-IPv4 penalty (covers IPv6, DNS, and any other network protocol).
	_, err := addr.ValueForProtocol(ma.P_IP4)
	if err != nil {
		score += 100
	}

	// Loopback penalty (higher than private — only reachable from same host).
	if manet.IsIPLoopback(addr) {
		score += 20
	} else if manet.IsPrivateAddr(addr) {
		// Private penalty (reachable within local network).
		score += 10
	}

	// Transport priority.
	score += ClassifyTransport(addr).Priority()

	return score
}

// SortUnderlaysByPriority returns a copy of the addresses sorted by priority.
// Priority: IPv4 public TCP first, then by transport, then private, then IPv6.
func SortUnderlaysByPriority(addrs []ma.Multiaddr) []ma.Multiaddr {
	sorted := make([]ma.Multiaddr, len(addrs))
	copy(sorted, addrs)
	sort.SliceStable(sorted, func(i, j int) bool {
		return underlayScore(sorted[i]) < underlayScore(sorted[j])
	})
	return sorted
}

// TruncateUnderlays sorts addresses by priority and truncates to fit within
// maxUnderlaysPerPeer count and maxUnderlayBytes byte budget. It returns
// true as the second value if truncation occurred.
func TruncateUnderlays(addrs []ma.Multiaddr) ([]ma.Multiaddr, bool) {
	if len(addrs) == 0 {
		return addrs, false
	}

	sorted := SortUnderlaysByPriority(addrs)
	truncated := false

	resultCap := min(len(sorted), maxUnderlaysPerPeer)
	result := make([]ma.Multiaddr, 0, resultCap)
	// Account for the 0x99 prefix byte when counting serialized size.
	totalSize := 1
	for _, addr := range sorted {
		if len(result) >= maxUnderlaysPerPeer {
			truncated = true
			break
		}
		addrBytes := addr.Bytes()
		addrSize := len(varint.ToUvarint(uint64(len(addrBytes)))) + len(addrBytes)
		if totalSize+addrSize > maxUnderlayBytes {
			truncated = true
			break
		}
		totalSize += addrSize
		result = append(result, addr)
	}
	return result, truncated
}

func SelectBestAdvertisedAddress(addrs []ma.Multiaddr, fallback ma.Multiaddr) ma.Multiaddr {
	if len(addrs) == 0 {
		return fallback
	}
	return SortUnderlaysByPriority(addrs)[0]
}
