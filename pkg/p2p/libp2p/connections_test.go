// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	"github.com/libp2p/go-eventbus"
	libp2pm "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/mux"
	"github.com/libp2p/go-libp2p-core/network"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	swarmt "github.com/libp2p/go-libp2p-swarm/testing"
	bhost "github.com/libp2p/go-libp2p/p2p/host/basic"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	testDisconnectMsg = "test disconnect"
	testBlocklistMsg  = "test blocklist"
)

func TestAddresses(t *testing.T) {
	s, _ := newService(t, 1, libp2pServiceOpts{})

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

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode: true,
	}})
	s2, overlay2 := newService(t, 1, libp2pServiceOpts{})

	addr := serviceUnderlayAddress(t, s1)

	bzzAddr, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if err := s2.Disconnect(bzzAddr.Overlay, testDisconnectMsg); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)
}

func TestConnectToLightPeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, _ := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode: false,
	}})
	s2, _ := newService(t, 1, libp2pServiceOpts{})

	addr := serviceUnderlayAddress(t, s1)

	_, err := s2.Connect(ctx, addr)
	if err != p2p.ErrDialLightNode {
		t.Fatalf("expected err %v, got %v", p2p.ErrDialLightNode, err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)
}

func TestLightPeerLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		limit     = 3
		container = lightnode.NewContainer(test.RandomAddress())
		sf, _     = newService(t, 1, libp2pServiceOpts{
			lightNodes: container,
			libp2pOpts: libp2p.Options{
				LightNodeLimit: limit,
				FullNode:       true,
			},
		})

		notifier = mockNotifier(noopCf, noopDf, true)
	)
	sf.SetPickyNotifier(notifier)

	addr := serviceUnderlayAddress(t, sf)

	for i := 0; i < 5; i++ {
		sl, _ := newService(t, 1, libp2pServiceOpts{
			libp2pOpts: libp2p.Options{
				FullNode: false,
			},
		})
		_, err := sl.Connect(ctx, addr)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < 20; i++ {
		if cnt := container.Count(); cnt == limit {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatal("timed out waiting for correct number of lightnodes")
}

func TestDoubleConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode: true,
	}})
	s2, overlay2 := newService(t, 1, libp2pServiceOpts{})

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

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode: true,
	}})
	s2, overlay2 := newService(t, 1, libp2pServiceOpts{})

	addr := serviceUnderlayAddress(t, s1)

	bzzAddr, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if err := s2.Disconnect(bzzAddr.Overlay, testDisconnectMsg); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)

	if err := s2.Disconnect(bzzAddr.Overlay, testDisconnectMsg); !errors.Is(err, p2p.ErrPeerNotFound) {
		t.Errorf("got error %v, want %v", err, p2p.ErrPeerNotFound)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)
}

func TestMultipleConnectDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode: true,
	}})

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{})

	addr := serviceUnderlayAddress(t, s1)

	bzzAddr, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if err := s2.Disconnect(bzzAddr.Overlay, testDisconnectMsg); err != nil {
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

	if err := s2.Disconnect(bzzAddr.Overlay, testDisconnectMsg); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)
}

func TestConnectDisconnectOnAllAddresses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode: true,
	}})

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{})

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

		if err := s2.Disconnect(bzzAddr.Overlay, testDisconnectMsg); err != nil {
			t.Fatal(err)
		}

		expectPeers(t, s2)
		expectPeersEventually(t, s1)
	}
}

func TestDoubleConnectOnAllAddresses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode: true,
	}})
	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addrs {
		// creating new remote host for each address
		s2, overlay2 := newService(t, 1, libp2pServiceOpts{})

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

		if err := s2.Disconnect(overlay1, testDisconnectMsg); err != nil {
			t.Fatal(err)
		}

		expectPeers(t, s2)
		expectPeersEventually(t, s1)

		s2.Close()
	}
}

func TestDifferentNetworkIDs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, _ := newService(t, 1, libp2pServiceOpts{})
	s2, _ := newService(t, 2, libp2pServiceOpts{})

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err == nil {
		t.Fatal("connect attempt should result with an error")
	}

	expectPeers(t, s1)
	expectPeers(t, s2)
}

