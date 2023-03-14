// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package skippeers_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/skippeers"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
)

func TestPeerSkipList(t *testing.T) {
	t.Parallel()
	skipList := skippeers.NewList()

	addr1 := testingc.GenerateTestRandomChunk().Address()
	addr2 := testingc.GenerateTestRandomChunk().Address()
	addr3 := testingc.GenerateTestRandomChunk().Address()

	skipList.Add(addr1, addr2, time.Millisecond*10)

	if !skipList.ChunkPeers(addr1)[0].Equal(addr2) {
		t.Fatal("peer should be skipped")
	}

	skipList.Add(addr1, addr3, time.Millisecond*10)

	time.Sleep(time.Millisecond * 11)

	skipList.PruneExpired()

	if len(skipList.ChunkPeers(addr1)) != 0 {
		t.Fatal("entry should be pruned")
	}
}
