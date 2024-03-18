// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package skippeers_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/skippeers"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestPruneExpiresAfter(t *testing.T) {
	t.Parallel()

	skipList := skippeers.NewList()
	t.Cleanup(func() { skipList.Close() })

	chunk := swarm.RandAddress(t)
	peer1 := swarm.RandAddress(t)
	peer2 := swarm.RandAddress(t)

	skipList.Add(chunk, peer1, time.Millisecond*10)
	skipList.Add(swarm.RandAddress(t), peer1, time.Millisecond*10)
	if !swarm.ContainsAddress(skipList.ChunkPeers(chunk), peer1) {
		t.Fatal("peer should be in skiplist")
	}

	skipList.Add(chunk, peer2, time.Millisecond*10)
	if skipList.PruneExpiresAfter(chunk, time.Millisecond) > 0 {
		t.Fatal("entry should NOT be pruned")
	}

	skipList.PruneExpiresAfter(chunk, time.Millisecond)
	if len(skipList.ChunkPeers(chunk)) == 0 {
		t.Fatal("entry should NOT be pruned")
	}

	if len(skipList.ChunkPeers(swarm.RandAddress(t))) != 0 {
		t.Fatal("there should be no entry")
	}

	if skipList.PruneExpiresAfter(chunk, time.Millisecond*10) == 0 {
		t.Fatal("entry should be pruned")
	}

	if len(skipList.ChunkPeers(chunk)) != 0 {
		t.Fatal("entry should be pruned")
	}

	if len(skipList.ChunkPeers(swarm.RandAddress(t))) != 0 {
		t.Fatal("there should be no entry")
	}
}

func TestPeerWait(t *testing.T) {
	t.Parallel()

	skipList := skippeers.NewList()
	t.Cleanup(func() { skipList.Close() })

	chunk1 := swarm.RandAddress(t)
	chunk2 := swarm.RandAddress(t)
	peer1 := swarm.RandAddress(t)
	peer2 := swarm.RandAddress(t)
	peer3 := swarm.RandAddress(t)

	skipList.Add(chunk1, peer1, time.Millisecond*100)
	if !swarm.ContainsAddress(skipList.ChunkPeers(chunk1), peer1) {
		t.Fatal("peer should be in skiplist")
	}

	skipList.Add(chunk2, peer1, time.Millisecond*150)
	if !swarm.ContainsAddress(skipList.ChunkPeers(chunk2), peer1) {
		t.Fatal("peer should be in skiplist")
	}

	skipList.Add(chunk1, peer2, time.Millisecond*50)
	if !swarm.ContainsAddress(skipList.ChunkPeers(chunk1), peer2) {
		t.Fatal("peer should be in skiplist")
	}

	skipList.Add(chunk1, peer3, -time.Millisecond*50)
	if swarm.ContainsAddress(skipList.ChunkPeers(chunk1), peer3) {
		t.Fatal("peer should NOT be in skiplist")
	}

	time.Sleep(time.Millisecond * 60)

	if len(skipList.ChunkPeers(chunk1)) != 1 || !swarm.ContainsAddress(skipList.ChunkPeers(chunk1), peer1) {
		t.Fatal("peer should be in skiplist")
	}

	time.Sleep(time.Millisecond * 60)

	if len(skipList.ChunkPeers(chunk1)) != 0 {
		t.Fatal("entry should be pruned")
	}

	time.Sleep(time.Millisecond * 60)

	if len(skipList.ChunkPeers(chunk2)) != 0 {
		t.Fatal("entry should be pruned")
	}
}
