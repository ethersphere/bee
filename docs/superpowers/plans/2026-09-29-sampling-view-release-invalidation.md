# Sampling View Release Invalidation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the sharky Hold quarantine behind the reserve sampling view with release invalidation: sharky tells the view which slots were released, and the view stops trusting those locations.

**Architecture:** `sharky.Store` gets a `Watch` observer API called from `Release` before a slot is freed. The sampling view registers an observer before building its location table, records released slots (buffered during the scan, then in per-shard atomic bitmaps), and checks the bitmap before and after each direct sharky read; a released slot sends the read to the live chunk store. The Hold API is removed.

**Tech Stack:** Go 1.26, `sync/atomic` (`atomic.Pointer`, `atomic.Uint64.Or`), sharky, leveldbstore, bee `transaction` storage.

**Spec:** `docs/superpowers/specs/2026-09-28-sampling-view-design.md`

## Global Constraints

- Follow `AGENTS.md`, `CODING.md`, `CODINGSTYLE.md`: `package foo_test` tests, `export_test.go` for test-only access, `t.Parallel()` where safe, errors wrapped with `fmt.Errorf("…: %w", err)`, American English.
- Every new `.go` file starts with the `Copyright 2026 The Swarm Authors` BSD header.
- No `go.mod` changes. The working tree has an unrelated uncommitted `go.mod`/`go.sum` downgrade; never stage it. Stage files by path, never `git add -A` / `commit -a`.
- Commit messages: subject line only, conventional prefix (`perf(storer): …`, `feat(sharky): …`, `docs: …`), no body, no trailers.
- No spinlocks or sleeps in tests; drive interleavings with hooks set through `export_test.go`.
- Commit on branch `perf/reserve-sampling-view`; do not push.

## Review Focus

1. A release that races with `publish` (observer loaded `nil` bitmaps, publish ran before it took the lock) must still be recorded — otherwise a reused slot is trusted. Pinned by `TestReleasedSlotsConcurrentPublish` (Task 3).
2. A slot released and overwritten between the direct read and the post-read check must fall back to the live store, not return the other chunk's bytes. Pinned by `TestSamplingViewFallsBackForSlotReleasedDuringRead` (Task 3).
3. A SOC replaced after the view opens must yield a `SampleItem` whose `ChunkData` and `TransformedAddress` belong to the same version. Pinned by `TestReserveSamplerReplacedSOC` (Task 5).
4. `NewSamplingView` failing (canceled context) must not leave its sharky observer registered. Pinned structurally by the `opened` flag + `defer` in Task 3 and by `TestWatchStop` (Task 1); `TestSamplingViewCanceledContext` (Task 3) checks the error path returns cleanly.
5. Removing Hold must leave sharky exactly as on `master` apart from `Watch`. Pinned by the `git diff master -- pkg/sharky` check in Task 4.

---

## File Structure

| File | Change | Responsibility |
| :--- | :--- | :--- |
| `pkg/sharky/store.go` | modify | `Watch` API, watcher notification in `Release`; later remove Hold |
| `pkg/sharky/watch_test.go` | create | `Watch` behavior |
| `pkg/sharky/hold_test.go` | delete (Task 4) | Hold tests |
| `pkg/sharky/metrics.go` | modify (Task 4) | remove `HeldSlots` |
| `pkg/storer/internal/chunkstore/locationtable.go` | modify | per-shard slot limits |
| `pkg/storer/internal/chunkstore/locationtable_test.go` | modify | slot limits test |
| `pkg/storer/internal/transaction/releasedslots.go` | create | released-slot set |
| `pkg/storer/internal/transaction/releasedslots_test.go` | create | released-slot set tests |
| `pkg/storer/internal/transaction/export_test.go` | create | test access to released slots and the view read seam |
| `pkg/storer/internal/transaction/samplingview.go` | modify | Watch + released set instead of Hold |
| `pkg/storer/internal/transaction/samplingview_test.go` | modify | tests for the new semantics |
| `pkg/storer/storer.go`, `pkg/storer/sample.go`, `pkg/storer/export_test.go`, `pkg/storer/sample_test.go` | modify (Task 5) | test hook after the view opens, replaced-SOC test, drop optimization asserts from `TestReserveSampler` |
| `docs/superpowers/specs/2026-09-28-sampling-view-design.md` | modify (Task 5) | status and testing section |

