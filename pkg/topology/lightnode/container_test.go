// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lightnode_test

import (
	"context"
	"errors"
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

		p1 := swarm.NewAddress([]byte("123"))
		p2 := swarm.NewAddress([]byte("456"))
		c.Connected(context.Background(), p2p.Peer{Address: p1})
		c.Connected(context.Background(), p2p.Peer{Address: p2})

		peerCount := len(c.PeerInfo().ConnectedPeers)

		if peerCount != 2 {
			t.Errorf("expected %d connected peer, got %d", 2, peerCount)
		}

		if cc := c.Count(); cc != 2 {
			t.Errorf("expected count 2 got %d", cc)
		}
		p, err := c.RandomPeer(p1)
		if err != nil {
			t.Fatal(err)
		}
		if !p.Equal(p2) {
			t.Fatalf("expected p1 but got %s", p.String())
		}

		p, err = c.RandomPeer(p2)
		if err != nil {
			t.Fatal(err)
		}
		if !p.Equal(p1) {
			t.Fatalf("expected p2 but got %s", p.String())
		}

		i := 0
		peers := []swarm.Address{p2, p1}
		if err = c.EachPeer(func(p swarm.Address, _ uint8) (bool, bool, error) {
			if !p.Equal(peers[i]) {
				return false, false, errors.New("peer not in order")
			}
			i++
			return false, false, nil
		}); err != nil {
			t.Fatal(err)
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
		if cc := c.Count(); cc != 0 {
			t.Errorf("expected count 0 got %d", cc)
		}
	})
}
