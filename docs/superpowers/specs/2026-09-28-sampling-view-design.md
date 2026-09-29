# Design Spec: Sampling View (Snapshot Location Table + Release Invalidation)

**Status:** Implemented 2026-09-29 (release invalidation); testnet numbers below are from the Hold revision  
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

### Why not Sharky Hold quarantine (first revision of this spec)?
The first revision kept every slot released during the round out of reuse until the last hold ended. Code review of `bca179d8` found that this costs more than it saves:
- **Store-wide effect:** every `Release` is deferred while a hold is open, including cache eviction, upload cleanup and unpins, not only reserve chunks in the table. Writes during the round must extend shard files instead of reusing freed slots. Shard files never shrink at runtime (only offline `bee db compact` truncates them), so the round leaves a higher high-water mark and can hit ENOSPC on a nearly full disk.
- **Nested holds:** overlapping `ReserveSample` calls (agent round plus rchash with a different anchor) keep the hold count above zero, so held slots may never return.
- **Drain under lock:** freeing held slots runs under `holdMu`, stalling every concurrent `Release` (and the commits holding multex locks behind it) for up to about a second.
- **Close and panic hazards:** `Close` frees held slots while a hold is open; a panic during the table build leaves the hold open for the life of the process.
- **Stale SOC reads:** a SOC replaced during the round stays readable at its old slot, so phase 2 hashes v1 while phase 3 loads v2 from the live store, producing a `SampleItem` whose `TransformedAddress` does not match its `ChunkData`.

---

## 2. Architecture & Design

Sampling View replaces random database lookups with a transient, in-memory snapshot of locations. Instead of preventing slot reuse, the view is told about every released slot and stops trusting it. Reads that cannot be trusted fall back to the live chunk store, which is what `master` does for every read.

