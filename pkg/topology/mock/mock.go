// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"

	"github.com/ethersphere/bee/pkg/swarm"
)

type TopologyDriver struct {
	peers      []swarm.Address
	addPeerErr error
}

func NewTopologyDriver() *TopologyDriver {
	return &TopologyDriver{}
}

func (d *TopologyDriver) SetAddPeerErr(err error) {
	d.addPeerErr = err
}

func (d *TopologyDriver) AddPeer(ctx context.Context, addr swarm.Address) error {
	if d.addPeerErr != nil {
		return d.addPeerErr
	}

	d.peers = append(d.peers, addr)
	return nil
}

func (d *TopologyDriver) Peers() []swarm.Address {
	return d.peers
}
