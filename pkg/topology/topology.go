// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package topology

import (
	"errors"
	"math/rand"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

var ErrNotFound = errors.New("no peer found")

func init() {
	rand.Seed(time.Now().UnixNano())
}

type Driver interface {
	AddPeer(overlay swarm.Address) error
	p2p.PeerSuggester
}

type ChunkPeerer interface {
	ChunkPeer(addr swarm.Address) (peerAddr swarm.Address, err error)
}
