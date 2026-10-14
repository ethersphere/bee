// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
)

// The tests in this file run on transaction.Storage over an on-disk
// leveldbstore (newReserveDiskStorage), as the storer builds it: the in-memory
// test storage writes through without atomic commits and blocks commits during
// iteration, so it cannot show transaction or snapshot behaviour.

func newAddrIndexReserve(t *testing.T, baseAddr swarm.Address, st transaction.Storage, enabled bool) *reserve.Reserve {
	t.Helper()

	r, err := reserve.New(baseAddr, st, 0, kademlia.NewTopologyDriver(), log.Noop, reserve.WithAddrIndex(enabled))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type observerEvent struct {
	kind          string // "commit", "radius" or "reset"
	adds, removes []reserve.ChunkID
	err           error
	radius        uint8
	radiusInForce uint8 // Reserve.Radius() when RadiusChanged was called
}

// recordingObserver records every call. It reports nesting (a Commit from
// inside run) unless concurrent is set: with one goroutine driving the
// reserve, overlapping Commits can only be nested, while batch eviction and
// Reset commit their items from several goroutines.
type recordingObserver struct {
	r          *reserve.Reserve
	concurrent bool

	mu     sync.Mutex
	events []observerEvent

	inRun  atomic.Int32
	nested atomic.Bool
}

func (o *recordingObserver) Commit(run func() (adds, removes []reserve.ChunkID, err error)) error {
	if o.inRun.Add(1) > 1 {
		o.nested.Store(true)
	}
	adds, removes, err := run()
	o.inRun.Add(-1)

	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, observerEvent{kind: "commit", adds: adds, removes: removes, err: err})
	return err
}

func (o *recordingObserver) RadiusChanged(radius uint8) {
	o.mu.Lock()
	defer o.mu.Unlock()
	e := observerEvent{kind: "radius", radius: radius}
	if o.r != nil {
		e.radiusInForce = o.r.Radius()
	}
	o.events = append(o.events, e)
}

func (o *recordingObserver) Reset() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, observerEvent{kind: "reset"})
}

// take returns the events recorded since the last call.
func (o *recordingObserver) take(t *testing.T) []observerEvent {
	t.Helper()

	if !o.concurrent && o.nested.Load() {
		t.Fatal("Commit was called from inside run")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	events := o.events
	o.events = nil
	return events
}

func register(t *testing.T, r *reserve.Reserve) *recordingObserver {
	t.Helper()

	o := &recordingObserver{r: r}
	if err := r.SetObserver(o); err != nil {
		t.Fatal(err)
	}
	return o
}

// registerConcurrent registers an observer that accepts overlapping Commits.
func registerConcurrent(t *testing.T, r *reserve.Reserve) *recordingObserver {
	t.Helper()

	o := &recordingObserver{r: r, concurrent: true}
	if err := r.SetObserver(o); err != nil {
		t.Fatal(err)
	}
	return o
}

func chunkIDOf(t *testing.T, ch swarm.Chunk) reserve.ChunkID {
	t.Helper()

	stampHash, err := ch.Stamp().Hash()
	if err != nil {
		t.Fatal(err)
	}
	return reserve.ChunkID{Address: ch.Address(), StampHash: stampHash, BatchID: ch.Stamp().BatchID()}
}

func idKeys(ids []reserve.ChunkID) []string {
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, elementID(id.Address, id.StampHash)+string(id.BatchID))
	}
	slices.Sort(keys)
	return keys
}

// checkCommit checks that e is a successful commit with exactly these deltas
// (including the batch IDs).
func checkCommit(t *testing.T, e observerEvent, adds, removes []reserve.ChunkID) {
	t.Helper()

	if e.kind != "commit" {
		t.Fatalf("event %q, want a commit", e.kind)
	}
	if e.err != nil {
		t.Fatalf("commit error: %v", e.err)
	}
	if !slices.Equal(idKeys(e.adds), idKeys(adds)) {
		t.Fatalf("adds %d ids, want %d (or different ids)", len(e.adds), len(adds))
	}
	if !slices.Equal(idKeys(e.removes), idKeys(removes)) {
		t.Fatalf("removes %d ids, want %d (or different ids)", len(e.removes), len(removes))
	}
}

// addrIndexItems returns the ChunkAddrItems in the store keyed by
// elementID, with the key fields restored from the iteration key.
func addrIndexItems(t *testing.T, st transaction.ReadOnlyStore) map[string]reserve.ChunkAddrItem {
	t.Helper()

	items := make(map[string]reserve.ChunkAddrItem)
	err := st.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &reserve.ChunkAddrItem{} },
	}, func(res storage.Result) (bool, error) {
		if len(res.ID) != 2*swarm.HashSize {
			return true, fmt.Errorf("chunkAddr key of %d bytes", len(res.ID))
		}
		item := *res.Entry.(*reserve.ChunkAddrItem)
		item.Address = swarm.NewAddress([]byte(res.ID[:swarm.HashSize]))
		item.StampHash = []byte(res.ID[swarm.HashSize:])
		items[res.ID] = item
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return items
}

// checkAddrIndexMatchesReserve checks that the ChunkAddrItems are exactly the
// (address, stamp hash) pairs of the ChunkBinItems, with the same batch ID and
// chunk type.
func checkAddrIndexMatchesReserve(t *testing.T, st transaction.ReadOnlyStore) map[string]int {
	t.Helper()

	items := addrIndexItems(t, st)
	bins := make(map[string]*reserve.ChunkBinItem)
	err := st.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &reserve.ChunkBinItem{} },
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*reserve.ChunkBinItem)
		k := elementID(item.Address, item.StampHash)
		if _, ok := bins[k]; ok {
			return true, fmt.Errorf("two ChunkBinItems for element %x", k)
		}
		bins[k] = item
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(bins) {
		t.Fatalf("%d ChunkAddrItems, %d ChunkBinItems", len(items), len(bins))
	}
	set := make(map[string]int, len(items))
	for k, item := range items {
		bin, ok := bins[k]
		if !ok {
			t.Fatalf("ChunkAddrItem %x has no ChunkBinItem", k)
		}
		if string(item.BatchID) != string(bin.BatchID) || item.ChunkType != bin.ChunkType {
			t.Fatalf("ChunkAddrItem %x: batch %x type %v, ChunkBinItem batch %x type %v", k, item.BatchID, item.ChunkType, bin.BatchID, bin.ChunkType)
		}
		set[k] = 1
	}
	return set
}

