// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package full

import (
	"context"
	"math/rand"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/discovery"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var _ topology.Driver = (*driver)(nil)

// driver drives the connectivity between nodes. It is a basic implementation of a connectivity driver.
// that enabled full connectivity in the sense that:
// - Every peer which is added to the driver gets broadcasted to every other peer regardless of its address.
// - A random peer is picked when asking for a peer to retrieve an arbitrary chunk (Peerer interface).
type driver struct {
	discovery   discovery.Driver
	addressBook addressbook.GetPutter
	connecter   p2p.Connecter
	logger      logging.Logger
	eg          errgroup.Group
}

func New(disc discovery.Driver, addressBook addressbook.GetPutter, connecter p2p.Connecter, logger logging.Logger) topology.Driver {
	return &driver{
		discovery:   disc,
		addressBook: addressBook,
		connecter:   connecter,
		logger:      logger,
	}
}

// AddPeer adds a new peer to the topology driver.
// The peer would be subsequently broadcasted to all connected peers.
// All conneceted peers are also broadcasted to the new peer.
func (d *driver) AddPeer(overlay swarm.Address) error {
	d.eg.Go(func() error {
		return d.addPeer(overlay)
	})

	return nil
}

// ChunkPeer is used to suggest a peer to ask a certain chunk from.
func (d *driver) ChunkPeer(addr swarm.Address) (peerAddr swarm.Address, err error) {
	connectedPeers := d.connecter.Peers()
	if len(connectedPeers) == 0 {
		return swarm.Address{}, topology.ErrNotFound
	}

	itemIdx := rand.Intn(len(connectedPeers))
	i := 0
	for _, v := range connectedPeers {
		if i == itemIdx {
			return v.Address, nil
		}
		i++
	}

	return swarm.Address{}, topology.ErrNotFound
}

func (d *driver) addPeer(peer swarm.Address) error {
	connectedPeers := d.connecter.Peers()
	ma, exists := d.addressBook.Get(peer)
	if !exists {
		return topology.ErrNotFound
	}

	peerAddr, err := d.connecter.Connect(context.Background(), ma)
	if err != nil {
		return err
	}

	if peer.String() != peerAddr.String() {
		peer = peerAddr
		d.addressBook.Put(peerAddr, ma)
	}

	connectedAddrs := []swarm.Address{}
	for _, addressee := range connectedPeers {
		connectedAddrs = append(connectedAddrs, addressee.Address)
		if err := d.discovery.BroadcastPeers(context.Background(), addressee.Address, peer); err != nil {
			return err
		}
	}

	if err := d.discovery.BroadcastPeers(context.Background(), peer, connectedAddrs...); err != nil {
		return err
	}

	return nil
}
