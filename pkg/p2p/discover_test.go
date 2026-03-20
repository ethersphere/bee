// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p_test

import (
	"context"
	"testing"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/v2/pkg/p2p"
)

// TestTCPPreferenceOrdering verifies that sortAddrsByTCPPreference places TCP
// addresses before non-TCP addresses regardless of input order.
func TestTCPPreferenceOrdering(t *testing.T) {
	t.Parallel()

	mustAddr := func(s string) ma.Multiaddr {
		a, err := ma.NewMultiaddr(s)
		if err != nil {
			t.Fatalf("parse multiaddr %q: %v", s, err)
		}
		return a
	}

	hasTCP := func(a ma.Multiaddr) bool {
		for _, p := range a.Protocols() {
			if p.Code == ma.P_TCP {
				return true
			}
		}
		return false
	}

	for _, tc := range []struct {
		name  string
		input []string
	}{
		{
			name: "quic before tcp",
			input: []string{
				"/ip4/1.2.3.4/udp/1234/quic-v1",
				"/ip4/1.2.3.4/tcp/1234",
			},
		},
		{
			name: "mixed quic tcp ws",
			input: []string{
				"/ip4/1.2.3.4/udp/1234/quic-v1",
				"/ip4/1.2.3.4/tcp/1234/ws",
				"/ip4/1.2.3.4/tcp/1234",
			},
		},
		{
			name: "all non-tcp",
			input: []string{
				"/ip4/1.2.3.4/udp/1234/quic-v1",
				"/ip4/5.6.7.8/udp/5678/quic-v1",
			},
		},
		{
			name: "all tcp",
			input: []string{
				"/ip4/1.2.3.4/tcp/1234",
				"/ip4/5.6.7.8/tcp/5678",
			},
		},
		{
			// Real addresses from _dnsaddr.bee-0.testnet.ethswarm.org, placed
			// with TLS/WS first so the sort must move the plain TCP entry ahead.
			name: "real testnet bee-0 tls before tcp",
			input: []string{
				"/ip4/49.12.172.37/tcp/32550/tls/sni/49-12-172-37.k2k4r8norg8tz5giqfwkcqyrbbtyoikcxaioqjwckz6lgbvhbs7yswy5.libp2p.direct/ws/p2p/QmZsYCbkUXWpfR34PmUwMJvHwJtGfbcMMoAp1G2EydkpRA",
				"/ip4/49.12.172.37/tcp/32490/p2p/QmZsYCbkUXWpfR34PmUwMJvHwJtGfbcMMoAp1G2EydkpRA",
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			addrs := make([]ma.Multiaddr, len(tc.input))
			for i, s := range tc.input {
				addrs[i] = mustAddr(s)
			}

			p2p.SortAddrsByTCPPreference(addrs)

			// Once we've seen a non-TCP address, no TCP address may follow it.
			seenNonTCP := false
			for _, a := range addrs {
				if !hasTCP(a) {
					seenNonTCP = true
				} else if seenNonTCP {
					t.Errorf("TCP address %s appears after a non-TCP address", a)
				}
			}
		})
	}
}

// TestDiscoverDNS performs a real DNS resolution of the testnet and mainnet
// bootnodes using p2p.Discover with the default DNS resolver.
func TestDiscoverDNS(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		bootnode string
	}{
		{
			name:    "testnet",
			bootnode: "/dnsaddr/testnet.ethswarm.org",
		},
		{
			name:    "mainnet",
			bootnode: "/dnsaddr/mainnet.ethswarm.org",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			addr, err := ma.NewMultiaddr(tc.bootnode)
			if err != nil {
				t.Fatalf("parse multiaddr %q: %v", tc.bootnode, err)
			}

			var resolved []ma.Multiaddr
			_, err = p2p.Discover(context.Background(), addr, func(a ma.Multiaddr) (bool, error) {
				resolved = append(resolved, a)
				return false, nil
			})
			if err != nil {
				t.Fatalf("Discover(%q): %v", tc.bootnode, err)
			}

			if len(resolved) == 0 {
				t.Fatalf("Discover(%q): resolved no addresses", tc.bootnode)
			}

			t.Logf("resolved %d address(es) for %s:", len(resolved), tc.name)
			for _, a := range resolved {
				t.Logf("  %s", a)
			}
		})
	}
}