func TestConnectWithEnabledWSTransports(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{
		libp2pOpts: libp2p.Options{
			EnableWS: true,
			FullNode: true,
		},
	})

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{
		libp2pOpts: libp2p.Options{
			EnableWS: true,
			FullNode: true,
		},
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

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode: true,
	}})
	s2, overlay2 := newService(t, 1, libp2pServiceOpts{})

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

	if _, err := s2.HandshakeService().Handshake(ctx, libp2p.NewStream(stream), info.Addrs[0], info.ID); err == nil {
		t.Fatalf("expected stream error")
	}

	expectPeersEventually(t, s2)
	expectPeersEventually(t, s1)
}

func TestBlocklisting(t *testing.T) {
	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode: true,
	}})
	s2, overlay2 := newService(t, 1, libp2pServiceOpts{})

	addr1 := serviceUnderlayAddress(t, s1)
	addr2 := serviceUnderlayAddress(t, s2)

	_, err := s2.Connect(context.Background(), addr1)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	if err := s2.Blocklist(overlay1, 0, testBlocklistMsg); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)

	_, err = s2.Connect(context.Background(), addr1)
	if err == nil {
		t.Fatal("expected error during connection, got nil")
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)

	_, err = s1.Connect(context.Background(), addr2)
	if err == nil {
		t.Fatal("expected error during connection, got nil")
	}

	expectPeersEventually(t, s1)
	expectPeers(t, s2)
}

func TestTopologyNotifier(t *testing.T) {
	var (
		mtx sync.Mutex
		ctx = context.Background()

		ab1, ab2 = addressbook.New(mock.NewStateStore()), addressbook.New(mock.NewStateStore())

		n1connectedPeer    p2p.Peer
		n1disconnectedPeer p2p.Peer
		n2connectedPeer    p2p.Peer
		n2disconnectedPeer p2p.Peer

		n1c = func(_ context.Context, p p2p.Peer, _ bool) error {
			mtx.Lock()
			defer mtx.Unlock()
			expectZeroAddress(t, n1connectedPeer.Address) // fail if set more than once
			expectFullNode(t, p)
			n1connectedPeer = p
			return nil
		}
		n1d = func(p p2p.Peer) {
			mtx.Lock()
			defer mtx.Unlock()
			n1disconnectedPeer = p
		}

		n2c = func(_ context.Context, p p2p.Peer, _ bool) error {
			mtx.Lock()
			defer mtx.Unlock()
			expectZeroAddress(t, n2connectedPeer.Address) // fail if set more than once
			n2connectedPeer = p
			expectFullNode(t, p)
			return nil
		}
		n2d = func(p p2p.Peer) {
			mtx.Lock()
			defer mtx.Unlock()
			n2disconnectedPeer = p
		}
	)
	notifier1 := mockNotifier(n1c, n1d, true)
	s1, overlay1 := newService(t, 1, libp2pServiceOpts{
		Addressbook: ab1,
		libp2pOpts: libp2p.Options{
			FullNode: true,
		},
	})
	s1.SetPickyNotifier(notifier1)

	notifier2 := mockNotifier(n2c, n2d, true)
	s2, overlay2 := newService(t, 1, libp2pServiceOpts{
		Addressbook: ab2,
		libp2pOpts: libp2p.Options{
			FullNode: true,
		},
	})
	s2.SetPickyNotifier(notifier2)

	addr := serviceUnderlayAddress(t, s1)

	// s2 connects to s1, thus the notifier on s1 should be called on Connect
	bzzAddr, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	// expect that n1 notifee called with s2 overlay
	waitAddrSet(t, &n1connectedPeer.Address, &mtx, overlay2)

	mtx.Lock()
	expectZeroAddress(t, n1disconnectedPeer.Address, n2connectedPeer.Address, n2disconnectedPeer.Address)
	mtx.Unlock()

	// check address book entries are there
	checkAddressbook(t, ab2, overlay1, addr)

	// s2 disconnects from s1 so s1 disconnect notifiee should be called
	if err := s2.Disconnect(bzzAddr.Overlay, testDisconnectMsg); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2)
	expectPeersEventually(t, s1)
	waitAddrSet(t, &n1disconnectedPeer.Address, &mtx, overlay2)

	// note that both n1disconnect and n2disconnect callbacks are called after just
	// one disconnect. this is due to the fact the when the libp2p abstraction is explicitly
	// called to disconnect from a peer, it will also notify the topology notifiee, since
	// peer disconnections can also result from components from outside the bound of the
	// topology driver
	mtx.Lock()
	expectZeroAddress(t, n2connectedPeer.Address)
	mtx.Unlock()

	addr2 := serviceUnderlayAddress(t, s2)
	// s1 connects to s2, thus the notifiee on s2 should be called on Connect
	bzzAddr2, err := s1.Connect(ctx, addr2)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s1, overlay2)
	expectPeersEventually(t, s2, overlay1)
	waitAddrSet(t, &n2connectedPeer.Address, &mtx, overlay1)

	// s1 disconnects from s2 so s2 disconnect notifiee should be called
	if err := s1.Disconnect(bzzAddr2.Overlay, testDisconnectMsg); err != nil {
		t.Fatal(err)
	}
	expectPeers(t, s1)
	expectPeersEventually(t, s2)
	waitAddrSet(t, &n2disconnectedPeer.Address, &mtx, overlay1)
}