func countAddrIndex(t *testing.T, st transaction.ReadOnlyStore) int {
	t.Helper()

	n, err := st.IndexStore().Count(&reserve.ChunkAddrItem{})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// pinChunks puts the chunks in a new pin collection, so eviction keeps their
// data and only removes their reserve metadata.
func pinChunks(t *testing.T, st transaction.Storage, chunks ...swarm.Chunk) {
	t.Helper()

	ctx := context.Background()
	var putter internal.PutterCloserWithReference
	err := st.Run(ctx, func(s transaction.Store) error {
		var err error
		putter, err = pinstore.NewCollection(s.IndexStore())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range chunks {
		if err := st.Run(ctx, func(s transaction.Store) error { return putter.Put(ctx, s, ch) }); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Run(ctx, func(s transaction.Store) error { return putter.Close(s.IndexStore(), chunks[0].Address()) }); err != nil {
		t.Fatal(err)
	}
}

// breakRelease points the chunk's retrieval index at a sharky shard that
// doesn't exist, so releasing its location after a commit fails.
func breakRelease(t *testing.T, st transaction.Storage, addr swarm.Address) {
	t.Helper()

	err := st.Run(context.Background(), func(s transaction.Store) error {
		item := &chunkstore.RetrievalIndexItem{Address: addr}
		if err := s.IndexStore().Get(item); err != nil {
			return err
		}
		item.Location.Shard = 200
		return s.IndexStore().Put(item)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func deleteAddrItem(t *testing.T, st transaction.Storage, id reserve.ChunkID) {
	t.Helper()

	err := st.Run(context.Background(), func(s transaction.Store) error {
		return s.IndexStore().Delete(&reserve.ChunkAddrItem{Address: id.Address, StampHash: id.StampHash})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestAddrIndexDisabled checks the default: with the address index disabled
// no ChunkAddrItem is ever written, no observer can be registered and so no
// Commit, RadiusChanged or Reset call happens.
func TestAddrIndexDisabled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	st := newReserveDiskStorage(t)
	r := newAddrIndexReserve(t, baseAddr, st, false)

	o := &recordingObserver{r: r}
	if err := r.SetObserver(o); !errors.Is(err, reserve.ErrAddrIndexDisabled) {
		t.Fatalf("SetObserver with the address index disabled: %v", err)
	}
	// reserve.New without options is the same as disabled.
	r0, err := reserve.New(baseAddr, newReserveDiskStorage(t), 0, kademlia.NewTopologyDriver(), log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	if err := r0.SetObserver(o); !errors.Is(err, reserve.ErrAddrIndexDisabled) {
		t.Fatalf("SetObserver on a default reserve: %v", err)
	}

	check := func(step string) {
		t.Helper()
		if n := countAddrIndex(t, st); n != 0 {
			t.Fatalf("%s: %d chunkAddr keys written", step, n)
		}
	}

	batch := postagetesting.MustNewBatch()
	ch1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))
	sameAddr := swarm.NewChunk(ch1.Address(), ch1.Data()).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 2))
	otherAddr := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 3))
	for i, ch := range []swarm.Chunk{ch1, sameAddr, otherAddr} {
		if err := r.Put(ctx, ch); err != nil {
			t.Fatal(err)
		}
		check(fmt.Sprintf("put %d", i))
	}

	pinned := chunk.GenerateTestRandomChunkAt(t, baseAddr, 1).WithStamp(postagetesting.MustNewFields(batch.ID, 1, 1))
	if err := r.Put(ctx, pinned); err != nil {
		t.Fatal(err)
	}
	pinChunks(t, st, pinned)
	if _, err := r.EvictBatchBin(ctx, batch.ID, 10, swarm.MaxBins); err != nil {
		t.Fatal(err)
	}
	check("evict")

	for range 3 {
		if err := r.Put(ctx, chunk.GenerateTestRandomChunkAt(t, baseAddr, 2)); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.SetRadius(3); err != nil {
		t.Fatal(err)
	}
	if err := r.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	check("reset")

	if events := o.take(t); len(events) != 0 {
		t.Fatalf("observer of a disabled reserve got %d calls", len(events))
	}
}

func TestAddrIndexSetObserver(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		t.Parallel()

		r := newAddrIndexReserve(t, swarm.RandAddress(t), newReserveDiskStorage(t), true)
		if err := r.SetObserver(nil); !errors.Is(err, reserve.ErrNilObserver) {
			t.Fatalf("SetObserver(nil): %v", err)
		}
		// a rejected registration doesn't use up the single slot
		register(t, r)
	})

	t.Run("second registration", func(t *testing.T) {
		t.Parallel()

		r := newAddrIndexReserve(t, swarm.RandAddress(t), newReserveDiskStorage(t), true)
		register(t, r)
		if err := r.SetObserver(&recordingObserver{}); !errors.Is(err, reserve.ErrObserverRegistered) {
			t.Fatalf("second SetObserver: %v", err)
		}
	})

	t.Run("second registration does not wait for transactions", func(t *testing.T) {
		t.Parallel()

		baseAddr := swarm.RandAddress(t)
		r := newAddrIndexReserve(t, baseAddr, newReserveDiskStorage(t), true)
		o := &blockingObserver{entered: make(chan struct{}), release: make(chan struct{})}
		if err := r.SetObserver(o); err != nil {
			t.Fatal(err)
		}

		// A Put blocks inside Commit, holding the registration read lock.
		ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)
		putErr := make(chan error, 1)
		go func() { putErr <- r.Put(context.Background(), ch) }()
		<-o.entered

		regErr := make(chan error, 1)
		go func() { regErr <- r.SetObserver(&recordingObserver{}) }()
		select {
		case err := <-regErr:
			if !errors.Is(err, reserve.ErrObserverRegistered) {
				t.Fatalf("second SetObserver: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("second SetObserver waited for an in-flight transaction")
		}

		close(o.release)
		if err := <-putErr; err != nil {
			t.Fatal(err)
		}
	})
}

// blockingObserver blocks every Commit before running the transaction until
// release is closed; entered is closed when the first Commit arrives.
type blockingObserver struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (o *blockingObserver) Commit(run func() (adds, removes []reserve.ChunkID, err error)) error {
	o.once.Do(func() { close(o.entered) })
	<-o.release
	_, _, err := run()
	return err
}
func (o *blockingObserver) RadiusChanged(uint8) {}
func (o *blockingObserver) Reset()              {}

func TestAddrIndexPut(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("new chunks", func(t *testing.T) {
		t.Parallel()

		baseAddr := swarm.RandAddress(t)
		st := newReserveDiskStorage(t)
		r := newAddrIndexReserve(t, baseAddr, st, true)
		o := register(t, r)

		cac := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)
		soc := chunk.GenerateTestRandomSoChunk(t, chunk.GenerateTestRandomChunk())
		for _, ch := range []swarm.Chunk{cac, soc} {
			if err := r.Put(ctx, ch); err != nil {
				t.Fatal(err)
			}
			events := o.take(t)
			if len(events) != 1 {
				t.Fatalf("%d events, want one commit", len(events))
			}
			checkCommit(t, events[0], []reserve.ChunkID{chunkIDOf(t, ch)}, nil)
		}
		checkAddrIndexMatchesReserve(t, st)

		// a chunk already in the reserve returns before any transaction
		if err := r.Put(ctx, cac); err != nil {
			t.Fatal(err)
		}
		if events := o.take(t); len(events) != 0 {
			t.Fatalf("duplicate put: %d events, want none", len(events))
		}
	})

	t.Run("same address, newer stamp", func(t *testing.T) {
		t.Parallel()

		baseAddr := swarm.RandAddress(t)
		st := newReserveDiskStorage(t)
		r := newAddrIndexReserve(t, baseAddr, st, true)
		o := register(t, r)

		batch := postagetesting.MustNewBatch()
		ch1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))
		ch2 := swarm.NewChunk(ch1.Address(), ch1.Data()).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 2))
		for _, ch := range []swarm.Chunk{ch1, ch2} {
			if err := r.Put(ctx, ch); err != nil {
				t.Fatal(err)
			}
		}
		events := o.take(t)
		if len(events) != 2 {
			t.Fatalf("%d events, want two commits", len(events))
		}
		checkCommit(t, events[0], []reserve.ChunkID{chunkIDOf(t, ch1)}, nil)
		checkCommit(t, events[1], []reserve.ChunkID{chunkIDOf(t, ch2)}, []reserve.ChunkID{chunkIDOf(t, ch1)})
		if set := checkAddrIndexMatchesReserve(t, st); len(set) != 1 {
			t.Fatalf("%d elements, want 1", len(set))
		}
	})

	t.Run("different address, same stamp index", func(t *testing.T) {
		t.Parallel()

		baseAddr := swarm.RandAddress(t)
		st := newReserveDiskStorage(t)
		r := newAddrIndexReserve(t, baseAddr, st, true)
		o := register(t, r)

		batch := postagetesting.MustNewBatch()
		ch1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))
		ch2 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 1).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 2))
		for _, ch := range []swarm.Chunk{ch1, ch2} {
			if err := r.Put(ctx, ch); err != nil {
				t.Fatal(err)
			}
		}
		events := o.take(t)
		if len(events) != 2 {
			t.Fatalf("%d events, want two commits", len(events))
		}
		checkCommit(t, events[1], []reserve.ChunkID{chunkIDOf(t, ch2)}, []reserve.ChunkID{chunkIDOf(t, ch1)})
		if set := checkAddrIndexMatchesReserve(t, st); len(set) != 1 {
			t.Fatalf("%d elements, want 1", len(set))
		}
	})

	t.Run("stale stamp fails with an empty delta", func(t *testing.T) {
		t.Parallel()

		baseAddr := swarm.RandAddress(t)
		st := newReserveDiskStorage(t)
		r := newAddrIndexReserve(t, baseAddr, st, true)
		o := register(t, r)

		batch := postagetesting.MustNewBatch()
		ch1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 2))
		stale := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))
		if err := r.Put(ctx, ch1); err != nil {
			t.Fatal(err)
		}
		_ = o.take(t)

		err := r.Put(ctx, stale)
		if !errors.Is(err, storage.ErrOverwriteNewerChunk) {
			t.Fatalf("put of a stale stamp: %v, want ErrOverwriteNewerChunk", err)
		}
		events := o.take(t)
		if len(events) != 1 {
			t.Fatalf("%d events, want one commit", len(events))
		}
		if !errors.Is(events[0].err, storage.ErrOverwriteNewerChunk) || len(events[0].adds) != 0 || len(events[0].removes) != 0 {
			t.Fatalf("failed commit: err %v, %d adds, %d removes", events[0].err, len(events[0].adds), len(events[0].removes))
		}
		set := checkAddrIndexMatchesReserve(t, st)
		if len(set) != 1 || set[elementID(ch1.Address(), chunkIDOf(t, ch1).StampHash)] != 1 {
			t.Fatal("the failed transaction changed the address index")
		}
	})
}

