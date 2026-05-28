// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz_test

import (
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bzz"

	"github.com/multiformats/go-multiaddr"
)

func TestSelectBestAdvertisedAddress(t *testing.T) {
	t.Parallel()

	mustMultiaddr := func(s string) multiaddr.Multiaddr {
		addr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			t.Fatalf("failed to create multiaddr %s: %v", s, err)
		}
		return addr
	}

	tests := []struct {
		name     string
		addrs    []multiaddr.Multiaddr
		fallback multiaddr.Multiaddr
		expected multiaddr.Multiaddr
	}{
		{
			name:     "empty addresses returns fallback",
			addrs:    []multiaddr.Multiaddr{},
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),
			expected: mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),
		},
		{
			name:     "nil addresses returns fallback",
			addrs:    nil,
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),
			expected: mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),
		},
		{
			name: "prefers public addresses",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/192.168.1.1/tcp/8080"), // private
				mustMultiaddr("/ip4/8.8.8.8/tcp/8080"),     // public
				mustMultiaddr("/ip4/10.0.0.1/tcp/8080"),    // private
			},
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),
			expected: mustMultiaddr("/ip4/8.8.8.8/tcp/8080"),
		},
		{
			name: "prefers first public address when multiple exist",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/192.168.1.1/tcp/8080"), // private
				mustMultiaddr("/ip4/8.8.8.8/tcp/8080"),     // public
				mustMultiaddr("/ip4/1.1.1.1/tcp/8080"),     // public
			},
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),
			expected: mustMultiaddr("/ip4/8.8.8.8/tcp/8080"),
		},
		{
			name: "prefers private over loopback when no public addresses",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),   // loopback (score 20)
				mustMultiaddr("/ip4/192.168.1.1/tcp/8080"), // private (score 10)
				mustMultiaddr("/ip4/10.0.0.1/tcp/8080"),    // private (score 10)
			},
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/9999"),
			expected: mustMultiaddr("/ip4/192.168.1.1/tcp/8080"),
		},
		{
			name: "returns first address when all are loopback",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),
				mustMultiaddr("/ip4/127.0.0.1/tcp/8081"),
				mustMultiaddr("/ip6/::1/tcp/8080"),
			},
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/9999"),
			expected: mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),
		},
		{
			name: "sorts TCP addresses first",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/192.168.1.1/udp/8080"), // UDP
				mustMultiaddr("/ip4/1.1.1.1/udp/8080"),     // UDP public
				mustMultiaddr("/ip4/8.8.8.8/tcp/8080"),     // TCP public
			},
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/9999"),
			expected: mustMultiaddr("/ip4/8.8.8.8/tcp/8080"),
		},
		{
			name: "prefers IPv4 private over IPv6 public",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip6/::1/tcp/8080"),         // IPv6 loopback (score 120)
				mustMultiaddr("/ip6/2001:db8::1/tcp/8080"), // IPv6 public (score 100)
				mustMultiaddr("/ip4/192.168.1.1/tcp/8080"), // IPv4 private (score 10)
			},
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/9999"),
			expected: mustMultiaddr("/ip4/192.168.1.1/tcp/8080"),
		},
		{
			name: "handles mixed protocols with preference order",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/192.168.1.1/udp/8080"), // private UDP
				mustMultiaddr("/ip4/192.168.1.2/tcp/8080"), // private TCP
				mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),   // loopback TCP
			},
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/9999"),
			expected: mustMultiaddr("/ip4/192.168.1.2/tcp/8080"), // first TCP, and it's non-loopback
		},
		{
			name: "single address",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/192.168.1.1/tcp/8080"),
			},
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/9999"),
			expected: mustMultiaddr("/ip4/192.168.1.1/tcp/8080"),
		},
		{
			name: "websocket addresses",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/127.0.0.1/tcp/8080/ws"),
				mustMultiaddr("/ip4/8.8.8.8/tcp/8080/ws"), // public with websocket
			},
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/9999"),
			expected: mustMultiaddr("/ip4/8.8.8.8/tcp/8080/ws"),
		},
		// Full underlay addresses tests
		{
			name: "full underlay addresses: prefers public TCP over private TCP",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/10.233.99.120/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),  // private
				mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"), // public
				mustMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),      // loopback
			},
			fallback: nil,
			expected: mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
		},
		{
			name: "full underlay addresses: prefers public TCP over WSS addresses",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/10.233.99.120/tcp/1635/tls/sni/10-233-99-120.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),  // private WSS
				mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),                                                                                                 // public TCP
				mustMultiaddr("/ip4/104.28.194.73/tcp/32532/tls/sni/104-28-194-73.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"), // public WSS
			},
			fallback: nil,
			expected: mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
		},
		{
			name: "full underlay addresses: full node underlay list selects public TCP",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/10.233.99.120/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip4/10.233.99.120/tcp/1635/tls/sni/10-233-99-120.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip4/104.28.194.73/tcp/32532/tls/sni/104-28-194-73.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip4/127.0.0.1/tcp/1635/tls/sni/127-0-0-1.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip6/::1/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip6/::1/tcp/1635/tls/sni/0--1.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
			},
			fallback: nil,
			expected: mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
		},
		{
			name: "full underlay addresses: WSS only list selects public WSS",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/10.233.99.120/tcp/1635/tls/sni/10-233-99-120.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip4/104.28.194.73/tcp/32532/tls/sni/104-28-194-73.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip4/127.0.0.1/tcp/1635/tls/sni/127-0-0-1.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip6/::1/tcp/1635/tls/sni/0--1.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
			},
			fallback: nil,
			expected: mustMultiaddr("/ip4/104.28.194.73/tcp/32532/tls/sni/104-28-194-73.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
		},
		{
			name: "full underlay addresses: TCP only list selects public TCP",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/10.233.99.120/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip6/::1/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
			},
			fallback: nil,
			expected: mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
		},
		{
			name: "full underlay addresses: IPv6 loopback list with no public addresses",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip6/::1/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
				mustMultiaddr("/ip6/::1/tcp/1635/tls/sni/0--1.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
			},
			fallback: nil,
			expected: mustMultiaddr("/ip6/::1/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
		},
		{
			name: "full underlay addresses: WSS before TCP in input - TCP is still selected due to transport priority",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/104.28.194.73/tcp/32532/tls/sni/104-28-194-73.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"), // public WSS (first in input)
				mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),                                                                                                 // public TCP (second in input)
			},
			fallback: nil,
			// TCP is selected because:
			// 1. Transport priority: TCP (0) > WS (1) > WSS (2)
			// 2. Sorting puts TCP addresses before WSS addresses
			// 3. Both are public, so the first public after sorting (TCP) is returned
			expected: mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
		},
		{
			name: "full underlay addresses: TCP before WSS in input - TCP is selected",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),                                                                                                 // public TCP (first)
				mustMultiaddr("/ip4/104.28.194.73/tcp/32532/tls/sni/104-28-194-73.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"), // public WSS (second)
			},
			fallback: nil,
			// TCP is selected because it has higher transport priority
			expected: mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
		},
		{
			name: "full underlay addresses: private TCP vs public WSS - public WSS is selected",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/10.233.99.120/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),                                                                                                  // private TCP
				mustMultiaddr("/ip4/104.28.194.73/tcp/32532/tls/sni/104-28-194-73.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"), // public WSS
			},
			fallback: nil,
			// Public WSS is selected because there is no public TCP address
			// Priority: public addresses > private addresses, then by transport type
			expected: mustMultiaddr("/ip4/104.28.194.73/tcp/32532/tls/sni/104-28-194-73.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bzz.SelectBestAdvertisedAddress(tt.addrs, tt.fallback)
			if !result.Equal(tt.expected) {
				t.Errorf("SelectBestAdvertisedAddress() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestClassifyTransport(t *testing.T) {
	t.Parallel()

	mustMultiaddr := func(s string) multiaddr.Multiaddr {
		addr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			t.Fatalf("failed to create multiaddr %s: %v", s, err)
		}
		return addr
	}

	tests := []struct {
		name     string
		addr     multiaddr.Multiaddr
		expected bzz.TransportType
	}{
		{
			name:     "plain TCP address",
			addr:     mustMultiaddr("/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
			expected: bzz.TransportTCP,
		},
		{
			name:     "plain TCP IPv6 address",
			addr:     mustMultiaddr("/ip6/::1/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
			expected: bzz.TransportTCP,
		},
		{
			name:     "plain WS address",
			addr:     mustMultiaddr("/ip4/127.0.0.1/tcp/8080/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
			expected: bzz.TransportWS,
		},
		{
			name:     "WSS address with TLS and SNI",
			addr:     mustMultiaddr("/ip4/104.28.194.73/tcp/32532/tls/sni/104-28-194-73.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
			expected: bzz.TransportWSS,
		},
		{
			name:     "WSS IPv6 address",
			addr:     mustMultiaddr("/ip6/::1/tcp/1635/tls/sni/0--1.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"),
			expected: bzz.TransportWSS,
		},
		{
			name:     "UDP address returns unknown",
			addr:     mustMultiaddr("/ip4/127.0.0.1/udp/8080"),
			expected: bzz.TransportUnknown,
		},
		{
			name:     "nil address returns unknown",
			addr:     nil,
			expected: bzz.TransportUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bzz.ClassifyTransport(tt.addr)
			if result != tt.expected {
				t.Errorf("ClassifyTransport() = %v (%s), want %v (%s)", result, result.String(), tt.expected, tt.expected.String())
			}
		})
	}
}

func TestTransportTypePriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		transport bzz.TransportType
		priority  int
	}{
		{bzz.TransportTCP, 0},
		{bzz.TransportWS, 1},
		{bzz.TransportWSS, 2},
		{bzz.TransportUnknown, 3},
	}

	for _, tt := range tests {
		t.Run(tt.transport.String(), func(t *testing.T) {
			if got := tt.transport.Priority(); got != tt.priority {
				t.Errorf("TransportType(%v).Priority() = %d, want %d", tt.transport, got, tt.priority)
			}
		})
	}

	// Verify priority ordering: TCP < WS < WSS < Unknown
	if bzz.TransportTCP.Priority() >= bzz.TransportWS.Priority() {
		t.Error("TCP priority should be lower (better) than WS")
	}
	if bzz.TransportWS.Priority() >= bzz.TransportWSS.Priority() {
		t.Error("WS priority should be lower (better) than WSS")
	}
	if bzz.TransportWSS.Priority() >= bzz.TransportUnknown.Priority() {
		t.Error("WSS priority should be lower (better) than Unknown")
	}
}

func TestSortUnderlaysByPriority(t *testing.T) {
	t.Parallel()

	mustMultiaddr := func(s string) multiaddr.Multiaddr {
		addr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			t.Fatalf("failed to create multiaddr %s: %v", s, err)
		}
		return addr
	}

	const peerID = "QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"
	const sni = "example.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct"

	// Build multiaddr helpers for each transport type.
	tcp := func(host string) string { return host + "/tcp/1634/p2p/" + peerID }
	ws := func(host string) string { return host + "/tcp/1634/ws/p2p/" + peerID }
	wss := func(host string) string { return host + "/tcp/1634/tls/sni/" + sni + "/ws/p2p/" + peerID }

	// Network hosts by type and visibility.
	var (
		ip4Public   = "/ip4/104.28.194.73"
		ip4Private  = "/ip4/10.233.99.46"
		ip4Loopback = "/ip4/127.0.0.1"
		ip6Public   = "/ip6/2001:db8::1"
		ip6Private  = "/ip6/fd00::1"
		ip6Loopback = "/ip6/::1"
		dns4Public  = "/dns4/example.com"
	)

	// Input in reverse-priority order to verify sorting.
	// Score = non-IPv4 penalty (0 or 100) + private/loopback penalty (0 or 10) + transport (0, 1, or 2).
	addrs := []multiaddr.Multiaddr{
		// IPv6 loopback — worst scores (112, 111, 110)
		mustMultiaddr(wss(ip6Loopback)),
		mustMultiaddr(ws(ip6Loopback)),
		mustMultiaddr(tcp(ip6Loopback)),
		// IPv6 private (112, 111, 110)
		mustMultiaddr(wss(ip6Private)),
		mustMultiaddr(ws(ip6Private)),
		mustMultiaddr(tcp(ip6Private)),
		// IPv6 public (102, 101, 100)
		mustMultiaddr(wss(ip6Public)),
		mustMultiaddr(ws(ip6Public)),
		mustMultiaddr(tcp(ip6Public)),
		// DNS4 public (102, 101, 100)
		mustMultiaddr(wss(dns4Public)),
		mustMultiaddr(ws(dns4Public)),
		mustMultiaddr(tcp(dns4Public)),
		// IPv4 loopback (12, 11, 10)
		mustMultiaddr(wss(ip4Loopback)),
		mustMultiaddr(ws(ip4Loopback)),
		mustMultiaddr(tcp(ip4Loopback)),
		// IPv4 private (12, 11, 10)
		mustMultiaddr(wss(ip4Private)),
		mustMultiaddr(ws(ip4Private)),
		mustMultiaddr(tcp(ip4Private)),
		// IPv4 public — best scores (2, 1, 0)
		mustMultiaddr(wss(ip4Public)),
		mustMultiaddr(ws(ip4Public)),
		mustMultiaddr(tcp(ip4Public)),
	}

	sorted := bzz.SortUnderlaysByPriority(addrs)

	// Expected order grouped by score.
	// Within the same score, SliceStable preserves input order.
	expectedOrder := []string{
		// IPv4 public: TCP(0), WS(1), WSS(2)
		tcp(ip4Public), // score: 0
		ws(ip4Public),  // score: 1
		wss(ip4Public), // score: 2
		// IPv4 private: TCP(10), WS(11), WSS(12)
		tcp(ip4Private), // score: 10
		ws(ip4Private),  // score: 11
		wss(ip4Private), // score: 12
		// IPv4 loopback: TCP(20), WS(21), WSS(22)
		tcp(ip4Loopback), // score: 20
		ws(ip4Loopback),  // score: 21
		wss(ip4Loopback), // score: 22
		// IPv6 public and DNS4 public: TCP(100), WS(101), WSS(102)
		// Same scores; stable sort preserves input order (IPv6 before DNS4).
		tcp(ip6Public),  // score: 100
		tcp(dns4Public), // score: 100
		ws(ip6Public),   // score: 101
		ws(dns4Public),  // score: 101
		wss(ip6Public),  // score: 102
		wss(dns4Public), // score: 102
		// IPv6 private: TCP(110), WS(111), WSS(112)
		tcp(ip6Private), // score: 110
		ws(ip6Private),  // score: 111
		wss(ip6Private), // score: 112
		// IPv6 loopback: TCP(120), WS(121), WSS(122)
		tcp(ip6Loopback), // score: 120
		ws(ip6Loopback),  // score: 121
		wss(ip6Loopback), // score: 122
	}

	if len(sorted) != len(expectedOrder) {
		t.Fatalf("expected %d addresses, got %d", len(expectedOrder), len(sorted))
	}
	for i, expected := range expectedOrder {
		exp := mustMultiaddr(expected)
		if !sorted[i].Equal(exp) {
			t.Errorf("position %d: expected %s, got %s", i, expected, sorted[i].String())
		}
	}

	// Verify original slice was not modified.
	if !addrs[0].Equal(mustMultiaddr(wss(ip6Loopback))) {
		t.Error("original slice was modified")
	}
}

func TestTruncateUnderlays(t *testing.T) {
	t.Parallel()

	mustMultiaddr := func(s string) multiaddr.Multiaddr {
		addr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			t.Fatalf("failed to create multiaddr %s: %v", s, err)
		}
		return addr
	}

	t.Run("no truncation needed", func(t *testing.T) {
		addrs := []multiaddr.Multiaddr{
			mustMultiaddr("/ip4/1.2.3.4/tcp/80"),
			mustMultiaddr("/ip4/5.6.7.8/tcp/80"),
		}
		result, truncated := bzz.TruncateUnderlays(addrs)
		if truncated {
			t.Error("expected no truncation")
		}
		if len(result) != 2 {
			t.Errorf("expected 2 addresses, got %d", len(result))
		}
	})

	t.Run("count truncation", func(t *testing.T) {
		addrs := make([]multiaddr.Multiaddr, bzz.MaxUnderlaysPerPeer+5)
		for i := range addrs {
			addrs[i] = mustMultiaddr("/ip4/1.2.3.4/tcp/80")
		}
		result, truncated := bzz.TruncateUnderlays(addrs)
		if !truncated {
			t.Error("expected truncation")
		}
		if len(result) != bzz.MaxUnderlaysPerPeer {
			t.Errorf("expected %d addresses, got %d", bzz.MaxUnderlaysPerPeer, len(result))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		result, truncated := bzz.TruncateUnderlays(nil)
		if truncated {
			t.Error("expected no truncation for nil input")
		}
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	})

	t.Run("truncation preserves priority order", func(t *testing.T) {
		privateTCP := mustMultiaddr("/ip4/10.0.0.1/tcp/80")
		publicTCP := mustMultiaddr("/ip4/8.8.8.8/tcp/80")
		addrs := []multiaddr.Multiaddr{privateTCP, publicTCP}
		result, _ := bzz.TruncateUnderlays(addrs)
		// Public TCP should come first.
		if !result[0].Equal(publicTCP) {
			t.Errorf("expected public TCP first, got %s", result[0].String())
		}
	})

	t.Run("byte budget truncation", func(t *testing.T) {
		// Create enough large WSS addresses that the byte budget (2048) is exceeded
		// before the count cap. Each WSS address is ~136 bytes serialized, so
		// ~16 addresses will exceed 2048 bytes.
		largeAddrs := make([]multiaddr.Multiaddr, 0, 40)
		for i := range 35 {
			ip := fmt.Sprintf("/ip4/%d.%d.%d.%d/tcp/32532/tls/sni/%d-%d-%d-%d.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj",
				1+i/256, 0, 0, i%256, 1+i/256, 0, 0, i%256)
			largeAddrs = append(largeAddrs, mustMultiaddr(ip))
		}

		result, truncated := bzz.TruncateUnderlays(largeAddrs)
		if !truncated {
			t.Error("expected byte-budget truncation")
		}
		if len(result) >= len(largeAddrs) {
			t.Errorf("expected fewer addresses after byte-budget truncation, got %d", len(result))
		}
		if len(result) == 0 {
			t.Error("expected at least one address after truncation")
		}
		// Verify the result fits within count cap.
		if len(result) > bzz.MaxUnderlaysPerPeer {
			t.Errorf("result should not exceed MaxUnderlaysPerPeer, got %d", len(result))
		}
	})
}
