// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

type MockDiscovery struct {
	mtx     sync.Mutex
	ctr     int //how many ops
	records map[string]broadcastRecord
}

type broadcastRecord struct {
	underlay libp2ppeer.ID
	overlay  swarm.Address
}

func NewMockDiscovery() *MockDiscovery {
	return &MockDiscovery{}
}

func (d *MockDiscovery) BroadcastPeer(addressee swarm.Address, peerID libp2ppeer.ID, overlay swarm.Address) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.ctr++
	d.records[addressee.String()] = broadcastRecord{underlay: peerID, overlay: overlay}
	return nil
}

func (d *MockDiscovery) Broadcasts() int {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.ctr
}

func (d *MockDiscovery) AddresseeRecord(addressee swarm.Address) (overlay swarm.Address, underlay libp2ppeer.ID) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	rec, exists := d.records[addressee.String()]
	if !exists {
		return swarm.Address{}, ""
	}
	return rec.overlay, rec.underlay
}
