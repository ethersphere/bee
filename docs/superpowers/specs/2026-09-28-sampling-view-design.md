# Design Spec: Sampling View (Snapshot Location Table + Release Invalidation)

**Status:** Implemented on `perf/reserve-sampling-view`  
**Base:** `master` (replaces PR #5615)  

---

## 1. Problem & Context

During `ReserveSample` phase 2, the node loads every chunk in the neighborhood via `chunkStore.GetInto(ctx, addr, buf)`. On `master`, each call performs:

1. A random LevelDB point lookup for `RetrievalIndexItem{Address}` under a per-address lock to find the chunk's Sharky location.
2. A random read from Sharky storage using that location.

On testnet (2.1M chunks, 1.5-core limit), LevelDB lookups and iterator allocations (`table.(*Reader).find`, `newBlockIter`) account for ~11% of sampler CPU and allocate ~1.3 KB across 24 objects per chunk.

### Why not persisted location hints (PR #5615)?

PR #5615 stored `Location` inside `ChunkBinItem`. While faster, it introduced critical flaws:

- **Duplicate mutable state:** A second copy of a pointer without liveness guarantees (breaks on SOC replacements, compacting, rollbacks).
- **Silent data corruption:** Reading a reused Sharky slot returns valid bytes from a different chunk, corrupting sample calculations silently.
- **Breaking migration:** Required a 47-second blocking migration (4.19M items) and made downgrading impossible without breaking the reserve.

---

## 2. Architecture & Design

The sampling view replaces per-chunk index lookups with a transient, in-memory snapshot of locations taken when the round starts. The view is told about every Sharky slot released while it is open and stops trusting those locations. Reads it cannot trust fall back to the live chunk store, which is what `master` does for every read.

```text
                    ┌─────────────────────────┐
                    │  ReserveSample Phase 2  │
                    └───────────┬─────────────┘
                                │
                      view.GetInto(addr, buf)
                                │
               ┌────────────────┴──────────────────┐
     Table hit, slot not released        Table miss, or slot released
               │                           (before or after the read)
     Direct Sharky Read                            │
 (No LevelDB, no mutex lock)             Fallback chunkStore.GetInto
                                         (Standard LevelDB lookup)
```

### 2.1 Sharky Release Observer (`pkg/sharky/store.go`)

- **`Watch(fn func(Location)) (stop func())`** registers an observer. `stop` is idempotent.
- `Release(loc)` calls every registered observer **before** handing the slot back to its shard. Observers must be fast and must not block or call back into the store.
- The observer list is an `atomic.Pointer` to an immutable slice, copied on write in `Watch`/`stop` and reset to nil when the last observer stops, so `Release` with no active view costs one atomic load.
- Sharky behavior is otherwise unchanged: slots are reused immediately and shard files do not grow because of a sampling round.

### 2.2 In-Memory Location Table (`pkg/storer/internal/chunkstore/locationtable.go`)

- Built by **a single sequential range scan** over `RetrievalIndexItem` for addresses within the committed depth of the anchor. One index item is reused for the whole scan.
- Stored as parallel slices, already sorted by the LevelDB key order:
  - `keys [][32]byte`: full chunk addresses. A lookup only matches the exact address; an address added after the build can never match another chunk's entry.
  - `locs []sharky.Location`: Sharky slot coordinates.
- **Binary search lookup:** O(log N) in memory, about 44 bytes per entry.
- `SlotLimits()` returns, per shard, one more than the highest slot in the table; it sizes the released-slot bitmaps.

### 2.3 Released-Slot Set (`pkg/storer/internal/transaction/releasedslots.go`)

Each view owns a record of slots released since it started watching.

- **Build phase:** `NewSamplingView` calls `Watch` **before** the table scan. Releases that arrive during the scan go into a small mutex-protected pending list.
- **Read phase:** after the scan, the view allocates one bitmap per shard (`[]atomic.Uint64`), sized by `SlotLimits`. It then takes the pending lock, applies the pending entries, publishes the bitmaps through an `atomic.Pointer` and releases the lock. From then on the observer sets bits with atomic `Or`; no locks, no allocations. The observer re-checks for published bitmaps under the lock, so no release racing with the publish is lost.
- **Out-of-range slots:** a released slot beyond a shard's bitmap cannot be in the table and is ignored; a lookup outside the bitmaps is treated as released.
- **Size:** at most (total slots)/8 bytes, a few hundred KB for a reserve of a few million chunks.

### 2.4 Read Protocol

```text
loc, ok := table.Lookup(addr)
if !ok || released(loc) { return fallback(addr) }   // miss
n := sharky.Read(loc, buf)
if released(loc)       { return fallback(addr) }    // released during the read
return n
```

**Why the second check is sufficient.** Let the observer mark `loc` at time T1. The slot enters the free list after T1, so any write of new data into it starts at T2 > T1. If the post-read check at T3 sees the bit clear, then T1 > T3, so T2 > T3 and the read finished before any overwrite began: the bytes are the ones indexed when the table was built. If the check sees the bit set, the bytes may be mixed and are discarded. This is a seqlock with the released bit as the sequence number. It relies on the atomic bit operations and the channel hand-off in the shard's release for ordering, and on the kernel ordering `pread`/`pwrite` on the same file.

**Why the scan-time ordering is safe.** A retrieval index entry is committed away before its slot is released (`transaction.Commit`), and the scan iterator's implicit LevelDB snapshot is taken after `Watch` returns. So the scan cannot see an entry whose slot was released before `Watch`, and every slot released after the scan starts is recorded. A slot released and reused by a new chunk that the scan then indexes produces a spurious fallback for that chunk: a cost, not a correctness problem.

**Replaced SOCs.** `chunkstore.Replace` always releases the old slot, so a SOC replaced during the round falls back to the live store and phase 2 reads the same version phase 3 loads. The only remaining window is a replacement between the phase-2 read and the phase-3 load, the same as on `master`.

### 2.5 Sampling View API & Sampler Integration (`pkg/storer/`)

- `transaction.NewSamplingView(ctx, sh, st, anchor, depth)` takes its dependencies explicitly: `sh` is the small `transaction.Sharky` interface (`Read`, `Watch`) satisfied by `*sharky.Store`, and `st` is a `transaction.ReadOnlyStore` for the index scan and the fallback. It registers the release observer, builds the table, publishes the bitmaps and returns a concrete `*SamplingView`.
- `DB` keeps the `*sharky.Store` it creates next to its `transaction.Storage` and passes both to `NewSamplingView`; no interface or type assertion is involved.
- `Close` stops the observer. `NewSamplingView` stops it on every non-success exit, including a panic during the table build.
- Phase 2 workers read through `view.GetInto()`. Hits bypass LevelDB and per-chunk locking; misses go through one read-only chunk store shared by the view.
- **Graceful degradation:** if the table build fails, the sampler reads through the chunk store instead. A failure in the optimization never fails the sampling round; a canceled context is not logged as a warning.
- Overlapping views are independent: each has its own observer, table and bitmaps.
- Metrics in `SampleStats`: `LocationTableSize`, `LocationTableBuildDuration` and `LocationTableMisses`. Misses include released-slot fallbacks; under heavy churn the miss rate rises, but results stay correct.

---

## 3. Key Benefits

- **Zero on-disk changes:** no schema changes, no migrations, trivial rollback to master.
- **No effect on Sharky space:** slots are reused as on `master`.
- **Safe by construction:** a read is returned only if its slot was not released before the read finished and the lookup matched the full address; otherwise the live store is used.
- **Hash equivalence with `master`:** every returned chunk is either the indexed content that was still live when read, or comes from the same live-store path `master` uses.

---

## 4. Testing

- **Sharky:** an observer is called with the released location before `Release` returns; `stop` removes it and is idempotent.
- **Location table:** addresses sharing a 128-bit prefix each find their own location, and an address missing from the index never matches one of them.
- **Released slots:** releases before and after publish are recorded; releases racing with publish are not lost.
- **View:** interleavings are driven by wrappers around the view's dependencies, not by hooks in production code:
  - a slot released during the table scan is honored (`afterScanStore` around the `ReadOnlyStore`); the test fails if `Watch` is registered after the scan;
  - a slot released and overwritten during the read falls back to the live store (`afterReadSharky` around `Sharky`); the test fails without the post-read check;
  - deleted and replaced chunks fall back to the live store; a canceled context fails `NewSamplingView` cleanly.
- **Sampler:** a `ReserveSample` test replaces a SOC after the view opens and checks that the sample item's `ChunkData` is the new version and its `TransformedAddress` matches it; `TestReserveSamplerSamplingViewEquivalence` checks the view and the index path produce the same sample.

---

## 5. Performance

Local measurements (Apple M4 Pro, interleaved runs compared with benchstat):

- `BenchmarkSamplingViewGetInto` (table hit): about 1.1 µs per read, dominated by the Sharky read, with 0 allocations.
- Table build over 100k index entries: 13.4 ms, one allocation fewer per entry than allocating an index item per entry (-20% allocations); full 32-byte keys add about 6% memory.
- `BenchmarkReserveSample10k`: no change in time; 2.7% fewer allocations than `master`.
- Opening and closing a view adds about 0.7 µs and 44 allocations per round for the observer and bitmaps.

Testnet numbers (`bee-light-testnet`, 2.1M chunks) have not yet been measured for this design.
