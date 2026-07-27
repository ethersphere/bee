// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package discovery exposes the discovery driver interface
// which is implemented by discovery protocols.
package discovery

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type Driver interface {
	// BroadcastPeers sends peer gossip to the addressee immediately.
	BroadcastPeers(ctx context.Context, addressee swarm.Address, peers ...swarm.Address) error
	// GossipPeer buffers a single peer for coalesced asynchronous gossip.
	GossipPeer(addressee, peer swarm.Address)
}
