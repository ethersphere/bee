// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestRegistry_PutLookupRemove(t *testing.T) {
	t.Parallel()

	r := chequebook.NewRegistry()

	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")
	overlay := swarm.MustParseHexAddress("aabb")

	if _, known := r.Peer(cb); known {
		t.Fatal("empty registry should report unknown")
	}

	if err := r.Put(overlay, cb, 10, bzz.TimestampSourceHandshake, nil); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, known := r.Peer(cb)
	if !known || !got.Equal(overlay) {
		t.Fatalf("lookup got (%s, known=%v), want (%s, true)", got, known, overlay)
	}

	r.Remove(overlay)
	if _, known := r.Peer(cb); known {
		t.Fatal("entry should be gone after Remove")
	}
}

func TestRegistry_OverlaySwitchesChequebook(t *testing.T) {
	t.Parallel()

	r := chequebook.NewRegistry()
	overlay := swarm.MustParseHexAddress("aabb")
	cb1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	cb2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	if err := r.Put(overlay, cb1, 10, bzz.TimestampSourceHandshake, nil); err != nil {
		t.Fatalf("first Put failed: %v", err)
	}
	if err := r.Put(overlay, cb2, 20, bzz.TimestampSourceHandshake, nil); err != nil {
		t.Fatalf("second Put failed: %v", err)
	}

	if _, known := r.Peer(cb1); known {
		t.Fatal("old chequebook mapping should be dropped when overlay switches chequebook")
	}
	got, known := r.Peer(cb2)
	if !known || !got.Equal(overlay) {
		t.Fatalf("new mapping missing: got (%s, known=%v)", got, known)
	}
}

func TestRegistry_RemoveUnknownPeerIsSafe(t *testing.T) {
	t.Parallel()

	r := chequebook.NewRegistry()
	r.Remove(swarm.MustParseHexAddress("ccdd"))
}

// TestRegistry_StaleChequebookEvictedOnUpdate verifies that once a peer
// updates its chequebook (cb1 → cb2), the old cb1 entry is removed from the
// registry. Without this clean-up an attacker could later present cb1 as
// belonging to a new sybil overlay, bypassing the uniqueness check.
func TestRegistry_StaleChequebookEvictedOnUpdate(t *testing.T) {
	t.Parallel()

	r := chequebook.NewRegistry()
	overlay := swarm.MustParseHexAddress("aabb")
	cb1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	cb2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sybil := swarm.MustParseHexAddress("ccdd")

	if err := r.Put(overlay, cb1, 10, bzz.TimestampSourceHandshake, nil); err != nil {
		t.Fatalf("first Put failed: %v", err)
	}
	if err := r.Put(overlay, cb2, 20, bzz.TimestampSourceHandshake, nil); err != nil {
		t.Fatalf("update Put failed: %v", err)
	}

	// cb1 must no longer be in the registry — a sybil peer presenting cb1
	// must not be found as already-known, so the uniqueness check would
	// allow it through unchallenged.
	if peer, known := r.Peer(cb1); known {
		t.Fatalf("stale cb1 still registered for %s; sybil could reuse it", peer)
	}

	// Confirm cb1 cannot be silently associated with a new overlay either.
	if err := r.Put(sybil, cb1, 30, bzz.TimestampSourceHandshake, nil); err != nil {
		t.Fatalf("sybil Put failed: %v", err)
	}
	got, known := r.Peer(cb1)
	if !known || !got.Equal(sybil) {
		t.Fatalf("expected sybil→cb1 association after Put, got (%s, known=%v)", got, known)
	}
	// And cb2 still maps correctly to the original overlay.
	got2, known2 := r.Peer(cb2)
	if !known2 || !got2.Equal(overlay) {
		t.Fatalf("cb2 mapping corrupted: got (%s, known=%v)", got2, known2)
	}
}

func TestRegistry_RejectsOlderTimestamp(t *testing.T) {
	t.Parallel()

	r := chequebook.NewRegistry()
	overlay := swarm.MustParseHexAddress("aabb")
	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")

	if err := r.Put(overlay, cb, 10, bzz.TimestampSourceHandshake, nil); err != nil {
		t.Fatalf("first Put failed: %v", err)
	}
	err := r.Put(overlay, cb, 5, bzz.TimestampSourceHandshake, nil)
	if !errors.Is(err, bzz.ErrTimestampStale) {
		t.Fatalf("expected ErrTimestampStale, got %v", err)
	}
}

