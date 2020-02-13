// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"sort"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook/inmem"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/discovery/mock"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/full"
	"github.com/multiformats/go-multiaddr"
)

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
	if o.AddressBook == nil {
		o.AddressBook = inmem.New()
	}

	if o.TopologyDriver == nil {
		disc := mock.NewDiscovery()
		o.TopologyDriver = full.New(disc, o.AddressBook)
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

func serviceUnderlayAddress(t *testing.T, s *libp2p.Service) multiaddr.Multiaddr {
	t.Helper()

	addrs, err := s.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	return addrs[0]
}
