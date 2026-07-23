// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestGossipBufferAddAndTakeAll(t *testing.T) {
	t.Parallel()

	b := newGossipBuffer(time.Second, maxBatchSize)
	addressee := swarm.RandAddress(t)
	peer1 := swarm.RandAddress(t)
	peer2 := swarm.RandAddress(t)

	if pending := b.takeAll(); len(pending) != 0 {
		t.Fatalf("want no pending entries, got %d", len(pending))
	}

	if _, flush := b.stagePeers(addressee, peer1); flush {
		t.Fatal("unexpected immediate flush")
	}

	if _, flush := b.stagePeers(addressee, peer2); flush {
		t.Fatal("unexpected immediate flush")
	}

	pending := b.takeAll()
	if len(pending) != 1 {
		t.Fatalf("want 1 pending entry, got %d", len(pending))
	}
	if got := len(pending[0].peers); got != 2 {
		t.Fatalf("want 2 coalesced peers, got %d", got)
	}
	if !pending[0].addressee.Equal(addressee) {
		t.Fatal("unexpected addressee in pending batch")
	}

	if pending := b.takeAll(); len(pending) != 0 {
		t.Fatalf("want empty buffer after takeAll, got %d pending", len(pending))
	}
}

func TestGossipBufferMaxBatchFlush(t *testing.T) {
	t.Parallel()

	b := newGossipBuffer(time.Second, 2)
	addressee := swarm.RandAddress(t)

	b.stagePeers(addressee, swarm.RandAddress(t))
	flushPeers, flush := b.stagePeers(addressee, swarm.RandAddress(t))
	if !flush {
		t.Fatal("want immediate flush at maxBatch")
	}
	if got := len(flushPeers); got != 2 {
		t.Fatalf("want 2 peers in full batch, got %d", got)
	}
	if pending := b.takeAll(); len(pending) != 0 {
		t.Fatalf("want empty buffer after maxBatch flush, got %d pending", len(pending))
	}
}
