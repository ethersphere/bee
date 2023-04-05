// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package skippeers_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/skippeers"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestPeerSkipList(t *testing.T) {
	t.Parallel()
	skipList := skippeers.NewList()

	addr1 := swarm.RandAddress(t)
	addr2 := swarm.RandAddress(t)
	addr3 := swarm.RandAddress(t)

	skipList.Add(addr1, addr2, time.Millisecond*10)

	if !skipList.ChunkPeers(addr1)[0].Equal(addr2) {
		t.Fatal("peer should be skipped")
	}

	skipList.Add(addr1, addr3, time.Millisecond*10)

	skipList.PruneExpiresAfter(time.Millisecond)
	if len(skipList.ChunkPeers(addr1)) == 0 {
		t.Fatal("entry should NOT be pruned")
	}

	skipList.PruneExpiresAfter(time.Millisecond * 10)

	if len(skipList.ChunkPeers(addr1)) != 0 {
		t.Fatal("entry should be pruned")
	}
}