func TestAddrIndexEvict(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	for _, pinned := range []bool{false, true} {
		t.Run(fmt.Sprintf("pinned=%v", pinned), func(t *testing.T) {
			t.Parallel()

			baseAddr := swarm.RandAddress(t)
			st := newReserveDiskStorage(t)
			r := newAddrIndexReserve(t, baseAddr, st, true)

			batch := postagetesting.MustNewBatch()
			chunks := make([]swarm.Chunk, 0, 6)
			for i := range cap(chunks) {
				ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, i%2).WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
				if err := r.Put(ctx, ch); err != nil {
					t.Fatal(err)
				}
				chunks = append(chunks, ch)
			}
			// a chunk of another batch stays
			other := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)
			if err := r.Put(ctx, other); err != nil {
				t.Fatal(err)
			}
			if pinned {
				pinChunks(t, st, chunks...)
			}
			o := registerConcurrent(t, r)

			evicted, err := r.EvictBatchBin(ctx, batch.ID, len(chunks), swarm.MaxBins)
			if err != nil {
				t.Fatal(err)
			}
			if evicted != len(chunks) {
				t.Fatalf("evicted %d, want %d", evicted, len(chunks))
			}

			// one commit per item, each removing exactly its id
			events := o.take(t)
			if len(events) != len(chunks) {
				t.Fatalf("%d events, want %d commits", len(events), len(chunks))
			}
			var removed []reserve.ChunkID
			for _, e := range events {
				if e.kind != "commit" || e.err != nil || len(e.adds) != 0 || len(e.removes) != 1 {
					t.Fatalf("eviction event %+v", e)
				}
				removed = append(removed, e.removes...)
			}
			want := make([]reserve.ChunkID, 0, len(chunks))
			for _, ch := range chunks {
				want = append(want, chunkIDOf(t, ch))
			}
			if !slices.Equal(idKeys(removed), idKeys(want)) {
				t.Fatal("evicted ids differ from the batch's chunks")
			}

			set := checkAddrIndexMatchesReserve(t, st)
			if len(set) != 1 || set[elementID(other.Address(), chunkIDOf(t, other).StampHash)] != 1 {
				t.Fatalf("address index after eviction has %d elements, want only the other batch's chunk", len(set))
			}
		})
	}
}