// TestTopologyAnnounce checks that announcement
// works correctly for full nodes and light nodes.
func TestTopologyAnnounce(t *testing.T) {
	var (
		mtx sync.Mutex
		ctx = context.Background()

		ab1, ab2, ab3 = addressbook.New(mock.NewStateStore()), addressbook.New(mock.NewStateStore()), addressbook.New(mock.NewStateStore())

		announceCalled   = false
		announceToCalled = false

		n1a = func(context.Context, swarm.Address, bool) error {
			mtx.Lock()
			announceCalled = true
			mtx.Unlock()
			return nil
		}
		n1at = func(context.Context, swarm.Address, swarm.Address, bool) error {
			mtx.Lock()
			announceToCalled = true
			mtx.Unlock()
			return nil
		}
	)
	// test setup: 2 full nodes and one light
	// light connect to full(1), then full(2)
	// connects to full(1), check that full(1)
	// tried to announce full(2) to light.

	notifier1 := mockAnnouncingNotifier(n1a, n1at)
	s1, overlay1 := newService(t, 1, libp2pServiceOpts{
		Addressbook: ab1,
		libp2pOpts: libp2p.Options{
			FullNode: true,
		},
	})
	s1.SetPickyNotifier(notifier1)

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{
		Addressbook: ab2,
		libp2pOpts: libp2p.Options{
			FullNode: true,
		},
	})

	s3, overlay3 := newService(t, 1, libp2pServiceOpts{
		Addressbook: ab3,
		libp2pOpts: libp2p.Options{
			FullNode: false,
		},
	})

	addr := serviceUnderlayAddress(t, s1)

	// s3 (light) connects to s1 (full)
	_, err := s3.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s3, overlay1)
	expectPeersEventually(t, s1, overlay3)
	called := false

	for i := 0; i < 20; i++ {
		mtx.Lock()
		called = announceCalled
		mtx.Unlock()
		if called {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !called {
		t.Error("expected announce to be called")
	}
	for i := 0; i < 10; i++ {
		mtx.Lock()
		called = announceToCalled
		mtx.Unlock()
		if called {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if announceToCalled {
		t.Error("announceTo called but should not")
	}

	// check address book entries are there
	checkAddressbook(t, ab3, overlay1, addr)

	// s2 (full) connects to s1 (full)
	_, err = s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2, overlay3)

	for i := 0; i < 20; i++ {
		mtx.Lock()
		called = announceToCalled
		mtx.Unlock()
		if called {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !called {
		t.Error("expected announceTo to be called")
	}
}

func TestTopologyOverSaturated(t *testing.T) {
	var (
		mtx sync.Mutex
		ctx = context.Background()

		ab1, ab2 = addressbook.New(mock.NewStateStore()), addressbook.New(mock.NewStateStore())

		n1connectedPeer    p2p.Peer
		n2connectedPeer    p2p.Peer
		n2disconnectedPeer p2p.Peer

		n1c = func(_ context.Context, p p2p.Peer, _ bool) error {
			mtx.Lock()
			defer mtx.Unlock()
			expectZeroAddress(t, n1connectedPeer.Address) // fail if set more than once
			n1connectedPeer = p
			return nil
		}
		n1d = func(p p2p.Peer) {}

		n2c = func(_ context.Context, p p2p.Peer, _ bool) error {
			mtx.Lock()
			defer mtx.Unlock()
			expectZeroAddress(t, n2connectedPeer.Address) // fail if set more than once
			n2connectedPeer = p
			return nil
		}
		n2d = func(p p2p.Peer) {
			mtx.Lock()
			defer mtx.Unlock()
			n2disconnectedPeer = p
		}
	)
	// this notifier will not pick the peer
	notifier1 := mockNotifier(n1c, n1d, false)
	s1, overlay1 := newService(t, 1, libp2pServiceOpts{Addressbook: ab1, libp2pOpts: libp2p.Options{
		FullNode: true,
	}})
	s1.SetPickyNotifier(notifier1)

	notifier2 := mockNotifier(n2c, n2d, false)
	s2, _ := newService(t, 1, libp2pServiceOpts{Addressbook: ab2})
	s2.SetPickyNotifier(notifier2)

	addr := serviceUnderlayAddress(t, s1)

	// s2 connects to s1, thus the notifier on s1 should be called on Connect
	_, err := s2.Connect(ctx, addr)
	if err == nil {
		t.Fatal("expected connect to fail but it didnt")
	}

	expectPeers(t, s1)
	expectPeersEventually(t, s2)

	waitAddrSet(t, &n2disconnectedPeer.Address, &mtx, overlay1)
}

func TestWithDisconnectStreams(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode: true,
	}})
	s2, overlay2 := newService(t, 1, libp2pServiceOpts{})

	testSpec := p2p.ProtocolSpec{
		Name:    testProtocolName,
		Version: testProtocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name: testStreamName,
				Handler: func(c context.Context, p p2p.Peer, s p2p.Stream) error {
					return nil
				},
			},
		},
	}

	p2p.WithDisconnectStreams(testSpec)

	_ = s1.AddProtocol(testSpec)

	s1_underlay := serviceUnderlayAddress(t, s1)

	expectPeers(t, s1)
	expectPeers(t, s2)

	if _, err := s2.Connect(ctx, s1_underlay); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s1, overlay2)
	expectPeers(t, s2, overlay1)

	s, err := s2.NewStream(ctx, overlay1, nil, testProtocolName, testProtocolVersion, testStreamName)

	expectStreamReset(t, s, err)

	expectPeersEventually(t, s2)
	expectPeersEventually(t, s1)
}

