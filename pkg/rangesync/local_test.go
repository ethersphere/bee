// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// bruteSummary returns the summary of the IDs of set in r.
func bruteSummary(set map[fingerprint.ID]struct{}, r fingerprint.Range) fingerprint.Summary {
	var h fingerprint.LtHash
	var n uint64
	for id := range set {
		if r.ContainsID(id) {
			h.Add(id)
			n++
		}
	}
	if n == 0 {
		return fingerprint.Summary{}
	}
	return fingerprint.Summary{Count: n, Digest: h.Digest()}
}

// allRanges returns root and all its sub-ranges down to depth bits below it.
func allRanges(root fingerprint.Range, depth uint8) []fingerprint.Range {
	out := []fingerprint.Range{root}
	for d := uint8(1); d <= depth; d++ {
		out = append(out, root.Children(d)...)
	}
	return out
}

// TestLocalIndexWiring checks the Local of a service over the mock reserve:
// after puts, evictions and replacements through the mock, Summary matches a
// brute-force summary for every range down to LeafDepthSlack bits below the
// index leaf depth (the deepest ones are scans), and IterateIDs and HasID
// match the set.
func TestLocalIndexWiring(t *testing.T) {
	t.Parallel()

	const (
		leafBits = 4
		radius   = 2
	)
	overlay := swarm.RandAddress(t)
	store := mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithRadius(radius))
	svc := rangesync.New(streamtest.NewRecorderDisconnecter(streamtest.New()), store, overlay, nil, nil, nil, log.Noop, rangesync.Options{LeafBits: leafBits})
	t.Cleanup(func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	})
	root := fingerprint.NewRange(overlay, radius)

	set := make(map[fingerprint.ID]struct{}) // the elements in root
	put := func(ch swarm.Chunk) fingerprint.ID {
		t.Helper()
		if err := store.ReservePutter().Put(context.Background(), ch); err != nil {
			t.Fatal(err)
		}
		c := chunkIDOf(t, ch)
		id := fingerprint.NewID(c.Address, c.StampHash)
		if root.ContainsID(id) {
			set[id] = struct{}{}
		}
		return id
	}
	evict := func(id fingerprint.ID) {
		t.Helper()
		if err := store.RemoveID(id.Address(), id.StampHash()); err != nil {
			t.Fatal(err)
		}
		delete(set, id)
	}
	// most chunks inside the root, some outside
	chunkIn := func() swarm.Chunk {
		addr := swarm.RandAddress(t).Bytes()
		addr[0] = addr[0]&0x3f | overlay.Bytes()[0]&0xc0
		return swarm.NewChunk(swarm.NewAddress(addr), []byte{1}).WithStamp(postagetesting.MustNewStamp())
	}

	var ids []fingerprint.ID
	for range 100 {
		ids = append(ids, put(chunkIn()))
	}
	for range 10 {
		put(stampedChunk(t))
	}

	if err := svc.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "index", func() bool {
		st, err := svc.LocalStatus()
		return err == nil && st.Root.Equal(root)
	})

	for range 200 {
		ids = append(ids, put(chunkIn()))
	}
	for _, id := range ids[:50] {
		evict(id)
	}
	// replace: the same address with a new stamp, the old one evicted
	for _, id := range ids[50:80] {
		ch := swarm.NewChunk(id.Address(), []byte{2}).WithStamp(postagetesting.MustNewStamp())
		put(ch)
		evict(id)
	}

	local := svc.Local()
	for _, r := range allRanges(root, leafBits+rangesync.LeafDepthSlack) {
		got, err := local.Summary(r)
		if err != nil {
			t.Fatalf("summary of %s: %v", r, err)
		}
		if want := bruteSummary(set, r); got != want {
			t.Fatalf("summary of %s: got count %d, want %d (or digests differ)", r, got.Count, want.Count)
		}
	}

	want := make([]fingerprint.ID, 0, len(set))
	for id := range set {
		want = append(want, id)
	}
	slices.SortFunc(want, fingerprint.ID.Compare)
	var got []fingerprint.ID
	if err := local.IterateIDs(root, nil, func(id fingerprint.ID) (bool, error) {
		got = append(got, id)
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("iterate: got %d ids, want %d", len(got), len(want))
	}
	got = got[:0]
	if err := local.IterateIDs(root, &want[9], func(id fingerprint.ID) (bool, error) {
		got = append(got, id)
		return len(got) == 5, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want[10:15]) {
		t.Fatal("iterate after: wrong ids")
	}

	for _, id := range ids {
		_, in := set[id]
		has, err := local.HasID(id)
		if err != nil {
			t.Fatal(err)
		}
		if has != in {
			t.Fatalf("has %x: got %v, want %v", id[:4], has, in)
		}
	}
}

// bruteSymbols returns the coded symbols [start, end) of the IDs of set in r.
func bruteSymbols(set map[fingerprint.ID]struct{}, r fingerprint.Range, start, end uint64) []fingerprint.CodedSymbol {
	syms := make([]fingerprint.CodedSymbol, end-start)
	for id := range set {
		if r.ContainsID(id) {
			fingerprint.Encode(fingerprint.NewFloatMapper(), syms, start, id, 1)
		}
	}
	return syms
}

// TestLocalSymbols checks Local.Symbols over the mock reserve: for the root,
// a level-1 range, a bucket and a range below leaf depth, windows inside the
// caches and past them (the ReserveIterateAddr scan path) equal a
// brute-force Encode of the set.
func TestLocalSymbols(t *testing.T) {
	t.Parallel()

	const (
		leafBits = 6
		radius   = 2
	)
	overlay := swarm.RandAddress(t)
	store := mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithRadius(radius))
	svc := rangesync.New(streamtest.NewRecorderDisconnecter(streamtest.New()), store, overlay, nil, nil, nil, log.Noop, rangesync.Options{LeafBits: leafBits})
	t.Cleanup(func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	})
	root := fingerprint.NewRange(overlay, radius)

	set := make(map[fingerprint.ID]struct{})
	put := func() {
		t.Helper()
		addr := swarm.RandAddress(t).Bytes()
		addr[0] = addr[0]&0x3f | overlay.Bytes()[0]&0xc0
		ch := swarm.NewChunk(swarm.NewAddress(addr), []byte{1}).WithStamp(postagetesting.MustNewStamp())
		if err := store.ReservePutter().Put(context.Background(), ch); err != nil {
			t.Fatal(err)
		}
		c := chunkIDOf(t, ch)
		set[fingerprint.NewID(c.Address, c.StampHash)] = struct{}{}
	}
	for range 300 {
		put()
	}
	if err := svc.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "index", func() bool {
		st, err := svc.LocalStatus()
		return err == nil && st.Root.Equal(root)
	})
	for range 200 {
		put()
	}
	waitFor(t, "commits", func() bool {
		s, err := svc.Local().Summary(root)
		return err == nil && s.Count == uint64(len(set))
	})

	local := svc.Local()
	ranges := map[string]fingerprint.Range{
		"root":    root,
		"level-1": root.Children(fingerprint.FanoutBits)[3],
		"bucket":  root.Children(leafBits)[17],
		"small":   root.Children(leafBits + 2)[70],
	}
	windows := [][2]uint64{{0, 0}, {0, 1}, {0, 32}, {5, 31}, {0, 512}, {100, 600}, {0, 4096}, {4000, 5000}}
	for name, r := range ranges {
		for _, w := range windows {
			got, exhausted, err := local.Symbols(r, w[0], w[1])
			if err != nil {
				t.Fatalf("%s %v: %v", name, w, err)
			}
			if exhausted {
				t.Fatalf("%s %v: exhausted with the scan path", name, w)
			}
			if want := bruteSymbols(set, r, w[0], w[1]); !slices.Equal(got, want) {
				t.Fatalf("%s %v: symbols differ from a brute-force encode", name, w)
			}
		}
	}
	if _, _, err := local.Symbols(root, 2, 1); err == nil {
		t.Fatal("inverted window: want error")
	}
}