func TestAddrIndexReset(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	st := newReserveDiskStorage(t)
	r := newAddrIndexReserve(t, baseAddr, st, true)

	want := make([]reserve.ChunkID, 0, 5)
	for i := range cap(want) {
		ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, i%3)
		if err := r.Put(ctx, ch); err != nil {
			t.Fatal(err)
		}
		want = append(want, chunkIDOf(t, ch))
	}
	o := registerConcurrent(t, r)

	if err := r.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if n := countAddrIndex(t, st); n != 0 {
		t.Fatalf("%d chunkAddr keys after reset", n)
	}

	events := o.take(t)
	if len(events) != len(want)+1 {
		t.Fatalf("%d events, want %d commits and a reset", len(events), len(want))
	}
	var removed []reserve.ChunkID
	for _, e := range events[:len(want)] {
		if e.kind != "commit" || e.err != nil || len(e.adds) != 0 || len(e.removes) != 1 {
			t.Fatalf("reset event %+v", e)
		}
		removed = append(removed, e.removes...)
	}
	if !slices.Equal(idKeys(removed), idKeys(want)) {
		t.Fatal("reset removed ids differ from the reserve")
	}
	if events[len(want)].kind != "reset" {
		t.Fatalf("last event %q, want reset", events[len(want)].kind)
	}
}

// TestAddrIndexAbsentItem covers items missing from the address index (an
// interrupted or running fill): deleting such a chunk emits no -id, and
// replacing it emits only the +id.
func TestAddrIndexAbsentItem(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	st := newReserveDiskStorage(t)
	r := newAddrIndexReserve(t, baseAddr, st, true)

	batch := postagetesting.MustNewBatch()
	evictee := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))
	replaced := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 1, 1))
	replacement := chunk.GenerateTestRandomChunkAt(t, baseAddr, 1).WithStamp(postagetesting.MustNewFields(batch.ID, 1, 2))
	resetee := chunk.GenerateTestRandomChunkAt(t, baseAddr, 1)
	for _, ch := range []swarm.Chunk{evictee, replaced, resetee} {
		if err := r.Put(ctx, ch); err != nil {
			t.Fatal(err)
		}
		deleteAddrItem(t, st, chunkIDOf(t, ch))
	}
	o := registerConcurrent(t, r) // Reset commits its items concurrently

	if err := r.Put(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	events := o.take(t)
	if len(events) != 1 {
		t.Fatalf("replace: %d events, want one commit", len(events))
	}
	checkCommit(t, events[0], []reserve.ChunkID{chunkIDOf(t, replacement)}, nil)

	if _, err := r.EvictBatchBin(ctx, batch.ID, 1, 1); err != nil {
		t.Fatal(err)
	}
	if has, err := r.Has(evictee.Address(), batch.ID, chunkIDOf(t, evictee).StampHash); err != nil || has {
		t.Fatalf("evictee still in the reserve (has %v, err %v)", has, err)
	}
	events = o.take(t)
	if len(events) != 1 {
		t.Fatalf("evict: %d events, want one commit", len(events))
	}
	checkCommit(t, events[0], nil, nil)

	if err := r.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	events = o.take(t)
	var removed []reserve.ChunkID
	for _, e := range events {
		removed = append(removed, e.removes...)
	}
	// only the replacement had a ChunkAddrItem
	if !slices.Equal(idKeys(removed), idKeys([]reserve.ChunkID{chunkIDOf(t, replacement)})) {
		t.Fatalf("reset removed %d ids, want only the indexed one", len(removed))
	}
}