func TestWithBlocklistStreams(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode: true,
	}})
	s2, overlay2 := newService(t, 1, libp2pServiceOpts{})

	testSpec := p2p.ProtocolSpec{
		Name:    testProtocolName,
		Version: testProtocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name: testStreamName,
				Handler: func(c context.Context, p p2p.Peer, s p2p.Stream) error {
					return nil
				},
			},
		},
	}

	p2p.WithBlocklistStreams(1*time.Minute, testSpec)

	_ = s1.AddProtocol(testSpec)

	s1_underlay := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, s1_underlay); err != nil {
		t.Fatal(err)
	}

	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	s, err := s2.NewStream(ctx, overlay1, nil, testProtocolName, testProtocolVersion, testStreamName)

	expectStreamReset(t, s, err)

	expectPeersEventually(t, s2)
	expectPeersEventually(t, s1)

	if _, err := s2.Connect(ctx, s1_underlay); err == nil {
		t.Fatal("expected error when connecting to blocklisted peer")
	}

	expectPeersEventually(t, s2)
	expectPeersEventually(t, s1)
}

func TestUserAgentLogging(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// use concurrent-safe buffers as handlers are logging concurrently
	s1Logs := new(buffer)
	s2Logs := new(buffer)

	s1, _ := newService(t, 1, libp2pServiceOpts{
		libp2pOpts: libp2p.Options{
			FullNode: true,
		},
		Logger: logging.New(s1Logs, 5),
	})
	s2, _ := newService(t, 1, libp2pServiceOpts{
		Logger: logging.New(s2Logs, 5),
	})

	addr := serviceUnderlayAddress(t, s1)

	_, err := s2.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	// wait for logs to be written to buffers
	for t := time.Now().Add(10 * time.Second); time.Now().Before(t); time.Sleep(50 * time.Microsecond) {
		if s1Logs.Len() > 0 && s2Logs.Len() > 0 {
			break
		}
	}

	testUserAgentLogLine(t, s1Logs, "(inbound)")
	testUserAgentLogLine(t, s2Logs, "(outbound)")
}

