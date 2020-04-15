// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package full_test

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/discovery/mock"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	p2pmock "github.com/ethersphere/bee/pkg/p2p/mock"
	mockstate "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/full"
	ma "github.com/multiformats/go-multiaddr"
)

func TestAddPeer(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	underlay := "/ip4/127.0.0.1/tcp/7070/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS"
	overlay := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59a")
	connectedPeers := []p2p.Peer{
		{
			Address: swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59b"),
		},
		{
			Address: swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
		},
		{
			Address: swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59d"),
		},
	}

	t.Run("OK - no connected peers", func(t *testing.T) {
		discovery := mock.NewDiscovery()
		statestore := mockstate.NewStateStore()
		ab := addressbook.New(statestore)

		p2p := p2pmock.New(p2pmock.WithConnectFunc(func(_ context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			if addr.String() != underlay {
				t.Fatalf("expected multiaddr %s, got %s", addr.String(), underlay)
			}
			return overlay, nil
		}))

		fullDriver := full.New(discovery, ab, p2p, logger, overlay)
		multiaddr, err := ma.NewMultiaddr(underlay)
		if err != nil {
			t.Fatal(err)
		}

		err = ab.Put(overlay, multiaddr)
		if err != nil {
			t.Fatal(err)
		}

		err = fullDriver.AddPeer(context.Background(), overlay)
		if err != nil {
			t.Fatalf("full conn driver returned err %s", err.Error())
		}

		if discovery.Broadcasts() != 0 {
			t.Fatalf("broadcasts expected %v, got %v ", 0, discovery.Broadcasts())
		}
	})

	t.Run("ERROR - peer not added", func(t *testing.T) {
		discovery := mock.NewDiscovery()
		statestore := mockstate.NewStateStore()
		ab := addressbook.New(statestore)
		p2p := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			t.Fatal("should not be called")
			return swarm.Address{}, nil
		}))

		fullDriver := full.New(discovery, ab, p2p, logger, overlay)
		err := fullDriver.AddPeer(context.Background(), overlay)
		if !errors.Is(err, topology.ErrNotFound) {
			t.Fatalf("full conn driver returned err %v", err)
		}

		if discovery.Broadcasts() != 0 {
			t.Fatalf("broadcasts expected %v, got %v ", 0, discovery.Broadcasts())
		}
	})

	t.Run("OK - connected peers - peer already connected", func(t *testing.T) {
		discovery := mock.NewDiscovery()
		statestore := mockstate.NewStateStore()
		ab := addressbook.New(statestore)
		alreadyConnected := connectedPeers[0].Address

		p2p := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			t.Fatal("should not be called")
			return swarm.Address{}, nil
		}), p2pmock.WithPeersFunc(func() []p2p.Peer {
			return connectedPeers
		}))

		fullDriver := full.New(discovery, ab, p2p, logger, overlay)
		multiaddr, err := ma.NewMultiaddr(underlay)
		if err != nil {
			t.Fatal("error creating multiaddr")
		}

		err = ab.Put(alreadyConnected, multiaddr)
		if err != nil {
			t.Fatal(err)
		}

		err = fullDriver.AddPeer(context.Background(), alreadyConnected)
		if err != nil {
			t.Fatalf("full conn driver returned err %s", err.Error())
		}

		if discovery.Broadcasts() != 3 {
			t.Fatalf("broadcasts expected %v, got %v ", 3, discovery.Broadcasts())
		}

		// check newly added node
		if err := checkAddreseeRecords(discovery, alreadyConnected, connectedPeers[1:]); err != nil {
			t.Fatal(err)
		}

		// check other nodes
		for _, p := range connectedPeers[1:] {
			if err := checkAddreseeRecords(discovery, p.Address, connectedPeers[0:1]); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("OK - connected peers - peer not already connected", func(t *testing.T) {
		discovery := mock.NewDiscovery()
		statestore := mockstate.NewStateStore()
		ab := addressbook.New(statestore)

		p2ps := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			if addr.String() != underlay {
				t.Fatalf("expected multiaddr %s, got %s", addr.String(), underlay)
			}
			return overlay, nil
		}), p2pmock.WithPeersFunc(func() []p2p.Peer {
			return connectedPeers
		}))

		fullDriver := full.New(discovery, ab, p2ps, logger, overlay)
		multiaddr, err := ma.NewMultiaddr(underlay)
		if err != nil {
			t.Fatal(err)
		}

		err = ab.Put(overlay, multiaddr)
		if err != nil {
			t.Fatal(err)
		}

		err = fullDriver.AddPeer(context.Background(), overlay)
		if err != nil {
			t.Fatalf("full conn driver returned err %s", err.Error())
		}

		if discovery.Broadcasts() != 4 {
			t.Fatalf("broadcasts expected %v, got %v ", 4, discovery.Broadcasts())
		}

		// check newly added node
		if err := checkAddreseeRecords(discovery, overlay, connectedPeers); err != nil {
			t.Fatal(err)
		}

		// check other nodes
		for _, p := range connectedPeers {
			if err := checkAddreseeRecords(discovery, p.Address, []p2p.Peer{{Address: overlay}}); err != nil {
				t.Fatal(err)
			}
		}
	})
}

