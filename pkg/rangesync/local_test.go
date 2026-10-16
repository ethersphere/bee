// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
	"slices"
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
	svc := rangesync.New(streamtest.NewRecorderDisconnecter(streamtest.New()), store, overlay, log.Noop, rangesync.Options{LeafBits: leafBits})
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
