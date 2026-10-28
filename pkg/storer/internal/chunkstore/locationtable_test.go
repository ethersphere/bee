// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore_test

import (
	"bytes"
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

func TestLocationTableSharedPrefix(t *testing.T) {
	t.Parallel()

	st := newTableIndex(t)
	a := swarm.RandAddress(t)
	withPrefix := func(i int) swarm.Address {
		b := slices.Clone(a.Bytes())
		b[swarm.HashSize-1-i] ^= 1 // same first 16 bytes as a
		return swarm.NewAddress(b)
	}
	addrs := []swarm.Address{a, withPrefix(0), withPrefix(1)}
	locs := putRetrievalItems(t, st, addrs)

	table, err := chunkstore.BuildLocationTable(context.Background(), st, a.Bytes(), 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, addr := range addrs {
		if loc, ok := table.Lookup(addr); !ok || loc != locs[addr.ByteString()] {
			t.Fatalf("address %s: got %v %v, want %v", addr, loc, ok, locs[addr.ByteString()])
		}
	}
	// An address added after the build must not match an entry that shares its prefix.
	if loc, ok := table.Lookup(withPrefix(2)); ok {
		t.Fatalf("address missing from the index found at %v", loc)
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

func TestLocationTableSlotLimits(t *testing.T) {
	t.Parallel()

	st := newTableIndex(t)
	addrs := make([]swarm.Address, 10)
	for i := range addrs {
		addrs[i] = swarm.RandAddress(t)
	}
	locs := putRetrievalItems(t, st, addrs) // shard i%4, slot i

	table, err := chunkstore.BuildLocationTable(context.Background(), st, swarm.ZeroAddress.Bytes(), 0)
	if err != nil {
		t.Fatal(err)
	}

	want := make([]uint32, 4)
	for _, loc := range locs {
		want[loc.Shard] = max(want[loc.Shard], loc.Slot+1)
	}
	if got := table.SlotLimits(); !slices.Equal(got, want) {
		t.Fatalf("slot limits: got %v, want %v", got, want)
	}
}

// rawIndexItem stores arbitrary bytes under a retrieval index key.
type rawIndexItem struct {
	addr swarm.Address
	data []byte
}

func (r *rawIndexItem) ID() string               { return r.addr.ByteString() }
func (r *rawIndexItem) Namespace() string        { return (&chunkstore.RetrievalIndexItem{}).Namespace() }
func (r *rawIndexItem) Marshal() ([]byte, error) { return r.data, nil }
func (r *rawIndexItem) Unmarshal([]byte) error   { return nil }
func (r *rawIndexItem) Clone() storage.Item {
	return &rawIndexItem{addr: r.addr, data: slices.Clone(r.data)}
}
func (r *rawIndexItem) String() string { return r.addr.String() }

// An unreadable entry ends the scan: the entries before it stay in the table
// and the build still succeeds.
func TestLocationTableStopsAtUnreadableEntry(t *testing.T) {
	t.Parallel()

	st := newTableIndex(t)
	anchor := swarm.RandAddress(t)
	good := make([]swarm.Address, 8)
	for i := range good {
		good[i] = swarm.RandAddressAt(t, anchor, 4)
	}
	locs := putRetrievalItems(t, st, good)
	bad := swarm.RandAddressAt(t, anchor, 4)
	if err := st.Put(&rawIndexItem{addr: bad, data: make([]byte, 7)}); err != nil {
		t.Fatal(err)
	}

	table, err := chunkstore.BuildLocationTable(context.Background(), st, anchor.Bytes(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if table.Err() == nil {
		t.Fatal("scan error not reported")
	}
	want := 0
	for _, a := range good {
		if bytes.Compare(a.Bytes(), bad.Bytes()) < 0 {
			want++
			if loc, ok := table.Lookup(a); !ok || loc != locs[a.ByteString()] {
				t.Fatalf("address %s: got %v %v, want %v", a, loc, ok, locs[a.ByteString()])
			}
		}
	}
	if table.Len() != want {
		t.Fatalf("table size %d, want %d", table.Len(), want)
	}
	if loc, ok := table.Lookup(bad); ok {
		t.Fatalf("unreadable entry found at %v", loc)
	}
}

// failingReader ends an iteration with err once n entries have been delivered.
type failingReader struct {
	storage.Reader
	n   int
	err error
}

func (r *failingReader) Iterate(q storage.Query, fn storage.IterateFn) error {
	seen := 0
	err := r.Reader.Iterate(q, func(res storage.Result) (bool, error) {
		if seen == r.n {
			return true, nil
		}
		seen++
		return fn(res)
	})
	if err != nil {
		return err
	}
	return r.err
}

func TestLocationTableKeepsEntriesReadBeforeScanError(t *testing.T) {
	t.Parallel()

	st := newTableIndex(t)
	addrs := make([]swarm.Address, 5)
	for i := range addrs {
		addrs[i] = swarm.RandAddress(t)
	}
	locs := putRetrievalItems(t, st, addrs)
	scanErr := errors.New("scan")

	table, err := chunkstore.BuildLocationTable(context.Background(), &failingReader{Reader: st, n: 3, err: scanErr}, swarm.ZeroAddress.Bytes(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(table.Err(), scanErr) {
		t.Fatalf("scan error %v, want %v", table.Err(), scanErr)
	}
	if table.Len() != 3 {
		t.Fatalf("table size %d, want 3", table.Len())
	}
	found := 0
	for _, a := range addrs {
		loc, ok := table.Lookup(a)
		if !ok {
			continue
		}
		found++
		if loc != locs[a.ByteString()] {
			t.Fatalf("address %s: location %v, want %v", a, loc, locs[a.ByteString()])
		}
	}
	if found != 3 {
		t.Fatalf("found %d addresses, want 3", found)
	}
}
