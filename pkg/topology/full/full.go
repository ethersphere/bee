// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package full

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/discovery"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"

	ma "github.com/multiformats/go-multiaddr"
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
	mtx         sync.Mutex
	connected   map[string]swarm.Address
	discovery   discovery.Driver
	addressBook addressbook.Getter
	connecter   Connecter
}

type Connecter interface {
	Connect(ctx context.Context, addr ma.Multiaddr) (overlay swarm.Address, err error)
}

func New(disc discovery.Driver, addressBook addressbook.Getter, connecter Connecter) topology.Driver {
	return &driver{
		connected:   make(map[string]swarm.Address),
		discovery:   disc,
		addressBook: addressBook,
		connecter:   connecter,
	}
}

// AddPeer adds a new peer to the topology driver.
// The connection to this peer will be performed.
// The peer would be subsequently broadcasted to all connected peers.
// All conneceted peers are also broadcasted to the new peer.
func (d *driver) AddPeer(overlay swarm.Address, addr ma.Multiaddr) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	overlay, err := d.connecter.Connect(context.Background(), addr)
	if err != nil {
		return err
	}

	var connectedNodes []discovery.BroadcastRecord
	for _, addressee := range d.connected {
		cma, exists := d.addressBook.Get(addressee)
		if !exists {
			return topology.ErrNotFound
		}

		if err := d.discovery.BroadcastPeers(context.Background(), addressee, discovery.BroadcastRecord{Overlay: overlay, Addr: cma}); err != nil {
			return err
		}

		connectedNodes = append(connectedNodes, discovery.BroadcastRecord{Overlay: addressee, Addr: cma})
	}

	if err := d.discovery.BroadcastPeers(context.Background(), overlay, connectedNodes...); err != nil {
		return err
	}

	// add new node to connected nodes to avoid double broadcasts
	d.connected[overlay.String()] = overlay
	return nil
}

// ChunkPeer is used to suggest a peer to ask a certain chunk from.
func (d *driver) ChunkPeer(addr swarm.Address) (peerAddr swarm.Address, err error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	if len(d.connected) == 0 {
		return swarm.Address{}, topology.ErrNotFound
	}

	itemIdx := rand.Intn(len(d.connected))
	i := 0
	for _, v := range d.connected {
		if i == itemIdx {
			return v, nil
		}
		i++
	}

	return swarm.Address{}, topology.ErrNotFound
}
