// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package discovery

import (
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"

	ma "github.com/multiformats/go-multiaddr"
)

type Driver interface {
	BroadcastPeer(addressee swarm.Address, overlay swarm.Address, addr ma.Multiaddr) error
}

// Peerer can suggest new known or connected peers to other peers
type Peerer interface {
	// Peers fetches up to limit amount of new known peers for provided peer & bin
	// Subsequent call to this method should fetch different, new peers, if any
	Peers(peer p2p.Peer, bin, limit int) (peers []p2p.Peer)
}