---

### Task 1: sharky `Watch`

**Files:**
- Modify: `pkg/sharky/store.go`
- Create: `pkg/sharky/watch_test.go`

**Interfaces:**
- Produces: `func (s *Store) Watch(fn func(Location)) (stop func())`

- [ ] **Step 1: Write the failing tests** — `pkg/sharky/watch_test.go`

```go
// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky_test

import (
	"context"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
)

func openWatchStore(t *testing.T) *sharky.Store {
	t.Helper()
	s, err := sharky.New(&dirFS{basedir: t.TempDir()}, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	})
	return s
}

func watchWrite(t *testing.T, s *sharky.Store) sharky.Location {
	t.Helper()
	loc, err := s.Write(context.Background(), []byte{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestWatchSeesReleaseBeforeItReturns(t *testing.T) {
	t.Parallel()

	s := openWatchStore(t)
	loc := watchWrite(t, s)

	var got []sharky.Location
	stop := s.Watch(func(l sharky.Location) { got = append(got, l) })
	defer stop()

	if err := s.Release(context.Background(), loc); err != nil {
		t.Fatal(err)
	}
	if want := []sharky.Location{loc}; !slices.Equal(got, want) {
		t.Fatalf("watched releases: got %v, want %v", got, want)
	}
}

func TestWatchStop(t *testing.T) {
	t.Parallel()

	s := openWatchStore(t)
	locs := []sharky.Location{watchWrite(t, s), watchWrite(t, s)}

	var stopped, kept []sharky.Location
	stop := s.Watch(func(l sharky.Location) { stopped = append(stopped, l) })
	defer s.Watch(func(l sharky.Location) { kept = append(kept, l) })()

	stop()
	stop() // idempotent: must not remove the other watcher

	for _, loc := range locs {
		if err := s.Release(context.Background(), loc); err != nil {
			t.Fatal(err)
		}
	}
	if len(stopped) != 0 {
		t.Fatalf("stopped watcher saw %v", stopped)
	}
	if !slices.Equal(kept, locs) {
		t.Fatalf("remaining watcher: got %v, want %v", kept, locs)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/sharky/ -run TestWatch`
Expected: build failure, `s.Watch undefined`.

- [ ] **Step 3: Implement** — `pkg/sharky/store.go`

Add `"slices"` and `"sync/atomic"` to the imports. Add two fields to `Store` below the existing hold fields:

```go
	watchMu  sync.Mutex                 // serializes changes to watchers
	watchers atomic.Pointer[[]*watcher] // replaced on change, read by Release
```

Add after `Write` (before `Hold`):

```go
type watcher struct {
	fn func(Location)
}

// Watch registers fn to be called with every location passed to Release,
// before its slot can be handed out to a Write. fn runs on the goroutine that
// calls Release, so it must be fast, must not block and must not call back
// into the store. A Release already running when stop returns may still call
// fn once. stop is idempotent.
func (s *Store) Watch(fn func(Location)) (stop func()) {
	w := &watcher{fn: fn}
	s.updateWatchers(func(ws []*watcher) []*watcher { return append(ws, w) })

	var once sync.Once
	return func() {
		once.Do(func() {
			s.updateWatchers(func(ws []*watcher) []*watcher {
				return slices.DeleteFunc(ws, func(x *watcher) bool { return x == w })
			})
		})
	}
}

// updateWatchers replaces the watcher list with update applied to a copy of it.
func (s *Store) updateWatchers(update func([]*watcher) []*watcher) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	var ws []*watcher
	if cur := s.watchers.Load(); cur != nil {
		ws = slices.Clone(*cur)
	}
	ws = update(ws)
	s.watchers.Store(&ws)
}
```

In `Release`, right after the `ErrShardNotFound` check and before the hold block:

```go
	if ws := s.watchers.Load(); ws != nil {
		for _, w := range *ws {
			w.fn(loc)
		}
	}
```

Extend the `Release` doc comment with the line: `// Watchers are notified before the slot is freed.`

- [ ] **Step 4: Run to verify it passes**

Run: `go test -race ./pkg/sharky/`
Expected: PASS (existing Hold tests still pass; Hold is removed in Task 4).

- [ ] **Step 5: Commit**