// TestLocalSymbolsExhausted checks that a level-1 range holding more than
// ScanLimit elements serves only its cache, with exhausted true, for a
// window past it.
func TestLocalSymbolsExhausted(t *testing.T) {
	t.Parallel()

	const radius = 2
	overlay := swarm.RandAddress(t)
	store := mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithRadius(radius))
	svc := rangesync.New(streamtest.NewRecorderDisconnecter(streamtest.New()), store, overlay, nil, nil, nil, log.Noop, rangesync.Options{LeafBits: 6})
	t.Cleanup(func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	})
	root := fingerprint.NewRange(overlay, radius)
	r := root.Children(fingerprint.FanoutBits)[0] // bits radius..radius+4 zero
	set := make(map[fingerprint.ID]struct{})
	for range fingerprint.ScanLimit + 100 {
		addr := swarm.RandAddress(t).Bytes()
		addr[0] = addr[0]&0x03 | overlay.Bytes()[0]&0xc0
		ch := swarm.NewChunk(swarm.NewAddress(addr), []byte{1}).WithStamp(postagetesting.MustNewStamp())
		if err := store.ReservePutter().Put(context.Background(), ch); err != nil {
			t.Fatal(err)
		}
		c := chunkIDOf(t, ch)
		set[fingerprint.NewID(c.Address, c.StampHash)] = struct{}{}
	}
	if err := svc.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "index", func() bool {
		s, err := svc.Local().Summary(root)
		return err == nil && s.Count == uint64(len(set))
	})

	got, exhausted, err := svc.Local().Symbols(r, 500, 600)
	if err != nil {
		t.Fatal(err)
	}
	if !exhausted {
		t.Fatal("want exhausted")
	}
	if want := bruteSymbols(set, r, 500, fingerprint.Level1Symbols); !slices.Equal(got, want) {
		t.Fatalf("got %d symbols, want the %d of the cache window", len(got), len(want))
	}
}

