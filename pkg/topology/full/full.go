// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package full

import (
	"context"
	"math/rand"
	"time"

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

var _ topology.Driver = (*Driver)(nil)

// Driver drives the connectivity between nodes. It is a basic implementation of a connectivity Driver.
// that enabled full connectivity in the sense that:
// - Every peer which is added to the Driver gets broadcasted to every other peer regardless of its address.
// - A random peer is picked when asking for a peer to retrieve an arbitrary chunk (Peerer interface).
type Driver struct {
	discovery     discovery.Driver
	addressBook   addressbook.GetPutter
	connecter     p2p.Service
	receivedPeers map[string]bool
	logger        logging.Logger
}

func New(disc discovery.Driver, addressBook addressbook.GetPutter, connecter p2p.Service, logger logging.Logger) *Driver {
	return &Driver{
		discovery:     disc,
		addressBook:   addressBook,
		connecter:     connecter,
		receivedPeers: make(map[string]bool),
		logger:        logger,
	}
}

// AddPeer adds a new peer to the topology driver.
// The peer would be subsequently broadcasted to all connected peers.
// All conneceted peers are also broadcasted to the new peer.
func (d *Driver) AddPeer(addr swarm.Address) error {
	if d.receivedPeers[addr.ByteString()] {
		return nil
	}

	d.receivedPeers[addr.ByteString()] = true
	connectedPeers := d.connecter.Peers()
	ma, exists := d.addressBook.Get(addr)
	if !exists {
		return topology.ErrNotFound
	}

	if !isConnected(addr, connectedPeers) {
		peerAddr, err := d.connecter.Connect(context.Background(), ma)
		if err != nil {
			return err
		}

		// update addr if it is wrong or it has been changed
		if !addr.Equal(peerAddr) {
			addr = peerAddr
			d.addressBook.Put(peerAddr, ma)
		}
	}

	connectedAddrs := []swarm.Address{}
	for _, addressee := range connectedPeers {
		// skip newly added peer
		if addressee.Address.Equal(addr) {
			continue
		}

		connectedAddrs = append(connectedAddrs, addressee.Address)
		if err := d.discovery.BroadcastPeers(context.Background(), addressee.Address, addr); err != nil {
			return err
		}
	}

	if err := d.discovery.BroadcastPeers(context.Background(), addr, connectedAddrs...); err != nil {
		return err
	}

	return nil
}

// ChunkPeer is used to suggest a peer to ask a certain chunk from.
func (d *Driver) ChunkPeer(addr swarm.Address) (peerAddr swarm.Address, err error) {
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

func isConnected(addr swarm.Address, connectedPeers []p2p.Peer) bool {
	for _, p := range connectedPeers {
		if p.Address.Equal(addr) {
			return true
		}
	}

	return false
}
