// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/sharky"
)

// releasedSlots records the sharky slots released while a sampling view is
// open. Releases seen before publish are buffered; publish moves them into
// per-shard bitmaps, after which releases set bits without locking.
type releasedSlots struct {
	mu      sync.Mutex                        // guards pending and the switch to bitmaps
	pending []sharky.Location                 // only the releases that happen during the table scan
	bitmaps atomic.Pointer[[][]atomic.Uint64] // by shard, then slot/64
}

// add records loc. It is the sharky.Watch callback of a sampling view.
func (r *releasedSlots) add(loc sharky.Location) {
	if b := r.bitmaps.Load(); b != nil {
		setSlot(*b, loc)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// publish may have run between the load above and the lock.
	if b := r.bitmaps.Load(); b != nil {
		setSlot(*b, loc)
		return
	}
	r.pending = append(r.pending, loc)
}

// publish sizes one bitmap per shard to cover limits and moves the buffered
// releases into them. It must be called once, before contains.
func (r *releasedSlots) publish(limits []uint32) {
	b := make([][]atomic.Uint64, len(limits))
	for shard, limit := range limits {
		b[shard] = make([]atomic.Uint64, (int(limit)+63)/64)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, loc := range r.pending {
		setSlot(b, loc)
	}
	r.pending = nil
	r.bitmaps.Store(&b)
}

// contains reports whether loc's slot was released since the view started
// watching. Slots outside the bitmaps are reported as released.
func (r *releasedSlots) contains(loc sharky.Location) bool {
	w := slotWord(*r.bitmaps.Load(), loc)
	return w == nil || w.Load()&slotBit(loc) != 0
}

// setSlot marks loc's slot. Slots outside the bitmaps cannot be in the
// location table, so they are not recorded.
func setSlot(b [][]atomic.Uint64, loc sharky.Location) {
	if w := slotWord(b, loc); w != nil {
		w.Or(slotBit(loc))
	}
}

// slotWord returns the bitmap word holding loc's slot, or nil if it is outside the bitmaps.
func slotWord(b [][]atomic.Uint64, loc sharky.Location) *atomic.Uint64 {
	if int(loc.Shard) >= len(b) || int(loc.Slot/64) >= len(b[loc.Shard]) {
		return nil
	}
	return &b[loc.Shard][loc.Slot/64]
}

func slotBit(loc sharky.Location) uint64 { return 1 << (loc.Slot % 64) }