// TestAddrIndexPostCommitError checks that a transaction whose batch was
// written but that reports an error afterwards (a sharky release) still
// delivers its full delta, and that the reserve returns the error.
func TestAddrIndexPostCommitError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("put replacing a different address", func(t *testing.T) {
		t.Parallel()

		baseAddr := swarm.RandAddress(t)
		st := newReserveDiskStorage(t)
		r := newAddrIndexReserve(t, baseAddr, st, true)

		batch := postagetesting.MustNewBatch()
		ch1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))
		ch2 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 2))
		if err := r.Put(ctx, ch1); err != nil {
			t.Fatal(err)
		}
		breakRelease(t, st, ch1.Address())
		o := register(t, r)

		err := r.Put(ctx, ch2)
		if !errors.Is(err, transaction.ErrAfterCommit) || !errors.Is(err, sharky.ErrShardNotFound) {
			t.Fatalf("put: %v, want the post-commit release error", err)
		}
		events := o.take(t)
		if len(events) != 1 {
			t.Fatalf("%d events, want one commit", len(events))
		}
		checkCommit(t, events[0], []reserve.ChunkID{chunkIDOf(t, ch2)}, []reserve.ChunkID{chunkIDOf(t, ch1)})
		set := checkAddrIndexMatchesReserve(t, st)
		if len(set) != 1 || set[elementID(ch2.Address(), chunkIDOf(t, ch2).StampHash)] != 1 {
			t.Fatal("the committed replacement is not in the address index")
		}
	})

	t.Run("eviction", func(t *testing.T) {
		t.Parallel()

		baseAddr := swarm.RandAddress(t)
		st := newReserveDiskStorage(t)
		r := newAddrIndexReserve(t, baseAddr, st, true)

		ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)
		if err := r.Put(ctx, ch); err != nil {
			t.Fatal(err)
		}
		breakRelease(t, st, ch.Address())
		o := register(t, r)

		_, err := r.EvictBatchBin(ctx, ch.Stamp().BatchID(), 1, swarm.MaxBins)
		if !errors.Is(err, transaction.ErrAfterCommit) {
			t.Fatalf("evict: %v, want the post-commit release error", err)
		}
		events := o.take(t)
		if len(events) != 1 {
			t.Fatalf("%d events, want one commit", len(events))
		}
		checkCommit(t, events[0], nil, []reserve.ChunkID{chunkIDOf(t, ch)})
		if n := countAddrIndex(t, st); n != 0 {
			t.Fatalf("%d chunkAddr keys after the committed eviction", n)
		}
	})
}

