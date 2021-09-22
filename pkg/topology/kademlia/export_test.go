// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

var (
	TimeToRetry                 = &timeToRetry
	QuickSaturationPeers        = &quickSaturationPeers
	SaturationPeers             = &saturationPeers
	OverSaturationPeers         = &overSaturationPeers
	BootnodeOverSaturationPeers = &bootNodeOverSaturationPeers
	LowWaterMark                = &nnLowWatermark
	PruneOversaturatedBinsFunc  = func(k *Kad) func(uint8) {
		return k.pruneOversaturatedBins
	}
	GenerateCommonBinPrefixes = generateCommonBinPrefixes
	ExtraPeersToPrune         = &extraPeersToPrune
)
