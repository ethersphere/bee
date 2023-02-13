// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import "github.com/ethersphere/bee/pkg/swarm"

var (
	IndexToBytes   = indexToBytes
	BytesToIndex   = bytesToIndex
	BlockThreshold = blockThreshold
)

func (si *StampIssuer) Inc(addr swarm.Address) ([]byte, error) {
	return si.inc(addr)
}