// TestSyncPeer tests that SyncPeer method returns closest connected peer
func TestSyncPeer(t *testing.T) {
	/*
		base = 0000
		connected:
		- 0100
		- 0110
		- 1000 -> po 0

		chunk starts with
		- 0111 -> node selected is 0110
		- 1100/1110/1010 -> 1000
		- 0100 -> to which node? undefined behaviour. introduce distance metric for this? should go to node id 0
		- 0101 -> node 0100
	*/

	logger := logging.New(ioutil.Discard, 0)
	baseOverlay := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000
	connectedPeers := []p2p.Peer{
		{
			Address: swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000"), // binary 1000 -> po 0
		},
		{
			Address: swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000"), // binary 0100 -> po 1
		},
		{
			Address: swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000"), // binary 0110 -> po 1
		},
	}

	discovery := mock.NewDiscovery()
	statestore := mockstate.NewStateStore()
	ab := addressbook.New(statestore)

	p2ps := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (swarm.Address, error) {
		return baseOverlay, nil
	}), p2pmock.WithPeersFunc(func() []p2p.Peer {
		return connectedPeers
	}))

	fullDriver := full.New(discovery, ab, p2ps, logger, baseOverlay)

	for _, tc := range []struct {
		chunkAddress swarm.Address // chunk address to test
		expectedPeer int           // points to the index of the connectedPeers slice. -1 means self (baseOverlay)
		//expectedSelf bool          // should the peer expect itself to be closest to the chunk
	}{
		{
			chunkAddress: swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000"), // 0111
			expectedPeer: 2,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("c000000000000000000000000000000000000000000000000000000000000000"), // 1100
			expectedPeer: 0,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("e000000000000000000000000000000000000000000000000000000000000000"), // 1110
			expectedPeer: 0,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("a000000000000000000000000000000000000000000000000000000000000000"), // 1010
			expectedPeer: 0,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000"), // 0100 - ambiguous with proximity function (po)
			expectedPeer: 2,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000"), // 0101
			expectedPeer: 1,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("0000001000000000000000000000000000000000000000000000000000000000"), // want self
			expectedPeer: -1,
		},
	} {
		peer, err := fullDriver.SyncPeer(tc.chunkAddress)
		if err != nil {
			if tc.expectedPeer == -1 && !errors.Is(err, topology.ErrWantSelf) {
				t.Fatalf("wanted ErrNodeWantSelf but got %v", err)
			}
			continue
		}

		expected := connectedPeers[tc.expectedPeer].Address

		if !peer.Equal(expected) {
			t.Fatalf("peers not equal. got %s expected %s", peer, expected)
		}

	}
}

func checkAddreseeRecords(discovery *mock.Discovery, addr swarm.Address, expected []p2p.Peer) error {
	got, exists := discovery.AddresseeRecords(addr)
	if exists != true {
		return errors.New("addressee record does not exist")
	}

	for i, e := range expected {
		if !e.Address.Equal(got[i]) {
			return fmt.Errorf("addressee record expected %s, got %s ", e.Address.String(), got[i].String())
		}
	}

	return nil
}
