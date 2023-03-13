// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

import "github.com/ethersphere/bee/pkg/swarm"

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

type PeerFilterFunc = peerFilterFunc

func (k *Kad) IsWithinDepth(addr swarm.Address) bool {
	return swarm.Proximity(k.base.Bytes(), addr.Bytes()) >= k.NeighborhoodDepth()
}
