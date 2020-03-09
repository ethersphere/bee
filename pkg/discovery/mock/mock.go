// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
)

type Discovery struct {
	mtx     sync.Mutex
	ctr     int //how many ops
	records map[string]swarm.Address
}

func NewDiscovery() *Discovery {
	return &Discovery{
		records: make(map[string]swarm.Address),
	}
}

func (d *Discovery) BroadcastPeers(ctx context.Context, addressee swarm.Address, peers ...swarm.Address) error {
	for _, peer := range peers {
		d.mtx.Lock()
		d.ctr++
		d.records[addressee.String()] = peer
		d.mtx.Unlock()
	}

	return nil
}

func (d *Discovery) Broadcasts() int {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.ctr
}

func (d *Discovery) AddresseeRecord(addressee swarm.Address) (overlay swarm.Address, exists bool) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	rec, exists := d.records[addressee.String()]
	if !exists {
		return swarm.Address{}, false
	}
	return rec, true
}
