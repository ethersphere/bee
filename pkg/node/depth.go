// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

type batchStoreDepther struct {
	rs   postage.ReserveStateGetter
	base swarm.Address
}

func (d *batchStoreDepther) NeighborhoodDepth() uint8 {
	return d.rs.GetReserveState().StorageRadius
}

func (d *batchStoreDepther) IsWithinDepth(addr swarm.Address) bool {
	return swarm.Proximity(d.base.Bytes(), addr.Bytes()) >= d.NeighborhoodDepth()
}

func newDepther(fullNode bool, base swarm.Address, kad topology.Driver, rs postage.ReserveStateGetter) topology.NeighborhoodDepther {

	if fullNode {
		d := batchStoreDepther{rs: rs, base: base}
		return &d
	}

	return kad
}
