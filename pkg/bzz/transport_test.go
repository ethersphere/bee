// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz_test

import (
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
			name: "prefers non-private when no public addresses",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),   // loopback
				mustMultiaddr("/ip4/192.168.1.1/tcp/8080"), // private but not loopback
				mustMultiaddr("/ip4/10.0.0.1/tcp/8080"),    // private but not loopback
			},
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/9999"),
			expected: mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),
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
			name: "handles IPv6 addresses",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip6/::1/tcp/8080"),         // loopback
				mustMultiaddr("/ip6/2001:db8::1/tcp/8080"), // public IPv6
				mustMultiaddr("/ip4/192.168.1.1/tcp/8080"), // private IPv4
			},
			fallback: mustMultiaddr("/ip4/127.0.0.1/tcp/9999"),
			expected: mustMultiaddr("/ip6/2001:db8::1/tcp/8080"),
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
