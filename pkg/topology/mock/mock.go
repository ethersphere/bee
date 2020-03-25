// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

type TopologyDriver struct {
	peers []swarm.Address
	err   error
}

func NewTopologyDriver() *TopologyDriver {
	return &TopologyDriver{}
}

func (d *TopologyDriver) SetErr(err error) {
	d.err = err
}

func (d *TopologyDriver) AddPeer(addr swarm.Address) error {
	if d.err != nil {
		return d.err
	}

	d.peers = append(d.peers, addr)
	return nil
}

func (d *TopologyDriver) Peers() []swarm.Address {
	return d.peers
}