func TestReachabilityUpdate(t *testing.T) {
	s1, _ := newService(t, 1, libp2pServiceOpts{
		libp2pOpts: libp2p.WithHostFactory(
			func(ctx context.Context, _ ...libp2pm.Option) (host.Host, error) {
				return bhost.NewHost(context.TODO(), swarmt.GenSwarm(t, context.TODO()), &bhost.HostOpts{})
			},
		),
	})
	defer s1.Close()

	emitReachabilityChanged, _ := s1.Host().EventBus().Emitter(new(event.EvtLocalReachabilityChanged), eventbus.Stateful)

	firstUpdate := make(chan struct{})
	s1.SetPickyNotifier(mockReachabilityNotifier(func(status p2p.ReachabilityStatus) {
		if status == p2p.ReachabilityStatusPublic {
			close(firstUpdate)
		}
	}))

	err := emitReachabilityChanged.Emit(event.EvtLocalReachabilityChanged{Reachability: network.ReachabilityPublic})
	if err != nil {
		t.Fatal(err)
	}
	<-firstUpdate

	secondUpdate := make(chan struct{})
	s1.SetPickyNotifier(mockReachabilityNotifier(func(status p2p.ReachabilityStatus) {
		if status == p2p.ReachabilityStatusPrivate {
			close(secondUpdate)
		}
	}))

	err = emitReachabilityChanged.Emit(event.EvtLocalReachabilityChanged{Reachability: network.ReachabilityPrivate})
	if err != nil {
		t.Fatal(err)
	}
	<-secondUpdate
}

func testUserAgentLogLine(t *testing.T, logs *buffer, substring string) {
	t.Helper()

	wantUserAgent := libp2p.UserAgent()
	if wantUserAgent == "" {
		t.Fatal("libp2p.UserAgent(): got empty user agent")
	}

	logLineMarker := "successfully connected to peer"
	var foundLogLine bool
	var lines []string
	for {
		line, err := logs.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		lines = append(lines, line)
		if strings.Contains(line, logLineMarker) && strings.Contains(line, substring) {
			foundLogLine = true
			if !strings.Contains(line, wantUserAgent) {
				t.Errorf("log line %q does not contain an expected User Agent %q", line, wantUserAgent)
			}
		}
	}
	if !foundLogLine {
		t.Errorf("log line with %q and %q strings was not found in %v", logLineMarker, substring, lines)
	}
}

// buffer is a bytes.Buffer with some methods exposed that are safe to be used
// concurrently.
type buffer struct {
	b bytes.Buffer
	m sync.Mutex
}

func (b *buffer) ReadString(delim byte) (string, error) {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.ReadString(delim)
}

func (b *buffer) Write(p []byte) (int, error) {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.Write(p)
}

func (b *buffer) Len() int {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.Len()
}

func expectStreamReset(t *testing.T, s io.ReadCloser, err error) {
	t.Helper()

	// due to the fact that disconnect method is asynchronous
	// stream reset error should occur either on creation or on first read attempt
	if err != nil && !errors.Is(err, mux.ErrReset) {
		t.Fatalf("expected stream reset error, got %v", err)
	}

	if err == nil {
		readErr := make(chan error)
		go func() {
			_, err := s.Read(make([]byte, 10))
			readErr <- err
		}()

		select {
		// because read could block without erroring we should also expect timeout
		case <-time.After(2 * time.Second):
			t.Error("expected stream reset error, got timeout reading")
		case err := <-readErr:
			if !errors.Is(err, mux.ErrReset) {
				t.Errorf("expected stream reset error, got %v", err)
			}
		}
	}
}

