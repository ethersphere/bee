// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"sort"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/multiformats/go-multiaddr"
)

type libp2pServiceOpts struct {
	Logger      log.Logger
	Addressbook addressbook.Interface
	PrivateKey  *ecdsa.PrivateKey
	MockPeerKey *ecdsa.PrivateKey
	libp2pOpts  libp2p.Options
	lightNodes  *lightnode.Container
	notifier    p2p.PickyNotifier
}

// newService constructs a new libp2p service.
func newService(t *testing.T, networkID uint64, o libp2pServiceOpts) (s *libp2p.Service, overlay swarm.Address) {
	t.Helper()

	swarmKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	nonce := common.HexToHash("0x1").Bytes()

	overlay, err = crypto.NewOverlayAddress(swarmKey.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	addr := ":0"

	if o.Logger == nil {
		o.Logger = log.Noop
	}

	statestore := mock.NewStateStore()
	if o.Addressbook == nil {
		o.Addressbook = addressbook.New(statestore)
	}

	if o.PrivateKey == nil {
		libp2pKey, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}

		o.PrivateKey = libp2pKey
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if o.lightNodes == nil {
		o.lightNodes = lightnode.NewContainer(overlay)
	}
	opts := o.libp2pOpts
	opts.Nonce = nonce

	s, err = libp2p.New(ctx, crypto.NewDefaultSigner(swarmKey), networkID, overlay, addr, o.Addressbook, statestore, o.lightNodes, o.Logger, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, s)

	if o.notifier != nil {
		s.SetPickyNotifier(o.notifier)
	}

	_ = s.Ready()

	return s, overlay
}

// expectPeers validates that peers with addresses are connected.
func expectPeers(t *testing.T, s *libp2p.Service, addrs ...swarm.Address) {
	t.Helper()

	peers := s.Peers()

	if len(peers) != len(addrs) {
		t.Fatalf("got peers %v, want %v", len(peers), len(addrs))
	}

	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i].Bytes(), addrs[j].Bytes()) == -1
	})
	sort.Slice(peers, func(i, j int) bool {
		return bytes.Compare(peers[i].Address.Bytes(), peers[j].Address.Bytes()) == -1
	})

	for i, got := range peers {
		want := addrs[i]
		if !got.Address.Equal(want) {
			t.Errorf("got %v peer %s, want %s", i, got.Address, want)
		}
	}
}

// expectPeersEventually validates that peers with addresses are connected with
// retries. It is supposed to be used to validate asynchronous connecting on the
// peer that is connected to.
func expectPeersEventually(t *testing.T, s *libp2p.Service, addrs ...swarm.Address) {
	t.Helper()

	var peers []p2p.Peer
	err := spinlock.Wait(time.Second, func() bool {
		peers = s.Peers()
		return len(peers) == len(addrs)

	})
	if err != nil {
		t.Fatalf("timed out waiting for peers, got  %v, want %v", len(peers), len(addrs))
	}

	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i].Bytes(), addrs[j].Bytes()) == -1
	})
	sort.Slice(peers, func(i, j int) bool {
		return bytes.Compare(peers[i].Address.Bytes(), peers[j].Address.Bytes()) == -1
	})

	for i, got := range peers {
		want := addrs[i]
		if !got.Address.Equal(want) {
			t.Errorf("got %v peer %s, want %s", i, got.Address, want)
		}
	}
}

func serviceUnderlayAddress(t *testing.T, s *libp2p.Service) []multiaddr.Multiaddr {
	t.Helper()

	addrs, err := s.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	return addrs
}

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
			name: "prefers non-loopback when no public addresses",
			addrs: []multiaddr.Multiaddr{
				mustMultiaddr("/ip4/127.0.0.1/tcp/8080"),   // loopback
				mustMultiaddr("/ip4/192.168.1.1/tcp/8080"), // private but not loopback
				mustMultiaddr("/ip4/10.0.0.1/tcp/8080"),    // private but not loopback
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := libp2p.SelectBestAdvertisedAddress(tt.addrs, tt.fallback)
			if !result.Equal(tt.expected) {
				t.Errorf("SelectBestAdvertisedAddress() = %v, want %v", result, tt.expected)
			}
		})
	}
}