```
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
- `Release(loc)` calls every registered observer **before** handing the slot back to its shard (`sh.release`). Observers must be fast and must not block or call back into the store.
- The observer list is an `atomic.Pointer` to an immutable slice (copy on write in `Watch`/`stop`), so `Release` with no active view costs one atomic load.
- Sharky behavior is otherwise unchanged: slots are reused immediately, no quarantine, no extra disk usage, no changes to `Close`. The `Hold` API and its `held` list, `holdMu` and `HeldSlots` metric are removed.

### 2.2 In-Memory Location Table (`pkg/storer/internal/chunkstore/locationtable.go`)
- Built via **a single sequential range scan** over `RetrievalIndexItem` for addresses matching the target proximity depth.
- Stored as compact parallel slices:
  - `keys []addrKey`: 16-byte address prefixes, already sorted by LevelDB key iteration order (zero sort overhead).
  - `locs []sharky.Location`: Sharky slot coordinates.
- **Binary search lookup:** O(log N) lookup in memory.
- Unusable duplicates fall back to LevelDB.
- Unchanged by this revision.

### 2.3 Released-Slot Set (`pkg/storer/internal/transaction/samplingview.go`)
Each view owns a record of slots released since it started watching.

- **Build phase:** `NewSamplingView` calls `sharky.Watch` **before** the table scan. Releases that arrive during the scan go into a small mutex-protected pending list.
- **Read phase:** after the scan, the view allocates one bitmap per shard (`[]atomic.Uint64`), sized to the highest slot in the table for that shard. It then takes the pending lock, applies the pending entries, publishes the bitmaps through an `atomic.Pointer` and releases the lock. From then on the observer sets bits with atomic `Or`; no locks, no allocations.
- **Out-of-range slots:** a released slot beyond a shard's bitmap cannot be in the table and is ignored. A table entry whose slot is beyond its bitmap cannot exist by construction.
- **Size:** one bit per slot up to each shard's highest slot in the table, i.e. at most (total slots)/8 bytes, a few hundred KB for a reserve of a few million chunks.

### 2.4 Read Protocol
```
loc, ok := table.Lookup(addr)
if !ok || released(loc) { return fallback(addr) }   // miss
n := sharky.Read(loc, buf)
if released(loc)       { return fallback(addr) }    // released during the read
return n
```

**Why the second check is sufficient.** Let the observer mark `loc` at time T1. The slot enters the free list after T1, so any write of new data into it starts at T2 > T1. If the post-read check at T3 sees the bit clear, then T1 > T3, so T2 > T3 and the read finished before any overwrite began: the bytes are the ones indexed when the table was built. If the check sees the bit set, the bytes may be mixed and are discarded. This is a seqlock with the released bit as the sequence number. It relies on the atomic bit operations and the channel hand-off in `sh.release` for ordering, and on the kernel ordering `pread`/`pwrite` on the same file.

**Why the scan-time ordering is safe.** It relies on two existing invariants: a retrieval index entry is committed away before its slot is released (`transaction.Commit`), and the scan iterator's implicit LevelDB snapshot is taken after `Watch` returns. So the scan cannot see an entry whose slot was released before `Watch`. The watcher is registered before the scan, so a slot released at any point after the scan starts is recorded. A location the scan reads was either still live when read, or was released after `Watch` and is recorded. A slot released and reused by a new chunk that the scan then indexes produces a spurious fallback for that chunk: a cost, not a correctness problem.

**Replaced SOCs.** `chunkstore.Replace` releases the old slot, so a SOC replaced during the round falls back to the live store and phase 2 reads the same version phase 3 loads. The only remaining window is a replacement between the phase-2 read and the phase-3 load, the same as on `master`.

### 2.5 Sampling View API & Sampler Integration (`pkg/storer/`)
- `db.storage.NewSamplingView(ctx, anchor, depth)`: registers the release observer, runs the range scan to build the table, publishes the released-slot bitmaps and returns a `SamplingView` (`GetterInto` + `io.Closer`).
- `Close` calls the observer's `stop`. `NewSamplingView` must stop the observer on every non-success exit, including a panic during the table build (`defer` guarded by a success flag).
- Phase 2 workers read through `view.GetInto()`. Hits bypass LevelDB and per-chunk locking entirely.
- **Graceful degradation:** If table build fails, the sampler logs the warning and falls back to standard `chunkStore.GetInto`. A failure in the optimization never fails the sampling round.
- Overlapping views are independent: each has its own observer, table and bitmaps. There is no shared counter to leak.
- Metrics in `SampleStats`: `LocationTableSize`, `LocationTableBuildDuration`, and `LocationTableMisses`. Misses now include released-slot fallbacks. Under heavy churn the miss rate rises, but results stay correct.

---

## 3. Key Benefits

- **Zero on-disk changes:** No schema changes, no migrations, trivial rollback to master.
- **No effect on Sharky space:** slots are reused as on `master`; shard files do not grow because of a sampling round.
- **Safe by construction:** a read is returned only if its slot was not released before the read finished; otherwise the live store is used.
- **Hash equivalence with `master`:** every returned chunk is either the indexed content that was still live when read, or comes from the same live-store path `master` uses.

---

## 4. Testing

- **Sharky:** an observer is called with the released location before any `Write` can reuse the slot; `stop` removes it; `Release` with no observers is unchanged.
- **View:** a slot released and overwritten between `Lookup` and the post-read check falls back to the live store (drive the interleaving with a sharky test hook or `synctest`, not a spinlock); a slot released during the table scan is honored after the bitmaps are published; a canceled context fails NewSamplingView cleanly; observer removal is covered by the sharky Watch tests.
- **Sampler:** a ReserveSample test replaces a SOC after the view opens (test hook between opening the view and reading chunks) and checks that the sample item's ChunkData is the new version and its TransformedAddress matches it. TestReserveSampler asserts sample correctness only, not that the table was used.

---

## 5. Empirical Testnet Verification

Benchmarked on `bee-light-testnet` (`bee-2-0` with Sampling View vs `bee-2-1` on `master`, 2.1M chunks). These numbers were measured with the Hold quarantine revision and must be re-measured after the switch to release invalidation. Locally (Apple M4 Pro, BenchmarkSamplingViewGetInto, interleaved A/B with benchstat), the hit path showed no regression from the switch (-3.6%) and got 12% faster from dropping the per-read metric, with 0 allocations per read before and after.

| Benchmark Scenario | Metric | Master (`bee-2-1`) | Sampling View (`bee-2-0`) | Improvement |
| :--- | :--- | :--- | :--- | :--- |
| **Standard Hashing** | Wall Time | 170.4 s | 134.6 s | **-21.0%** |
| | CPU (cgroup) | 121.6 s | 95.8 s | **-21.2%** |
| **SIMD Hashing (AVX-512)** | Wall Time | 45.46 s | 28.43 s | **-37.5%** |
| | CPU (cgroup) | 72.31 s | 42.22 s | **-41.6%** |
| **Table Metrics** | Build Duration | N/A | 469 ms (1.09M entries) | < 0.5s overhead |
| | Miss Rate | N/A | 0 misses (100% hit rate) | Perfect hit rate |
| **Hash Verification** | Sample Hash | Identical | Identical | **100% Match** |

---

## 6. Out of Scope

Other review findings on `bca179d8` that this revision does not address: 16-byte table keys (prefix collision on addresses added after the build), per-miss `ChunkStore()` allocations, table pre-sizing and `Location` padding, the `SamplingViewer` type assertion, dead `SampleStats.add` lines, and the warning logged on a canceled context.
