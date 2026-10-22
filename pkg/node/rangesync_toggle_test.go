// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	batchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	stabilmock "github.com/ethersphere/bee/v2/pkg/stabilization/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	topologymock "github.com/ethersphere/bee/v2/pkg/topology/mock"
)

// The tests of this file are the node half of plan task T6.2 (toggle safety
// with the node's rangesync wiring); the storer half is in pkg/storer.
//
// NewBee cannot be built in a unit test (a full node always enables the
// chain backend), so toggleNode builds the parts of it that the flags change
// the way NewBee does: syncWiring decides whether the storer opens the
// address index and whether the rangesync service is built; the service gets
// a reconciler and rangesyncOptions, the reconciler gets the service, and
// Start registers the Indexer, wrapped in the fetch credit's removal record,
// as the reserve observer.

func acceptStamp(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// toggleNode is a node's storer and, when its flags enable it, its rangesync
// service.
type toggleNode struct {
	overlay swarm.Address
	db      *storer.DB
	rec     *streamtest.RecorderDisconnecter
	svc     *rangesync.Service // nil without rangesync
	closer  io.Closer          // the node's rangesync closer, nil without rangesync
	cancel  context.CancelFunc // ends New's context, as NewBee's shutdown does
}

// startToggleNode opens the node's storer in dir with the flags
// --rangesync-enable (rangesyncEnable) and --puller-enable (true), on a full
// node or a light one, and builds and starts its rangesync service when the
// flags enable it, waiting for its Index.
func startToggleNode(t *testing.T, dir string, overlay swarm.Address, full, rangesyncEnable bool) *toggleNode {
	t.Helper()

	addrIndex, registerRangesync, _, err := node.ResolveSyncWiring(full, false, rangesyncEnable, true)
	if err != nil {
		t.Fatal(err)
	}
	opts := &storer.Options{
		Address:           overlay,
		Logger:            log.Noop,
		Batchstore:        batchstore.New(batchstore.WithAcceptAllExistsFunc()),
		ValidStamp:        acceptStamp,
		RadiusSetter:      topologymock.NewTopologyDriver(),
		StartupStabilizer: stabilmock.NewSubscriber(true),
		CacheCapacity:     1000,
	}
	if full {
		// as NewBee: the reserve, and with it the address index, only on full nodes
		opts.ReserveCapacity = 1000
		opts.ReserveAddrIndex = addrIndex
	}
	ctx, cancel := context.WithCancel(context.Background())
	db, err := storer.New(ctx, dir, opts)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	n := &toggleNode{
		overlay: overlay,
		db:      db,
		cancel:  cancel,
		rec:     streamtest.NewRecorderDisconnecter(streamtest.New(streamtest.WithBaseAddr(overlay))),
	}
	if registerRangesync {
		r := reconciler.New(overlay, nil, nil, log.Noop, reconciler.Options{})
		n.svc = rangesync.New(n.rec, db, overlay, nil, nil, acceptStamp, log.Noop, node.RangesyncOptions(r))
		n.closer = node.NewRangesyncCloser(r, n.svc)
		r.SetService(n.svc)
		if err := n.svc.Start(); err != nil {
			n.stop(t)
			t.Fatalf("rangesync start: %v", err)
		}
		waitFor(t, "the index", func() bool { _, err := n.svc.LocalStatus(); return err == nil })
	}
	return n
}

// stop closes the rangesync service, then the storer, in NewBee's Shutdown
// order.
func (n *toggleNode) stop(t *testing.T) {
	t.Helper()

	defer n.cancel()
	if n.closer != nil {
		if err := n.closer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := n.db.Close(); err != nil {
		t.Fatal(err)
	}
}

// putNew puts count new chunks of the batch into the reserve.
func putNew(t *testing.T, db *storer.DB, overlay swarm.Address, batchID []byte, count int) []swarm.Chunk {
	t.Helper()

	chunks := make([]swarm.Chunk, count)
	for i := range chunks {
		chunks[i] = chunktest.GenerateTestRandomChunkAt(t, overlay, i%4).WithStamp(postagetesting.MustNewBatchStamp(batchID))
	}
	putChunks(t, db, chunks...)
	return chunks
}

func putChunks(t *testing.T, db *storer.DB, chunks ...swarm.Chunk) {
	t.Helper()

	for _, ch := range chunks {
		if err := db.ReservePutter().Put(context.Background(), ch); err != nil {
			t.Fatal(err)
		}
	}
}

// newerStamp returns a stamp of st's batch and index with a newer timestamp.
func newerStamp(st swarm.Stamp) swarm.Stamp {
	return postagetesting.MustNewFields(st.BatchID(), binary.BigEndian.Uint64(st.Index()), binary.BigEndian.Uint64(st.Timestamp())+1)
}

// restamped returns ch with a newer stamp: the reserve replaces its element.
func restamped(ch swarm.Chunk) swarm.Chunk {
	return swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(newerStamp(ch.Stamp()))
}

// overwriting returns a new chunk that takes ch's stamp index over: the
// reserve removes ch.
func overwriting(t *testing.T, overlay swarm.Address, ch swarm.Chunk) swarm.Chunk {
	t.Helper()

	return chunktest.GenerateTestRandomChunkAt(t, overlay, 0).WithStamp(newerStamp(ch.Stamp()))
}

// reserveIDs returns the IDs of the reserve, read through its bin index
// (ReserveIterateChunks), which the address index does not maintain.
func reserveIDs(t *testing.T, db *storer.DB) map[fingerprint.ID]struct{} {
	t.Helper()

	ids := make(map[fingerprint.ID]struct{})
	err := db.ReserveIterateChunks(func(ch swarm.Chunk) (bool, error) {
		sh, err := ch.Stamp().Hash()
		if err != nil {
			return true, err
		}
		ids[fingerprint.NewID(ch.Address(), sh)] = struct{}{}
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// minus returns the IDs of a that are not in b.
func minus(a, b map[fingerprint.ID]struct{}) []fingerprint.ID {
	var out []fingerprint.ID
	for id := range a {
		if _, ok := b[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

// bruteSummary returns the summary of the IDs in r.
func bruteSummary(ids map[fingerprint.ID]struct{}, r fingerprint.Range) fingerprint.Summary {
	var (
		h fingerprint.LtHash
		s fingerprint.Summary
	)
	for id := range ids {
		if r.ContainsID(id) {
			h.Add(id)
			s.Count++
		}
	}
	if s.Count > 0 {
		s.Digest = h.Digest()
	}
	return s
}

// idSet returns the set of the chunk IDs.
func idSet(cs []storer.ChunkID) map[fingerprint.ID]struct{} {
	set := make(map[fingerprint.ID]struct{}, len(cs))
	for _, c := range cs {
		set[fingerprint.NewID(c.Address, c.StampHash)] = struct{}{}
	}
	return set
}

func chunkIDs(ids []fingerprint.ID) []storer.ChunkID {
	cs := make([]storer.ChunkID, len(ids))
	for i, id := range ids {
		cs[i] = storer.ChunkID{Address: id.Address(), StampHash: id.StampHash()}
	}
	return cs
}

// checkServed checks what n's rangesync serves against a brute force over
// its reserve, with stale the IDs the reserve no longer holds:
//   - the Index (the node's Status and the summary of every range down to 4
//     bits below its root) equals the brute force;
//   - the address index reports each reserve ID and none of stale;
//   - an empty peer reconciling with n wants exactly the reserve (no stale
//     ID is listed) and fetches all of it from n, and n delivers no stale ID.
func checkServed(t *testing.T, n *toggleNode, stale []fingerprint.ID) {
	t.Helper()

	ctx := context.Background()
	ids := reserveIDs(t, n.db)
	if len(ids) == 0 {
		t.Fatal("empty reserve: the check covers nothing")
	}
	st, err := n.svc.LocalStatus()
	if err != nil {
		t.Fatal(err)
	}
	if want := fingerprint.NewRange(n.overlay, n.db.StorageRadius()); !st.Root.Equal(want) {
		t.Fatalf("index root %v, want %v", st.Root, want)
	}
	if want := bruteSummary(ids, st.Root); st.Summary != want {
		t.Fatalf("index root summary %+v, brute force %+v", st.Summary, want)
	}
	for d := uint8(1); d <= 4; d++ {
		for _, r := range st.Root.Children(d) {
			got, err := n.svc.RangeSummary(r)
			if err != nil {
				t.Fatal(err)
			}
			if want := bruteSummary(ids, r); got != want {
				t.Fatalf("index summary of %v: %+v, brute force %+v", r, got, want)
			}
		}
	}

	for id := range ids {
		if has, err := n.db.ReserveHasID(id.Address(), id.StampHash()); err != nil || !has {
			t.Fatalf("ReserveHasID of reserve element %s: %v, %v", id.Address(), has, err)
		}
	}
	for _, id := range stale {
		if _, ok := ids[id]; ok {
			t.Fatalf("stale element %s is in the reserve: the check covers nothing", id.Address())
		}
		if has, err := n.db.ReserveHasID(id.Address(), id.StampHash()); err != nil || has {
			t.Fatalf("ReserveHasID of stale element %s: %v, %v", id.Address(), has, err)
		}
		if _, err := n.db.ReserveGetByID(ctx, id.Address(), id.StampHash()); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("ReserveGetByID of stale element %s: %v", id.Address(), err)
		}
	}

	peerAddr := swarm.RandAddress(t)
	peerStore := mockstorer.NewReserve(mockstorer.WithAddrIndex())
	peerRec := streamtest.NewRecorderDisconnecter(streamtest.New(streamtest.WithBaseAddr(peerAddr)))
	peer := rangesync.New(peerRec, peerStore, peerAddr, nil, nil, acceptStamp, log.Noop, rangesync.Options{})
	defer func() {
		if err := peer.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := peer.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the peer's index", func() bool { _, err := peer.LocalStatus(); return err == nil })
	n.rec.SetProtocols(peer.Protocol())
	peerRec.SetProtocols(n.svc.Protocol())
	if err := n.svc.SendStatus(ctx, peerAddr); err != nil {
		t.Fatal(err)
	}
	if err := peer.SendStatus(ctx, n.overlay); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the node's status at the peer", func() bool { _, ok := peer.LastStatus(n.overlay); return ok })

	wants, err := peer.Reconcile(ctx, n.overlay)
	if err != nil {
		t.Fatal(err)
	}
	if got := idSet(wants); !maps.Equal(got, ids) {
		t.Fatalf("the peer wants %d IDs (%d stale), the node's reserve holds %d", len(got), len(minus(got, ids)), len(ids))
	}
	// A few IDs per fetch stream: a streamtest stream deadlocks when its
	// reader lags more than 16 writes behind and then closes it.
	fetched := 0
	for batch := range slices.Chunk(wants, 4) {
		stored, err := peer.Fetch(ctx, n.overlay, batch)
		if err != nil {
			t.Fatal(err)
		}
		fetched += stored
	}
	if fetched != len(ids) {
		t.Fatalf("the peer fetched %d of %d chunks", fetched, len(ids))
	}
	if len(stale) > 0 {
		stored, _ := peer.Fetch(ctx, n.overlay, chunkIDs(stale)) // the node leaves them missing
		if stored != 0 {
			t.Fatalf("the node delivered %d stale chunks", stored)
		}
	}
}

// checkNoRangesync checks a node whose flags do not enable rangesync: no
// rangesync service (so no protocol and no Index), and a storer whose reserve
// observer cannot be registered and whose address index is not ready.
func checkNoRangesync(t *testing.T, n *toggleNode) {
	t.Helper()

	if n.svc != nil || n.closer != nil {
		t.Fatal("rangesync service built")
	}
	if err := n.db.SetReserveObserver(nopObserver{}); !errors.Is(err, storer.ErrAddrIndexDisabled) {
		t.Fatalf("SetReserveObserver: %v", err)
	}
	if n.db.ReserveAddrIndexReady() {
		t.Fatal("address index ready")
	}
}

type nopObserver struct{}

func (nopObserver) Commit(run func() (adds, removes []storer.ChunkID, err error)) error {
	_, _, err := run()
	return err
}
func (nopObserver) RadiusChanged(uint8) {}
func (nopObserver) Reset()              {}

// rawItem reads the entries of a namespace without their item type.
type rawItem struct{ ns, id string }

func (r *rawItem) ID() string               { return r.id }
func (r *rawItem) Namespace() string        { return r.ns }
func (r *rawItem) Marshal() ([]byte, error) { return nil, nil }
func (r *rawItem) Unmarshal([]byte) error   { return nil }
func (r *rawItem) Clone() storage.Item      { return &rawItem{ns: r.ns, id: r.id} }
func (r *rawItem) String() string           { return r.ns + "/" + r.id }

// addrIndexKeys returns the number of address index entries and markers in
// the node's store.
func addrIndexKeys(t *testing.T, db *storer.DB) (entries, markers int) {
	t.Helper()

	count := func(ns string) int {
		n, err := db.Storage().IndexStore().Count(&rawItem{ns: ns})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	return count("chunkAddr"), count("addrIndexValid")
}

// waitAddrIndexDropped waits for the background drop of a disabled start.
func waitAddrIndexDropped(t *testing.T, n *toggleNode) {
	t.Helper()

	waitFor(t, "the drop of the address index", func() bool { e, m := addrIndexKeys(t, n.db); return e == 0 && m == 0 })
}

// checkNoAddrIndexKeys waits for the background drop of a disabled start,
// then checks that reserve changes write no address index entry.
func checkNoAddrIndexKeys(t *testing.T, n *toggleNode, batchID []byte) {
	t.Helper()

	waitAddrIndexDropped(t, n)
	chunks := putNew(t, n.db, n.overlay, batchID, 4)
	putChunks(t, n.db, restamped(chunks[0]))
	if e, m := addrIndexKeys(t, n.db); e != 0 || m != 0 {
		t.Fatalf("%d address index entries and %d markers written without rangesync", e, m)
	}
}

// TestRangesyncToggle runs a full node through the toggle cases with the
// node's rangesync wiring: defaults, then rangesync on (the first fill), off
// with reserve changes, on again (the refill), `bee db repair-reserve`, on
// again, and the defaults once more. Whenever rangesync is on, the Index it
// builds and keeps current through its reserve observer equals a brute force
// over the reserve, and no element removed while it was off is served, to an
// empty peer's session or fetch included. Whenever it is off, no rangesync
// service (and so no protocol and no Index) is built, no reserve observer can
// be registered, the leftovers of the index are dropped and nothing of it is
// written.
func TestRangesyncToggle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	overlay := swarm.RandAddress(t)
	b1, b2, b3 := postagetesting.MustNewBatch(), postagetesting.MustNewBatch(), postagetesting.MustNewBatch()

	n := startToggleNode(t, dir, overlay, true, false)
	checkNoRangesync(t, n)
	c1 := putNew(t, n.db, overlay, b1.ID, 16)
	n.stop(t)

	// on: the first fill
	n = startToggleNode(t, dir, overlay, true, true)
	checkServed(t, n, nil)
	// changes with rangesync running reach the Index through the observer
	before := reserveIDs(t, n.db)
	c2 := putNew(t, n.db, overlay, b2.ID, 8)
	putChunks(t, n.db, restamped(c1[0]), overwriting(t, overlay, c1[1]))
	checkServed(t, n, minus(before, reserveIDs(t, n.db)))
	before = reserveIDs(t, n.db)
	n.stop(t)

	// off, with changes the address index does not see
	n = startToggleNode(t, dir, overlay, true, false)
	checkNoRangesync(t, n)
	checkNoAddrIndexKeys(t, n, b3.ID)
	putChunks(t, n.db, restamped(c2[0]), restamped(c2[1]), overwriting(t, overlay, c2[2]))
	n.stop(t)

	// on: the refill
	n = startToggleNode(t, dir, overlay, true, true)
	stale := minus(before, reserveIDs(t, n.db))
	if len(stale) != 3 {
		t.Fatalf("%d elements removed while off, want 3", len(stale))
	}
	checkServed(t, n, stale)
	before = reserveIDs(t, n.db)
	n.stop(t)

	// bee db repair-reserve: the storer opened with the defaults, then the
	// repairer, which renumbers the bins and drops the address index
	n = startToggleNode(t, dir, overlay, true, false)
	if err := localmigration.ReserveRepairer(n.db.Storage(), storage.ChunkType, log.Noop)(); err != nil {
		t.Fatal(err)
	}
	n.stop(t)

	n = startToggleNode(t, dir, overlay, true, true)
	if got := reserveIDs(t, n.db); !maps.Equal(got, before) {
		t.Fatalf("the repair changed the reserve: %d elements, had %d", len(got), len(before))
	}
	checkServed(t, n, nil)
	n.stop(t)

	n = startToggleNode(t, dir, overlay, true, false)
	defer n.stop(t)
	checkNoRangesync(t, n)
	checkNoAddrIndexKeys(t, n, b3.ID)
}

// TestRangesyncDefaultsNoIndex checks the defaults and a light node, which
// ignores --rangesync-enable, on a store whose address index was filled: no
// rangesync service, protocol or Index; the storer opened without the address
// index (no observer can be registered, the leftovers are dropped, the full
// node's reserve changes write nothing); and a rangesync service built on
// such a storer anyway (a wiring error) does not start.
func TestRangesyncDefaultsNoIndex(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		full bool
		rs   bool
	}{
		{name: "full node defaults", full: true},
		{name: "light node with rangesync", rs: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			overlay := swarm.RandAddress(t)
			batch := postagetesting.MustNewBatch()
			n := startToggleNode(t, dir, overlay, true, true)
			putNew(t, n.db, overlay, batch.ID, 8)
			checkServed(t, n, nil)
			n.stop(t)

			n = startToggleNode(t, dir, overlay, tc.full, tc.rs)
			defer n.stop(t)
			checkNoRangesync(t, n)
			if tc.full {
				checkNoAddrIndexKeys(t, n, batch.ID)
			} else {
				waitAddrIndexDropped(t, n) // no reserve: the index is disabled
			}
			svc := rangesync.New(n.rec, n.db, overlay, nil, nil, acceptStamp, log.Noop, rangesync.Options{})
			defer svc.Close()
			if err := svc.Start(); !errors.Is(err, storer.ErrAddrIndexDisabled) {
				t.Fatalf("rangesync start on a storer without the address index: %v", err)
			}
		})
	}
}
