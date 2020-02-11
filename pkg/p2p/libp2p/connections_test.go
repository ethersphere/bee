// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
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

	addr := serviceUnderlayAddress(t, s1)

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

	addr := serviceUnderlayAddress(t, s1)

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

	addr := serviceUnderlayAddress(t, s1)

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

	addr := serviceUnderlayAddress(t, s1)

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

	addr := serviceUnderlayAddress(t, s1)

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

	addr := serviceUnderlayAddress(t, s1)

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

func TestConnectWithDisabledQUICAndWSTransports(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1, cleanup1 := newService(t, libp2p.Options{
		NetworkID:   1,
		DisableQUIC: true,
		DisableWS:   true,
	})
	defer cleanup1()

	s2, overlay2, cleanup2 := newService(t, libp2p.Options{
		NetworkID:   1,
		DisableQUIC: true,
		DisableWS:   true,
	})
	defer cleanup2()

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)
}
