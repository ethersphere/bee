// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"sort"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestAddresses(t *testing.T) {
	s, _, cleanup := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup()

	addrs, err := s.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	if l := len(addrs); l == 0 {
		t.Fatal("no addresses")
	}
}

func TestConnectDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1, cleanup1 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup1()

	s2, overlay2, cleanup2 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup2()

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	addr := addrs[0]

	overlay, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if err := s2.Disconnect(overlay); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)
}

func TestDoubleConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1, cleanup1 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup1()

	s2, overlay2, cleanup2 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup2()

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	addr := addrs[0]

	if _, err := s2.Connect(ctx, addr); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if _, err := s2.Connect(ctx, addr); err == nil {
		t.Fatal("second connect attempt should result with an error")
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)
}

func TestDoubleDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1, cleanup1 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup1()

	s2, overlay2, cleanup2 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup2()

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	addr := addrs[0]

	overlay, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if err := s2.Disconnect(overlay); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)

	if err := s2.Disconnect(overlay); !errors.Is(err, p2p.ErrPeerNotFound) {
		t.Errorf("got error %v, want %v", err, p2p.ErrPeerNotFound)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)
}

func TestReconnectAfterDoubleConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1, cleanup1 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup1()

	s2, overlay2, cleanup2 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup2()

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	addr := addrs[0]

	overlay, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if _, err := s2.Connect(ctx, addr); err == nil {
		t.Fatal("second connect attempt should result with an error")
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)

	overlay, err = s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	if !overlay.Equal(overlay1) {
		t.Errorf("got overlay %s, want %s", overlay, overlay1)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)
}

func TestMultipleConnectDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1, cleanup1 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup1()

	s2, overlay2, cleanup2 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup2()

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	addr := addrs[0]

	overlay, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if err := s2.Disconnect(overlay); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)

	overlay, err = s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if err := s2.Disconnect(overlay); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)
}

func TestConnectDisconnectOnAllAddresses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1, cleanup1 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup1()

	s2, overlay2, cleanup2 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup2()

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addrs {
		overlay, err := s2.Connect(ctx, addr)
		if err != nil {
			t.Fatal(err)
		}

		expectPeers(t, s2, overlay1)
		expectPeersEventually(t, s1, overlay2)

		if err := s2.Disconnect(overlay); err != nil {
			t.Fatal(err)
		}

		expectPeers(t, s2)
		expectPeersEventually(t, s1)
	}
}

func TestDoubleConnectOnAllAddresses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1, cleanup1 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup1()

	s2, overlay2, cleanup2 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup2()

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addrs {
		if _, err := s2.Connect(ctx, addr); err != nil {
			t.Fatal(err)
		}

		expectPeers(t, s2, overlay1)
		expectPeersEventually(t, s1, overlay2)

		if _, err := s2.Connect(ctx, addr); err == nil {
			t.Fatal("second connect attempt should result with an error")
		}

		expectPeers(t, s2)
		expectPeersEventually(t, s1)
	}
}

func TestDifferentNetworkIDs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, _, cleanup1 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup1()

	s2, _, cleanup2 := newService(t, libp2p.Options{NetworkID: 2})
	defer cleanup2()

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	addr := addrs[0]

	if _, err := s2.Connect(ctx, addr); err == nil {
		t.Fatal("connect attempt should result with an error")
	}

	expectPeers(t, s1)
	expectPeers(t, s2)
}

func TestBootnodes(t *testing.T) {
	s1, overlay1, cleanup1 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup1()

	s2, overlay2, cleanup2 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup2()

	addrs1, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	addrs2, err := s2.Addresses()
	if err != nil {
		t.Fatal(err)
	}

	s3, overlay3, cleanup3 := newService(t, libp2p.Options{
		NetworkID: 1,
		Bootnodes: []string{
			addrs1[0].String(),
			addrs2[0].String(),
		},
	})
	defer cleanup3()

	expectPeers(t, s3, overlay1, overlay2)
	expectPeers(t, s1, overlay3)
	expectPeers(t, s2, overlay3)
}

// newService constructs a new libp2p service.
func newService(t *testing.T, o libp2p.Options) (s *libp2p.Service, overlay swarm.Address, cleanup func()) {
	t.Helper()

	// disable ws until the race condition in:
	// github.com/gorilla/websocket@v1.4.1/conn.go:614
	// github.com/gorilla/websocket@v1.4.1/conn.go:781
	// using github.com/libp2p/go-libp2p-transport-upgrader@v0.1.1
	// is solved
	o.DisableWS = true

	if o.PrivateKey == nil {
		var err error
		o.PrivateKey, err = crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
	}

	if o.Overlay.IsZero() {
		var err error
		swarmPK, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		o.Overlay = crypto.NewAddress(swarmPK.PublicKey)
	}

	if o.Logger == nil {
		o.Logger = logging.New(ioutil.Discard, 0)
	}

	if o.Addr == "" {
		o.Addr = ":0"
	}

	ctx, cancel := context.WithCancel(context.Background())
	s, err := libp2p.New(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	return s, o.Overlay, func() {
		cancel()
		s.Close()
	}
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
// retires. It is supposed to be used to validate asynchronous connecting on the
// peer that is connected to.
func expectPeersEventually(t *testing.T, s *libp2p.Service, addrs ...swarm.Address) {
	t.Helper()

	var peers []p2p.Peer
	for i := 0; i < 100; i++ {
		peers = s.Peers()
		if len(peers) == len(addrs) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

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
