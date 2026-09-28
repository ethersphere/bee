// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func newTableIndex(t *testing.T) *leveldbstore.Store {
	t.Helper()
	st, _, err := leveldbstore.New("", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// putRetrievalItems stores one retrieval index entry per address, each with a
// distinct location, and returns the locations by address.
func putRetrievalItems(t *testing.T, st storage.Writer, addrs []swarm.Address) map[string]sharky.Location {
	t.Helper()
	locs := make(map[string]sharky.Location, len(addrs))
	for i, a := range addrs {
		loc := sharky.Location{Shard: uint8(i % 4), Slot: uint32(i), Length: 4096}
		if err := st.Put(&chunkstore.RetrievalIndexItem{Address: a, Location: loc, RefCnt: 1}); err != nil {
			t.Fatal(err)
		}
		locs[a.ByteString()] = loc
	}
	return locs
}

func TestLocationTableRange(t *testing.T) {
	t.Parallel()

	for _, depth := range []uint8{0, 3, 8, 13, 31} {
		t.Run(fmt.Sprintf("depth %d", depth), func(t *testing.T) {
			t.Parallel()

			st := newTableIndex(t)
			anchor := swarm.RandAddress(t)
			addrs := make([]swarm.Address, 0, 3*(int(swarm.MaxPO)+1))
			for po := range int(swarm.MaxPO) + 1 {
				for range 3 {
					addrs = append(addrs, swarm.RandAddressAt(t, anchor, po))
				}
			}
			locs := putRetrievalItems(t, st, addrs)

			table, err := chunkstore.BuildLocationTable(context.Background(), st, anchor.Bytes(), depth)
			if err != nil {
				t.Fatal(err)
			}

			inRange := 0
			for _, a := range addrs {
				want := swarm.Proximity(a.Bytes(), anchor.Bytes()) >= depth
				loc, ok := table.Lookup(a)
				if ok != want {
					t.Fatalf("address %s (po %d): found %v, want %v", a, swarm.Proximity(a.Bytes(), anchor.Bytes()), ok, want)
				}
				if want {
					inRange++
					if loc != locs[a.ByteString()] {
						t.Fatalf("address %s: location %v, want %v", a, loc, locs[a.ByteString()])
					}
				}
			}
			if table.Len() != inRange {
				t.Fatalf("table size %d, want %d", table.Len(), inRange)
			}
		})
	}
}

func TestLocationTableKeyCollision(t *testing.T) {
	t.Parallel()

	st := newTableIndex(t)
	a := swarm.RandAddress(t)
	twin := slices.Clone(a.Bytes())
	twin[swarm.HashSize-1] ^= 1 // same first 16 bytes as a
	triplet := slices.Clone(a.Bytes())
	triplet[swarm.HashSize-2] ^= 1
	other := swarm.RandAddress(t)
	addrs := []swarm.Address{a, swarm.NewAddress(twin), swarm.NewAddress(triplet), other}
	locs := putRetrievalItems(t, st, addrs)

	table, err := chunkstore.BuildLocationTable(context.Background(), st, a.Bytes(), 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, colliding := range addrs[:3] {
		if _, ok := table.Lookup(colliding); ok {
			t.Fatalf("colliding address %s must miss the table", colliding)
		}
	}
	if loc, ok := table.Lookup(other); !ok || loc != locs[other.ByteString()] {
		t.Fatalf("other address: got %v %v, want %v", loc, ok, locs[other.ByteString()])
	}
	if table.Len() != 1 {
		t.Fatalf("table size %d, want 1", table.Len())
	}
}

func TestLocationTableCanceledContext(t *testing.T) {
	t.Parallel()

	st := newTableIndex(t)
	putRetrievalItems(t, st, []swarm.Address{swarm.RandAddress(t)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := chunkstore.BuildLocationTable(ctx, st, swarm.ZeroAddress.Bytes(), 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}