func TestAddrIndexDeleteCorruptedChunkMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	st := newReserveDiskStorage(t)
	r := newAddrIndexReserve(t, baseAddr, st, true)

	ch1 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)
	ch2 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)
	for _, ch := range []swarm.Chunk{ch1, ch2} {
		if err := r.Put(ctx, ch); err != nil {
			t.Fatal(err)
		}
	}
	// ch2's ChunkAddrItem is absent: the delete must still succeed.
	deleteAddrItem(t, st, chunkIDOf(t, ch2))

	for _, ch := range []swarm.Chunk{ch1, ch2} {
		err := st.Run(ctx, func(s transaction.Store) error {
			return reserve.DeleteCorruptedChunkMetadata(s.IndexStore(), baseAddr, ch.Address(), true)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := countAddrIndex(t, st); n != 0 {
		t.Fatalf("%d chunkAddr keys after deleting corrupted metadata", n)
	}
	checkAddrIndexMatchesReserve(t, st)

	// With the address index disabled the namespace is not touched.
	ch3 := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)
	if err := r.Put(ctx, ch3); err != nil {
		t.Fatal(err)
	}
	err := st.Run(ctx, func(s transaction.Store) error {
		return reserve.DeleteCorruptedChunkMetadata(s.IndexStore(), baseAddr, ch3.Address(), false)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countAddrIndex(t, st); n != 1 {
		t.Fatalf("%d chunkAddr keys, want the untouched one", n)
	}
}

func TestAddrIndexRadiusChanged(t *testing.T) {
	t.Parallel()

	r := newAddrIndexReserve(t, swarm.RandAddress(t), newReserveDiskStorage(t), true)
	o := register(t, r)

	for _, radius := range []uint8{3, 3, 5, 2} {
		if err := r.SetRadius(radius); err != nil {
			t.Fatal(err)
		}
		events := o.take(t)
		if len(events) != 1 || events[0].kind != "radius" {
			t.Fatalf("SetRadius(%d): events %+v, want one RadiusChanged", radius, events)
		}
		if events[0].radius != radius || events[0].radiusInForce != radius {
			t.Fatalf("SetRadius(%d): RadiusChanged(%d) with Radius() %d", radius, events[0].radius, events[0].radiusInForce)
		}
	}
}

// TestAddrIndexRadiusRegistrationRace checks that a SetRadius racing with
// registration either notifies the observer or has stored its radius by the
// time registration returns, so the observer's first rebuild reads it.
func TestAddrIndexRadiusRegistrationRace(t *testing.T) {
	t.Parallel()

	for i := range 50 {
		r, err := reserve.New(swarm.RandAddress(t), internal.NewInmemStorage(), 0, kademlia.NewTopologyDriver(), log.Noop, reserve.WithAddrIndex(true))
		if err != nil {
			t.Fatal(err)
		}
		o := &recordingObserver{}
		var (
			wg          sync.WaitGroup
			afterSetObs uint8
		)
		setRadius := func() {
			if err := r.SetRadius(7); err != nil {
				t.Error(err)
			}
		}
		setObserver := func() {
			if err := r.SetObserver(o); err != nil {
				t.Error(err)
			}
			afterSetObs = r.Radius()
		}
		// alternate the start order, the scheduler tends to run the
		// goroutine started last first
		if i%2 == 0 {
			wg.Go(setRadius)
			wg.Go(setObserver)
		} else {
			wg.Go(setObserver)
			wg.Go(setRadius)
		}
		wg.Wait()

		notified := false
		for _, e := range o.take(t) {
			notified = notified || (e.kind == "radius" && e.radius == 7)
		}
		if !notified && afterSetObs != 7 {
			t.Fatalf("round %d: radius 7 neither notified nor visible after registration (saw %d)", i, afterSetObs)
		}
	}
}

// modelObserver is a minimal fingerprint index observer: a set of ids, the
// index lock and the rebuild protocol of §5.2. Until its first rebuild opens a
// snapshot it drops deltas; it switches to queuing in the write-locked section
// that opens the snapshot iterator.
type modelObserver struct {
	mu sync.RWMutex // ix.mu

	dmu        sync.Mutex // guards the fields below; commits apply under mu.RLock
	built      bool
	rebuilding bool
	live       map[string]int
	pending    map[string]int

	dropped atomic.Int64
	queued  atomic.Int64
	applied atomic.Int64
}

func (m *modelObserver) Commit(run func() (adds, removes []reserve.ChunkID, err error)) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	adds, removes, err := run()
	if err != nil {
		return err
	}
	if len(adds)+len(removes) == 0 {
		return nil
	}

	m.dmu.Lock()
	defer m.dmu.Unlock()
	if !m.built && !m.rebuilding {
		m.dropped.Add(1)
		return nil
	}
	apply := func(set map[string]int) {
		for _, id := range adds {
			set[elementID(id.Address, id.StampHash)]++
		}
		for _, id := range removes {
			set[elementID(id.Address, id.StampHash)]--
		}
	}
	if m.rebuilding {
		apply(m.pending)
		m.queued.Add(1)
	}
	if m.built {
		apply(m.live)
		m.applied.Add(1)
	}
	return nil
}

func (m *modelObserver) RadiusChanged(uint8) {}
func (m *modelObserver) Reset()              {}

// rebuild runs one rebuild over the ChunkAddrItems: open the snapshot under the
// write lock (switching to queuing in the same section), iterate without the
// lock, then apply the queued deltas and swap under the write lock.
func (m *modelObserver) rebuild(st transaction.ReadOnlyStore, pauseEvery int) error {
	m.mu.Lock()
	m.dmu.Lock()
	m.rebuilding = true
	m.pending = make(map[string]int)
	m.dmu.Unlock()

	var once sync.Once
	release := func() { once.Do(m.mu.Unlock) }

	slab := make(map[string]int)
	n := 0
	err := st.IndexStore().Iterate(storage.Query{
		Factory:      func() storage.Item { return &reserve.ChunkAddrItem{} },
		ItemProperty: storage.QueryItemID,
	}, func(res storage.Result) (bool, error) {
		release()
		slab[res.ID]++
		n++
		if pauseEvery > 0 && n%pauseEvery == 0 {
			time.Sleep(time.Millisecond)
		}
		return false, nil
	})
	release()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.dmu.Lock()
	defer m.dmu.Unlock()
	for k, v := range m.pending {
		slab[k] += v
		if slab[k] == 0 {
			delete(slab, k)
		}
	}
	m.live = slab
	m.pending = nil
	m.rebuilding = false
	m.built = true
	return nil
}

func (m *modelObserver) set() map[string]int {
	m.dmu.Lock()
	defer m.dmu.Unlock()
	out := make(map[string]int, len(m.live))
	for k, v := range m.live {
		if v != 0 {
			out[k] = v
		}
	}
	return out
}

// slowStorage widens every transaction: it sleeps after the transaction's
// writes and before its commit, so a transaction that started before an
// observer was registered is likely to commit after the observer's first
// snapshot opened unless registration waits for it.
type slowStorage struct {
	transaction.Storage
	delay time.Duration
}

func (s *slowStorage) Run(ctx context.Context, f func(transaction.Store) error) error {
	return s.Storage.Run(ctx, func(st transaction.Store) error {
		err := f(st)
		time.Sleep(s.delay)
		return err
	})
}

// TestAddrIndexRegistrationRace registers an observer while Puts (new chunks,
// same- and different-address replacements, stale stamps) and evictions run
// concurrently, then rebuilds from a snapshot right away, as rangesync does.
// Registration must wait for transactions that started without the observer,
// so every commit is either in the snapshot or in the delta stream, and the
// observer's set equals the address index at the end.
//
// Concurrency test: run it with -race in CI.
func TestAddrIndexRegistrationRace(t *testing.T) {
	t.Parallel()

	for round := range 3 {
		t.Run(fmt.Sprintf("round %d", round), func(t *testing.T) {
			t.Parallel()
			runRegistrationRace(t, int64(round))
		})
	}
}

func runRegistrationRace(t *testing.T, seed int64) {
	t.Helper()

	const (
		writers       = 4
		putsPerWriter = 120
		stampIndexes  = 40
		bins          = 3
		evictPerCall  = 4
	)

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	disk := newReserveDiskStorage(t)
	st := &slowStorage{Storage: disk, delay: 500 * time.Microsecond}
	r := newAddrIndexReserve(t, baseAddr, st, true)

	rnd := rand.New(rand.NewSource(seed))
	batchIDs := make([][]byte, writers)
	work := make([][]swarm.Chunk, writers)
	for w := range writers {
		batchIDs[w] = postagetesting.MustNewBatch().ID
		for i := range putsPerWriter {
			ts := uint64(i) + 1
			index := uint64(rnd.Intn(stampIndexes))
			var ch swarm.Chunk
			switch {
			case i > 0 && i%10 == 0:
				prev := work[w][rnd.Intn(i)]
				index = indexOf(prev)
				ch = swarm.NewChunk(prev.Address(), prev.Data())
			case i%15 == 0:
				ts = 0
				ch = chunk.GenerateTestRandomChunkAt(t, baseAddr, rnd.Intn(bins))
			default:
				ch = chunk.GenerateTestRandomChunkAt(t, baseAddr, rnd.Intn(bins))
			}
			work[w] = append(work[w], ch.WithStamp(postagetesting.MustNewFields(batchIDs[w], index, ts)))
		}
	}

	var (
		puts      atomic.Int64
		writersWG sync.WaitGroup
		bgWG      sync.WaitGroup
		stop      = make(chan struct{})
	)
	for w := range writers {
		writersWG.Go(func() {
			for _, ch := range work[w] {
				if err := r.Put(ctx, ch); err != nil && !errors.Is(err, storage.ErrOverwriteNewerChunk) {
					t.Errorf("put: %v", err)
					return
				}
				puts.Add(1)
			}
		})
	}
	bgWG.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			for _, id := range batchIDs {
				if _, err := r.EvictBatchBin(ctx, id, evictPerCall, swarm.MaxBins); err != nil {
					t.Errorf("evict: %v", err)
					return
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
	})

	m := &modelObserver{}
	var registeredAt int64
	bgWG.Go(func() {
		for puts.Load() < writers*putsPerWriter/4 {
			time.Sleep(100 * time.Microsecond)
		}
		if err := r.SetObserver(m); err != nil {
			t.Error(err)
			return
		}
		registeredAt = puts.Load()
		if err := m.rebuild(disk, 8); err != nil {
			t.Error(err)
		}
	})

	done := make(chan struct{})
	go func() {
		writersWG.Wait()
		close(stop)
		bgWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Minute):
		t.Fatal("timed out: writers, eviction and registration deadlocked")
	}
	if t.Failed() {
		return
	}

	t.Logf("registered after %d puts; dropped %d, queued %d, applied %d commits", registeredAt, m.dropped.Load(), m.queued.Load(), m.applied.Load())
	if registeredAt >= writers*putsPerWriter {
		t.Fatal("registration happened after all puts")
	}
	if m.applied.Load() == 0 {
		t.Fatal("no commit reached the built index")
	}

	truth := checkAddrIndexMatchesReserve(t, disk)
	if err := sameSet(m.set(), truth); err != nil {
		t.Fatalf("observer set differs from the address index: %v", err)
	}
	if err := sameSet(truth, chunkBinSet(t, disk)); err != nil {
		t.Fatalf("address index differs from the reserve: %v", err)
	}
}

// TestAddrIndexProperty applies random Put (new, same-address and
// different-address replacement, stale stamp) and eviction sequences and
// checks that the ChunkAddrItems always equal the set derived from the
// ChunkBinItems, and that the deltas delivered to an observer registered on
// the empty reserve add up to the same set.
func TestAddrIndexProperty(t *testing.T) {
	t.Parallel()

	for seed := range int64(4) {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			baseAddr := swarm.RandAddress(t)
			st := newReserveDiskStorage(t)
			r := newAddrIndexReserve(t, baseAddr, st, true)
			m := &modelObserver{}
			if err := r.SetObserver(m); err != nil {
				t.Fatal(err)
			}
			if err := m.rebuild(st, 0); err != nil {
				t.Fatal(err)
			}

			rnd := rand.New(rand.NewSource(seed))
			batches := [][]byte{postagetesting.MustNewBatch().ID, postagetesting.MustNewBatch().ID}
			var stored []swarm.Chunk
			pinned := make(map[string]bool)
			ts := uint64(10)
			for step := range 200 {
				ts++
				batch := batches[rnd.Intn(len(batches))]
				index := uint64(rnd.Intn(12))
				var err error
				switch op := rnd.Intn(20); {
				case op < 9 || len(stored) == 0: // new chunk, maybe replacing a different address
					ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, rnd.Intn(3)).WithStamp(postagetesting.MustNewFields(batch, index, ts))
					err = r.Put(ctx, ch)
					stored = append(stored, ch)
				case op < 12: // same address, newer stamp
					prev := stored[rnd.Intn(len(stored))]
					ch := swarm.NewChunk(prev.Address(), prev.Data()).WithStamp(postagetesting.MustNewFields(prev.Stamp().BatchID(), indexOf(prev), ts))
					err = r.Put(ctx, ch)
					stored = append(stored, ch)
				case op < 13: // stale stamp
					ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, rnd.Intn(3)).WithStamp(postagetesting.MustNewFields(batch, index, 0))
					err = r.Put(ctx, ch)
				case op < 17: // eviction, pinned chunks keep their data
					_, err = r.EvictBatchBin(ctx, batch, 1+rnd.Intn(3), uint8(1+rnd.Intn(3)))
				case op < 18: // the same address under the other batch
					prev := stored[rnd.Intn(len(stored))]
					other := batches[0]
					if string(other) == string(prev.Stamp().BatchID()) {
						other = batches[1]
					}
					ch := swarm.NewChunk(prev.Address(), prev.Data()).WithStamp(postagetesting.MustNewFields(other, index, ts))
					err = r.Put(ctx, ch)
					stored = append(stored, ch)
				case op < 19: // pin a chunk, so its eviction takes the metadata-only path
					ch := stored[rnd.Intn(len(stored))]
					if !pinned[ch.Address().ByteString()] {
						pinChunks(t, st, ch)
						pinned[ch.Address().ByteString()] = true
					}
				default:
					err = r.Reset(ctx)
				}
				if err != nil && !errors.Is(err, storage.ErrOverwriteNewerChunk) {
					t.Fatalf("step %d: %v", step, err)
				}
				truth := checkAddrIndexMatchesReserve(t, st)
				if err := sameSet(m.set(), truth); err != nil {
					t.Fatalf("step %d: observer set differs from the address index: %v", step, err)
				}
			}
			if m.dropped.Load() != 0 {
				t.Fatalf("%d commits dropped after the first rebuild", m.dropped.Load())
			}
		})
	}
}

