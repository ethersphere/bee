// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func stampHashOf(t *testing.T, ch swarm.Chunk) []byte {
	t.Helper()
	h, err := ch.Stamp().Hash()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestSimReserve_PutIdempotentAndPresence(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(1))
	base := randAddr(rng)

	var puts int
	r := NewSimReserve(base, 8, 0, 1, func(PutEvent) { puts++ })

	ch := chunkAt(rng, base, 0)
	if err := r.Inject(ch); err != nil {
		t.Fatal(err)
	}
	// Re-inject the same chunk: must be a no-op.
	if err := r.Inject(ch); err != nil {
		t.Fatal(err)
	}
	if puts != 1 {
		t.Fatalf("expected 1 put hook, got %d", puts)
	}
	if got := r.ReserveSize(); got != 1 {
		t.Fatalf("expected reserve size 1, got %d", got)
	}

	has, err := r.ReserveHas(ch.Address(), ch.Stamp().BatchID(), stampHashOf(t, ch))
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected chunk present")
	}

	got, err := r.ReserveGet(context.Background(), ch.Address(), ch.Stamp().BatchID(), stampHashOf(t, ch))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Address().Equal(ch.Address()) {
		t.Fatal("got wrong chunk")
	}

	// Absent chunk.
	if _, err := r.ReserveGet(context.Background(), randAddr(rng), nil, nil); err == nil {
		t.Fatal("expected error for absent chunk")
	}
}

func TestSimReserve_LastBinIDsAndEpoch(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(2))
	base := randAddr(rng)
	r := NewSimReserve(base, 8, 0, 42, nil)

	// Two chunks in the same (capped) bin get consecutive BinIDs.
	c1 := chunkAt(rng, base, 4)
	bin := r.binOf(c1.Address())
	var c2 swarm.Chunk
	for {
		cand := chunkAt(rng, base, 4)
		if r.binOf(cand.Address()) == bin {
			c2 = cand
			break
		}
	}
	if err := r.Inject(c1); err != nil {
		t.Fatal(err)
	}
	if err := r.Inject(c2); err != nil {
		t.Fatal(err)
	}

	ids, epoch, err := r.ReserveLastBinIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 8 {
		t.Fatalf("expected 8 cursors, got %d", len(ids))
	}
	if epoch != 42 {
		t.Fatalf("expected epoch 42, got %d", epoch)
	}
	if ids[bin] != 2 {
		t.Fatalf("expected bin %d cursor 2, got %d", bin, ids[bin])
	}
}

func TestSimReserve_SubscribeBinExistingAndLive(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(3))
	base := randAddr(rng)
	r := NewSimReserve(base, 8, 0, 1, nil)
	defer r.Close()

	// Pre-existing chunk in bin 7.
	c1 := chunkAt(rng, base, 7)
	bin := r.binOf(c1.Address())
	if err := r.Inject(c1); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsub, errC := r.SubscribeBin(ctx, bin, 1)
	defer unsub()

	recv := func() *storer.BinC {
		select {
		case c := <-ch:
			return c
		case err := <-errC:
			t.Fatalf("unexpected error: %v", err)
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for chunk")
		}
		return nil
	}

	first := recv()
	if !first.Address.Equal(c1.Address()) {
		t.Fatal("first delivered chunk mismatch")
	}

	// Now inject a live chunk in the same bin; the subscriber must wake.
	var c2 swarm.Chunk
	for {
		cand := chunkAt(rng, base, 0)
		if r.binOf(cand.Address()) == bin {
			c2 = cand
			break
		}
	}
	if err := r.Inject(c2); err != nil {
		t.Fatal(err)
	}
	second := recv()
	if !second.Address.Equal(c2.Address()) {
		t.Fatal("live delivered chunk mismatch")
	}
	if second.BinID <= first.BinID {
		t.Fatalf("expected increasing binID, got %d then %d", first.BinID, second.BinID)
	}
}

func TestSimReserve_CloseUnblocksSubscribers(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(4))
	base := randAddr(rng)
	r := NewSimReserve(base, 8, 0, 1, nil)

	ch, unsub, _ := r.SubscribeBin(context.Background(), 3, 1)
	defer unsub()

	r.Close()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel on Close")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not unblock subscriber")
	}
}

func TestSimReserve_RadiusChecker(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(5))
	base := randAddr(rng)
	r := NewSimReserve(base, 8, 4, 1, nil)

	near := randAddrAt(rng, base, 5)
	far := randAddrAt(rng, base, 1)

	if !r.IsWithinStorageRadius(near) {
		t.Fatal("expected near address within radius")
	}
	if r.IsWithinStorageRadius(far) {
		t.Fatal("expected far address outside radius")
	}

	r.SetRadius(0)
	if !r.IsWithinStorageRadius(far) {
		t.Fatal("expected far address within radius 0")
	}
	if r.StorageRadius() != 0 {
		t.Fatal("radius not updated")
	}
}
