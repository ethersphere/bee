// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pslice

import "github.com/ethersphere/bee/pkg/swarm"

func PSlicePeers(p *PSlice) []swarm.Address {
	return p.peers
}

func PSliceBins(p *PSlice) []uint {
	return p.bins
}
