// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package topology

import (
	"context"
	"errors"
	"io"

	"github.com/ethersphere/bee/pkg/swarm"
)

var ErrNotFound = errors.New("no peer found")
var ErrWantSelf = errors.New("node wants self")

type Driver interface {
	PeerAdder
	ClosestPeerer
	io.Closer
}

type PeerAdder interface {
	AddPeer(ctx context.Context, addr swarm.Address) error
}

type ClosestPeerer interface {
	ClosestPeer(addr swarm.Address) (peerAddr swarm.Address, err error)
}

// ScoreFunc is implemented by components that need to score peers in a different way than XOR distance.
type ScoreFunc func(peer swarm.Address) (score float32)

// EachPeerFunc is a callback that is called with a peer and its PO
type EachPeerFunc func(swarm.Address, uint8) (stop, jumpToNext bool, err error)
