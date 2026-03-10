// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"testing"

	ma "github.com/multiformats/go-multiaddr"
)

func mustMultiaddr(t *testing.T, s string) ma.Multiaddr {
	t.Helper()
	addr, err := ma.NewMultiaddr(s)
	if err != nil {
		t.Fatalf("failed to create multiaddr from %q: %v", s, err)
	}
	return addr
}

func TestFilterSupportedAddresses(t *testing.T) {
	t.Parallel()

	// Plain TCP addresses (IPv4)
	tcpPrivate := "/ip4/10.233.99.120/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"
	tcpPublic := "/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"
	tcpLoopback := "/ip4/127.0.0.1/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"
	tcpIPv6Loopback := "/ip6/::1/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"

	// WSS addresses with TLS and SNI (full underlay format)
	wssPrivate := "/ip4/10.233.99.120/tcp/1635/tls/sni/10-233-99-120.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"
	wssPublic := "/ip4/104.28.194.73/tcp/32532/tls/sni/104-28-194-73.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"
	wssLoopback := "/ip4/127.0.0.1/tcp/1635/tls/sni/127-0-0-1.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"
	wssIPv6Loopback := "/ip6/::1/tcp/1635/tls/sni/0--1.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"

	// Plain WS address (no TLS)
	wsPlain := "/ip4/127.0.0.1/tcp/1635/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj"

	// All TCP addresses
	allTCP := []string{tcpPrivate, tcpPublic, tcpLoopback, tcpIPv6Loopback}
	// All WSS addresses
	allWSS := []string{wssPrivate, wssPublic, wssLoopback, wssIPv6Loopback}

	tests := []struct {
		name          string
		hasTCP        bool
		hasWS         bool
		hasWSS        bool
		inputAddrs    []string
		expectedCount int
	}{
		{
			name:          "TCP only transport accepts all TCP addresses",
			hasTCP:        true,
			hasWS:         false,
			hasWSS:        false,
			inputAddrs:    allTCP,
			expectedCount: 4,
		},
		{
			name:          "TCP only transport rejects WSS addresses",
			hasTCP:        true,
			hasWS:         false,
			hasWSS:        false,
			inputAddrs:    allWSS,
			expectedCount: 0,
		},
		{
			name:          "WSS only transport accepts all WSS addresses",
			hasTCP:        false,
			hasWS:         false,
			hasWSS:        true,
			inputAddrs:    allWSS,
			expectedCount: 4,
		},
		{
			name:          "WSS only transport rejects TCP addresses",
			hasTCP:        false,
			hasWS:         false,
			hasWSS:        true,
			inputAddrs:    allTCP,
			expectedCount: 0,
		},
		{
			name:          "TCP and WSS transports accept mixed addresses",
			hasTCP:        true,
			hasWS:         false,
			hasWSS:        true,
			inputAddrs:    append(allTCP, allWSS...),
			expectedCount: 8,
		},
		{
			name:          "WS transport accepts plain WS but not WSS",
			hasTCP:        false,
			hasWS:         true,
			hasWSS:        false,
			inputAddrs:    []string{wsPlain, wssPublic},
			expectedCount: 1,
		},
		{
			name:          "WSS transport does not accept plain WS",
			hasTCP:        false,
			hasWS:         false,
			hasWSS:        true,
			inputAddrs:    []string{wsPlain},
			expectedCount: 0,
		},
		{
			name:          "No transports reject all addresses",
			hasTCP:        false,
			hasWS:         false,
			hasWSS:        false,
			inputAddrs:    append(allTCP, allWSS...),
			expectedCount: 0,
		},
		{
			name:          "Empty input returns empty output",
			hasTCP:        true,
			hasWS:         true,
			hasWSS:        true,
			inputAddrs:    []string{},
			expectedCount: 0,
		},
		{
			name:          "Real node addresses with TCP and WSS",
			hasTCP:        true,
			hasWS:         false,
			hasWSS:        true,
			inputAddrs:    []string{tcpPrivate, wssPrivate, tcpPublic, wssPublic, tcpLoopback, wssLoopback, tcpIPv6Loopback, wssIPv6Loopback},
			expectedCount: 8,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a service for testing
			s, _ := newService(t, 1, libp2pServiceOpts{})

			// Set transport flags
			s.SetTransportFlags(tc.hasTCP, tc.hasWS, tc.hasWSS)

			// Create multiaddrs from strings
			addrs := make([]ma.Multiaddr, len(tc.inputAddrs))
			for i, addrStr := range tc.inputAddrs {
				addrs[i] = mustMultiaddr(t, addrStr)
			}

			// Filter addresses
			filtered := s.FilterSupportedAddresses(addrs)

			// Verify count
			if len(filtered) != tc.expectedCount {
				t.Errorf("expected %d addresses, got %d", tc.expectedCount, len(filtered))
			}
		})
	}
}

func TestFilterSupportedAddresses_FullUnderlayAddresses(t *testing.T) {
	t.Parallel()

	// Complete set of underlay addresses from a real full underlay node
	fullAddrs := []string{
		"/ip4/10.233.99.120/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj",
		"/ip4/10.233.99.120/tcp/1635/tls/sni/10-233-99-120.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj",
		"/ip4/104.28.194.73/tcp/32002/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj",
		"/ip4/104.28.194.73/tcp/32532/tls/sni/104-28-194-73.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj",
		"/ip4/127.0.0.1/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj",
		"/ip4/127.0.0.1/tcp/1635/tls/sni/127-0-0-1.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj",
		"/ip6/::1/tcp/1634/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj",
		"/ip6/::1/tcp/1635/tls/sni/0--1.k2k4r8pr3m3aug5nudg2y039qfj2gxw6wnlx0e0ghzxufcn38soyp9z4.libp2p.direct/ws/p2p/QmfSx1ujzboapD5h2CiqTJqUy46FeTDwXBszB3XUCfKEEj",
	}

	// Create multiaddrs
	addrs := make([]ma.Multiaddr, len(fullAddrs))
	for i, addrStr := range fullAddrs {
		addrs[i] = mustMultiaddr(t, addrStr)
	}

	t.Run("TCP and WSS enabled accepts all full underlay addresses", func(t *testing.T) {
		t.Parallel()
		s, _ := newService(t, 1, libp2pServiceOpts{})
		s.SetTransportFlags(true, false, true)

		filtered := s.FilterSupportedAddresses(addrs)
		// 4 TCP addresses + 4 WSS addresses = 8
		if len(filtered) != 8 {
			t.Errorf("expected 8 addresses, got %d", len(filtered))
		}
	})

	t.Run("TCP only accepts half of full underlay addresses", func(t *testing.T) {
		t.Parallel()
		s, _ := newService(t, 1, libp2pServiceOpts{})
		s.SetTransportFlags(true, false, false)

		filtered := s.FilterSupportedAddresses(addrs)
		// Only 4 TCP addresses
		if len(filtered) != 4 {
			t.Errorf("expected 4 addresses (TCP only), got %d", len(filtered))
		}
	})

	t.Run("WSS only accepts half of full underlay addresses", func(t *testing.T) {
		t.Parallel()
		s, _ := newService(t, 1, libp2pServiceOpts{})
		s.SetTransportFlags(false, false, true)

		filtered := s.FilterSupportedAddresses(addrs)
		// Only 4 WSS addresses
		if len(filtered) != 4 {
			t.Errorf("expected 4 addresses (WSS only), got %d", len(filtered))
		}
	})
}
