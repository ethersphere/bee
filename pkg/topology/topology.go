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

var (
	ErrNotFound = errors.New("no peer found")
	ErrWantSelf = errors.New("node wants self")
)

type Driver interface {
	PeerAdder
	ClosestPeerer
	Notifier
	io.Closer
}

type Notifier interface {
	Connecter
	Disconnecter
}

type PeerAdder interface {
	// AddPeer is called when a peer is added to the topology backlog
	// for further processing by connectivity strategy.
	AddPeer(ctx context.Context, addr swarm.Address) error
}

type Connecter interface {
	// Connected is called when a peer dials in.
	Connected(context.Context, swarm.Address) error
}

type Disconnecter interface {
	// Disconnected is called when a peer disconnects.
	Disconnected(swarm.Address)
}

type ClosestPeerer interface {
	ClosestConnectedPeer(addr swarm.Address) (peerAddr swarm.Address, err error)
}

// EachPeerFunc is a callback that is called with a peer and its PO
type EachPeerFunc func(swarm.Address, uint8) (stop, jumpToNext bool, err error)
