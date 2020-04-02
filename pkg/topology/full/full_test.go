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

	"github.com/ethersphere/bee/pkg/addressbook/inmem"
	"github.com/ethersphere/bee/pkg/discovery/mock"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	p2pmock "github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/full"

	ma "github.com/multiformats/go-multiaddr"
)

func TestAddPeer(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	underlay := "/ip4/127.0.0.1/tcp/7070/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS"
	overlay := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59a")
	testError := errors.New("test error")
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
		addressbook := inmem.New()
		p2p := p2pmock.New(p2pmock.WithConnectFunc(func(_ context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			if addr.String() != underlay {
				t.Fatalf("expected multiaddr %s, got %s", addr.String(), underlay)
			}
			return overlay, nil
		}))

		fullDriver := full.New(discovery, addressbook, p2p, logger)
		multiaddr, err := ma.NewMultiaddr(underlay)
		if err != nil {
			t.Fatal(err)
		}

		addressbook.Put(overlay, multiaddr)
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
		addressbook := inmem.New()
		p2p := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			t.Fatal("should not be called")
			return swarm.Address{}, nil
		}))

		fullDriver := full.New(discovery, addressbook, p2p, logger)
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
		addressbook := inmem.New()
		alreadyConnected := connectedPeers[0].Address

		p2p := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			t.Fatal("should not be called")
			return swarm.Address{}, nil
		}), p2pmock.WithPeersFunc(func() []p2p.Peer {
			return connectedPeers
		}))

		fullDriver := full.New(discovery, addressbook, p2p, logger)
		multiaddr, err := ma.NewMultiaddr(underlay)
		if err != nil {
			t.Fatal("error creating multiaddr")
		}

		addressbook.Put(alreadyConnected, multiaddr)
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
		addressbook := inmem.New()

		p2ps := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			if addr.String() != underlay {
				t.Fatalf("expected multiaddr %s, got %s", addr.String(), underlay)
			}
			return overlay, nil
		}), p2pmock.WithPeersFunc(func() []p2p.Peer {
			return connectedPeers
		}))

		fullDriver := full.New(discovery, addressbook, p2ps, logger)
		multiaddr, err := ma.NewMultiaddr(underlay)
		if err != nil {
			t.Fatal(err)
		}

		addressbook.Put(overlay, multiaddr)
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

	t.Run("ERROR - connect fails", func(t *testing.T) {
		discovery := mock.NewDiscovery()
		addressbook := inmem.New()
		p2p := p2pmock.New(p2pmock.WithConnectFunc(func(_ context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			if addr.String() != underlay {
				t.Fatalf("expected multiaddr %s, got %s", addr.String(), underlay)
			}
			return swarm.Address{}, testError
		}))

		fullDriver := full.New(discovery, addressbook, p2p, logger)
		multiaddr, err := ma.NewMultiaddr(underlay)
		if err != nil {
			t.Fatal(err)
		}

		addressbook.Put(overlay, multiaddr)
		err = fullDriver.AddPeer(context.Background(), overlay)
		if !errors.Is(err, testError) {
			t.Fatalf("expected %s, got %s", testError, err.Error())
		}

		if discovery.Broadcasts() != 0 {
			t.Fatalf("broadcasts expected %v, got %v ", 0, discovery.Broadcasts())
		}

		if _, exists := addressbook.Get(overlay); exists != false {
			t.Fatalf("%s found in addressbook", overlay)
		}
	})
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