// gatedStorage blocks the next transaction after its writes and before its
// commit, once armed, until gate is closed; entered is closed when it blocks.
type gatedStorage struct {
	transaction.Storage
	armed   atomic.Bool
	entered chan struct{}
	gate    chan struct{}
}

func (g *gatedStorage) Run(ctx context.Context, f func(transaction.Store) error) error {
	if !g.armed.CompareAndSwap(true, false) {
		return g.Storage.Run(ctx, f)
	}
	return g.Storage.Run(ctx, func(s transaction.Store) error {
		err := f(s)
		close(g.entered)
		<-g.gate
		return err
	})
}

// TestAddrIndexSetObserverWaits checks deterministically that registration
// returns only after a transaction that started without the observer has
// committed, and that such a transaction is not reported.
func TestAddrIndexSetObserverWaits(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	st := &gatedStorage{Storage: newReserveDiskStorage(t), entered: make(chan struct{}), gate: make(chan struct{})}
	r := newAddrIndexReserve(t, baseAddr, st, true)

	ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)
	st.armed.Store(true)
	putErr := make(chan error, 1)
	go func() { putErr <- r.Put(ctx, ch) }()
	<-st.entered // the Put is inside its transaction, not yet committed

	o := &recordingObserver{r: r}
	registered := make(chan error, 1)
	go func() { registered <- r.SetObserver(o) }()
	select {
	case err := <-registered:
		t.Fatalf("SetObserver returned (%v) while an unwrapped transaction was in flight", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(st.gate)
	if err := <-registered; err != nil {
		t.Fatal(err)
	}
	if err := <-putErr; err != nil {
		t.Fatal(err)
	}
	if events := o.take(t); len(events) != 0 {
		t.Fatalf("the unwrapped transaction was reported: %d events", len(events))
	}
	if n := countAddrIndex(t, st); n != 1 {
		t.Fatalf("%d chunkAddr keys, want 1", n)
	}
}

// TestAddrIndexSharedAddress covers one chunk address held under two stamps
// (the chunk store refcount is 2): evicting or resetting one stamp removes only
// its own ChunkAddrItem and emits exactly that id.
func TestAddrIndexSharedAddress(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	for _, pinned := range []bool{false, true} {
		t.Run(fmt.Sprintf("pinned=%v", pinned), func(t *testing.T) {
			t.Parallel()

			baseAddr := swarm.RandAddress(t)
			st := newReserveDiskStorage(t)
			r := newAddrIndexReserve(t, baseAddr, st, true)

			batchA, batchB := postagetesting.MustNewBatch().ID, postagetesting.MustNewBatch().ID
			chA := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewFields(batchA, 0, 1))
			chB := swarm.NewChunk(chA.Address(), chA.Data()).WithStamp(postagetesting.MustNewFields(batchB, 0, 1))
			for _, ch := range []swarm.Chunk{chA, chB} {
				if err := r.Put(ctx, ch); err != nil {
					t.Fatal(err)
				}
			}
			if pinned {
				pinChunks(t, st, chA)
			}
			if set := checkAddrIndexMatchesReserve(t, st); len(set) != 2 {
				t.Fatalf("%d elements, want 2", len(set))
			}
			o := registerConcurrent(t, r)

			if _, err := r.EvictBatchBin(ctx, batchA, 10, swarm.MaxBins); err != nil {
				t.Fatal(err)
			}
			events := o.take(t)
			if len(events) != 1 {
				t.Fatalf("evict: %d events, want one commit", len(events))
			}
			checkCommit(t, events[0], nil, []reserve.ChunkID{chunkIDOf(t, chA)})
			set := checkAddrIndexMatchesReserve(t, st)
			if len(set) != 1 || set[elementID(chB.Address(), chunkIDOf(t, chB).StampHash)] != 1 {
				t.Fatal("evicting one stamp touched the other stamp's ChunkAddrItem")
			}
			got, err := r.Get(ctx, chB.Address(), batchB, chunkIDOf(t, chB).StampHash)
			if err != nil || !got.Equal(chB) {
				t.Fatalf("chunk of the remaining stamp: %v", err)
			}

			if err := r.Reset(ctx); err != nil {
				t.Fatal(err)
			}
			events = o.take(t)
			if len(events) != 2 || events[1].kind != "reset" {
				t.Fatalf("reset: %d events, want one commit and a reset", len(events))
			}
			checkCommit(t, events[0], nil, []reserve.ChunkID{chunkIDOf(t, chB)})
			if n := countAddrIndex(t, st); n != 0 {
				t.Fatalf("%d chunkAddr keys after reset", n)
			}
		})
	}
}

