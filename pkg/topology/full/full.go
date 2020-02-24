// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package full

import (
	"math/rand"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/discovery"
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
	mtx         sync.Mutex
	connected   map[string]swarm.Address
	discovery   discovery.Driver
	addressBook addressbook.Getter
}

func New(disc discovery.Driver, addressBook addressbook.Getter) topology.Driver {
	return &driver{
		connected:   make(map[string]swarm.Address),
		discovery:   disc,
		addressBook: addressBook,
	}
}

// AddPeer adds a new peer to the topology driver.
// the peer would be subsequently broadcasted to all connected peers.
func (d *driver) AddPeer(overlay swarm.Address) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	d.connected[overlay.String()] = overlay
	ma, exists := d.addressBook.Get(overlay)
	if !exists {
		return topology.ErrNotFound
	}

	for _, addressee := range d.connected {
		err := d.discovery.BroadcastPeers(addressee, discovery.BroadcastRecord{Overlay: addressee, Addr: ma})
		if err != nil {
			return err
		}
	}
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
