# Design Spec: Sampling View (Snapshot Location Table + Sharky Quarantine)

**Status:** Implemented & Verified  
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

Sampling View replaces random database lookups with a transient, in-memory snapshot view backed by slot quarantine in Sharky.

```
                    ┌─────────────────────────┐
                    │  ReserveSample Phase 2  │
                    └───────────┬─────────────┘
                                │
                      view.GetInto(addr, buf)
                                │
               ┌────────────────┴────────────────┐
          Table Hit                         Table Miss
               │                                 │
     Direct Sharky Read                Fallback chunkStore.GetInto
 (No LevelDB, no mutex lock)           (Standard LevelDB lookup)
```

### 2.1 Sharky Slot Quarantine (`pkg/sharky/store.go`)
- **`Hold() (release func())`**: While at least one hold is active, `Release(loc)` appends freed slots to a `limbo` list instead of making them reusable in the free list.
- When the last hold ends, `limbo` is drained back into the free list.
- `Close()` drains limbo before flushing `free_NNN` bitmasks, ensuring clean shutdown without leaking slots.
- **Correctness Guarantee:** Any chunk existing when the view opens remains untouched in its Sharky slot for the duration of the sample, even if concurrent deletions or evictions occur.

### 2.2 In-Memory Location Table (`pkg/storer/internal/chunkstore/locationtable.go`)
- Built via **a single sequential range scan** over `RetrievalIndexItem` for addresses matching the target proximity depth.
- Stored as compact parallel slices:
  - `keys []addrKey`: 16-byte address prefixes, already sorted by LevelDB key iteration order (zero sort overhead).
  - `locs []sharky.Location`: 8-byte Sharky slot coordinates.
- **Binary search lookup:** O(log N) lookup in memory (~50 MB RAM for 1.1M entries, ~52 bytes/entry).
- Unusable duplicates fall back to LevelDB.

### 2.3 Sampling View API & Sampler Integration (`pkg/storer/`)
- `db.storage.NewSamplingView(ctx, anchor, depth)`: Acquires the Sharky hold, runs the range scan to build the table, and returns a `SamplingView` (`GetterInto` + `io.Closer`).
- Phase 2 workers read through `view.GetInto()`. Hits bypass LevelDB and per-chunk locking entirely.
- **Graceful degradation:** If table build fails, the sampler logs the warning and falls back to standard `chunkStore.GetInto`. A failure in the optimization never fails the sampling round.
- Metrics added to `SampleStats`: `LocationTableSize`, `LocationTableBuildDuration`, and `LocationTableMisses`.

---

## 3. Key Benefits

- **Zero on-disk changes:** No schema changes, no migrations, trivial rollback to master.
- **Safe by construction:** Quarantine prevents slot reuse during the round; no stale pointer risks.
- **100% Hash Equivalence:** Evaluates to the exact same sample hash as `master`.

---

## 4. Empirical Testnet Verification

Benchmarked on `bee-light-testnet` (`bee-2-0` with Sampling View vs `bee-2-1` on `master`, 2.1M chunks):

| Benchmark Scenario | Metric | Master (`bee-2-1`) | Sampling View (`bee-2-0`) | Improvement |
| :--- | :--- | :--- | :--- | :--- |
| **Standard Hashing** | Wall Time | 170.4 s | 134.6 s | **-21.0%** |
| | CPU (cgroup) | 121.6 s | 95.8 s | **-21.2%** |
| **SIMD Hashing (AVX-512)** | Wall Time | 45.46 s | 28.43 s | **-37.5%** |
| | CPU (cgroup) | 72.31 s | 42.22 s | **-41.6%** |
| **Table Metrics** | Build Duration | N/A | 469 ms (1.09M entries) | < 0.5s overhead |
| | Miss Rate | N/A | 0 misses (100% hit rate) | Perfect hit rate |
| **Hash Verification** | Sample Hash | Identical | Identical | **100% Match** |
