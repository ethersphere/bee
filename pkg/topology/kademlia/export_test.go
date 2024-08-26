// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

import (
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/pslice"
)

var (
	PruneOversaturatedBinsFunc = func(k *Kad) func(uint8) {
		return k.pruneOversaturatedBins
	}
	GenerateCommonBinPrefixes = generateCommonBinPrefixes
)

const (
	DefaultBitSuffixLength     = defaultBitSuffixLength
	DefaultSaturationPeers     = defaultSaturationPeers
	DefaultOverSaturationPeers = defaultOverSaturationPeers
)

type PeerExcludeFunc = peerExcludeFunc
type ExcludeFunc = excludeFunc

func (k *Kad) IsWithinConnectionDepth(addr swarm.Address) bool {
	return swarm.Proximity(k.base.Bytes(), addr.Bytes()) >= k.ConnectionDepth()
}

func (k *Kad) ConnectionDepth() uint8 {
	k.depthMu.RLock()
	defer k.depthMu.RUnlock()
	return k.depth
}

func (k *Kad) StorageRadius() uint8 {
	k.depthMu.RLock()
	defer k.depthMu.RUnlock()
	return k.storageRadius
}

// IsBalanced returns if Kademlia is balanced to bin.
func (k *Kad) IsBalanced(bin uint8) bool {
	if int(bin) >= len(k.commonBinPrefixes) {
		return false
	}

	// for each pseudo address
	for i := range k.commonBinPrefixes[bin] {
		pseudoAddr := k.commonBinPrefixes[bin][i]
		closestConnectedPeer, err := closestPeer(k.connectedPeers, pseudoAddr)
		if err != nil {
			return false
		}

		closestConnectedPO := swarm.ExtendedProximity(closestConnectedPeer.Bytes(), pseudoAddr.Bytes())
		if int(closestConnectedPO) < int(bin)+k.opt.BitSuffixLength+1 {
			return false
		}
	}

	return true
}

func closestPeer(peers *pslice.PSlice, addr swarm.Address) (swarm.Address, error) {
	closest := swarm.ZeroAddress
	err := peers.EachBinRev(func(peer swarm.Address, po uint8) (bool, bool, error) {
		if closest.IsZero() {
			closest = peer
			return false, false, nil
		}

		closer, err := peer.Closer(addr, closest)
		if err != nil {
			return false, false, err
		}
		if closer {
			closest = peer
		}
		return false, false, nil
	})
	if err != nil {
		return closest, err
	}

	// check if found
	if closest.IsZero() {
		return closest, topology.ErrNotFound
	}

	return closest, nil
}

func (k *Kad) Trigger() {
	k.manageC <- struct{}{}
}