func TestRegistry_RejectsGossipUpdateWithinMinInterval(t *testing.T) {
	t.Parallel()

	r := chequebook.NewRegistry()
	overlay := swarm.MustParseHexAddress("aabb")
	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")

	if err := r.Put(overlay, cb, 100, bzz.TimestampSourceGossip, nil); err != nil {
		t.Fatalf("first Put failed: %v", err)
	}
	err := r.Put(overlay, cb, 200, bzz.TimestampSourceGossip, nil)
	if !errors.Is(err, bzz.ErrTimestampTooSoon) {
		t.Fatalf("expected ErrTimestampTooSoon, got %v", err)
	}
}

// Handshake source bypasses MinimumUpdateInterval (spec §5.3 Rule 2).
func TestRegistry_HandshakeIgnoresMinInterval(t *testing.T) {
	t.Parallel()

	r := chequebook.NewRegistry()
	overlay := swarm.MustParseHexAddress("aabb")
	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")

	if err := r.Put(overlay, cb, 100, bzz.TimestampSourceGossip, nil); err != nil {
		t.Fatalf("first Put failed: %v", err)
	}
	if err := r.Put(overlay, cb, 200, bzz.TimestampSourceHandshake, nil); err != nil {
		t.Fatalf("handshake Put within min-interval must be accepted, got %v", err)
	}
}

// TestRegistry_Put_AtomicWithCallback drives concurrent Put calls for
// the same overlay and asserts that writeAddressbook callbacks never run
// in parallel — the registry mutex must serialise the (registry,
// addressbook) write pair.
//
// Without serialisation, the in-flight callback counter would exceed one
// and the assertion would fire.
func TestRegistry_Put_AtomicWithCallback(t *testing.T) {
	t.Parallel()

	r := chequebook.NewRegistry()
	overlay := swarm.MustParseHexAddress("aabb")
	cb1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	cb2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	const iterations = 200

	var (
		bookkeep    sync.Mutex
		inFlight    int
		maxObserved int
		callbacks   int
	)

	work := func(cb common.Address, baseTS int64) {
		for i := range iterations {
			ts := baseTS + int64(i)*int64(bzz.MinimumUpdateInterval.Seconds()+1)
			_ = r.Put(overlay, cb, ts, bzz.TimestampSourceHandshake, func() error {
				bookkeep.Lock()
				inFlight++
				if inFlight > maxObserved {
					maxObserved = inFlight
				}
				callbacks++
				bookkeep.Unlock()

				// widen the window so any missing serialisation would
				// reliably show up as inFlight > 1.
				time.Sleep(time.Microsecond)

				bookkeep.Lock()
				inFlight--
				bookkeep.Unlock()
				return nil
			})
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); work(cb1, 1_000_000) }()
	go func() { defer wg.Done(); work(cb2, 2_000_000) }()
	wg.Wait()

	bookkeep.Lock()
	defer bookkeep.Unlock()
	if maxObserved != 1 {
		t.Fatalf("Put callbacks ran concurrently: max in-flight was %d, want 1", maxObserved)
	}
	if callbacks == 0 {
		t.Fatal("expected at least one accepted ingestion; got zero callbacks")
	}
	// Sanity: the final registry mapping points to the overlay regardless
	// of which chequebook won the race.
	for _, cb := range []common.Address{cb1, cb2} {
		if peer, ok := r.Peer(cb); ok && !peer.Equal(overlay) {
			t.Fatalf("registry maps %s to %s, want %s", cb.Hex(), peer, overlay)
		}
	}
}

func TestRegistry_Put_PropagatesAddressbookError(t *testing.T) {
	t.Parallel()

	r := chequebook.NewRegistry()
	overlay := swarm.MustParseHexAddress("aabb")
	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")
	cbErr := errors.New("addressbook write failed")

	err := r.Put(overlay, cb, 10, bzz.TimestampSourceHandshake, func() error {
		return cbErr
	})
	if !errors.Is(err, cbErr) {
		t.Fatalf("expected callback error to be propagated, got %v", err)
	}

	// The registry mapping landed before the callback ran, so r.Peer
	// reflects it. The addressbook write is the caller's responsibility
	// on retry.
	got, known := r.Peer(cb)
	if !known || !got.Equal(overlay) {
		t.Fatalf("registry mapping should still be set; got (%s, known=%v)", got, known)
	}
}

func TestRegistry_Put_EmptyChequebookSkipsMappingButRunsCallback(t *testing.T) {
	t.Parallel()

	r := chequebook.NewRegistry()
	overlay := swarm.MustParseHexAddress("aabb")

	called := false
	if err := r.Put(overlay, common.Address{}, 10, bzz.TimestampSourceHandshake, func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("Put(empty cb): unexpected error: %v", err)
	}
	if !called {
		t.Fatal("writeAddressbook callback must run even when chequebook is empty")
	}
	if _, known := r.Peer(common.Address{}); known {
		t.Fatal("registry must not map the zero chequebook to an overlay")
	}
}
