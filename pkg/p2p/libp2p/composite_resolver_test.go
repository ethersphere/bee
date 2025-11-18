// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake"
	ma "github.com/multiformats/go-multiaddr"
)

// mockResolver is a mock implementation of handshake.AdvertisableAddressResolver.
type mockResolver struct {
	resolveFunc func(ma.Multiaddr) (ma.Multiaddr, error)
	called      bool
}

func (m *mockResolver) Resolve(addr ma.Multiaddr) (ma.Multiaddr, error) {
	m.called = true
	if m.resolveFunc != nil {
		return m.resolveFunc(addr)
	}
	return addr, nil
}

func TestCompositeAddressResolver(t *testing.T) {
	t.Parallel()

	resolvedAddr, _ := ma.NewMultiaddr("/ip4/1.1.1.1/tcp/1634")

	tests := []struct {
		name          string
		addr          string
		setup         func() (handshake.AdvertisableAddressResolver, *mockResolver, *mockResolver)
		wantTCPCalled bool
		wantWSSCalled bool
		wantErr       error
		wantResolved  bool
	}{
		{
			name: "wss address",
			addr: "/ip4/10.233.99.40/tcp/1635/tls/sni/*.libp2p.direct/ws/p2p/QmWbXocGMpfa8zApx9kCNwfmc35bbRJv136bdtuQjbR4wL",
			setup: func() (handshake.AdvertisableAddressResolver, *mockResolver, *mockResolver) {
				tcpR := &mockResolver{}
				wssR := &mockResolver{
					resolveFunc: func(addr ma.Multiaddr) (ma.Multiaddr, error) {
						return resolvedAddr, nil
					},
				}
				return libp2p.NewCompositeAddressResolver(tcpR, wssR), tcpR, wssR
			},
			wantWSSCalled: true,
			wantResolved:  true,
		},
		{
			name: "tcp address",
			addr: "/ip4/10.233.99.40/tcp/1634/p2p/QmWbXocGMpfa8zApx9kCNwfmc35bbRJv136bdtuQjbR4wL",
			setup: func() (handshake.AdvertisableAddressResolver, *mockResolver, *mockResolver) {
				tcpR := &mockResolver{
					resolveFunc: func(addr ma.Multiaddr) (ma.Multiaddr, error) {
						return resolvedAddr, nil
					},
				}
				wssR := &mockResolver{}
				return libp2p.NewCompositeAddressResolver(tcpR, wssR), tcpR, wssR
			},
			wantTCPCalled: true,
			wantResolved:  true,
		},
		{
			name: "deprecated wss address",
			addr: "/ip4/10.233.99.40/tcp/1635/wss/p2p/QmWbXocGMpfa8zApx9kCNwfmc35bbRJv136bdtuQjbR4wL",
			setup: func() (handshake.AdvertisableAddressResolver, *mockResolver, *mockResolver) {
				tcpR := &mockResolver{
					resolveFunc: func(addr ma.Multiaddr) (ma.Multiaddr, error) {
						return resolvedAddr, nil
					},
				}
				wssR := &mockResolver{}
				return libp2p.NewCompositeAddressResolver(tcpR, wssR), tcpR, wssR
			},
			wantTCPCalled: true,
			wantResolved:  true,
		},
		{
			name: "ws without tls",
			addr: "/ip4/10.233.99.40/tcp/1635/ws/p2p/QmWbXocGMpfa8zApx9kCNwfmc35bbRJv136bdtuQjbR4wL",
			setup: func() (handshake.AdvertisableAddressResolver, *mockResolver, *mockResolver) {
				tcpR := &mockResolver{
					resolveFunc: func(addr ma.Multiaddr) (ma.Multiaddr, error) {
						return resolvedAddr, nil
					},
				}
				wssR := &mockResolver{}
				return libp2p.NewCompositeAddressResolver(tcpR, wssR), tcpR, wssR
			},
			wantTCPCalled: true,
			wantResolved:  true,
		},
		{
			name: "nil wss resolver",
			addr: "/ip4/10.233.99.40/tcp/1635/tls/sni/*.libp2p.direct/ws/p2p/QmWbXocGMpfa8zApx9kCNwfmc35bbRJv136bdtuQjbR4wL",
			setup: func() (handshake.AdvertisableAddressResolver, *mockResolver, *mockResolver) {
				tcpR := &mockResolver{}
				return libp2p.NewCompositeAddressResolver(tcpR, nil), tcpR, nil
			},
			wantWSSCalled: false,
			wantTCPCalled: false,
			wantResolved:  false,
		},
		{
			name: "nil tcp resolver",
			addr: "/ip4/10.233.99.40/tcp/1634/p2p/QmWbXocGMpfa8zApx9kCNwfmc35bbRJv136bdtuQjbR4wL",
			setup: func() (handshake.AdvertisableAddressResolver, *mockResolver, *mockResolver) {
				wssR := &mockResolver{}
				return libp2p.NewCompositeAddressResolver(nil, wssR), nil, wssR
			},
			wantTCPCalled: false,
			wantWSSCalled: false,
			wantResolved:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resolver, tcpResolver, wssResolver := tc.setup()

			addr, err := ma.NewMultiaddr(tc.addr)
			if err != nil {
				t.Fatal(err)
			}

			got, err := resolver.Resolve(addr)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("got error %v, want %v", err, tc.wantErr)
			}

			if tcpResolver != nil && tc.wantTCPCalled != tcpResolver.called {
				t.Errorf("tcpResolver called: got %v, want %v", tcpResolver.called, tc.wantTCPCalled)
			}
			if wssResolver != nil && tc.wantWSSCalled != wssResolver.called {
				t.Errorf("wssResolver called: got %v, want %v", wssResolver.called, tc.wantWSSCalled)
			}

			if tc.wantResolved {
				if !got.Equal(resolvedAddr) {
					t.Errorf("got resolved addr %s, want %s", got, resolvedAddr)
				}
			} else {
				if !got.Equal(addr) {
					t.Errorf("got addr %s, want %s", got, addr)
				}
			}
		})
	}
}
