// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package topology

import (
	"errors"

	"github.com/ethersphere/bee/pkg/swarm"
)

var ErrNotFound = errors.New("no peer found")

type Driver interface {
	Peerer
	ChunkPeerer
}

type Peerer interface {
	AddPeer(addr swarm.Address) error
}

type ChunkPeerer interface {
	ChunkPeer(addr swarm.Address) (peerAddr swarm.Address, err error)
}
