// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

type batchstoreSyncRadius struct {
	rs   postage.ReserveStateGetter
	base swarm.Address
}

type kademliaSyncRadius struct {
	kad  topology.Driver
	base swarm.Address
}

func (d *batchstoreSyncRadius) SyncRadius() uint8 {
	return d.rs.GetReserveState().StorageRadius
}

func (d *batchstoreSyncRadius) IsWithinSyncRadius(addr swarm.Address) bool {
	return swarm.Proximity(d.base.Bytes(), addr.Bytes()) >= d.SyncRadius()
}

func (d *kademliaSyncRadius) SyncRadius() uint8 {
	return d.kad.NeighborhoodDepth()
}

func (d *kademliaSyncRadius) IsWithinSyncRadius(addr swarm.Address) bool {
	return d.kad.IsWithinDepth(addr)
}

func newSyncRadius(fullNode bool, base swarm.Address, kad topology.Driver, rs postage.ReserveStateGetter) topology.SyncRadius {

	if fullNode {
		return &batchstoreSyncRadius{rs: rs, base: base}
	}

	return &kademliaSyncRadius{kad: kad, base: base}
}
