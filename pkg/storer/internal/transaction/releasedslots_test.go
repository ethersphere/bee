// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
)

func TestReleasedSlots(t *testing.T) {
	t.Parallel()

	var r transaction.ReleasedSlots
	beforePublish := sharky.Location{Shard: 1, Slot: 70}
	r.Add(beforePublish)
	r.Add(sharky.Location{Shard: 5, Slot: 1}) // no bitmap for shard 5: ignored
	r.Publish([]uint32{10, 100})
	afterPublish := sharky.Location{Shard: 0, Slot: 3}
	r.Add(afterPublish)

	for _, tc := range []struct {
		loc  sharky.Location
		want bool
	}{
		{beforePublish, true},
		{afterPublish, true},
		{sharky.Location{Shard: 1, Slot: 71}, false},
		{sharky.Location{Shard: 0, Slot: 9}, false},
		{sharky.Location{Shard: 1, Slot: 200}, true}, // outside the bitmap
		{sharky.Location{Shard: 2}, true},            // no bitmap for shard 2
	} {
		if got := r.Contains(tc.loc); got != tc.want {
			t.Errorf("Contains(%v) = %t, want %t", tc.loc, got, tc.want)
		}
	}
}

// TestReleasedSlotsConcurrentPublish checks that no release racing with
// Publish is lost between the buffer and the bitmaps.
func TestReleasedSlotsConcurrentPublish(t *testing.T) {
	t.Parallel()

	const n = 1000
	var (
		r  transaction.ReleasedSlots
		wg sync.WaitGroup
	)
	for i := range n {
		wg.Go(func() { r.Add(sharky.Location{Slot: uint32(i)}) })
	}
	r.Publish([]uint32{n})
	wg.Wait()

	for i := range n {
		if loc := (sharky.Location{Slot: uint32(i)}); !r.Contains(loc) {
			t.Fatalf("release of %v lost", loc)
		}
	}
}
