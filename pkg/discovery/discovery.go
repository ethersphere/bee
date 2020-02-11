// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package discovery

import (
	"github.com/ethersphere/bee/pkg/swarm"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

type Driver interface {
	BroadcastPeer(addressee swarm.Address, peerID libp2ppeer.ID, overlay swarm.Address) error
}
