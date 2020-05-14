// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
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

	if _, err := s2.Connect(ctx, addr); !errors.Is(err, p2p.ErrAlreadyConnected) {
		t.Fatalf("expected %s error, got %s error", p2p.ErrAlreadyConnected, err)
	}

	expectPeers(t, s2, overlay1)
	expectPeers(t, s1, overlay2)
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

// TestConnectRepeatHandshake tests if handshake was attempted more then once by the same peer
func TestConnectRepeatHandshake(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1, cleanup1 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup1()

	s2, overlay2, cleanup2 := newService(t, libp2p.Options{NetworkID: 1})
	defer cleanup2()

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

func TestTopologyNotifiee(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mtx                sync.Mutex
		n1connectedAddr    swarm.Address
		n1disconnectedAddr swarm.Address
		n2connectedAddr    swarm.Address
		n2disconnectedAddr swarm.Address

		n1c = func(_ context.Context, a swarm.Address) error {
			mtx.Lock()
			defer mtx.Unlock()
			expectZeroAddress(t, n1connectedAddr) // fail if set more than once
			n1connectedAddr = a
			return nil
		}
		n1d = func(a swarm.Address) {
			mtx.Lock()
			defer mtx.Unlock()
			n1disconnectedAddr = a
		}

		n2c = func(_ context.Context, a swarm.Address) error {
			mtx.Lock()
			defer mtx.Unlock()
			expectZeroAddress(t, n2connectedAddr) // fail if set more than once
			n2connectedAddr = a
			return nil
		}
		n2d = func(a swarm.Address) {
			mtx.Lock()
			defer mtx.Unlock()
			n2disconnectedAddr = a
		}
	)
	s1, overlay1, cleanup1 := newService(t, libp2p.Options{NetworkID: 1, Notifiee: mockNotifiee(n1c, n1d)})
	defer cleanup1()
	s2, overlay2, cleanup2 := newService(t, libp2p.Options{NetworkID: 1, Notifiee: mockNotifiee(n2c, n2d)})
	defer cleanup2()

	addr := serviceUnderlayAddress(t, s1)

	// s2 connects to s1, thus the notifiee on s1 should be called on Connect
	overlay, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	// expect that n1 notifee called with s2 overlay
	waitAddrSet(t, &n1connectedAddr, &mtx, overlay2)

	mtx.Lock()
	expectZeroAddress(t, n1disconnectedAddr, n2connectedAddr, n2disconnectedAddr)
	mtx.Unlock()

	// s2 disconnects from s1 so s1 disconnect notifiee should be called
	if err := s2.Disconnect(overlay); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)
	waitAddrSet(t, &n1disconnectedAddr, &mtx, overlay2)

	// note that both n1disconnect and n2disconnect callbacks are called after just
	// one disconnect. this is due to the fact the when the libp2p abstraction is explicitly
	// called to disconnect from a peer, it will also notify the topology notifiee, since
	// peer disconnections can also result from components from outside the bound of the
	// topology driver
	mtx.Lock()
	expectZeroAddress(t, n2connectedAddr)
	mtx.Unlock()

	addr2 := serviceUnderlayAddress(t, s2)
	// s1 connects to s2, thus the notifiee on s2 should be called on Connect
	o2, err := s1.Connect(ctx, addr2)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s1, overlay2)
	expectPeersEventually(t, s2, overlay1)
	waitAddrSet(t, &n2connectedAddr, &mtx, overlay1)

	// s1 disconnects from s2 so s2 disconnect notifiee should be called
	if err := s1.Disconnect(o2); err != nil {
		t.Fatal(err)
	}
	expectPeers(t, s1)
	expectPeersEventually(t, s2)
	waitAddrSet(t, &n2disconnectedAddr, &mtx, overlay1)
}

func waitAddrSet(t *testing.T, addr *swarm.Address, mtx *sync.Mutex, exp swarm.Address) {
	t.Helper()
	for i := 0; i < 20; i++ {
		mtx.Lock()
		if addr.Equal(exp) {
			mtx.Unlock()
			return
		}
		mtx.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for address to be set")
}

type notifiee struct {
	connected    func(context.Context, swarm.Address) error
	disconnected func(swarm.Address)
}

func (n *notifiee) Connected(c context.Context, a swarm.Address) error {
	return n.connected(c, a)
}

func (n *notifiee) Disconnected(a swarm.Address) {
	n.disconnected(a)
}

func mockNotifiee(c cFunc, d dFunc) topology.Notifiee {
	return &notifiee{connected: c, disconnected: d}
}

type cFunc func(context.Context, swarm.Address) error
type dFunc func(swarm.Address)
