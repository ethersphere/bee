// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lightnode

import (
	"crypto/rand"
	"math/big"

	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/pslice"
)

func (c *Container) Count() int {
	return c.connectedPeers.Length()
}

func (c *Container) RandomPeer(not swarm.Address) (swarm.Address, error) {
	c.peerMu.Lock()
	defer c.peerMu.Unlock()
	var (
		cnt   = big.NewInt(int64(c.Count()))
		addr  = swarm.ZeroAddress
		count = int64(0)
	)

PICKPEER:
	i, e := rand.Int(rand.Reader, cnt)
	if e != nil {
		return swarm.ZeroAddress, e
	}
	i64 := i.Int64()

	count = 0
	_ = c.connectedPeers.EachBinRev(func(peer swarm.Address, _ uint8) (bool, bool, error) {
		if count == i64 {
			addr = peer
			return true, false, nil
		}
		count++
		return false, false, nil
	})

	if addr.Equal(not) {
		goto PICKPEER
	}

	return addr, nil
}

func (c *Container) EachPeer(pf topology.EachPeerFunc) error {
	return c.connectedPeers.EachBin(pf)
}

func (c *Container) PeerInfo() topology.BinInfo {
	return topology.BinInfo{
		BinPopulation:     uint(c.connectedPeers.Length()),
		BinConnected:      uint(c.connectedPeers.Length()),
		DisconnectedPeers: peersInfo(c.disconnectedPeers),
		ConnectedPeers:    peersInfo(c.connectedPeers),
	}
}

func peersInfo(s *pslice.PSlice) []*topology.PeerInfo {
	if s.Length() == 0 {
		return nil
	}
	peers := make([]*topology.PeerInfo, 0, s.Length())
	_ = s.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
		peers = append(peers, &topology.PeerInfo{Address: addr})
		return false, false, nil
	})
	return peers
}
