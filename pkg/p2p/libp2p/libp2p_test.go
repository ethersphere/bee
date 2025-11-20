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
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

)

type libp2pServiceOpts struct {
	Logger      log.Logger
	Addressbook addressbook.Interface
	PrivateKey  *ecdsa.PrivateKey
	MockPeerKey *ecdsa.PrivateKey
	libp2pOpts  libp2p.Options
	lightNodes  *lightnode.Container
	notifier    p2p.PickyNotifier
	CertManager libp2p.CertificateManager
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

	if o.CertManager != nil {
		opts.CertManager = o.CertManager
	}

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

func TestRewriteForgeWebSocketDomain(t *testing.T) {
	testCases := []struct {
		name         string
		input        string
		peerID       string
		forgeDomain  string
		expected     string
		produceShort bool
	}{
		{
			name:        "rewrite forge websocket domain - ip4",
			input:       "/ip4/207.154.217.73/tcp/1635/tls/sni/*.libp2p.direct/ws/p2p/Qma9wTqw9LFpxPG3UeJvFse6HsVMLcDbyKaoHb5gXBAHru",
			peerID:      "Qma9wTqw9LFpxPG3UeJvFse6HsVMLcDbyKaoHb5gXBAHru",
			forgeDomain: "libp2p.direct",
			expected:    "/ip4/207.154.217.73/tcp/1635/tls/sni/207-154-217-73.k2k4r8nsj60mzz2w7j4ux06zj16jmj08eim0hzutdrq48x8ft0rpqy3a.libp2p.direct/ws/p2p/Qma9wTqw9LFpxPG3UeJvFse6HsVMLcDbyKaoHb5gXBAHru",
			produceShort: false,
		},
		{
			name:        "rewrite forge websocket domain - ip6",
			input:       "/ip6/::1/tcp/1635/tls/sni/*.libp2p.direct/ws/p2p/Qma9wTqw9LFpxPG3UeJvFse6HsVMLcDbyKaoHb5gXBAHru",
			peerID:      "Qma9wTqw9LFpxPG3UeJvFse6HsVMLcDbyKaoHb5gXBAHru",
			forgeDomain: "libp2p.direct",
			expected:    "/ip6/::1/tcp/1635/tls/sni/0--1.k2k4r8nsj60mzz2w7j4ux06zj16jmj08eim0hzutdrq48x8ft0rpqy3a.libp2p.direct/ws/p2p/Qma9wTqw9LFpxPG3UeJvFse6HsVMLcDbyKaoHb5gXBAHru",
			produceShort: false,
		},
		{
			name:        "leave other multiaddress unchanged",
			input:       "/ip6/::1/tcp/1634/p2p/Qma9wTqw9LFpxPG3UeJvFse6HsVMLcDbyKaoHb5gXBAHru",
			peerID:      "Qma9wTqw9LFpxPG3UeJvFse6HsVMLcDbyKaoHb5gXBAHru",
			forgeDomain: "libp2p.direct",
			expected:    "/ip6/::1/tcp/1634/p2p/Qma9wTqw9LFpxPG3UeJvFse6HsVMLcDbyKaoHb5gXBAHru",
			produceShort: false,
		},
		{
			name:        "rewrite forge websocket domain - short form",
			input:       "/ip4/207.154.217.73/tcp/1635/tls/sni/*.libp2p.direct/ws/p2p/Qma9wTqw9LFpxPG3UeJvFse6HsVMLcDbyKaoHb5gXBAHru",
			peerID:      "Qma9wTqw9LFpxPG3UeJvFse6HsVMLcDbyKaoHb5gXBAHru",
			forgeDomain: "libp2p.direct",
			expected:    "/dns4/207-154-217-73.k2k4r8nsj60mzz2w7j4ux06zj16jmj08eim0hzutdrq48x8ft0rpqy3a.libp2p.direct/tcp/1635/tls/ws/p2p/Qma9wTqw9LFpxPG3UeJvFse6HsVMLcDbyKaoHb5gXBAHru",
			produceShort: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inputMa, err := multiaddr.NewMultiaddr(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			peerID, err := libp2ppeer.Decode(tc.peerID)
			if err != nil {
				t.Fatal(err)
			}

			// Call the function under test
			rewrittenAddrs, err := libp2p.RewriteForgeWebSocketDomain([]multiaddr.Multiaddr{inputMa}, peerID, tc.forgeDomain, tc.produceShort)
			if err != nil {
				t.Fatal(err)
			}

			if len(rewrittenAddrs) != 1 {
				t.Fatalf("expected 1 rewritten address, got %d", len(rewrittenAddrs))
			}

			if rewrittenAddrs[0].String() != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, rewrittenAddrs[0].String())
			}
		})
	}
}