// TestLocalStoreErrors checks that the errors of the reserve a session's
// Local reads wrap rangesync.ErrLocal (a cause on this node, which the
// reconciler neither scores nor backs off), and that the error of an
// IterateIDs callback is returned as it is.
func TestLocalStoreErrors(t *testing.T) {
	t.Parallel()

	storeErr := errors.New("leveldb: closed")
	var fail atomic.Bool
	n := newTestNodeOpts(t, nodeOpts{storeErr: func() error {
		if fail.Load() {
			return storeErr
		}
		return nil
	}}, validChunks(20)...)
	n.start(t)
	local := n.svc.Local()
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	small := root.Children(serviceLeafBits + 2)[5] // below the leaf depth: scanned

	fnErr := errors.New("callback")
	if err := local.IterateIDs(root, nil, func(fingerprint.ID) (bool, error) { return false, fnErr }); !errors.Is(err, fnErr) || errors.Is(err, rangesync.ErrLocal) {
		t.Fatalf("callback error: got %v", err)
	}

	fail.Store(true)
	for name, call := range map[string]func() error{
		"iterate ids": func() error {
			return local.IterateIDs(root, nil, func(fingerprint.ID) (bool, error) { return false, nil })
		},
		"has id": func() error {
			_, err := local.HasID(fingerprint.ID{})
			return err
		},
		"summary scan": func() error {
			_, err := local.Summary(small)
			return err
		},
		"symbols scan": func() error {
			_, _, err := local.Symbols(small, 0, 8)
			return err
		},
	} {
		err := call()
		if !errors.Is(err, rangesync.ErrLocal) || !errors.Is(err, storeErr) {
			t.Errorf("%s: got %v, want %v wrapping %v", name, err, rangesync.ErrLocal, storeErr)
		}
	}
}
