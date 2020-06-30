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

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
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

	bzzAddr, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if err := s2.Disconnect(bzzAddr.Overlay); err != nil {
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

	bzzAddr, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if err := s2.Disconnect(bzzAddr.Overlay); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)

	if err := s2.Disconnect(bzzAddr.Overlay); !errors.Is(err, p2p.ErrPeerNotFound) {
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

	bzzAddr, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if err := s2.Disconnect(bzzAddr.Overlay); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)

	bzzAddr, err = s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if err := s2.Disconnect(bzzAddr.Overlay); err != nil {
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
		bzzAddr, err := s2.Connect(ctx, addr)
		if err != nil {
			t.Fatal(err)
		}

		expectPeers(t, s2, overlay1)
		expectPeersEventually(t, s1, overlay2)

		if err := s2.Disconnect(bzzAddr.Overlay); err != nil {
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

func TestConnectWithEnabledQUICAndWSTransports(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2p.Options{
		EnableQUIC: true,
		EnableWS:   true,
	})

	s2, overlay2 := newService(t, 1, libp2p.Options{
		EnableQUIC: true,
		EnableWS:   true,
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

	if _, err := s2.HandshakeService().Handshake(libp2p.NewStream(stream), info.Addrs[0], info.ID); err == nil {
		t.Fatalf("expected stream error")
	}

	expectPeersEventually(t, s2)
	expectPeersEventually(t, s1)
}

func TestTopologyNotifier(t *testing.T) {

	var (
		mtx sync.Mutex
		ctx = context.Background()

		ab1, ab2 = addressbook.New(mock.NewStateStore()), addressbook.New(mock.NewStateStore())

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
	notifier1 := mockNotifier(n1c, n1d)
	s1, overlay1 := newService(t, 1, libp2p.Options{Addressbook: ab1})
	s1.SetNotifier(notifier1)

	notifier2 := mockNotifier(n2c, n2d)
	s2, overlay2 := newService(t, 1, libp2p.Options{Addressbook: ab2})
	s2.SetNotifier(notifier2)

	addr := serviceUnderlayAddress(t, s1)

	// s2 connects to s1, thus the notifier on s1 should be called on Connect
	bzzAddr, err := s2.Connect(ctx, addr)
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

	// check address book entries are there
	// TODO: this is wrong. bzzAddr.Underlay should be in fact just `addr`
	// but this is necessary for the test to pass. should be fixed when
	// the handshake scheme is fixed
	checkAddressbook(t, ab2, overlay1, bzzAddr.Underlay)

	// s2 disconnects from s1 so s1 disconnect notifiee should be called
	if err := s2.Disconnect(bzzAddr.Overlay); err != nil {
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
	bzzAddr2, err := s1.Connect(ctx, addr2)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s1, overlay2)
	expectPeersEventually(t, s2, overlay1)
	waitAddrSet(t, &n2connectedAddr, &mtx, overlay1)

	// s1 disconnects from s2 so s2 disconnect notifiee should be called
	if err := s1.Disconnect(bzzAddr2.Overlay); err != nil {
		t.Fatal(err)
	}
	expectPeers(t, s1)
	expectPeersEventually(t, s2)
	waitAddrSet(t, &n2disconnectedAddr, &mtx, overlay1)
}

func TestTopologyLocalNotifier(t *testing.T) {
	var (
		mtx             sync.Mutex
		n2connectedAddr swarm.Address

		n2c = func(_ context.Context, a swarm.Address) error {
			mtx.Lock()
			defer mtx.Unlock()
			n2connectedAddr = a
			return nil
		}
		n2d = func(a swarm.Address) {
		}
	)

	s1, overlay1 := newService(t, 1, libp2p.Options{})
	notifier2 := mockNotifier(n2c, n2d)

	s2, overlay2 := newService(t, 1, libp2p.Options{})
	s2.SetNotifier(notifier2)

	addr := serviceUnderlayAddress(t, s1)

	// s2 connects to s1, thus the notifier on s1 should be called on Connect
	_, err := s2.ConnectNotify(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	// expect that n1 notifee called with s2 overlay
	waitAddrSet(t, &n2connectedAddr, &mtx, overlay1)
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

func checkAddressbook(t *testing.T, ab addressbook.Getter, overlay swarm.Address, under ma.Multiaddr) {
	t.Helper()
	v, err := ab.Get(overlay)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Overlay.Equal(overlay) {
		t.Fatalf("overlay mismatch. got %s want %s", v.Overlay, overlay)
	}

	if !v.Underlay.Equal(under) {
		t.Fatalf("underlay mismatch. got %s, want %s", v.Underlay, under)
	}
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

func mockNotifier(c cFunc, d dFunc) topology.Notifier {
	return &notifiee{connected: c, disconnected: d}
}

type cFunc func(context.Context, swarm.Address) error
type dFunc func(swarm.Address)

var noopNotifier = mockNotifier(
	func(_ context.Context, _ swarm.Address) error { return nil },
	func(_ swarm.Address) {},
)
