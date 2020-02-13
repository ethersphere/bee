// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

type Discovery struct {
	mtx     sync.Mutex
	ctr     int //how many ops
	records map[string]broadcastRecord
}

type broadcastRecord struct {
	overlay   swarm.Address
	multiaddr ma.Multiaddr
}

func NewDiscovery() *Discovery {
	return &Discovery{
		records: make(map[string]broadcastRecord),
	}
}

func (d *Discovery) BroadcastPeer(addressee swarm.Address, overlay swarm.Address, addr ma.Multiaddr) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.ctr++
	d.records[addressee.String()] = broadcastRecord{overlay: overlay, multiaddr: addr}
	return nil
}

func (d *Discovery) Broadcasts() int {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.ctr
}

func (d *Discovery) AddresseeRecord(addressee swarm.Address) (overlay swarm.Address, addr ma.Multiaddr) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	rec, exists := d.records[addressee.String()]
	if !exists {
		return swarm.Address{}, nil
	}
	return rec.overlay, rec.multiaddr
}
