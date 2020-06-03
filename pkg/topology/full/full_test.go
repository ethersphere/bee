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
	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/crypto"
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
	underlay, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7070/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS")
	if err != nil {
		t.Fatal(err)
	}

	overlay := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59a")
	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	bzzAddr, err := bzz.NewAddress(crypto.NewDefaultSigner(pk), underlay, overlay, 1)
	if err != nil {
		t.Fatal(err)
	}
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
		p2p := p2pmock.New(p2pmock.WithConnectFunc(func(_ context.Context, addr ma.Multiaddr, _ bool) (*bzz.Address, error) {
			if !addr.Equal(underlay) {
				t.Fatalf("expected multiaddr %s, got %s", addr, underlay)
			}

			return bzzAddr, nil
		}))

		fullDriver := full.New(discovery, ab, p2p, logger, overlay)
		defer fullDriver.Close()

		if err := ab.Put(overlay, *bzzAddr); err != nil {
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
		p2p := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr, _ bool) (*bzz.Address, error) {
			t.Fatal("should not be called")
			return nil, nil
		}))

		fullDriver := full.New(discovery, ab, p2p, logger, overlay)
		defer fullDriver.Close()
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
		addrAlreadyConnected, err := bzz.NewAddress(crypto.NewDefaultSigner(pk), underlay, alreadyConnected, 1)
		if err != nil {
			t.Fatal(err)
		}

		p2p := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr, _ bool) (*bzz.Address, error) {
			t.Fatal("should not be called")
			return nil, nil
		}), p2pmock.WithPeersFunc(func() []p2p.Peer {
			return connectedPeers
		}))

		fullDriver := full.New(discovery, ab, p2p, logger, overlay)
		defer fullDriver.Close()

		err = ab.Put(alreadyConnected, *addrAlreadyConnected)
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

		p2ps := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr, _ bool) (*bzz.Address, error) {
			if !addr.Equal(underlay) {
				t.Fatalf("expected multiaddr %s, got %s", addr.String(), underlay)
			}

			return bzzAddr, nil
		}), p2pmock.WithPeersFunc(func() []p2p.Peer {
			return connectedPeers
		}))

		fullDriver := full.New(discovery, ab, p2ps, logger, overlay)
		defer fullDriver.Close()
		if err := ab.Put(overlay, *bzzAddr); err != nil {
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

// TestSyncPeer tests that SyncPeer method returns closest connected peer to a given chunk.
func TestClosestPeer(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	baseOverlay := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000
	bzzAddr := &bzz.Address{
		Overlay: baseOverlay,
	}

	connectedPeers := []p2p.Peer{
		{
			Address: swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000"), // binary 1000 -> po 0 to base
		},
		{
			Address: swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000"), // binary 0100 -> po 1 to base
		},
		{
			Address: swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000"), // binary 0110 -> po 1 to base
		},
	}

	discovery := mock.NewDiscovery()
	statestore := mockstate.NewStateStore()
	ab := addressbook.New(statestore)

	p2ps := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr, _ bool) (*bzz.Address, error) {
		return bzzAddr, nil
	}), p2pmock.WithPeersFunc(func() []p2p.Peer {
		return connectedPeers
	}))

	fullDriver := full.New(discovery, ab, p2ps, logger, baseOverlay)
	defer fullDriver.Close()

	for _, tc := range []struct {
		chunkAddress swarm.Address // chunk address to test
		expectedPeer int           // points to the index of the connectedPeers slice. -1 means self (baseOverlay)
	}{
		{
			chunkAddress: swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000"), // 0111, wants peer 2
			expectedPeer: 2,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("c000000000000000000000000000000000000000000000000000000000000000"), // 1100, want peer 0
			expectedPeer: 0,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("e000000000000000000000000000000000000000000000000000000000000000"), // 1110, want peer 0
			expectedPeer: 0,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("a000000000000000000000000000000000000000000000000000000000000000"), // 1010, want peer 0
			expectedPeer: 0,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000"), // 0100, want peer 1
			expectedPeer: 1,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000"), // 0101, want peer 1
			expectedPeer: 1,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("0000001000000000000000000000000000000000000000000000000000000000"), // want self
			expectedPeer: -1,
		},
	} {
		peer, err := fullDriver.ClosestPeer(tc.chunkAddress)
		if err != nil {
			if tc.expectedPeer == -1 && !errors.Is(err, topology.ErrWantSelf) {
				t.Fatalf("wanted %v but got %v", topology.ErrWantSelf, err)
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
