// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestGossipBufferAddAndDue(t *testing.T) {
	t.Parallel()

	const interval = 100 * time.Millisecond

	b := newGossipBuffer(interval, maxBatchSize)
	addressee := swarm.RandAddress(t)
	peer1 := swarm.RandAddress(t)
	peer2 := swarm.RandAddress(t)

	now := time.Now()
	if full := b.add(now, addressee, peer1); full != nil {
		t.Fatal("unexpected immediate flush")
	}

	if due := b.takeDue(now); len(due) != 0 {
		t.Fatalf("want no due entries, got %d", len(due))
	}

	if full := b.add(now, addressee, peer2); full != nil {
		t.Fatal("unexpected immediate flush")
	}

	afterDeadline := now.Add(interval + interval/4 + time.Millisecond)
	due := b.takeDue(afterDeadline)
	if len(due) != 1 {
		t.Fatalf("want 1 due entry, got %d", len(due))
	}
	if got := len(due[0].addresses()); got != 2 {
		t.Fatalf("want 2 coalesced peers, got %d", got)
	}
}

func TestGossipBufferMaxBatchFlush(t *testing.T) {
	t.Parallel()

	b := newGossipBuffer(time.Second, 2)
	addressee := swarm.RandAddress(t)
	now := time.Now()

	b.add(now, addressee, swarm.RandAddress(t))
	full := b.add(now, addressee, swarm.RandAddress(t))
	if full == nil {
		t.Fatal("want immediate flush at maxBatch")
	}
	if got := len(full.addresses()); got != 2 {
		t.Fatalf("want 2 peers in full batch, got %d", got)
	}
	if due := b.takeDue(now.Add(time.Second)); len(due) != 0 {
		t.Fatalf("want empty buffer after maxBatch flush, got %d due", len(due))
	}
}

func TestGossipBufferClearAddressee(t *testing.T) {
	t.Parallel()

	b := newGossipBuffer(time.Second, maxBatchSize)
	addressee := swarm.RandAddress(t)
	now := time.Now()

	b.add(now, addressee, swarm.RandAddress(t))
	b.clearAddressee(addressee)

	if all := b.takeAll(); len(all) != 0 {
		t.Fatalf("want buffer cleared on disconnect, got %d entries", len(all))
	}
}