```bash
git add pkg/sharky/store.go pkg/sharky/watch_test.go
git commit -m "feat(sharky): add Watch to observe released slots"
```

---

### Task 2: location table slot limits

**Files:**
- Modify: `pkg/storer/internal/chunkstore/locationtable.go`
- Modify: `pkg/storer/internal/chunkstore/locationtable_test.go`

**Interfaces:**
- Produces: `func (t *LocationTable) SlotLimits() []uint32` — indexed by shard; every location in the table has `Slot < SlotLimits()[Shard]`; the slice is only as long as the highest shard in the table plus one.

- [ ] **Step 1: Write the failing test** — append to `locationtable_test.go`

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/storer/internal/chunkstore/ -run TestLocationTableSlotLimits`
Expected: build failure, `table.SlotLimits undefined`.

- [ ] **Step 3: Implement** — `locationtable.go`

Add a field to `LocationTable`:

```go
type LocationTable struct {
	keys   []locationKey // ascending, as the index is ordered by address
	locs   []sharky.Location
	limits []uint32 // by shard, above every slot in locs
}
```

In `BuildLocationTable`, after `t.locs = append(t.locs, item.Location)`:

```go
		t.coverSlot(item.Location)
```

Add after `Len`:

```go
// SlotLimits returns, indexed by shard, a bound above every slot in the
// table. Entries dropped as duplicates may leave a bound higher than needed.
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
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./pkg/storer/internal/chunkstore/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/storer/internal/chunkstore/locationtable.go pkg/storer/internal/chunkstore/locationtable_test.go
git commit -m "feat(storer): track per-shard slot limits in location table"
```

---

### Task 3: released-slot set and the view switch

**Files:**
- Create: `pkg/storer/internal/transaction/releasedslots.go`
- Create: `pkg/storer/internal/transaction/releasedslots_test.go`
- Create: `pkg/storer/internal/transaction/export_test.go`
- Modify: `pkg/storer/internal/transaction/samplingview.go`
- Modify: `pkg/storer/internal/transaction/samplingview_test.go`

**Interfaces:**
- Consumes: `(*sharky.Store).Watch` (Task 1), `(*chunkstore.LocationTable).SlotLimits` (Task 2).
- Produces (package-internal): `releasedSlots` with `add(sharky.Location)`, `publish([]uint32)`, `contains(sharky.Location) bool`; test-only `transaction.ReleasedSlots`, `(*ReleasedSlots).Add/Publish/Contains`, `transaction.SetSamplingViewAfterRead(SamplingView, func())`. `SamplingView`/`SamplingViewer` interfaces are unchanged.

- [ ] **Step 1: Write the released-slot tests** — `releasedslots_test.go`

```go
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
```

- [ ] **Step 2: Rewrite the view tests for the new semantics** — `samplingview_test.go`

Add `"errors"` and `"github.com/ethersphere/bee/v2/pkg/storage"` to the imports. Add a helper after `putViewChunks`:

```go
func deleteViewChunk(t *testing.T, st transaction.Storage, addr swarm.Address) {
	t.Helper()
	err := st.Run(context.Background(), func(s transaction.Store) error {
		return s.ChunkStore().Delete(context.Background(), addr)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertViewNotFound(t *testing.T, view transaction.SamplingView, addr swarm.Address) {
	t.Helper()
	_, err := view.GetInto(context.Background(), addr, make([]byte, swarm.SocMaxChunkSize))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("chunk %s: got error %v, want %v", addr, err, storage.ErrNotFound)
	}
}
```

Update the `newViewStorage` doc comment to: `// newViewStorage uses a single sharky shard so that a write after a release takes the released slot.`

Replace `TestSamplingViewKeepsDeletedChunkContent` with:

```go
func TestSamplingViewFallsBackForReleasedSlots(t *testing.T) {
	t.Parallel()

	st, viewer := newViewStorage(t)
	ch := test.GenerateTestRandomChunk()
	putViewChunks(t, st, ch)

	view := openView(t, viewer)

	deleteViewChunk(t, st, ch.Address())
	putViewChunks(t, st, test.GenerateTestRandomChunks(16)...) // one takes the released slot

	assertViewNotFound(t, view, ch.Address())
	if view.Misses() != 1 {
		t.Fatalf("misses %d, want 1", view.Misses())
	}
}

func TestSamplingViewFallsBackForSlotReleasedDuringRead(t *testing.T) {
	t.Parallel()

	st, viewer := newViewStorage(t)
	ch := test.GenerateTestRandomChunk()
	putViewChunks(t, st, ch)

	view := openView(t, viewer)
	transaction.SetSamplingViewAfterRead(view, func() {
		deleteViewChunk(t, st, ch.Address())
		putViewChunks(t, st, test.GenerateTestRandomChunks(16)...)
	})

	assertViewNotFound(t, view, ch.Address())
}

func TestSamplingViewCanceledContext(t *testing.T) {
	t.Parallel()

	_, viewer := newViewStorage(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := viewer.NewSamplingView(ctx, swarm.ZeroAddress.Bytes(), 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, want %v", err, context.Canceled)
	}
}
```

Replace `TestSamplingViewReadsSnapshotVersionOfReplacedChunk` with:

```go
func TestSamplingViewReadsCurrentVersionOfReplacedChunk(t *testing.T) {
	t.Parallel()

	st, viewer := newViewStorage(t)
	addr := swarm.RandAddress(t)
	v1 := swarm.NewChunk(addr, []byte("version in the table"))
	v2 := swarm.NewChunk(addr, []byte("version written during the round"))
	putViewChunks(t, st, v1)

	view := openView(t, viewer)

	err := st.Run(context.Background(), func(s transaction.Store) error {
		return s.ChunkStore().Replace(context.Background(), v2, false)
	})
	if err != nil {
		t.Fatal(err)
	}
	putViewChunks(t, st, test.GenerateTestRandomChunks(16)...)

	assertViewReads(t, view, addr, v2.Data())
	if view.Misses() != 1 {
		t.Fatalf("misses %d, want 1", view.Misses())
	}
}
```

In `TestSamplingViewConcurrentReadsAndWrites`, replace the reader body so deleted chunks may be reported as not found but never with other bytes:

```go
		g.Go(func() error {
			buf := make([]byte, swarm.SocMaxChunkSize)
			for i, ch := range chs {
				n, err := view.GetInto(context.Background(), ch.Address(), buf)
				switch {
				case errors.Is(err, storage.ErrNotFound) && i < deleted:
					// deleted by the writer; the live store agrees
				case err != nil:
					return err
				case !bytes.Equal(buf[:n], ch.Data()):
					t.Errorf("chunk %s: read another chunk's content", ch.Address())
				}
			}
			return nil
		})
```

and declare `const deleted = 32` at the top of the test, using `chs[:deleted]` in the writer.

- [ ] **Step 3: Add test access** — `export_test.go`

```go
// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/sharky"
)

type ReleasedSlots = releasedSlots

func (r *releasedSlots) Add(loc sharky.Location)           { r.add(loc) }
func (r *releasedSlots) Publish(limits []uint32)           { r.publish(limits) }
func (r *releasedSlots) Contains(loc sharky.Location) bool { return r.contains(loc) }

// SetSamplingViewAfterRead makes view call fn after each direct sharky read,
// before it checks whether the slot was released.
func SetSamplingViewAfterRead(view SamplingView, fn func()) {
	v := view.(*samplingView)
	read := v.read
	v.read = func(ctx context.Context, loc sharky.Location, buf []byte) error {
		err := read(ctx, loc, buf)
		fn()
		return err
	}
}
```

- [ ] **Step 4: Run to verify the tests fail**

Run: `go test ./pkg/storer/internal/transaction/`
Expected: build failure, `undefined: releasedSlots` and `v.read undefined`.

- [ ] **Step 5: Implement the released-slot set** — `releasedslots.go`

```go
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
	mu      sync.Mutex // guards pending and the switch to bitmaps
	pending []sharky.Location
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
	b := *r.bitmaps.Load()
	if int(loc.Shard) >= len(b) || int(loc.Slot/64) >= len(b[loc.Shard]) {
		return true
	}
	return b[loc.Shard][loc.Slot/64].Load()&(1<<(loc.Slot%64)) != 0
}

// setSlot marks loc's slot. Slots outside the bitmaps cannot be in the
// location table, so they are not recorded.
func setSlot(b [][]atomic.Uint64, loc sharky.Location) {
	if int(loc.Shard) < len(b) && int(loc.Slot/64) < len(b[loc.Shard]) {
		b[loc.Shard][loc.Slot/64].Or(1 << (loc.Slot % 64))
	}
}
```

- [ ] **Step 6: Switch the view** — `samplingview.go`

Add `"github.com/ethersphere/bee/v2/pkg/sharky"` to the imports. Replace the `SamplingView` doc comment:

```go
// SamplingView reads chunks within depth of anchor using the locations the
// retrieval index held when the view was opened. A location whose sharky slot
// has been released since is not trusted; the chunk is then read through the
// retrieval index, as the chunk store would. GetInto is safe for concurrent
// use. Close must be called when sampling ends.
```

Replace everything from the `NewSamplingView` doc comment to the end of the file with:

```go
// NewSamplingView watches sharky releases and only then snapshots the
// locations. An index entry is committed away before its slot is released (see
// transaction.Commit), so a slot the snapshot references that is freed later
// is recorded before a write can reuse it.
func (s *store) NewSamplingView(ctx context.Context, anchor []byte, depth uint8) (SamplingView, error) {
	released := new(releasedSlots)
	stop := s.sharky.Watch(released.add)
	opened := false
	defer func() {
		if !opened {
			stop()
		}
	}()

	table, err := chunkstore.BuildLocationTable(ctx, s.IndexStore(), anchor, depth)
	if err != nil {
		return nil, err
	}
	released.publish(table.SlotLimits())

	opened = true
	return &samplingView{
		store:    s,
		table:    table,
		released: released,
		stop:     stop,
		read:     s.sharky.Read,
	}, nil
}

type samplingView struct {
	store    *store
	table    *chunkstore.LocationTable
	released *releasedSlots
	stop     func()
	read     func(context.Context, sharky.Location, []byte) error
	misses   atomic.Int64
}

func (v *samplingView) GetInto(ctx context.Context, addr swarm.Address, buf []byte) (int, error) {
	if loc, ok := v.table.Lookup(addr); ok && !v.released.contains(loc) {
		n, err := v.readAt(ctx, addr, loc, buf)
		// A slot released during the read may already hold another chunk.
		if !v.released.contains(loc) {
			return n, err
		}
	}
	v.misses.Add(1)
	return v.store.ChunkStore().GetInto(ctx, addr, buf)
}

func (v *samplingView) readAt(ctx context.Context, addr swarm.Address, loc sharky.Location, buf []byte) (n int, err error) {
	defer handleMetric("sampling_view_get", v.store.metrics)(&err)
	n = int(loc.Length)
	if len(buf) < n {
		return 0, fmt.Errorf("sampling view: buffer too small: %d < %d", len(buf), n)
	}
	if err = v.read(ctx, loc, buf[:n]); err != nil {
		return 0, fmt.Errorf("sampling view: read %s at %v: %w", addr, loc, err)
	}
	return n, nil
}

func (v *samplingView) Len() int { return v.table.Len() }

func (v *samplingView) Misses() int64 { return v.misses.Load() }

func (v *samplingView) Close() error {
	v.stop()
	return nil
}
```

- [ ] **Step 7: Run to verify it passes**

Run: `go test -race ./pkg/storer/internal/transaction/ ./pkg/storer/internal/chunkstore/`
Expected: PASS. If `assertViewNotFound` reports a non-`ErrNotFound` error, check how `chunkstore.GetInto` wraps the retrieval index miss and match it rather than loosening the assertion.

- [ ] **Step 8: Commit**

```bash
git add pkg/storer/internal/transaction/releasedslots.go pkg/storer/internal/transaction/releasedslots_test.go pkg/storer/internal/transaction/export_test.go pkg/storer/internal/transaction/samplingview.go pkg/storer/internal/transaction/samplingview_test.go
git commit -m "perf(storer): invalidate released slots in sampling view instead of holding them"
```

---

### Task 4: remove sharky Hold

**Files:**
- Modify: `pkg/sharky/store.go`
- Modify: `pkg/sharky/metrics.go`
- Delete: `pkg/sharky/hold_test.go`

**Interfaces:**
- Removes: `(*sharky.Store).Hold`. Nothing else in the repo calls it after Task 3 (verify with `grep -rn "\.Hold()" pkg/`).

- [ ] **Step 1: Delete the Hold code**

In `store.go`:
- Remove the `holdMu`, `holds`, `held`, `closed` fields.
- Restore `Close` to master: drop the `holdMu` prologue so it starts with `close(s.quit)`.
- Remove `Hold` and `freeHeldLocked`.
- Merge `release` back into `Release` and drop the hold branch, so `Release` is master's body plus the watcher loop:

```go
// Release gives back the slot to the shard
// From here on the slot can be reused and overwritten
// Watchers are notified before the slot is freed.
// Release is meant to be called when an entry in the upstream db is removed
// Note that releasing is not safe for obfuscating earlier content, since
// even after reuse, the slot may be used by a very short blob and leaves the
// rest of the old blob bytes untouched
func (s *Store) Release(ctx context.Context, loc Location) error {
	if int(loc.Shard) >= len(s.shards) {
		return ErrShardNotFound
	}
	if ws := s.watchers.Load(); ws != nil {
		for _, w := range *ws {
			w.fn(loc)
		}
	}
	sh := s.shards[loc.Shard]
	err := sh.release(ctx, loc.Slot)
	s.metrics.TotalReleaseCalls.Inc()
	if err == nil {
		shard := strconv.Itoa(int(sh.index))
		s.metrics.CurrentShardSize.WithLabelValues(shard).Dec()
		s.metrics.ShardFragmentation.WithLabelValues(shard).Sub(float64(s.maxDataSize - int(loc.Length)))
		s.metrics.LastReleasedShardSlot.WithLabelValues(shard).Set(float64(loc.Slot))
	} else {
		s.metrics.TotalReleaseCallsErr.Inc()
	}
	return err
}
```

In `metrics.go`, remove the `HeldSlots` field and its constructor entry. Delete `pkg/sharky/hold_test.go`.

- [ ] **Step 2: Check the diff against master**

Run: `git diff master -- pkg/sharky`
Expected: only the `watcher` type, `Watch`, `updateWatchers`, the two `Store` fields, the imports, the `Release` watcher loop + doc line, and the new `watch_test.go`. `metrics.go` shows no diff.

- [ ] **Step 3: Run tests**

Run: `go test -race ./pkg/sharky/ ./pkg/storer/internal/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/sharky/store.go pkg/sharky/metrics.go pkg/sharky/hold_test.go
git commit -m "refactor(sharky): remove Hold quarantine"
```

---

### Task 5: sampler test for a replaced SOC, spec status

**Files:**
- Modify: `pkg/storer/storer.go`, `pkg/storer/sample.go`, `pkg/storer/export_test.go`, `pkg/storer/sample_test.go`
- Modify: `docs/superpowers/specs/2026-09-28-sampling-view-design.md`

**Interfaces:**
- Produces (test-only): `func (db *DB) OnSamplingViewOpened(fn func())`.

- [ ] **Step 1: Write the failing test** — append to `sample_test.go`

Add imports `"bytes"` (if missing), `"github.com/ethersphere/bee/v2/pkg/cac"`, `"github.com/ethersphere/bee/v2/pkg/crypto"`, `"github.com/ethersphere/bee/v2/pkg/soc"`, `"github.com/ethersphere/bee/v2/pkg/util/testutil"` as needed (check existing imports first).

```go
// TestReserveSamplerReplacedSOC replaces a SOC after the sampling view opens
// and checks that the sample item carries one version for both its data and
// its transformed address.
func TestReserveSamplerReplacedSOC(t *testing.T) {
	t.Parallel()

	for name, open := range sampleTestStorers(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			st := open(t, swarm.RandAddress(t))

			key, err := crypto.GenerateSecp256k1Key()
			if err != nil {
				t.Fatal(err)
			}
			signer := crypto.NewDefaultSigner(key)
			id := testutil.RandBytes(t, swarm.HashSize)
			batch := testutil.RandBytes(t, swarm.HashSize)
			timeVar := uint64(time.Now().UnixNano())

			version := func(payload string, ts uint64) swarm.Chunk {
				t.Helper()
				ch, err := cac.New([]byte(payload))
				if err != nil {
					t.Fatal(err)
				}
				sch, err := soc.New(id, ch).Sign(signer)
				if err != nil {
					t.Fatal(err)
				}
				return sch.WithStamp(postagetesting.MustNewFields(batch, 0, ts)).WithBatch(3, 2, false)
			}
			v1 := version("version in the table", timeVar-2)
			v2 := version("version written during the round", timeVar-1)

			putter := st.ReservePutter()
			if err := putter.Put(context.Background(), v1); err != nil {
				t.Fatal(err)
			}
			st.OnSamplingViewOpened(func() {
				if err := putter.Put(context.Background(), v2); err != nil {
					t.Errorf("replace soc: %v", err)
				}
			})

			anchor := v1.Address().Bytes()
			sample, err := st.ReserveSample(context.Background(), anchor, 5, timeVar, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertSampleNoErrors(t, sample)
			if len(sample.Items) != 1 {
				t.Fatalf("got %d sample items, want 1", len(sample.Items))
			}

			item := sample.Items[0]
			if !bytes.Equal(item.ChunkData, v2.Data()) {
				t.Fatal("sample item must carry the version written during the round")
			}
			want, err := storer.TransformedAddress(bmt.NewPrefixHasher(anchor), swarm.NewChunk(item.ChunkAddress, item.ChunkData), swarm.ChunkTypeSingleOwner)
			if err != nil {
				t.Fatal(err)
			}
			if !item.TransformedAddress.Equal(want) {
				t.Fatalf("transformed address %s does not match chunk data (want %s)", item.TransformedAddress, want)
			}
		})
	}
}
```

Also remove from `TestReserveSampler` the two blocks asserting `LocationTableSize == 0` and `LocationTableMisses != 0` (spec §4: `TestReserveSampler` checks sample correctness only; `TestReserveSamplerSamplingViewEquivalence` keeps checking that the table is used).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/storer/ -run TestReserveSamplerReplacedSOC`
Expected: build failure, `st.OnSamplingViewOpened undefined`.

- [ ] **Step 3: Implement the hook**

`storer.go`, next to `samplingViewDisabled` in `DB`:

```go
	samplingViewOpened   func() // set by tests to act between opening the view and reading chunks
```

`sample.go`, right after the `if view != nil { defer … }` block in `ReserveSample`:

```go
	if db.samplingViewOpened != nil {
		db.samplingViewOpened()
	}
```

`export_test.go`, after `DisableSamplingView`:

```go
// OnSamplingViewOpened makes ReserveSample call fn after it opens its sampling
// view and before it starts reading chunks.
func (db *DB) OnSamplingViewOpened(fn func()) {
	db.samplingViewOpened = fn
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test -race ./pkg/storer/ -run 'TestReserveSampler'`
Expected: PASS. If `len(sample.Items) != 1`, the SOC update did not take the reserve's same-address replace path; check `reserve.Put` for the stamp-index and timestamp conditions rather than changing the assertion.

- [ ] **Step 5: Update the spec**

In `docs/superpowers/specs/2026-09-28-sampling-view-design.md`:
- Status line: `**Status:** Implemented 2026-09-29 (release invalidation); testnet numbers below are from the Hold revision`.
- §4 Sampler bullet: replace with `**Sampler:** a ReserveSample test replaces a SOC after the view opens (test hook between opening the view and reading chunks) and checks that the sample item's ChunkData is the new version and its TransformedAddress matches it. TestReserveSampler asserts sample correctness only, not that the table was used.`
- §4 View bullet: replace `the view does not leak its observer on build error or Close` with `a canceled context fails NewSamplingView cleanly; observer removal is covered by the sharky Watch tests`.

- [ ] **Step 6: Commit**

```bash
git add pkg/storer/storer.go pkg/storer/sample.go pkg/storer/export_test.go pkg/storer/sample_test.go docs/superpowers/specs/2026-09-28-sampling-view-design.md
git commit -m "test(storer): cover SOC replaced during reserve sampling"
```

---

### Task 6: pre-commit checklist

- [ ] **Step 1:** `make format` — then `git status --short`; if it changed files from Tasks 1–5, stage those paths and commit `style: format`.
- [ ] **Step 2:** `make build`
- [ ] **Step 3:** `go test -race ./pkg/sharky/... ./pkg/storer/...` (the full `make test` is also fine; `pkg/api` is slow under `-race` on go1.27 and unrelated).
- [ ] **Step 4:** `make lint` and `make vet` (golangci-lint needs the sandbox off locally). Fix findings in the owning file and commit with a subject naming the fix.
- [ ] **Step 5:** `git status --short` — only `go.mod` and `go.sum` remain modified and unstaged.
