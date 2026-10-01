// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// locationTableCtxCheck is how many index entries are scanned between context checks.
const locationTableCtxCheck = 4096

// LocationTable maps the addresses of one proximity range to the sharky
// locations the retrieval index held when the table was built. It is
// read-only after BuildLocationTable returns and safe for concurrent use.
type LocationTable struct {
	keys   [][swarm.HashSize]byte // ascending, as the index is ordered by address
	locs   []sharky.Location
	limits []uint32 // by shard, above every slot in locs
}

// BuildLocationTable scans the retrieval index over every address with
// proximity of at least depth to anchor.
func BuildLocationTable(ctx context.Context, r storage.Reader, anchor []byte, depth uint8) (*LocationTable, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("chunk store: build location table: %w", err)
	}

	var (
		t       = new(LocationTable)
		item    = new(RetrievalIndexItem) // reused: the callback copies what it keeps
		scanned int
	)
	err := r.Iterate(storage.Query{
		Factory:       func() storage.Item { return item },
		Prefix:        string(rangeStart(anchor, depth)),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		scanned++
		if scanned%locationTableCtxCheck == 0 {
			if err := ctx.Err(); err != nil {
				return true, err
			}
		}

		if swarm.Proximity(item.Address.Bytes(), anchor) < depth {
			return true, nil // past the end of the range
		}

		t.keys = append(t.keys, [swarm.HashSize]byte(item.Address.Bytes()))
		t.locs = append(t.locs, item.Location)
		t.coverSlot(item.Location)
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("chunk store: build location table: %w", err)
	}
	return t, nil
}

// rangeStart returns the lowest address with proximity of at least depth to anchor.
func rangeStart(anchor []byte, depth uint8) []byte {
	start := make([]byte, swarm.HashSize)
	copy(start, anchor)
	full, rem := int(depth/8), depth%8
	if full < len(start) {
		start[full] &^= byte(0xff) >> rem
		clear(start[full+1:])
	}
	return start
}

// Lookup returns the location recorded for addr, or false if addr was not in
// the range when the table was built.
func (t *LocationTable) Lookup(addr swarm.Address) (sharky.Location, bool) {
	i, found := slices.BinarySearchFunc(t.keys, addr.Bytes(), func(k [swarm.HashSize]byte, target []byte) int {
		return bytes.Compare(k[:], target)
	})
	if !found {
		return sharky.Location{}, false
	}
	return t.locs[i], true
}

// Len returns the number of addresses in the table.
func (t *LocationTable) Len() int {
	return len(t.keys)
}

// SlotLimits returns, indexed by shard, one more than the highest slot in the
// table.
func (t *LocationTable) SlotLimits() []uint32 {
	return t.limits
}

// coverSlot raises the limit of loc's shard to cover loc.
func (t *LocationTable) coverSlot(loc sharky.Location) {
	if n := int(loc.Shard) + 1; n > len(t.limits) {
		t.limits = append(t.limits, make([]uint32, n-len(t.limits))...)
	}
	t.limits[loc.Shard] = max(t.limits[loc.Shard], loc.Slot+1)
}