// skippingObserver breaks the Commit contract: it returns without running
// the transaction, or runs it twice.
type skippingObserver struct {
	runs int
}

func (o *skippingObserver) Commit(run func() (adds, removes []reserve.ChunkID, err error)) error {
	for range o.runs {
		if _, _, err := run(); err != nil {
			return err
		}
	}
	return nil
}
func (o *skippingObserver) RadiusChanged(uint8) {}
func (o *skippingObserver) Reset()              {}

// TestAddrIndexObserverContract checks that the reserve reports an observer
// that skips the transaction or runs it twice instead of silently dropping
// or repeating the write.
func TestAddrIndexObserverContract(t *testing.T) {
	t.Parallel()

	for _, runs := range []int{0, 2} {
		t.Run(fmt.Sprintf("runs=%d", runs), func(t *testing.T) {
			t.Parallel()

			baseAddr := swarm.RandAddress(t)
			st := newReserveDiskStorage(t)
			r := newAddrIndexReserve(t, baseAddr, st, true)
			if err := r.SetObserver(&skippingObserver{runs: runs}); err != nil {
				t.Fatal(err)
			}
			err := r.Put(context.Background(), chunk.GenerateTestRandomChunkAt(t, baseAddr, 0))
			if !errors.Is(err, reserve.ErrObserverContract) {
				t.Fatalf("put: %v, want ErrObserverContract", err)
			}
			want := 0
			if runs > 0 {
				want = 1 // the first run committed
			}
			if n := countAddrIndex(t, st); n != want {
				t.Fatalf("%d chunkAddr keys, want %d", n, want)
			}
		})
	}
}
