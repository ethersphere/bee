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
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

func TestAddresses(t *testing.T) {
	s, _ := newService(t, 1, libp2p.Options{})

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

	s1, overlay1 := newService(t, 1, libp2p.Options{})

	s2, overlay2 := newService(t, 1, libp2p.Options{})

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

	s1, overlay1 := newService(t, 1, libp2p.Options{})

	s2, overlay2 := newService(t, 1, libp2p.Options{})

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if _, err := s2.Connect(ctx, addr); !errors.Is(err, p2p.ErrAlreadyConnected) {
		t.Fatalf("expected %s error, got %s error", p2p.ErrAlreadyConnected, err)
	}

	expectPeers(t, s2, overlay1)
	expectPeers(t, s1, overlay2)
}

func TestDoubleDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2p.Options{})

	s2, overlay2 := newService(t, 1, libp2p.Options{})

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

func TestMultipleConnectDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2p.Options{})

	s2, overlay2 := newService(t, 1, libp2p.Options{})

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

	s1, overlay1 := newService(t, 1, libp2p.Options{})

	s2, overlay2 := newService(t, 1, libp2p.Options{})

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

	s1, overlay1 := newService(t, 1, libp2p.Options{})

	s2, overlay2 := newService(t, 1, libp2p.Options{})

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

		if _, err := s2.Connect(ctx, addr); !errors.Is(err, p2p.ErrAlreadyConnected) {
			t.Fatalf("expected %s error, got %s error", p2p.ErrAlreadyConnected, err)
		}

		expectPeers(t, s2, overlay1)
		expectPeers(t, s1, overlay2)

		if err := s2.Disconnect(overlay1); err != nil {
			t.Fatal(err)
		}

		expectPeers(t, s2)
		expectPeersEventually(t, s1)
	}
}

func TestDifferentNetworkIDs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, _ := newService(t, 1, libp2p.Options{})

	s2, _ := newService(t, 2, libp2p.Options{})

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err == nil {
		t.Fatal("connect attempt should result with an error")
	}

	expectPeers(t, s1)
	expectPeers(t, s2)
}

func TestConnectWithDisabledQUICAndWSTransports(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2p.Options{
		DisableQUIC: true,
		DisableWS:   true,
	})

	s2, overlay2 := newService(t, 1, libp2p.Options{
		DisableQUIC: true,
		DisableWS:   true,
	})

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)
}

// TestConnectRepeatHandshake tests if handshake was attempted more then once by the same peer
func TestConnectRepeatHandshake(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2p.Options{})

	s2, overlay2 := newService(t, 1, libp2p.Options{})

	addr := serviceUnderlayAddress(t, s1)

	_, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	info, err := libp2ppeer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		t.Fatal(err)
	}

	stream, err := s2.NewStreamForPeerID(info.ID, handshake.ProtocolName, handshake.ProtocolVersion, handshake.StreamName)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s2.HandshakeService().Handshake(libp2p.NewStream(stream)); err == nil {
		t.Fatalf("expected stream error")
	}

	expectPeersEventually(t, s2)
	expectPeersEventually(t, s1)
}