func expectFullNode(t *testing.T, p p2p.Peer) {
	t.Helper()
	if !p.FullNode {
		t.Fatal("expected peer to be a full node")
	}
}

func expectZeroAddress(t *testing.T, addrs ...swarm.Address) {
	t.Helper()
	for i, a := range addrs {
		if !a.Equal(swarm.ZeroAddress) {
			t.Fatalf("address did not equal zero address. index %d", i)
		}
	}
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

func checkAddressbook(t *testing.T, ab addressbook.Getter, overlay swarm.Address, underlay ma.Multiaddr) {
	t.Helper()
	addr, err := ab.Get(overlay)
	if err != nil {
		t.Fatal(err)
	}
	if !addr.Overlay.Equal(overlay) {
		t.Fatalf("overlay mismatch. got %s want %s", addr.Overlay, overlay)
	}

	if !addr.Underlay.Equal(underlay) {
		t.Fatalf("underlay mismatch. got %s, want %s", addr.Underlay, underlay)
	}
}

type notifiee struct {
	connected          cFunc
	disconnected       dFunc
	pick               bool
	announce           announceFunc
	announceTo         announceToFunc
	updateReachability reachabilityFunc
	reachable          reachableFunc
}

func (n *notifiee) Connected(c context.Context, p p2p.Peer, f bool) error {
	return n.connected(c, p, f)
}

func (n *notifiee) Disconnected(p p2p.Peer) {
	n.disconnected(p)
}

func (n *notifiee) Pick(p2p.Peer) bool {
	return n.pick
}

func (n *notifiee) Announce(ctx context.Context, a swarm.Address, full bool) error {
	return n.announce(ctx, a, full)
}

func (n *notifiee) AnnounceTo(ctx context.Context, a, b swarm.Address, full bool) error {
	return n.announceTo(ctx, a, b, full)
}

func (n *notifiee) UpdateReachability(status p2p.ReachabilityStatus) {
	n.updateReachability(status)
}

func (n *notifiee) Reachable(addr swarm.Address, status p2p.ReachabilityStatus) {
	n.reachable(addr, status)
}

func mockNotifier(c cFunc, d dFunc, pick bool) p2p.PickyNotifier {
	return &notifiee{
		connected:          c,
		disconnected:       d,
		pick:               pick,
		announce:           noopAnnounce,
		announceTo:         noopAnnounceTo,
		updateReachability: noopReachability,
		reachable:          noopReachable,
	}
}

func mockAnnouncingNotifier(a announceFunc, at announceToFunc) p2p.PickyNotifier {
	return &notifiee{
		connected:          noopCf,
		disconnected:       noopDf,
		pick:               true,
		announce:           a,
		announceTo:         at,
		updateReachability: noopReachability,
		reachable:          noopReachable,
	}
}

func mockReachabilityNotifier(r reachabilityFunc) p2p.PickyNotifier {
	return &notifiee{
		connected:          noopCf,
		disconnected:       noopDf,
		pick:               true,
		announce:           noopAnnounce,
		announceTo:         noopAnnounceTo,
		updateReachability: r,
		reachable:          noopReachable,
	}
}

type (
	cFunc            func(context.Context, p2p.Peer, bool) error
	dFunc            func(p2p.Peer)
	announceFunc     func(context.Context, swarm.Address, bool) error
	announceToFunc   func(context.Context, swarm.Address, swarm.Address, bool) error
	reachabilityFunc func(p2p.ReachabilityStatus)
	reachableFunc    func(swarm.Address, p2p.ReachabilityStatus)
)

var noopCf = func(context.Context, p2p.Peer, bool) error { return nil }
var noopDf = func(p2p.Peer) {}
var noopAnnounce = func(context.Context, swarm.Address, bool) error { return nil }
var noopAnnounceTo = func(context.Context, swarm.Address, swarm.Address, bool) error { return nil }
var noopReachability = func(p2p.ReachabilityStatus) {}
var noopReachable = func(swarm.Address, p2p.ReachabilityStatus) {}
