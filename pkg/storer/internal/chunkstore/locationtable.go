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

// locationKey is the first half of a chunk address. Two addresses in one
// table share it only if they agree on 128 bits; such pairs are left out.
type locationKey [swarm.HashSize / 2]byte

func keyOf(addr []byte) (k locationKey) {
	copy(k[:], addr)
	return k
}

// LocationTable maps the addresses of one proximity range to the sharky
// locations the retrieval index held when the table was built. It is
// read-only after BuildLocationTable returns and safe for concurrent use.
type LocationTable struct {
	keys []locationKey // ascending, as the index is ordered by address
	locs []sharky.Location
}

// BuildLocationTable scans the retrieval index over every address with
// proximity of at least depth to anchor.
func BuildLocationTable(ctx context.Context, r storage.Reader, anchor []byte, depth uint8) (*LocationTable, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("chunk store: build location table: %w", err)
	}

	var (
		t       = new(LocationTable)
		dupKey  locationKey
		hasDup  bool
		scanned int
	)
	err := r.Iterate(storage.Query{
		Factory:       func() storage.Item { return new(RetrievalIndexItem) },
		Prefix:        string(rangeStart(anchor, depth)),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		scanned++
		if scanned%locationTableCtxCheck == 0 {
			if err := ctx.Err(); err != nil {
				return true, err
			}
		}

		item := res.Entry.(*RetrievalIndexItem)
		if swarm.Proximity(item.Address.Bytes(), anchor) < depth {
			return true, nil // past the end of the range
		}

		k := keyOf(item.Address.Bytes())
		switch last := len(t.keys) - 1; {
		case hasDup && k == dupKey:
			return false, nil
		case last >= 0 && t.keys[last] == k:
			t.keys, t.locs = t.keys[:last], t.locs[:last]
			dupKey, hasDup = k, true
			return false, nil
		}
		t.keys = append(t.keys, k)
		t.locs = append(t.locs, item.Location)
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
	i, found := slices.BinarySearchFunc(t.keys, keyOf(addr.Bytes()), func(a, b locationKey) int {
		return bytes.Compare(a[:], b[:])
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
