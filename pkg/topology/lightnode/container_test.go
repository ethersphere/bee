// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lightnode_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
)

func TestContainer(t *testing.T) {

	base := test.RandomAddress()

	t.Run("new container is empty container", func(t *testing.T) {
		c := lightnode.NewContainer(base)

		var empty topology.BinInfo

		if !reflect.DeepEqual(empty, c.PeerInfo()) {
			t.Errorf("expected %v, got %v", empty, c.PeerInfo())
		}
	})

	t.Run("can add peers to container", func(t *testing.T) {
		c := lightnode.NewContainer(base)

		c.Connected(context.Background(), p2p.Peer{Address: swarm.NewAddress([]byte("123"))})
		c.Connected(context.Background(), p2p.Peer{Address: swarm.NewAddress([]byte("456"))})

		peerCount := len(c.PeerInfo().ConnectedPeers)

		if peerCount != 2 {
			t.Errorf("expected %d connected peer, got %d", 2, peerCount)
		}
	})
	t.Run("empty container after peer disconnect", func(t *testing.T) {
		c := lightnode.NewContainer(base)

		peer := p2p.Peer{Address: swarm.NewAddress([]byte("123"))}

		c.Connected(context.Background(), peer)
		c.Disconnected(peer)

		discPeerCount := len(c.PeerInfo().DisconnectedPeers)
		if discPeerCount != 1 {
			t.Errorf("expected %d connected peer, got %d", 1, discPeerCount)
		}

		connPeerCount := len(c.PeerInfo().ConnectedPeers)
		if connPeerCount != 0 {
			t.Errorf("expected %d connected peer, got %d", 0, connPeerCount)
		}
	})
}
