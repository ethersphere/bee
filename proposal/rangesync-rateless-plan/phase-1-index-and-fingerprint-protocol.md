# Phase 1 — Foundations and `rangesync` with fingerprints + item lists

**Goal:** a new `pkg/rangesync` package tree with its own protocol, fingerprint index, want
set and a minimal reconciler. A node started with `--puller-enable=false
--rangesync-enable=true` can sync its neighborhood using **only range fingerprints and item
lists** (Meyer). No coded symbols yet. With default flags the node behaves exactly as today.

**Proposal sections:** §4.1–4.3.3, §4.3.5 (fingerprints/counts), §4.3.6, §5.1–§5.4, §6
(FP + Items), §7.6, §8.2 (want set, rate limits), §8.3, §8.4, §9, §10, §11, §12 row 1, §13.

**Depends on:** nothing. Read the README "Ground rules" first — **`pkg/pullsync` and
`pkg/puller` are not modified.**

---

## T1.0 Spike: snapshot isolation and lock ordering (open question 6)

**Why first:** T1.4 fixes the storer's `ReserveObserver` interface and T1.6 builds the rebuild
protocol on it (§5.2). Both assume the IndexStore iterator reads a consistent snapshot through
`transaction.Storage` and its caching layer. If that's false, the interface changes.

**Scope:** a test-only investigation, no production code.
1. In `pkg/storer/internal/transaction` (or a `_test.go` next to the reserve), open an iterator
   over a namespace via the same path the reserve uses, then commit transactions that add and
   delete keys in that namespace while iterating. Assert the iterator sees exactly the pre-open
   state. Cover the caching layer if the reserve's `IndexStore` goes through one (check
   `pkg/storer/storer.go` for how the index store is constructed).
2. Map the lock order: `Reserve.Put` and eviction take `r.multx` per batch ID and per bin, then
   run the transaction. Confirm no reserve code path is reachable while holding a lock the Index
   would hold in write mode (the rule in §5.2 "Lock ordering").
3. Record in the Decisions log: snapshot holds (yes/no, with the test), and the final shape of
   `ReserveObserver.Commit`. If the snapshot does **not** hold, propose the alternative
   (e.g. rebuild under the reserve's own locks, or sequence numbers per commit) and update T1.4 and
   T1.6 here before they start.

Keep the test as a regression test.

**Size:** S (timeboxed). **Parallel:** wave 0. **Blocks:** T1.4, T1.6.

---

## T1.1 Package skeleton, protobuf, constants

**Do this first (wave 0):** T1.2, T1.3, T1.7 and T1.8 build on the package and types it creates.
It is small; merge it before starting wave A.

**Scope:** create the tree from README "Package layout" with `doc.go` files.
`pkg/rangesync/pb/rangesync.proto` exactly as §6.2 — including `Symbols`, `More`,
`Status.Mapping` and `Message.AuditRound` (reserved now so Phase 2 and Phase 5 need no version
bump; README issue 8) — gogo, like `pkg/pullsync/pb` (a `doc.go` with the same `go:generate`
line). Generate **only this package**: `GOFLAGS=-mod=mod go generate -run protoc ./pkg/rangesync/pb/`.
Don't run `make protobuf`: it regenerates every `*.pb.go` in the repo, and with Go 1.27 the gzipped
descriptors of all twelve existing files change bytes (same content), which is out of scope. Use
`protoc-gen-gogofaster` **v1.3.2** (matches `go.mod` and the committed files); the Makefile's
`GOGOPROTOBUF_VERSION ?= v1.3.1` is stale and produces different unmarshal code.
Two constants files, split by the import rule (`fingerprint` is the leaf package):
- `pkg/rangesync/fingerprint/constants.go`: index constants (`LeafBits`, `RootSymbols`,
  `Level1Symbols`, `BucketSymbols`, `ScanLimit`) and `DefaultMapperID = 1` (the float mapper of
  T2.1, whose `ID()` must return it);
- `pkg/rangesync/constants.go`: every protocol constant from the README table.
Phase 1 handlers ignore a non-zero `Message.AuditRound` (salted sessions arrive in T5.3).
`pkg/rangesync/rangesync.go`: `Service`, `New(...)`, `Protocol() p2p.ProtocolSpec` with name
`rangesync`, version `1.0.0`, streams `status`, `reconcile`, `fetch` (handlers return "not
implemented" until T1.9/T1.10).

Shared value types used by both `fingerprint` and the protocol (`ID [64]byte`, `Range{Prefix,
Bits}`, `Summary{Count, Digest}`) go in `pkg/rangesync/fingerprint/types.go` so it stays the leaf
package; `rangesync` converts to/from `storer.ChunkID` at the boundary.

**Tests:** proto round-trip; `Protocol()` spec test; `Range` containment helpers.

**Size:** S. **Parallel:** wave 0 (alongside T1.0).

---

## T1.2 LtHash16 primitive (+ ECMH benchmark)

**Depends on:** T1.1 (`fingerprint.ID`).

**Scope:** `pkg/rangesync/fingerprint/lthash.go` per §5.2: `type LtHash [1024]uint16`,
`Add(id)`, `Sub(id)`, `AddHash`, `SubHash`, `Digest() [32]byte` (keccak256 of the lanes,
little-endian — fix and document the byte order), element expansion
`SHAKE128("rangesync/v1" ‖ id)` → 2048 B, plus a salted expansion `Expand(salt, id)` reused by
Phase 5. Use `golang.org/x/crypto/sha3`.

**Benchmark (open question 1):** a throwaway ECMH in a `_test.go` file using
`github.com/btcsuite/btcd/btcec/v2` (already in `go.mod`). Measure `Add` ns/op and memory per
bucket; compare with reserve `Put` throughput (write a small `BenchmarkReservePut` in the
storer tests if none exists). Record numbers + decision in the Decisions log.

**Tests:** group laws (identity, inverse, commutativity, associativity), `Add` then `Sub` =
zero, digest test vector in `testdata/`, fuzz `Add/Sub` sequences.

**Size:** S–M. **Parallel:** wave A.

---

## T1.3 Fenwick tree

**Depends on:** T1.1 (package only).

**Scope:** `pkg/rangesync/fingerprint/fenwick.go`: a **generic** Fenwick tree over a flat slab of
`B` elements of any type satisfying a small group interface (in-place `Add(other)`,
`Sub(other)`, `Zero()`): `Add(i, delta)`, `Prefix(j, out)`, `Range(i, j, out)`. No allocation per
query (accumulate into `out *T`; an LtHash element is 2 KB). It must not depend on `LtHash` or
`CodedSymbol`: their group adapters are added where those types are used (T1.6 for `LtHash`,
T2.2 for symbol arrays).

**Tests:** range sums vs brute force (B = 1, 2, 4096) with a `uint32` and a small fixed-size
array test type; fuzz. A benchmark with a 2 KB array type at B = 4096 (target: a few µs, §4.3.5).

**Size:** S. **Parallel:** wave A.

---

## T1.4 Storer: opt-in `ChunkAddrItem`, write/delete paths, `ReserveObserver` hook

**Depends on:** T1.0.

**Scope (additive changes to `pkg/storer` only)**

1. `ChunkAddrItem` in `pkg/storer/internal/reserve/items.go` per §5.1: namespace `chunkAddr`,
   `ID() = addr ‖ stampHash`, value = `BatchID (32 B) ‖ ChunkType (1 B)`;
   `Marshal/Unmarshal/Clone/String`; extend `items_test.go` and `items_fuzz_test.go`. Also the
   `chunkAddrValid` marker item (used by T1.5).
2. Public types in `pkg/storer/storer.go`: `ChunkID`, `ReserveObserver` (§5.4: `Commit(run)`,
   `RadiusChanged(radius)`, `Reset()`).
3. A reserve option `AddrIndex bool` (default false). **When false, every code path below is
   skipped and the reserve's behaviour is byte-identical to today.** When true, write/delete
   `ChunkAddrItem` **in the same transaction** as `ChunkBinItem`
   (`pkg/storer/internal/reserve/reserve.go`):

   | Path | Location | Change |
   |---|---|---|
   | `Put`, new chunk | `Put`, final `errors.Join(... ChunkBinItem ...)` | put, delta `+id` |
   | `Put`, same address, newer stamp | `Put`, `oldStampIndex.ChunkAddress.Equal(...)` branch | delete old, put new, `−old +new` |
   | `Put`, different address, same stamp index | `removeChunk` → `RemoveChunkWithItem` | covered there |
   | eviction | `RemoveChunkWithItem`, `RemoveChunkMetaData` | delete, `−id` |
   | corruption | `DeleteCorruptedChunkMetadata` (also used by `pkg/storer/recover.go`) | delete if present; `−id` only if it was present |
   | reset | `Reset` | delete in the per-item transaction; call `observer.Reset()` at the end |

   `RemoveChunkWithItem`, `RemoveChunkMetaData` and `DeleteCorruptedChunkMetadata` are
   package-level functions (also used by migrations) — pass the option/observer explicitly
   rather than adding globals.

   **Deltas must be exact (§5.4).** On every delete path, check inside the same transaction
   whether the `ChunkAddrItem` exists and emit `−id` only if it did; emit `+id` only for an item
   actually written. During a background fill (T1.5) or after an interrupted one, items can be
   missing, and a `−id` for an element the Index never added leaves a phantom difference forever.
4. **Observer plumbing.** When an observer is registered, each set-changing `r.st.Run(...)` is
   wrapped as `observer.Commit(func() (adds, removes, err) { ...Run...; return delta })`; the
   delta is empty when `Run` fails. `EvictBatchBin` runs one transaction per item in an
   errgroup — wrap each. With no observer: no wrapping, no allocation. Radius changes go
   through `db.reserve.SetRadius` (three call sites in `pkg/storer/reserve.go`:
   `startReserveWorkers`, `reserveWorker`, `unreserve`) — call `observer.RadiusChanged` from
   one place (wrap `SetRadius` inside `Reserve`).

**Tests (first):** with `AddrIndex=false`, a test asserts no `chunkAddr` keys are ever written
across Put/replace/evict/reset. With `AddrIndex=true`: a table test per path (item
exists/doesn't, exact delta delivered), failed transaction → empty delta, deleting a chunk whose
`ChunkAddrItem` is absent → no `−id`, property test that
`ChunkAddrItem`s always equal the set derived from `ChunkBinItem`s after random
Put/replace/evict. Existing `pkg/storer/...` tests pass unchanged.

**Size:** M. **Parallel:** wave B (the only wave-B task in `pkg/storer`).

---

## T1.5 Storer: fill/marker toggle logic, address iteration, `GetByID`, public API, mock

**Depends on:** T1.4.

**Scope**

1. **Toggle logic (§5.1 "Filling and toggling").** In storer start-up: enabled + marker present
   → nothing; enabled + no marker → drop the `chunkAddr` namespace, fill it from `ChunkBinItem` in
   batched transactions (follow the structure/logging of
   `pkg/storer/migration/removeRogueChunks.go`), write the marker **last**; disabled + marker
   present → delete the marker, then drop the namespace in a background goroutine (low priority,
   cancellable on shutdown). **Not** a numbered migration step. `storer.Options` gets
   `ReserveAddrIndex bool`.
2. **Fill strategy (§5.1 "Blocking or background fill").** First write a benchmark that fills
   4.2M items on the leveldb store and record the time. If < 60 s: fill blocking, before the
   reserve workers start. Otherwise implement the background variant: `AddrIndex` writes are on
   from start-up; the filler takes `r.multx.Lock(batchID)` per item and re-checks that the
   `BatchRadiusItem` still exists inside the same transaction that writes the `ChunkAddrItem`
   (an item evicted during the fill must not come back). Record the measurement and the chosen
   variant in the Decisions log. In both variants `ReserveAddrIndexReady() bool` reports whether
   the marker exists (with the blocking variant it is true by the time anyone can ask).
3. `pkg/storer/migration/reserveRepair.go` (`ReserveRepairer`): drop the namespace and marker.
4. `pkg/storer/internal/reserve/addr.go`: `IterateAddr(prefix, bits, after, cb)` over
   `ChunkAddrItem`; `GetByID(addr, stampHash)` (resolve `BatchID`, then the existing `Get`).
5. `ReserveAddrStore` interface (§5.4) implemented on `*DB`: `ReserveIterateAddr`,
   `ReserveGetByID`, `ReserveHasID` (existence of the `ChunkAddrItem`; peers exchange ids without
   the batch ID, so `ReserveHas` can't be used), `ReserveAddrIndexReady`, `SetReserveObserver`.
   Calls return an error when the address index is disabled. Extend `pkg/storer/mock` with an in-memory implementation for protocol tests.

**Tests:** enable → fill matches `ChunkBinItem`; disable → marker gone, no writes, namespace
eventually empty; disable → mutate reserve → enable → refill, index equals truth (no stale
element); crash mid-fill (cancel the context) → no marker → next start refills; if the
background variant is chosen: concurrent Put/evict during the fill → final index equals truth
(`-race`); iteration with prefix/after cursors at several bit lengths; `ReserveGetByID`
round-trip; `ReserveHasID` true/false/after eviction; repair tool drops both.

**Size:** M.

---

## T1.6 `fingerprint.Index` (counts + LtHash) + observer adapter

**Depends on:** T1.0, T1.2, T1.3. *Not* on T1.4/T1.5 (same or later wave): the observer adapter
declares the subset of `storer.ReserveObserver` / `ReserveAddrStore` it needs as local interfaces
and is tested with a fake; T1.9 wires it to the real storer.

**Scope**

- `pkg/rangesync/fingerprint/index.go` per §5.2 **without symbol caches** (T2.2), plus the
  `LtHash` group adapter for T1.3's generic Fenwick tree:
  `Index{mu, root, leafBits, counts, hashes}`, `Apply(adds, removes)`, `Summary(r)` (levels
  0..`leafBits` via Fenwick; deeper → `ErrBelowLeaf`, the caller scans), `Rebuild(root,
  iterate)`, `SubscribeRootChange()` (debounce interval is a constructor option, 0 in tests).
  Bucket = address bits `r … r+leafBits−1` (`LeafBits = 12`). Elements outside the root are
  ignored by `Apply` and counted in a metric.
- `pkg/rangesync/indexer.go` (or similar): implements `storer.ReserveObserver`:
  - `Commit` takes `mu.RLock()` around `run` and applies the returned delta under an inner
    slab mutex (lock order `ix.mu`(R) → `slabMu`; Fenwick mutations need exclusive access, and
    `Summary` reads take `slabMu` too). It **drops deltas until the first rebuild opens its
    snapshot**, keyed on the observer's own state, never on a storer call: the switch from
    dropping to queuing happens in the same `ix.mu` write-locked section that opens the first
    snapshot iterator (amended after T1.4's contract review; see its Decisions entries);
  - `RadiusChanged` **only queues an asynchronous rebuild and returns**. It is a *hint*: calls
    can arrive out of order with a stale argument, so it never drops a call because the argument
    equals the root. The single rebuild goroutine takes the wake-up token, *then* reads
    `StorageRadius()`, and skips only if that equals the root of the last *completed* rebuild (none
    initially; a failed rebuild clears it). It's called from the reserve worker and from
    `unreserve`; a synchronous rebuild would stall eviction or deadlock against writers waiting
    for `ix.mu` (amended after T1.4's contract review);
  - `Reset` clears.
  - **Start-up order:** `SetReserveObserver` first, then wait for `ReserveAddrIndexReady()`, then
    the initial rebuild from the persisted radius (`StorageRadius()`), so no commit is missed. The
    start-up `SetRadius` in `startReserveWorkers` then only triggers a second rebuild if the radius
    actually changed.
  - Also from earlier tasks' follow-ups: map ranges to bucket spans with `root.BucketSpan` and
    treat `ok == false` as an invalid range; assert `Len() == 1<<leafBits` for every slab;
    `Summary.Digest` is the zero value when `Count == 0`; build rebuild slabs as plain bucket sums
    and convert with `NewFenwickFromBuckets`.
- **Rebuild protocol (§5.2):** steps 1–5 exactly, as confirmed (or amended) by T1.0, and the
  lock-ordering rule: never hold `ix.mu` in write mode while calling into the reserve or
  iterating. While rebuilding, `Summary` for uncovered ranges returns `ErrBusy`.
- **Open question 5 (resolved):** the root is `(overlay, StorageRadius())` = `reserve.Radius()`,
  the same value `DB.IsWithinStorageRadius` uses. `CommittedDepth()` is `Radius() +
  capacityDoubling`, i.e. a sub-prefix of the root; only the audit (Phase 5) uses it. Test: a
  chunk accepted by `IsWithinStorageRadius` is inside the root.

**Tests:** fuzz: random add/remove sequences, `Rebuild()` equals the live index for every
range at every level; concurrent writers during `Rebuild` (`-race`); memory at B = 4096
matches §5.3 (counts 16 KB, LtHash 8 MB); a local fake reserve (implementing the adapter's local
interfaces, not `pkg/storer/mock`, which arrives in T1.5): put/evict/replace, summaries match brute
force; radius change → root moves, and `RadiusChanged` returns
immediately while the rebuild runs; deltas before the first snapshot opens are dropped and the
first rebuild still equals brute force; a Put and an evict committing after the first rebuild's
iterator opened appear in the final index; out-of-order `RadiusChanged` (5→6→5) converges to the
reserve's radius; a failed rebuild followed by the same radius rebuilds again; concurrent
`Commit`s and `Summary` readers during a rebuild (flag for `-race` in CI); a chunk evicted before the fill reached it produces no
phantom difference.

**Size:** M–L.

---

## T1.7 Want set (`pkg/rangesync/wantset`)

**Depends on:** T1.1 (`fingerprint.ID`).

**Scope (§7.8, §8.2):** node-wide in-flight set used by every `rangesync` fetch:
```go
type Set struct{ ... }
func New(deadline time.Duration) *Set              // default 30s
func (s *Set) Claim(id fingerprint.ID, peer swarm.Address) bool
func (s *Set) Done(id fingerprint.ID)              // stored (or ErrOverwriteNewerChunk)
func (s *Set) Release(id fingerprint.ID)           // not delivered / invalid
func (s *Set) Len() int
func (s *Set) Close() error
```
Expired claims are reclaimable through another peer. Bounded memory (≈ 10k IDs, §5.3).
The rejected cache (T4.4) will live here too; leave room for it.

**Tests:** claim/deny/expire/reclaim; concurrent claims (`-race`); bounded size.

**Size:** S. **Parallel:** wave A.

---

## T1.8 Pure session decision logic (fingerprints + items) and session-root relation

**Depends on:** T1.1.

**Scope:** `pkg/rangesync/session.go` — **no I/O**. A `Session` value takes incoming
`Message`s and local summaries (through a small interface) and returns outgoing `Message`s and
wants.

1. **Session root (§7.6):** `SessionRoot(rootA, rootB) (Range, bool)` — narrower root if one
   contains the other, else not ok. Edge-peer cases from §7.6 as table tests.
2. **`Fingerprint` rule (§6.3) in Phase-1 form:** equal → nothing; `cR == 0` → `Items{all
   local, paged}`; `cL == 0` → `Items{none, Reply: true}`; `d̂ > LargeDiff` → a `Striper`
   interface (no-op until T3.1, falls through to split); otherwise split into `2^FanoutBits`
   children; at session leaf depth (`min(Root.Bits + LeafBits)` over both peers) →
   `Items{local, Reply: true}`. Leave the "fits in symbols" branch as a seam for T2.4.
3. **`Items`:** queue IDs not held; if `Reply`, answer with local IDs not in the list; paging
   with `After`/`More`, `ItemsPageSize = 1024`.
4. Validation helpers for T1.9 (§6.4): range inside session root and ≤ leaf depth (+4 bits
   slack), children must partition a range this side sent in its previous message, `MaxParts`.
5. Phase 1 rejects `Symbols`/`More` parts.

**Tests:** table tests per branch; property test: two in-memory sets, two `Session`s driven
against each other until both send empty messages → both hold the union (diffs 0, 1, 20, 1k,
100%, equal counts hiding a diff). Fuzz the message handler.

**Size:** M–L.

---

## T1.9 `status` and `reconcile` stream handlers + validation

**Depends on:** T1.5, T1.6, T1.8.

**Scope:**
- `status` stream: send/receive `Status{Root, Count, Digest, LeafBits, Mapping}` (audit fields
  empty). `SendStatus(ctx, peer)`, handler, `LastStatus(peer)`. Gossip timing is Phase 4.
  A peer whose `Mapping` differs from ours is not eligible for sessions (metric
  `mapping_mismatch_total`); Phase-1 nodes send `fingerprint.DefaultMapperID` (`1`, defined in
  T1.1 so Phase 1 doesn't depend on T2.1).
- `reconcile` stream: initiator `Reconcile(ctx, peer) (wants []storer.ChunkID, err error)` and
  the responder handler, driving `Session` until an empty message. Below-leaf summaries come from
  a `ReserveIterateAddr` scan. Enforce §6.4: invalid range / non-partition / oversize → reset the
  stream and **disconnect** the peer (same approach as `pullsync`'s `Syncer.disconnect`; no
  blocklisting) and report the failure through a `PeerScorer` interface defined here
  (`Fail(peer, reason)`, `Succeed(peer)`), no-op by default. The reconciler implements it (T1.11)
  and owns the single per-peer failure score that T3.3 and T4.4 also feed.
  `MaxConcurrentSessions = 16` node-wide, 1 per peer; over cap, while the
  requested range is rebuilding, or while the storer's address index is still filling
  (`ReserveAddrIndexReady()`, T1.5) → `Message{Busy: true}`.
- `Service` start-up wires T1.6's observer adapter to the real storer in the order T1.6
  prescribes (`SetReserveObserver`, wait for `ReserveAddrIndexReady()`, initial rebuild).
- Per-peer stream limits: **copy** `pkg/pullsync/streamlimit.go` into
  `pkg/rangesync/streamlimit.go` (do not import or move it).

**Tests:** `pkg/p2p/streamtest` two-node tests: equal sets → one round trip; small diff → wants
on both sides; Busy at cap; malformed messages reset the stream. Index wiring through the real
`pkg/storer/mock` (moved here from T1.6, which uses a local fake): put/evict/replace via the mock,
then `Summary` matches brute force for every range.

**Size:** M.

---

## T1.10 `fetch` stream + delivery validation

**Depends on:** T1.5, T1.7.

**Scope:** `pkg/rangesync/fetch.go`: `Fetch(ctx, peer, ids)` sends `Want` (≤ 250 IDs) and reads
`Delivery`s; handler resolves via `ReserveGetByID` with a per-peer delivery limiter
(`pkg/ratelimit`, same budget as `pullsync`'s `handleMaxChunksPerSecond = 250`) and a global
requester-side fetch limiter (1000 chunks/s, configurable).

- Before each `Want`: drop IDs the want set denies, IDs `ReserveHasID` now reports present (cuts
  cross-protocol duplicates in dual mode), **and IDs outside `IsWithinStorageRadius`**.
  `Reserve.Put` does not check the radius itself — `pullsync.Sync` filters at offer time — and
  the radius can grow mid-session, so `rangesync` must filter too.
- Delivery validation applies the same rules as `pullsync.Sync`
  (`pkg/pullsync/pullsync.go:329–410`), **re-implemented** in `rangesync` (no import, no
  extraction): unsolicited check against the requested set, stamp unmarshal + hash,
  `validStamp`, CAC → `unwrap`, SOC → `gsocHandler`, otherwise invalid;
  `ReservePutter().Put`; `ErrOverwriteNewerChunk` not fatal. `New` takes the same
  `unwrap`, `gsocHandler`, `validStamp` arguments `pullsync.New` gets in `pkg/node/node.go`.
- Want-set bookkeeping: `Claim` before `Want`; `Done`/`Release` after; every claim released on
  any early return.
- Metric `cross_protocol_duplicates_total`: chunk already present when `Put` is called.

**Tests:** happy path; unsolicited chunk; invalid stamp; unknown ID (no delivery, claim
released); `ReserveHasID` true right before `Want` → not requested; out-of-radius ID → not requested; fuzz the handler with
malformed `Want`.

**Size:** M.

---

## T1.11 Minimal reconciler (topology subscription, timer-driven sessions, `SyncRate`)

**Depends on:** T1.9, T1.10.

**Scope:** `pkg/rangesync/reconciler/` — a minimal version Phase 4 grows into the full one:
- Own `topology.Driver.SubscribeTopologyChange()` subscription (multiple subscribers are
  supported) and peer iteration; for each connected peer, exchange `Status`; eligible if
  `SessionRoot` is ok. Peers without the protocol are skipped (not handed anywhere).
- Every 60 s and on topology change: for eligible peers with differing digests where our overlay
  is lower, run `Reconcile` then `Fetch`.
- **Owns the per-peer failure score:** implements T1.9's `PeerScorer`; T3.3 (withholding) and T4.4
  (claim-and-stall) report into the same score. Peers above a threshold are deprioritised.
- Implements **`storer.Syncer`** (`SyncRate() float64`, `Start(ctx)`; see
  `pkg/storer/reserve.go:39`). `SyncRate` = chunks/s fetched by catch-up work over a 10-minute
  window, where in Phase 1 "catch-up" means sessions whose resolved difference (the number of
  wants they produced) is ≥ `CatchUpMinDiff = 256` (Phase 3 adds stripes). Use `pkg/rate` like the puller does.
  **Start-up guard:** the reserve worker ticks (and may lower the radius when `SyncRate() == 0`)
  as soon as the storer starts. The puller's rate rises within seconds of `Start`; rangesync's
  wouldn't until it has exchanged `Status` with its neighbors and detected a gap (§8.4). So `SyncRate()`
  must return a non-zero sentinel until the first status exchange with every eligible connected
  neighbor has completed **and** any catch-up it triggered has started, **or** until
  `2 × StatusMaxInterval` (120 s) after `Start`, whichever comes first — a neighbor that never
  answers must not pin the rate above zero. Tests: a fresh rangesync-only node with an empty
  reserve never lowers its radius before that point; a silent neighbor releases the guard after
  120 s.
  `Start` must be safe to call once from the storer's reserve-worker start-up.
- `Close()`; metrics registration hook.

**Tests:** topology mock: peer join/leave; only lower overlay initiates; `SyncRate` > 0 during a
large catch-up and 0 with only small sessions (fake clock); failure score raised on `Fail`.

**Size:** M.

---

## T1.12 Node wiring: flags, combined syncer, metrics registration

**Depends on:** T1.11.

**Scope (`cmd/bee/cmd/cmd.go`, `pkg/node/node.go` only):**
- Flags `--rangesync-enable` (default false) and `--puller-enable` (default true), options
  plumbed into `node.Options`. Full node with both false → start-up error. Document in the flag
  help that `--puller-enable=false` only stops *pulling* via pullsync; the pullsync handlers
  stay registered.
- **Full nodes only.** `rangesync` applies only when `o.FullNodeMode && !o.BootnodeMode`. On
  light, ultra-light and boot nodes, `--rangesync-enable` is ignored with a Warning log: the
  storer is opened without the address index and the protocol is not registered (unlike
  `pullsync`, which is registered everywhere and wrapped in `WithBlocklistStreams` on light nodes,
  `node.go:~1196`).
- `rangesync` enabled on a full node: open the storer with `ReserveAddrIndex: true`; construct
  `rangesync.New` (next to `pullsync.New`, `node.go:~1184`) and register its protocol; construct
  the reconciler inside the existing `if o.FullNodeMode && !o.BootnodeMode` block.
- `pullsync` disabled: don't construct `puller.New`; `pullsync.New` + its protocol registration
  stay.
- **Combined syncer (§8.4)** — a small type in `pkg/node` implementing `storer.Syncer` and
  `status`'s `SyncReporter`: `Start` starts the enabled drivers, `SyncRate` sums them. Replace the
  direct uses of `pullerService` with it: `localStore.StartReserveWorker(...)`,
  `nodeStatus.SetSync(...)`, `isFullySynced` and the warm-up check (`node.go:~1290`, `~1330`),
  the debug log at `~1302`, and metric registration (`~1404`, add rangesync metrics). With
  default flags the combined syncer wraps only the puller and returns its exact values. The
  status protocol and `/status` API keep reporting it in the existing `PullsyncRate` field; don't
  rename it (wire and API compatibility) — add a code comment saying it is the combined rate.
- Closers: register the rangesync service and reconciler like `b.pullerCloser`.

**Tests:** node-level: defaults → no `rangesync` protocol registered, storer opened without the
address index; rangesync-only → puller not constructed, pullsync still registered; both false →
error; light node with `--rangesync-enable` → warning, no address index, protocol not registered.
Unit tests for the combined syncer.

**Size:** M.

---

## T1.13 Integration, coexistence and adversarial tests, metrics

**Depends on:** T1.12.

**Scope**
- Metrics (§10) for Phase 1: `sessions_total{result}`, `session_rounds`,
  `parts_sent/received{type}`, `bytes_metadata{dir}`, `bytes_chunks{dir}`, `pairwise_diff`,
  `index_rebuild_seconds`, `index_apply_seconds`, `index_memory_bytes`, `sync_rate`,
  `cross_protocol_duplicates_total`.
- Protocol (§13): two in-memory reserves via `streamtest`, random overlapping sets, diffs 0, 1,
  20, 1k, 4k, 100%, hidden diffs → both hold the union; message/byte counts within Meyer bounds.
- **Coexistence (§13):** `go test ./pkg/pullsync/... ./pkg/puller/...` unchanged and green; a test
  with one node running both protocols against a `pullsync`-only peer and a `rangesync`-only peer
  converges.
- Adversarial (§13): ranges outside root, non-partitioning children, oversized messages,
  endless `Items` paging, `Symbols`/`More` sent to a Phase-1 node. Fuzz targets modelled on
  `pkg/pullsync/client_fuzz_test.go` and `handler_fuzz_test.go` (copied patterns, new files).

**Size:** M.

---

## Decisions log
- **Penalty policy (pre-implementation):** protocol violations reset the stream and disconnect
  the peer, as `pullsync` does; the reconciler's failure score is raised. No blocklisting for now.
- **Fill strategy (pre-implementation):** blocking fill if it takes < 60 s on a full reserve,
  otherwise background fill with per-batch locking and revalidation (T1.5 measures).
- **Disk cleanup (pre-implementation):** disabling `rangesync` drops the namespace in the
  background.
- **Exact deltas (review):** `−id`/`+id` only for items actually deleted/written; the observer
  drops deltas until the address index is ready.
- **Async rebuild (review):** `RadiusChanged` queues; one rebuild goroutine; observer registered
  before the first rebuild.
- **Full nodes only (review):** the flag is ignored with a warning elsewhere.
- **Failure score (review):** one per-peer score in the reconciler (T1.11), fed through
  `PeerScorer`.
- **Start-up guard timeout (review):** 2 × `StatusMaxInterval`.

_(sessions append here — T1.0 snapshot finding; T1.2 LtHash vs ECMH; T1.5 fill measurement)_
- **T1.1 proto (contract):** `pb/rangesync.proto` is §6.2 verbatim plus the copyright header.
  Points §6.2 leaves open, for G2: (a) `Range.Prefix` strictness — exactly `ceil(Bits/8)` bytes
  and zero bits past `Bits`, or lenient; (b) `Items.After` is `bytes`, assumed to be the 64-byte
  `addr ‖ stampHash` of the last ID of the previous page (`fingerprint.ID`); (c) `Fingerprint.Digest`
  "omitted when Count == 0" — receivers should treat a missing digest as zero; (d) `Range.Bits` is
  `uint32` on the wire and `uint8` in Go, so values above 255 must be rejected at decode time.
  Proposed answers are under "G2 open items" below.
- **T1.1 `fingerprint.Range` (contract):** `Prefix swarm.Address` as in §5.2. It is not comparable,
  so `RangeKey` + `Range.Key()` give a map key (for "ranges sent in the previous message" in T1.8
  and "ranges being streamed" in T2.4). `NewRange` returns the canonical form (exactly
  `ceil(Bits/8)` bytes, bits past `Bits` zeroed); the containment helpers ignore bits past `Bits`.
  Shared bit-level helpers live on `Range` so T1.6/T1.8/T2.2 don't each hand-roll them:
  `Child(bits, i)`, `Children(bits)` and `ChildIndex(addr, bits)` (bucket index with
  `bits = LeafBits` below the root, child/level-1 index with `bits = FanoutBits`).
- **T1.1 `fingerprint.ID` (contract):** `ParseID(addr, stampHash []byte) (ID, error)` is for
  untrusted input and requires exactly 32 + 32 bytes (`ErrInvalidID`); `NewID` is for trusted
  storer values and pads/truncates without validating.
- **T1.1 `fingerprint.Summary` (contract):** `Digest` is the zero value when `Count == 0`, matching
  the wire (omitted), so equal empty ranges compare equal without a special case. T1.6 must keep it.
- **T1.1 constants (contract):** `FanoutBits` lives in `fingerprint/constants.go` (the Index and the
  session both split by it; `LeafBits` is a multiple of it). The §6.4 "+4 bits" slack below leaf
  depth is named `rangesync.LeafDepthSlack = fingerprint.FanoutBits` (one split level). Counts are
  untyped constants, `DefaultMapperID` is `uint32` (the
  `Mapper.ID()` type), intervals are `time.Duration`. The "adaptive 5–30 s" `RoundInterval` is
  `MinRoundInterval`/`MaxRoundInterval`; the claim deadline is `ClaimDeadline`; the global fetch
  limit is `DefaultFetchRate`; the start-up guard is `StartupGuardTimeout`. The fill-blocking
  threshold (60 s) is a storer constant and is **not** in `pkg/rangesync` (the storer cannot import
  it); T1.5 defines it in `pkg/storer`.
- **T1.1 `rangesync.go` (contract):** protocol and stream names are exported (`ProtocolName`,
  `ProtocolVersion`, `StatusStreamName`, `ReconcileStreamName`, `FetchStreamName`) so other
  packages' tests can open streams. `New(streamer, logger)` is minimal; T1.9/T1.10 add their
  dependencies. Stub handlers reset the stream and return an unexported "not implemented" error.
- **T1.1 (implementation):** `Range` helpers treat missing prefix/address bytes as zero and ignore
  bits past `Bits`; `Contains` alone rejects an address shorter than `ceil(Bits/8)` bytes. Programmer
  errors panic: `Child` when `bits > 64` or `r.Bits + bits > 255` (the contract said
  `8·swarm.HashSize`, but `Bits` is a `uint8`), `ChildIndex` when `bits > 64`, and `Children` when
  `bits > 16` or past `MaxBits`, checked before allocating (splits use 4 and buckets 12). `ID.Address()`/`StampHash()`
  return copies. Tests: `fingerprint/types_test.go` (brute-force reference via binary strings),
  `constants_test.go`, `pb/rangesync_test.go` (round trip of every message and oneof kind + pinned
  field numbers of the reserved fields), `rangesync_test.go` (spec, stub handlers via
  `streamtest`). Fuzz: `FuzzParseID`, `FuzzRange`, `pb.FuzzUnmarshal` (stable re-encoding of
  every top-level stream message).
- **T1.1 (review round 1):** `fingerprint.MaxBits = 255` is exported (the largest `Range.Bits`;
  peer input implying a deeper range is rejected against it). `Range.BucketSpan(r, leafBits)
  (lo, n uint64, ok bool)` maps a range to its run of buckets below a root, canonicalizing `r`
  first; it reports violations (root doesn't contain `r`, `r` deeper than `leafBits` below the
  root, `leafBits > 63`) with `ok == false`, not a panic, since `r` may be peer-derived.
  `ChildIndex` reads every bit of the address it is given, so a range prefix passed to it must be
  canonical — use `BucketSpan` for ranges.
- **T1.1 wantset (contract):** `pkg/rangesync/wantset` receives `ClaimDeadline` and `RejectTTL` as
  parameters and never imports `pkg/rangesync` (that would be a cycle once `fetch.go` imports
  `wantset`).
- **T1.1 — G2 items (decided at gate G2, 2026-10-14: all accepted as proposed below; `MaxMessageBytes` stays 1 MiB on the `reconcile` stream only):**
  - **Message size (M2).** `MaxMessageBytes = 1 MiB` (§6.2) exceeds `pkg/p2p/protobuf`'s
    `delimitedReaderMaxSize = 128 KiB`, and a root `Symbols` batch can reach ~336 KB (4096 × 82 B).
    Proposed: the `reconcile` stream builds its reader as
    `protobuf.Reader{Reader: ggio.NewDelimitedReader(stream, MaxMessageBytes)}` (no `pkg/p2p`
    change); `status` and `fetch` keep the 128 KiB default. gogo pre-allocates the declared length,
    so per-stream memory is ≤ 1 MiB, bounded node-wide by `MaxConcurrentSessions = 16`.
    Alternative: lower `MaxMessageBytes` to 128 KiB and cap symbol batches to fit.
  - **Strict decoding.** `Range.Prefix` exactly `ceil(Bits/8)` bytes with zero bits past `Bits`;
    `Range.Bits > 255` rejected; `Items.After` either empty or 64 bytes, and `Range.ContainsID(After)`
    for the part's range; `Fingerprint`: `Count == 0` ⇒ empty `Digest`, `Count > 0` ⇒ exactly 32
    bytes; `ID.Address`/`ID.StampHash` exactly 32 bytes each (`fingerprint.ParseID`).
  - **`Status.Digest` when `Count == 0`.** Proposed: empty, like `Fingerprint.Digest` (one rule for
    both messages; decodes to the zero `Summary.Digest`).
  - **`Status.LeafBits` bound.** Proposed: reject unless `Root.Bits + LeafBits ≤ fingerprint.MaxBits`
    (255, the largest `Range.Bits`), computed in `int` so a large `uint32` cannot wrap.
  - **Ownership.** Strict `pb.Range`/`pb.ID`/`pb.Fingerprint` → `fingerprint` decoding belongs to
    T1.8 (exported for T1.9); stream-level enforcement (reset, disconnect, `PeerScorer`) to T1.9.

- **T1.0 snapshot (open question 6): the snapshot holds.** The storer's `IndexStore` is
  `transaction.NewStorage(sharky, leveldbstore)` in both `initDiskRepository` and
  `initInmemRepository`; there is **no caching layer** on it (`pkg/storage/cache.Wrap` is used
  only for the node statestore, `pkg/storer/internal/cache` is the chunk cache, and
  `cache.Cache` does not override `Iterate` anyway). `transaction.Storage.IndexStore().Iterate`
  goes straight to `leveldbstore.Store.Iterate`, which opens one goleveldb iterator (an implicit
  snapshot) before the first callback; a transaction commit is one atomic `leveldb.Batch` write.
  Tests (kept as regression tests):
  - `TestIndexStoreIterateSnapshot` (`pkg/storer/internal/transaction/snapshot_test.go`):
    on-disk leveldb with a 16 KiB write buffer; transactions that delete, insert (in front of and
    behind the cursor) and overwrite keys commit from inside the callback while memtables are
    flushed (asserted). The iteration returns exactly the pre-open state (ascending,
    `PrefixAtStart`, descending). Negative control (manual, not committed): committing the same
    waves before opening fails all three cases.
  - `TestReserveRebuildSnapshot` (`pkg/storer/internal/reserve/snapshot_test.go`): the §5.2
    rebuild protocol over the real `Reserve` on that stack, with every `Run` wrapped as `Commit`
    will wrap it (read lock across the transaction and the delta, exact delta from
    `ChunkBinItem` writes/deletes), while `Put` (new, same-address and different-address
    replacement, rejected stale stamp) and `EvictBatchBin` run concurrently. At every swap the
    snapshot plus the queued deltas equals the live index, at the end the live index equals the
    reserve, and nothing deadlocks. Negative control (manual, not committed): applying the delta
    *after* the transaction (lock only around the apply) failed 10/10 runs with elements counted
    twice, −1 or missing.
- **T1.0 how step 1 is realized:** `storage.Reader.Iterate` is callback-only, so the open and
  the iteration are one call. Take `ix.mu.Lock()`, call **one** `Iterate` over the root range,
  and release the lock in the first callback (`sync.Once`), or after `Iterate` returns if the
  range is empty. The whole rebuild must be that single `Iterate` call: paging with `after` (a
  new iterator per page) takes a new snapshot per page and breaks step 4.
- **T1.0 lock order (as found in the code).**
  `Reserve.Put`: `r.multx[batchID]` → `r.multx[bin]` → `r.st.Run` (inside: per-address
  `chunkLocker` locks taken by the chunkstore ops, released in `Commit`) → leveldb batch write.
  `EvictBatchBin`: `r.multx[batchID]` only (**no bin lock**, unlike the T1.0 task text), an
  unlocked `Iterate`, then one `Run` per item in an errgroup; the goroutine holding the batch
  lock waits in `eg.Wait()` while the items commit. `Reset`: no `r.multx`, one `Run` per item.
  `SetRadius`: no lock, one `Run` (not set-changing). `Reserve.Get` (hence the future
  `ReserveGetByID`): `r.multx[batchID]` → per-address lock. `DeleteCorruptedChunkMetadata`
  (recovery) and the migrations write the store directly before `reserve.New`, so before any
  observer can exist. No reserve transaction nests another `Run`; `removeChunk` reuses the
  caller's transaction. Nothing in the storer takes `ix.mu`, so with `Commit` the order is
  `r.multx` → `ix.mu` (read) → `chunkLocker` → leveldb, and the rebuild's write-locked section
  only opens a leveldb iterator, which never waits on `r.multx`, `chunkLocker` or `ix.mu`.
  **No reserve code path is reachable while `ix.mu` is write-held.**
- **T1.0 final shape of `ReserveObserver.Commit`: unchanged from §5.4**,
  `Commit(run func() (adds, removes []ChunkID, err error)) error`. The wrapping form is needed:
  a delta reported after the transaction (`Commit(adds, removes)`) can be both in the snapshot
  and in the delta stream (the negative control above). Contract, to be written in the godoc
  (T1.4) and honoured by the adapter (T1.6):
  1. the observer calls `run` exactly once, synchronously, and returns its error; on error it
     applies nothing;
  2. the reserve calls `Commit` once per set-changing transaction and never from inside `run`
     (no nesting: a recursive `RLock` deadlocks as soon as a rebuild is waiting for the write
     lock). `EvictBatchBin` and `Reset` wrap each per-item `Run`, never the whole loop;
  3. the reserve may hold `r.multx` when it calls `Commit`; the observer must not call into the
     storer from `Commit`;
  4. **stricter than §5.2:** `rangesync` never holds `ix.mu` in *either* mode across a storer
     call. In read mode it deadlocks too: a reader holding `RLock` calls `ReserveGetByID`
     (`Reserve.Get` waits for `r.multx[batch]`), `Put` holds that batch lock and waits for
     `RLock` behind a waiting rebuild, and the rebuild waits for the reader. Index locks cover
     in-memory slab work only; scans (`ReserveIterateAddr`) and fetches run without them.
- **Gates G1 and G2 (2026-10-14).** G1: accepted. The snapshot holds; `ReserveObserver.Commit`
  keeps the §5.4 shape with the T1.0 four-point contract, including the stricter rule that
  `ix.mu` is never held in either mode across a storer call. T1.4 and T1.6 proceed unchanged.
  G2: the wire schema is frozen as committed in T1.1, with the decided items above (strict
  decoding, empty digest at `Count == 0` in both `Fingerprint` and `Status`,
  `Root.Bits + LeafBits ≤ fingerprint.MaxBits`, 1 MiB reader on `reconcile` only, strict
  decoders owned by T1.8 and enforced by T1.9).
- **T1.7 API (implementation, after review round 1):** `New(deadline) *Set`,
  `Claim(id, peer) Result`, `Done(id)`, `Release(id, peer)`, `Len()`, `Close() error`, and the
  exported caps `wantset.MaxClaims = 10000` (§5.3) and `wantset.MaxClaimsPerPeer = 1000`. The
  deadline is a parameter (callers pass `rangesync.ClaimDeadline`); `wantset` imports only
  `fingerprint` and `swarm`. A non-positive deadline panics in `New` (programmer error at
  start-up, like the T1.1 helpers). The clock and both caps are injected only through
  `export_test.go` (`NewWithClock(deadline, maxClaims, maxPerPeer, now)`). §8.2 writes
  `claim(id, peer, deadline)` with a per-call deadline; the task fixes it per set, and the
  expiry list relies on that. Deviations from the task text (review round 1, made now because
  the API has no callers yet): `Claim` returns a `Result` instead of `bool`, and `Release` takes
  the peer.
- **T1.7 `Result`:** `Granted`, `Held` (active claim through any peer), `Full` (node-wide cap),
  `PeerFull` (per-peer cap; another peer may still work), `Closed`; starts at `iota + 1`.
  `String()` gives metric labels (`granted`, `held`, `full`, `peer_full`, `closed`) for T4.4's
  `want_claims{result}`. Checks run in the order Closed, Held, Full, PeerFull: when the node is
  full another peer does not help, so `Full` wins over `PeerFull`.
- **T1.7 semantics:** `Claim` reports `Held` for an ID with an active claim through **any**
  peer, the holder included (a second `Want` to the same peer from a parallel stripe or session
  would be a duplicate delivery). A claim expires at exactly `claimed + deadline`
  (`now >= expires`); a reclaim gets a full deadline of its own. `Done(id)` ends the claim
  whoever holds it (once stored, any claim is moot). `Release(id, peer)` ends it only if `peer`
  holds it, so a fetch that outlived its claim cannot end a later claim through another peer
  (the duplicate-grant case found in review). Both are no-ops on an unknown ID. `Close` drops
  every claim, is idempotent and returns nil (fits the node's closers).
- **T1.7 per-peer cap (review round 1):** `MaxClaimsPerPeer = 1000` = four `Want` messages of
  250 IDs (§6.2), i.e. one session fetch plus a few pipelined stripe pages (T3.2) in flight for a
  peer at once; at the 250 chunks/s per-peer delivery limit that is 4 s of deliveries, well inside
  the 30 s deadline. Without it one peer could make IDs look `onlyRemote`, win every claim up to
  the node-wide cap and stall, denying all `rangesync` fetches for 30 s, repeatably. With it one
  peer holds at most 10 % of the cap; filling the set takes 10 stalling peers.
- **T1.7 no goroutines:** expiry is lazy: `Claim` and `Len` drop expired claims first. With one
  deadline per set and the time read under the lock, expiry order is insertion order, so the
  claims sit in a `container/list` in expiry order and pruning is O(1) amortized. Memory is
  strictly bounded by `MaxClaims`. Nothing to stop, so nothing leaks (`goleak` in `TestMain`).
  Per-peer counts live in a map keyed by the peer's address bytes and are deleted at zero.
- **T1.7 clock:** `now` must not go backwards; `time.Now` carries a monotonic reading, so wall
  steps do not matter. With an injected clock that steps back, `Claim` still checks the expiry of
  the claim on its own ID exactly, but a claim made after the step keeps counting in `Len` and
  against both caps until every earlier claim has expired: by at most the size of the step.
  Pinned by `TestClockBackwards`; no full-scan fallback.
- **T1.7 memory (measured):** 268 B per claim, ≈ 2.7 MB at the 10k cap, against §5.3's
  "≈ 1.2 MB". The `map[fingerprint.ID]*list.Element` alone is ≈ 1.2 MB (64-byte keys); the list
  element and the claim (ID, peer counter pointer, expiry) are the rest. An index-linked slab
  would save < 20 %, so the simple form stays; the total budget (≈ 21 MB) is not affected
  materially.
- **T1.3 Fenwick API (implementation).** `fingerprint/fenwick.go` exports
  `Group[T any] interface{ *T; Add(other *T); Sub(other *T); Zero() }` and
  `Fenwick[T any, P Group[T]]`, used as `Fenwick[T, *T]`; `NewFenwick[T](n)` infers `P`. The
  group methods take pointers and work in place, so a 2 KB element is never copied. Exported,
  unlike the lowercase `fenwick[...]` of §5.2: until T1.6/T2.2 use it, an unexported type would
  have no production caller (the `unused` linter), and the tests stay in `package fingerprint_test`.
  §5.2's `fenwick[uint32]` therefore needs a defined count type with these methods (T1.6), just as
  `LtHash` (whose `Add` adds an ID) and the symbol arrays need adapters.
  - Buckets are `int`, 0-based; runs are half-open: `Prefix(j, out)` = `[0, j)` and
    `Range(i, j, out)` = `[i, j)`, so a `root.BucketSpan(r, leafBits)` result `(lo, n)` is
    `Range(int(lo), int(lo+n), out)`. Queries overwrite `out` (`Zero` first) and never allocate.
    `Range` walks the two node chains until they meet, so shared nodes are skipped (≤ 23 nodes at
    B = 4096 instead of 24).
  - Beyond the task text, for T1.6/T2.2: `Sub(i, delta)` (removals); `Update(i, fn)` calls `fn`
    on each of the ≤ log2(B)+1 nodes covering bucket `i`, for sparse deltas (one ID touches ~6 of
    a bucket's 32 symbols; materializing a 2.8 KB delta array per node would be wasteful);
    `NewFenwickFromBuckets(buckets)` converts plain bucket sums into a tree in place in O(B)
    (B·log B calls to `Add` would cost a rebuild ~12 extra LtHash additions per element);
    `Reset()` and `Len()`. The zero value is an empty tree.
  - Bucket indices out of range panic, like slice indexing: they are programmer errors, because
    peer-derived ranges go through `BucketSpan` (`ok == false`) first.
  - No internal locking. `Prefix`/`Range` only read, so they can run under the Index's `RLock`.
  - Benchmark (`BenchmarkFenwick`, 2 KB element = 1024 16-bit lanes packed four per `uint64`
    with SWAR add/sub, B = 4096, Intel Core Ultra 7 165U, noisy laptop): `Range` worst case
    (2047, 4095), 23 nodes, ≈ 6.0–6.3 µs; random ranges ≈ 3.2 µs; `Prefix` worst ≈ 3.2 µs; `Add`
    worst (13 nodes) ≈ 3.5 µs; all 0 allocs. `NewFenwickFromBuckets` on 8 MB ≈ 2 ms (with the
    copy). Meets the §4.3.5 "few µs" target.
- **T1.3 (review round 1, supersedes "No internal locking … can run under the Index's
  `RLock`" above).** A `Fenwick` mutation (`Add`, `Sub`, `Update`, `Reset`) needs exclusive
  access: it writes the upper nodes that every query reads. `ix.mu.RLock` does not give that,
  because writers hold it too (§5.2 step 4 and the T1.0 `Commit` contract: concurrent `Commit`s
  from `Put`s on different batches and from `EvictBatchBin`'s errgroup workers all apply deltas
  under the read lock). T1.6 therefore needs an inner slab mutex, lock order `ix.mu` (R) →
  `slabMu`, held for `Apply` and for the `Summary`/`Range` reads (or one mutex per slab with a
  fixed order). The `out` and `delta` arguments must not point into the tree's own storage
  (now in the godoc). Correction to the entry above: drop "instead of 24"; a range reads
  `popcount(i)+popcount(j) ≤ 2·bits.Len(n)−1` nodes, 23 at most at B = 4096.
- **T1.4 types (contract):** `internal/reserve` cannot import `pkg/storer`, so the element type
  is defined once as `reserve.ChunkID` (`Address`, `StampHash`, `BatchID`, per §5.4) and
  `storer.ChunkID = reserve.ChunkID` is an alias (precedent: `storer.SessionInfo =
  upload.TagItem`). `storer.ReserveObserver` is a real interface in `storer.go` and carries the
  full godoc with the T1.0 four-point contract; the reserve calls `reserve.Observer`, which has
  the identical method set. `storer.go` checks at compile time that the two are assignable both
  ways, so they cannot drift. Consumers use only `storer.ChunkID`/`storer.ReserveObserver`.
- **T1.4 observer semantics (contract), beyond §5.4:** `Commit` returns `run`'s error as is (the
  reserve's callers test `errors.Is(err, storage.ErrOverwriteNewerChunk)`); the delta slices are
  fresh per transaction and may be retained; `adds`/`removes` are a group difference, so the
  order of application doesn't matter. `RadiusChanged` is called at the end of **every**
  `Reserve.SetRadius` with the radius in effect (the in-memory value is stored first, so also
  when persisting it fails), possibly unchanged — the observer dedupes, as T1.6 already assumes
  for the start-up `SetRadius`. `Reset()` is called once, only at the end of a successful
  `Reserve.Reset`, after its per-item commits; on a failed `Reset` the per-item deltas already
  delivered keep the observer exact. Set changes made before the observer is registered
  (recovery's `DeleteCorruptedChunkMetadata`, migrations, the T1.5 fill) are never reported.
- **T1.4 option placement (contract):** T1.4 adds no `storer.Options` field; T1.5 adds
  `ReserveAddrIndex` and `SetReserveObserver`. The reserve-level `AddrIndex` option and the
  reserve's observer registration are internal to `pkg/storer` and arrive with the T1.4
  implementation commit.
- **T1.4 registration (contract review, HIGH):** an unwrapped `Run` that started before
  `SetReserveObserver` and commits after the first rebuild opened its snapshot would be in
  neither the snapshot nor the delta stream, forever (a re-`Put` returns early at `r.Has`).
  Registration happens after `storer.New`, with reserve workers, pushsync and the API already
  running. Fix: a reserve-level registration `sync.RWMutex`; every set-changing path of `Put`,
  `EvictBatchBin` (each per-item goroutine) and `Reset` loads the observer and runs its
  transaction, wrapped or not, under its read lock; registration takes its write lock, so it
  returns only after in-flight unwrapped transactions committed, and every later one is
  wrapped. Lock order `r.multx` → registration lock (R) → `ix.mu` (R, inside `Commit`) →
  `chunkLocker` → leveldb; a holder of the registration read lock never waits for `r.multx`.
  Registration is **once-only**, nil rejected (single observer, §5.4).
- **T1.4 `RadiusChanged` is a hint (contract review, MEDIUM 2; supersedes the "observer
  dedupes" sentence of the semantics entry above):** concurrent `SetRadius` calls
  (`startReserveWorkers`, `reserveWorker`, `unreserve`) are not serialized and `SetRadius` is
  store → persist → notify, so `RadiusChanged(5)` can arrive after `RadiusChanged(6)` while
  `Radius()` is 6; and dropping a call whose argument equals the current root is wrong under
  coalescing (root 5, rebuild to 6 queued, a later 5 dropped → index at 6, reserve at 5).
  `SetRadius` locking is **not** changed. The observer must not trust the argument: it always
  queues; the rebuild goroutine re-reads `StorageRadius()` when it starts (lock-free, no `ix.mu`
  held) and skips only if it equals the root of the last rebuild it started. Correct because
  every `SetRadius` notifies after its in-memory store, so a wake follows the final value.
- **T1.4 dropping deltas before the first index (contract review, MEDIUM 3):** T1.6's "drops
  deltas while `ReserveAddrIndexReady()` is false" would be a storer call inside `Commit`
  (point 3). The observer keys the drop on its own state ("no index built yet"), read under the
  `ix.mu` read lock it holds across `run`; the first rebuild's snapshot, taken under the write
  lock once the index is ready, covers anything dropped. Stated in the `Commit` godoc.
- **T1.4 first-snapshot switch (contract review round 2, MEDIUM; supersedes "no index built
  yet" in the entry above):** keying the drop on "no index built yet" (slab set at the swap)
  loses every commit made during the 10–20 s first iteration: it is in neither the snapshot nor
  the delta stream. The drop state ends at rebuild **step 1**: the observer switches from
  dropping to queuing for the new slab in the same `ix.mu` write-locked section that opens the
  first snapshot iterator. A commit is then before that section (in the snapshot) or after it
  (queued). Stated in the `Commit` godoc.
- **T1.4 registration lock details (contract review round 2; extends the T1.0 lock order to
  `r.multx` → registration lock (R) → `ix.mu` (R, inside `Commit`) → `chunkLocker` →
  leveldb):** the read lock is taken per transaction, in the goroutine that runs it, and
  released when that transaction and its `Commit` returned; never held across `eg.Wait` (the
  `EvictBatchBin`/`Reset` parents don't hold it) and never taken twice by one goroutine
  (a parent holding R + a pending registration + a worker blocked on R deadlocks). A second
  registration is rejected by an atomic flag checked before the write lock, so it never
  waits for in-flight transactions. **Decision:** with `AddrIndex` disabled the reserve rejects
  registration, so no observer can exist and the registration lock is skipped entirely (the
  disabled path stays as today: no lock, no wrapping, no allocation).
- **T1.4 `RadiusChanged` hint protocol (contract review round 2; supersedes "the root of the
  last rebuild it started"):** (1) the rebuild goroutine takes the wake-up token *before* it
  reads `StorageRadius()`, so a `SetRadius` that lands after the read leaves a token behind;
  (2) it skips only if the value equals the root of the last rebuild that **completed**; a
  failed rebuild clears it, and the initial value is "none", not 0 (0 is a valid radius);
  (3) `SetRadius` loads the observer **after** storing the radius, through a synchronized load
  (an atomic pointer, or the registration read lock released before calling `RadiusChanged`),
  so a `SetRadius` racing with registration either notifies the observer or stored its radius
  before the observer's first rebuild reads it; `Reset`'s final `observer.Reset()` uses the same
  load. (1)–(2) are T1.6's, (3) the T1.4 implementation's; all are in the godoc.
- **T1.4 "failed" transaction (contract review round 2, LOW C):** `transaction.Commit` can
  return an error after the leveldb batch was written (sharky `Release` errors joined after
  `batch.Commit`, `pkg/storer/internal/transaction/transaction.go:186–196`), so `Run`'s error
  alone doesn't say whether the transaction committed. Definition: a transaction failed iff its
  leveldb batch was not written. **Choice:** contract point 1 stays as is (error ⇒ apply
  nothing); the reserve keeps a post-commit error out of `run` — `run` returns the full delta
  and nil, and the reserve returns the stashed error to its caller after `Commit` (the caller
  sees the same error as today). To tell the cases apart, the implementation adds an additive,
  identifiable post-commit error in `pkg/storer/internal/transaction` (a sentinel wrapped into
  the post-commit release errors only; rollback-path errors are unchanged). Stated in the
  `Commit` godoc.
- **T1.4 superseded phrases (contract review round 2):** in "T1.4 observer semantics" above,
  "`RadiusChanged` … with the radius in effect … the observer dedupes" is superseded by the
  hint entries (the argument is the value that call set, possibly stale); and "Set changes made
  before the observer is registered (… the T1.5 fill)" is superseded: the fill is never
  reported regardless of timing (a background fill runs concurrently with `Commit`); only the
  `Put`, eviction and `Reset` paths are.
- **T1.4 (implementation) reserve API:** `reserve.New(..., opts ...Option)` with
  `WithAddrIndex(bool)` (default off; existing callers unchanged). `(*Reserve).SetObserver(o)
  error` checks, in order: index disabled → `ErrAddrIndexDisabled`, nil → `ErrNilObserver`, an
  atomic once-flag → `ErrObserverRegistered` (a rejected nil registration does not use up the
  slot), then takes the registration write lock. Every set-changing transaction goes through
  one helper, `(*Reserve).run`, which with the index disabled is the old `r.st.Run` call (no
  lock, no wrapping), with the index on and no observer runs under the registration read lock
  without wrapping or allocating, and otherwise wraps the transaction in `Commit`. The exported
  package-level `RemoveChunkWithItem`, `RemoveChunkMetaData` and
  `DeleteCorruptedChunkMetadata` take an explicit `addrIndex bool`; `recover.go` and the
  migrations (`removeRogueChunks.go`, `reserveRepair.go`) pass `false` until T1.5 plumbs
  `storer.Options.ReserveAddrIndex` (follow-up below).
- **T1.4 (implementation) exact deltas:** a `ChunkAddrItem` delete always checks existence first
  (inside the same transaction, under the locks that serialize that key) and emits `−id` only
  for a deleted entry; a put reads existence only when recording a delta (so the index-on,
  no-observer path costs no extra read) and emits `+id` only for an entry that did not exist.
  `Reset` deletes the `ChunkAddrItem` in each per-item transaction and leaves the validity marker
  (an emptied index is still complete); it does not sweep `chunkAddr` keys that have no
  `BatchRadiusItem`, since none can exist by construction.
- **T1.4 (implementation) `chunkAddrValid` marker:** `ChunkAddrValidItem{Version uint8}`, value
  is one version byte (`ChunkAddrIndexVersion = 1`). The storage item test suite rejects an
  empty value, and the version lets a later layout change force a refill.
- **T1.4 (implementation) post-commit errors:** `transaction.ErrAfterCommit` is carried (via
  `errors.Is`) only by the post-commit sharky release errors of `Commit`; an unexported wrapper
  keeps the error message unchanged, so callers that don't look for it see the same error as
  before. The reserve's `run` keeps such an error out of `run`'s return (full delta delivered)
  and returns it after `Commit`.
- **T1.4 (implementation) test sensitivity (manual negative controls, not committed):**
  releasing the registration read lock before an unwrapped transaction made
  `TestAddrIndexRegistrationRace` fail in 9/9 rounds; recording `−id` without the existence
  check failed `TestAddrIndexAbsentItem`; treating `ErrAfterCommit` as a failure failed
  `TestAddrIndexPostCommitError`. Loading the observer before storing the radius in `SetRadius`
  is caught by `TestAddrIndexRadiusRegistrationRace` only with the window widened (100 µs
  sleep): the natural window is a few instructions, so that test is a smoke test and relies on
  `-race` in CI.
- **T1.4 review fixes (supersede the "reserve API" and "exact deltas" entries above where they
  differ):**
  - *Call shape.* `run(ctx, tx, f)` takes the transaction callback in the old
    `func(transaction.Store) error` shape and the writer separately (`tx := r.newAddrTx()`,
    captured by `f`); with the index disabled `tx` is nil and `run` is exactly `r.st.Run(ctx, f)`.
    The previous version wrapped `f` in a second closure, one extra heap allocation per
    transaction on the default path. `TestRunDisabledAllocs` compares `testing.AllocsPerRun` of
    `run` against the plain `r.st.Run` call (prebuilt and per-call closures): equal, and it
    reports 1 vs 0 with the old wrapper (manual control). `-gcflags=-m` shows no new escapes in
    the callers; the only new heap values (`ran`, `afterCommitErr`, the `Commit` closure) are on
    the observer path. The index-on paths allocate one writer per transaction.
  - *Exactness without pending reads.* A transaction's `Has` reads the committed state, not its
    own pending batch. When recording, the writer remembers per key whether the entry exists
    after its own writes and deletes, so a delete then put of one id gives −id +id, a put then
    delete +id −id, and repeated deletes or puts count once (`TestAddrTxSameKeyTwice`). No reserve
    path touches one key twice today. Without an observer the writer does plain `Put`/`Delete`
    (no existence read; deleting an absent key is a no-op in leveldb).
  - *What serializes a key.* The existence check is exact because nothing else writes that
    `(address, stamp hash)` key concurrently: the stamp hash binds the key to one batch, and
    `Put` and eviction hold that batch's `r.multx` lock; `Reset` relies on its precondition that
    it runs before any other call to the reserve; the T1.5 fill must take the batch lock too.
  - *Observer contract check.* If `Commit` returns without calling `run`, or calls it a second
    time, the reserve returns `ErrObserverContract` (the second call doesn't run) instead of
    silently dropping or repeating the write (`TestAddrIndexObserverContract`).
  - *No shared writer.* The shared package-level non-recording writer is gone;
    `addrTxFor(true)` returns a new one per call.
  - *New tests.* `TestAddrIndexSetObserverWaits` (deterministic: a gated transaction blocks
    `SetObserver` until it commits; the transaction is not reported), `TestAddrIndexSharedAddress`
    (one address under two batches: evicting, pinned or not, and resetting remove only their own
    `ChunkAddrItem`), and `TestAddrIndexProperty` now also mixes in same-address puts under the
    other batch, pinned evictions and `Reset`.

- **T1.8 (contract): files and names.** `session.go` (`Session`, `NewInitiator`, `NewResponder`,
  `Handle`, `Config`, `Local`, `Summarizer`, `Striper`, `Decide`/`Decision`, `ErrPeerBusy`),
  `scope.go` (`SessionRoot`, `Scope{Root, LeafDepth}`, `NewScope`, `Scope.CheckRange`,
  `Scope.CheckMessage`, `ErrDisjointRoots`, `ErrInvalidScope`) and `wire.go` (strict decoders
  `ParseRange`, `ParseID`, `ParseFingerprint`, `ParseItems`, `ParseStatus`; encoders `EncodeRange`,
  `EncodeID`, `EncodeFingerprint`, `EncodeItems`, `EncodeStatus`; decoded `Status` and `Items`;
  `ErrInvalidMessage`). Every peer violation (strict decoding, scope, unsolicited part, paging,
  `MaxParts`) wraps the one sentinel `ErrInvalidMessage`, which T1.9 maps to reset + disconnect +
  `PeerScorer.Fail`; `NewScope` errors and `ErrPeerBusy` are not violations. `Decide` is exported as
  the pure function of §6.3 so its branches are table-tested on their own.
- **T1.8 (contract): local interfaces, no `ctx`.** `Summarizer{Summary(r) (Summary, error)}` is what
  `*fingerprint.Index` (T1.6) must satisfy. `Local = Summarizer + IterateIDs(r, after *ID, fn) +
  HasID(id)` is what a `Session` reads; T1.9 builds it from the Index, `ReserveIterateAddr` and
  `ReserveHasID`, answering summaries below the index leaf depth with a scan. No method takes a
  `context.Context`, matching the ctx-less storer calls (§5.4) and the in-memory Index.
- **T1.8 (contract): `Status` lives here.** `NewScope(local, remote Status)` consumes it and the
  G2 `Status` rules (empty digest at `Count == 0`, `Root.Bits + LeafBits ≤ MaxBits` in `int`) are
  part of strict decoding, so `Status`, `ParseStatus` and `EncodeStatus` are T1.8's; T1.9 uses
  them for the status stream. Phase 1 does not decode `AuditRound`/`AuditDigest` (Phase 5 adds).
- **T1.8 (contract): scope bounds.** `NewScope` rejects a session root deeper than the session leaf
  depth (T1.1 follow-up) **and** `LeafDepth + LeafDepthSlack > fingerprint.MaxBits`: the G2 bound
  alone admits a leaf depth of 255, and a split just above it would ask `Child` for depth > 255
  (panic). Items branch: `R.Bits >= LeafDepth`.
- **T1.8 (contract): decision order.** Equal → `cR == 0` (send items) → `cL == 0` (request items)
  → stripe (initiator only, `R.Bits < LeafDepth`, `max(|cL−cR|, 1) > LargeDiff`; declined → split)
  → *T2.4 seam: fits in symbols* → `R.Bits >= LeafDepth` (exchange items with `Reply`) → split into
  `R.Children(FanoutBits)`. The responder never stripes.
- **T1.8 (contract): deviation from §6.4 "in the previous message".** That rule cannot hold with
  `MaxParts = 1024`: a message with more than 64 fingerprints that all need a split must be
  answered with more than 1024 parts. It happens honestly in Phase 1 (no symbols): ~100 spread
  differences make ~100 of the 256 level-2 ranges differ. Adopted instead: work that does not fit
  (`MaxParts`, `MaxMessageBytes`) is deferred to later messages; a split always arrives whole (all
  `1<<FanoutBits` children in one message, exactly `R.Children(FanoutBits)`, not an arbitrary
  partition) and must answer a range this side sent a fingerprint for that the peer has not
  answered yet; a range is answered at most once (a split or an `Items` stream). The
  anti-amplification property of §6.4 is kept: the peer can only ask about children of ranges
  this side raised, once each. An equal range is never answered, which cannot be told apart from
  a deferred one, so it stays "unanswered" until the session ends (memory bounded by the
  fingerprints this side sent, at most the range tree of the scope). The opening message (the
  responder's first) is one or more `Fingerprint` parts of pairwise disjoint ranges inside the
  scope and nothing else, which leaves room for T3.2 stripes and T3.4 halves.
- **T1.8 (contract): `Items` paging.** A stream is the pages of one range from one side, in
  strictly ascending ID order; `After` is empty on the first page and the last listed ID on every
  later page; a page with `More` lists at least one ID; at most one peer stream per range. A page
  covers `(After, last ID]` while `More`, `(After, end of R]` on the last page. `Reply` is answered
  page by page with the local IDs in the covered interval that the page does not list, so the
  peer's list is never accumulated and the cursor continuity bounds the listing work by `|R|` per
  stream. An `Items` stream with an empty `After` must answer an unanswered fingerprint of this
  side, or (without `Reply`) a range this side sent `Reply` pages for.
- **T1.8 (contract): termination.** A side sends an empty `Message` only with nothing pending (no
  deferred work, no own stream with `More`). The receiver of an empty `Message` with nothing
  pending ends the session **without replying** (`Handle` returns a nil `out`), so equal sets cost
  one round trip; the sender of the empty `Message` then reads the peer's next message or the end
  of the stream, both normal. The peer sending an empty `Message` while one of its streams has
  `More` set is a violation.

- **§6.4 deviation, decided by the user 2026-10-14 (T1.8 contract review; supersedes the
  "deviation from §6.4" entry above where they differ).** The §6.4 wording "a `Fingerprint` child
  set must partition a range the receiver sent in the previous message" is replaced by: a split
  must answer a range this side sent a `Fingerprint` for and has not had answered, at most once,
  as exactly `R.Children(FanoutBits)` in one message (answers can be deferred to any later
  message). Plus a depth bound: (a) a peer split of `R` is rejected unless
  `R.Bits + FanoutBits ≤ LeafDepth` (this covers the review's `R.Bits >= LeafDepth` case and
  every other split the honest rule never makes; before, `CheckRange`'s slack let such splits
  through); (b) the decision rule splits only when `R.Bits + FanoutBits ≤ LeafDepth`, otherwise
  it exchanges items, so children are never deeper than the leaf depth. **State bound
  (corrects "at most the range tree of the scope" above):** with `LeafDepth − Root.Bits ≤
  LeafBits = 12` (the local `LeafBits` bounds it, since the session root lies inside the local
  root), an honest session opened with the root holds at most 16 + 4096 unanswered ranges on the
  responder and 1 + 256 on the initiator, ~80 B each (`RangeKey` + map overhead), ≈ 330 KB worst;
  misaligned adversarial openings stay at ~4096 per side because splits stop at the leaf depth.
- **T1.8 contract review fixes (2026-10-14).**
  - `Config.Open []fingerprint.Range` (default `{Scope.Root}`; each passes `CheckRange`, pairwise
    disjoint, ≤ `MaxParts`) lets T3.2/T3.3 open with a stripe and T3.4 with the two halves of a
    new root.
  - Decision order is now Equal → send items → request items → stripe (initiator,
    splittable, `d̂ > LargeDiff`) → *T2.4 seam* → split (splittable) → exchange items.
    `Decide(DecisionInput)` takes a struct so T2.4 can add `cap(R)` and the hint without
    changing callers. `Decision` integers are not stable (T2.4 inserts `DecisionSymbols`): never
    send, store, log or use them as metric labels by value.
  - **Pending Reply pages (`MaxPendingReplies = 16`, `constants.go`).** Answers are produced
    lazily: an own `Items` stream is a cursor over `Local.IterateIDs`; an answer to a peer `Reply`
    page is its covered interval plus the page's IDs this node holds (the exclusion). At most 16
    pending Reply pages keep their exclusion lists (≤ 16 × 1024 IDs, 1 MiB). Past that, a Reply
    page is kept as its interval only, merged with the earlier pending intervals of the same range,
    and answered with every local ID in it: the peer gets IDs it already holds (no want), memory
    stays bounded by the number of ranges, and an honest session never stalls or errors. A hard
    cap with an error was rejected: the sender cannot observe the receiver's backlog (an empty
    answer is silent), so honest sessions could not stay under it, and ending the session on it
    could livelock (the next session hits the same backlog).
  - The peer's answer stream to this side's `Reply` stream for a range may end (no `More`) only
    after this side sent its last `Reply` page for that range; earlier is a violation.
  - `Busy` is valid only as the first message an initiator handles, with no parts
    (`ErrPeerBusy`); anywhere else it wraps `ErrInvalidMessage`.
  - Salted sessions (for T1.9/T5.3): the responder's handler reads the opening's `AuditRound`
    before `NewResponder` to choose a salted `Local`; the initiator's caller sets `AuditRound` on
    the opening message. The `Session` ignores `AuditRound`.
  - `Handle`'s per-call work is bounded and stated in its doc: one `Summary` per received and per
    emitted `Fingerprint` (each ≤ `MaxParts`), one `HasID` per received ID, `IterateIDs` only as
    far as the emitted IDs plus the received Reply pages; output ≤ `MaxMessageBytes`. A `Local`
    error (e.g. the Index busy rebuilding) ends the session, does not wrap `ErrInvalidMessage` and
    is not scored.
  - `NewScope` no longer rejects `LeafDepth + LeafDepthSlack > MaxBits` (supersedes that part of
    "T1.8 (contract): scope bounds"): with (b) no split goes past the leaf depth, so `Child` cannot
    be asked for depth > 255. It still rejects a root deeper than the leaf depth, and re-checks
    `Root.Bits + LeafBits ≤ MaxBits` for both statuses.
- **Correction (T1.8 contract review, L3; corrects "computed in `int`" in the G2 items and in
  "T1.8 (contract): `Status` lives here"):** `Status.LeafBits` is a `uint32` on the wire, and
  `int(Root.Bits) + int(LeafBits)` wraps on 32-bit builds (goreleaser ships 386 and arm).
  `ParseStatus` compares in `uint64` (equivalently, rejects `LeafBits > MaxBits` first). In
  `NewScope` both terms are already `uint8`, so the `int` sum is safe there.
- **T1.2 open question 1 (LtHash vs ECMH): LtHash16.** Benchmarks on the 4-core dev laptop
  (Core Ultra 7 165U, shared with other agents, load 1.6–3.5: expect ±30% noise), `go test
  -bench -count 5`, all 0 allocs/op:

  | | LtHash16 | ECMH (secp256k1, `btcec/v2`) |
  |---|---|---|
  | element update (expand + add into one bucket) | 4.5–5.0 µs (`Add`) | 27–33 µs (try-and-increment hash-to-curve + Jacobian add) |
  | group op (Fenwick node) | 200–230 ns (`AddHash`/`SubHash`) | 0.79–0.86 µs (`AddNonConst`) |
  | worst-case range query, 24 nodes (§4.3.5) | 5.7–6.4 µs | 15–19 µs |
  | digest | 6.9–7.3 µs (keccak256 of 2 KB) | 14–20 µs (`ToAffine` inversion + keccak of 33 B) |
  | memory per bucket / at B = 4096 | 2048 B / 8 MB | 120 B Jacobian / 480 KB (33 B / 132 KB compressed, but a Fenwick slab must stay Jacobian to avoid an inversion per op) |

  Baseline: `BenchmarkReservePut` (new, `pkg/storer/reserve_bench_test.go`; disk storer, 20 000
  stamped chunks over bins 0–7, no eviction) **150–255 µs/op** (16 KB, 315 allocs/op). Per element
  committed, the index does one expansion plus ≤ 12 Fenwick node updates (log2 4096): LtHash ≈
  4.7 + 12 × 0.21 ≈ **7 µs (≈ 3–5 % of a Put)**, ECMH ≈ 30 + 12 × 0.83 ≈ **40 µs (≈ 16–26 %)**. A
  4.2M-element rebuild is ≈ 20 s of expansions for LtHash vs ≈ 2 min for ECMH. ECMH's only win is
  ~7.5 MB of memory, which §5.3 already budgets; it loses ~5.5× per update, ~3× per range query and
  ~2× per digest. **Decision: LtHash16** (§5.2 unchanged; §5.3 keeps the 8 MB row). ECMH code stays
  in `fingerprint/ecmh_bench_test.go` as a throwaway benchmark (test-only, `TestECMHGroup` checks it).
  - *Caveat (T1.2 review):* the benchmarks run with hot caches (a few nodes reused in a loop). A
    real 8 MB LtHash slab misses cache on each 2 KB node of a range query, while ECMH's 480 KB slab
    fits in L2, so the range-query gap is narrower in production than the 3× above. It does not
    change the decision: the update cost (expansion) and the rebuild cost dominate, and both are
    cache-independent.
  - *Correction (T1.2 review):* a Fenwick update at B = 4096 touches **up to 13** nodes
    (log2 4096 + 1), not 12: LtHash ≈ 4.7 + 13 × 0.21 ≈ 7.4 µs, ECMH ≈ 30 + 13 × 0.83 ≈ 41 µs per
    element; the percentages of a Put are unchanged. The 20 000-chunk Put figure was measured with
    `-benchtime=20000x` (`BenchmarkReservePut` generates `b.N` chunks before the timer starts).
- **T1.2 `LtHash` representation (deviation from the §5.2 sketch).** `type LtHash struct{ w
  [256]uint64 }` instead of `[1024]uint16`: four lanes per word, added with SWAR (`add16`/`sub16`,
  carries masked at bit 15 of each lane) — `AddHash` ~3× faster than the per-uint16 loop (scratch
  benchmark, same run: 0.47–0.66 µs vs 1.6–1.8 µs; ~210 ns in a quieter run), which is what makes the T1.3 "few µs" range target (≈ 6 µs above)
  reachable. No `unsafe` (bee uses none). The byte format is unchanged: lane `i` is bits
  `16·(i mod 4)` of word `i/4`, and words are read/written little-endian, so the 2048-byte
  expansion and digest input are exactly the lanes as little-endian `uint16`, lane 0 first.
  The method set is §5.2's (`Add(id)`, `Sub(id)`, `AddHash`, `SubHash`, `Digest`) on pointer
  receivers, in place, aliasing allowed (`h.SubHash(h)`), 0 allocations (SHAKE/keccak state and
  buffers come from `sync.Pool`s). Added: `Lane(i) uint16` (panics out of range), constants
  `LtHashLanes = 1024`, `LtHashSize = 2048`. The zero value is the empty set and values compare
  with `==`.
- **T1.2 expansion and salt (contract for T1.6 and Phase 5).** `Expand(salt []byte, id ID) LtHash`
  = SHAKE128(`"rangesync/v1"` ‖ salt ‖ id) → 2048 B → 1024 little-endian lanes; `Add`/`Sub` use
  `Expand(nil, id)` (an empty salt equals nil). The input is injective (fixed prefix, fixed-size id
  last). §7.9's `SHAKE128(anchor ‖ id)` is read as this domain-separated form: the audit (T5.1) sums
  `e := Expand(anchor, id); acc.AddHash(&e)`. Phase 5 must use the same function on both sides.
- **T1.2 digest.** `Digest()` = keccak256 (legacy Keccak, as `swarm` hashes) of the 2048 lane bytes.
  The digest of the zero value is `aeffb38c…4a22da`, **not** zero (the T1.1 follow-up stands: T1.6
  sets `Summary.Digest` to zero when `Count == 0`). Test vectors (`fingerprint/testdata/
  lthash_vectors.json`: empty, 1/2/3 elements, negated, add-then-sub, salted) were produced by an
  independent Python implementation (`hashlib.shake_128`, pure-Python Keccak-256, plain uint16
  lanes); changing them is a wire change.
- **T1.5 (contract): `SetReserveObserver` returns an error** (settles the T1.4 → T1.5 round-2
  follow-up; deviates from §5.4's signature). `SetReserveObserver(o ReserveObserver) error`
  returns `storer.ErrAddrIndexDisabled` (index disabled, or no reserve), `storer.ErrNilObserver`
  and `storer.ErrObserverRegistered`. The three are the reserve's sentinels re-exported (the same
  values, so `errors.Is` matches either name). Not a panic: the disabled case is reachable from
  configuration (a wiring that enables `rangesync` on a storer opened without the index; light
  nodes have no reserve at all, D15), CODINGSTYLE "Panic with care" rules out production panics,
  and an error lets T1.9/T1.12 refuse to start `rangesync` instead of crashing the node. Local
  interfaces that include registration (T1.6 left it out, T1.9 wires it) declare
  `SetReserveObserver(storer.ReserveObserver) error`. `ReserveAddrIndexReady() bool` keeps
  §5.4's shape and reports false when the index is disabled.
- **T1.5 (contract): disabled index.** With `Options.ReserveAddrIndex` false or no reserve
  (`ReserveCapacity` 0), every `ReserveAddrStore` method that returns an error returns
  `ErrAddrIndexDisabled`, and `ReserveAddrIndexReady` is false. That is the task's "calls return
  an error when the address index is disabled", applied to the methods §5.4 gives an error.
- **T1.5 (contract): "not ready" is not an error.** While the index is enabled but not complete
  (a background fill, should G4 choose it), the reading methods don't fail; they read the entries
  written so far, a subset of the reserve set (never more: the filler re-checks the
  `BatchRadiusItem` under the batch lock). Callers gate on `ReserveAddrIndexReady`: T1.6's rebuild
  waits for it, T1.9 answers `Busy`, and T1.10's `ReserveHasID` before a `Want` can at worst
  request a chunk the node already holds. With the blocking fill the case does not arise once `New`
  returns.
- **T1.5 (contract): `ReserveIterateAddr`.** The range is the first `bits` bits of `prefix`; bits
  past `bits` are ignored and missing prefix bytes count as zero, as in `fingerprint.Range`.
  `after` is compared bytewise with `address ‖ stamp hash` (any length, usually a 64-byte ID) and
  may lie outside the range. So T1.9's `Local.IterateIDs(r, after, fn)` is
  `ReserveIterateAddr(r.Prefix, r.Bits, after[:] or nil, …)` with
  `fingerprint.NewID(c.Address, c.StampHash)`, and `HasID(id)` is
  `ReserveHasID(id.Address(), id.StampHash())`; T1.6's `IndexerStore` matches the signature as
  committed. One call is one leveldb iterator (T1.0), opened without any reserve lock (T1.6 →
  T1.5); paging with `after` opens a new snapshot (documented). `cb`'s error is returned wrapped
  (`errors.Is` matches it, which T1.6's `Close` relies on). Each `ChunkID` passed to `cb` is fresh
  and may be retained: address and stamp hash come from the **last 64 bytes** of the result ID,
  with a length check (the iterated item has a zero `Address` (T1.4); `leveldbstore` returns
  `"/" + id`, 65 bytes, under `PrefixAtStart`, `inmemstore` the bare 64), the batch ID from the
  value. *(Corrected after the contract review, H1.)*
- **T1.5 (contract): `ReserveIterateAddr` is the one exception to `ReserveObserver` contract
  point 4** (orchestrator note from the T1.6 contract review, binding). T1.6's rebuild holds the
  Index lock in write mode from opening the snapshot until the first callback (or the return on an
  empty range), while reserve writers hold `r.multx` and wait for that lock in read mode. The
  godoc therefore requires, for this and any later change: (1) the leveldb iterator is opened
  before any blocking operation (no reserve lock, channel or wait); (2) it seeks straight to the
  masked start of the range (or to `after` inside it), so the first callback or the empty return
  comes after O(1) work, with no scanning or filtering of keys outside the range (no other
  namespace may start with `chunkAddr`, see the marker rename below);
  (3) a prefix of `ceil(bits/8)` bytes, empty at `bits == 0`, is accepted; (4) one `Iterate`
  call is one snapshot. `*storer.DB` satisfies T1.6's `IndexerStore{ReserveIterateAddr,
  ReserveAddrIndexReady, StorageRadius}` (checked with a scratch build against
  `bee-wt/T1.6/pkg/rangesync/indexer.go`'s method set; no compile-time check in `pkg/storer`, which
  cannot import `pkg/rangesync`).
- **T1.5 (contract): `ReserveGetByID`** reads the batch ID from the `ChunkAddrItem`, then calls
  `Reserve.Get` (per-batch lock). It returns a wrapped `storage.ErrNotFound` when the element is
  absent or was removed between the two steps.
- **T1.5 (contract): leftovers are dropped whenever the index is disabled (beyond §5.1's table).**
  §5.1 says "disabled, no marker: nothing", so an interrupted background drop (D10) would leave its
  keys on disk until the index is enabled again. Instead the disabled start-up path checks for any
  `chunkAddr` key (one iterator seek, no write) and drops leftovers in the background whether or
  not a marker was present. On the default path that is one read at start-up and nothing else.
- **T1.5 (contract): mock.** `mockstorer.ReserveStore` implements `storer.ReserveAddrStore`
  (compile-time check; new file `pkg/storer/mock/mockreserveaddr.go`), and without the new option
  it behaves as before. Exported: `WithAddrIndex()` (enabled and ready), `SetAddrIndexReady(bool)`
  (Busy tests), the five interface methods, `RemoveID(addr, stampHash) error` (eviction of one
  element) and `ResetReserve(ctx) error`. `ReservePutter` puts, `RemoveID` and `ResetReserve` go
  through a registered observer's `Commit` (exact delta, one call each, no mock lock held), and
  `SetStorageRadius` calls `RadiusChanged`. `ReserveIterateAddr` iterates a copy and holds no lock
  during `cb`. Documented as not modeling leveldb snapshots, eviction locking or the registration
  guarantee (T1.0 follow-up).
- **T1.5 (contract): fill variant.** Pending G4. The contract is the same for both variants: `New`
  either completes the fill before it returns or leaves it to a background goroutine, and
  `ReserveAddrIndexReady` covers both (D16).
- **T1.5 contract review (2026-10-15), H1: marker namespace renamed (amends T1.4).**
  `ChunkAddrValidItem.Namespace()` is now `"addrIndexValid"` (was `"chunkAddrValid"`;
  `pkg/storer/internal/reserve/items.go`, unreleased). `leveldbstore.Iterate` with
  `PrefixAtStart` opens its iterator over `util.BytesPrefix("chunkAddr")`, with no separator, so
  the old marker key `chunkAddrValid/` was inside every seeking iteration of the index and failed
  to unmarshal ("invalid size") whenever one ran to the top of the address space: `bits == 0`
  (a rebuild at radius 0), any range reaching `ff…`, an empty top range, an `after` past the last
  element. Rule, stated on the item: no namespace may start with `"chunkAddr"`. Planned tests:
  `TestIterateAddr` covers `bits == 0` and the top range on **both** `leveldbstore` (via
  `transaction.NewStorage`) and `inmemstore`, with the marker present; plus a check that no
  registered storer namespace has `"chunkAddr"` as a byte prefix.
- **T1.5 contract review, H2: `ResetReserve` vs a background fill, option (b).** `node.go` calls
  `ResetReserve` right after `storer.New`, and `Reserve.Reset` takes no batch lock, so a background
  fill could write the entry of a chunk the reset just removed, behind a valid marker. Chosen:
  `ResetReserve` cancels a running fill and waits for it, resets the reserve, then writes the
  marker (an emptied index is complete), so it is correct in any call order. Rejected (a), starting
  the background fill from `StartReserveWorker`: it depends on the order of calls in `node.go` and
  leaves any later `ResetReserve` caller exposed. Documented on `ResetReserve`. Planned test
  (background variant only): a `ResetReserve` during a slowed fill leaves an empty index, a valid
  marker and `ReserveAddrIndexReady() == true`. The blocking variant has no fill running after
  `New`.
- **T1.5 contract review, M1: the marker carries a digest of the reserve state.** The marker
  value becomes `version ‖ epoch ‖ last bin ID of each bin ‖ reserve size`. It is written when a
  fill completes and rewritten at a clean `Close` after every background goroutine stopped; an
  enabled start trusts the index only if the version and the digest match the reserve it opened.
  Bin IDs only grow and every put takes a new one, a removal without a put lowers the size, and a
  reset replaces the epoch, so any change by something that does not maintain the index (an older
  build after a downgrade, a disabled or keep-mode start) makes the digest differ and forces a
  refill. **Deviation from the review's example:** an "in use" bit set at start and cleared at a
  clean `Close` alone does not catch a downgrade, because the older build runs after the newer
  one's clean `Close`, never touches the marker, and leaves it "clean". The digest catches it. A
  crash is not a correctness problem (the index is written in the reserve's own transactions), but
  after one the digest normally differs, so a crash costs a refill. The ChunkAddrValidItem layout
  change lands with the implementation. The refill cost (drop + fill) goes into the G4 numbers,
  so the user can weigh it and relax the rule to "start once disabled after a downgrade" instead.
  Planned tests: a clean close and reopen does not refill; a reserve changed with the index off
  (put; evict only; reset) refills; a simulated unclean shutdown refills; a marker of another
  version refills.
- **T1.5 contract review, M2: keep mode for tooling.** `Options.ReserveAddrIndexKeep bool`: with
  `ReserveAddrIndex` false it leaves the marker and the entries alone, so `bee db info` and the
  other read-only `bee db` commands no longer force a refill. Commands that change the reserve
  (import) should open with the index disabled; a change in keep mode anyway is caught by the
  digest (M1). Ignored when `ReserveAddrIndex` is true. Plumbing the call sites in
  `cmd/bee/cmd/db.go` is T1.12's (follow-up below). Planned test: open in keep mode, close, reopen
  enabled → no refill; keep mode + a put, reopen enabled → refill.
- **T1.5 contract review, M3: no reserve means disabled.** With `ReserveCapacity` 0 the start-up
  toggle runs as disabled (the marker is deleted, the entries dropped in the background), instead of
  ignoring the option. Recovery (`DeleteCorruptedChunkMetadata`) is passed
  `opts.ReserveAddrIndex || markerPresent`, stated in the `Options` godoc. Planned tests: an enabled
  storer reopened with `ReserveCapacity` 0 loses its marker and entries; recovery with the index
  disabled but a marker present deletes the entries of corrupted chunks.
- **T1.5 contract review, M4: the drop is synchronous in both fill variants.** On an enabled start
  without a valid marker, `New` drops the `chunkAddr` namespace before index writes are switched
  on (otherwise a background drop would delete entries the reserve just wrote); only the fill may
  run in the background. The disabled path keeps its background drop (D10): no index writes happen
  there. The G4 benchmark reports drop + fill.
- **T1.5 contract review, L1–L3.** (L1) The `ReserveAddrIndexReady` godoc now states that the flag is
  set by an atomic store only after every fill transaction has committed and the marker write
  returned, so "true" means visible to any iterator opened afterwards, and that a background filler
  holds the per-batch lock across the read, the re-check, the write and the commit. (L2) Mock:
  `SetAddrIndexReady` is documented as a test switch (the storer only goes false → true);
  `ReserveGet` now takes `s.mtx` (an existing data race with `put`; fixed in this commit since
  `ReserveGetByID` builds on it); a re-put of an existing id gives an empty delta;
  `IsWithinStorageRadius` is always true, so T1.10's out-of-radius tests cannot use the mock;
  `WithEvilChunk` affects `ReserveGetByID` only for an address in the set. (L3) `Close` cancels and
  waits for the background fill/drop goroutines before closing leveldb (in the `Options`
  godoc). A large drop leaves tombstones that the next fill's iterations and `ReserveIterateAddr`
  seeks skip until compaction; the implementation compacts the `chunkAddr` key range after a drop
  if the G4 benchmark shows that seeks right after a drop are slow.

- **T1.6 (contract): files and API.** `fingerprint/index.go` + `fingerprint/metrics.go` and
  `rangesync/indexer.go`. `fingerprint`: `NewIndex(IndexOptions{LeafBits, RootChangeDebounce})`
  (`LeafBits` 0 = `fingerprint.LeafBits`, other values ≤ 16 for tests; panics above),
  `Index.{LeafBits, Commit, Apply, Summary, RootSummary, Rebuild, Reset, SubscribeRootChange,
  Close, Metrics}`, sentinels `ErrBusy`, `ErrBelowLeaf`, `ErrOutsideRoot`. `Rebuild(root,
  iterate func(fn func(ID) error) error) error` and `Summary(r) (Summary, error)` are §5.2's
  signatures, so `*Index` satisfies T1.8's `Summarizer`. Beyond the task text, each needed by a
  consumer: `Commit(run func() (adds, removes []ID, err error)) error` (the adapter in
  `pkg/rangesync` must hold `ix.mu` across `run`; exposing the lock instead would leak it),
  `RootSummary()` (root and its summary under one lock for `Status`; `Summary(root)` after a
  separate root read races a swap), `LeafBits()` (`Status.LeafBits`), `ErrOutsideRoot` (a range of
  a session scoped on a root the index has moved away from, e.g. after a radius increase; distinct
  from `ErrBelowLeaf`, which means "scan"), `Close()` (stops the debounce timer), `Metrics()`.
  `SubscribeRootChange() (c <-chan struct{}, unsubscribe func())` follows kademlia's
  `SubscribeTopologyChange`: buffer of one, coalescing, trailing debounce, `unsubscribe` closes.
- **T1.6 (contract): group adapters are unexported.** `countNode uint32` (mod 2^32) and
  `ltHashNode LtHash` (`Add`/`Sub` → `AddHash`/`SubHash`, `Zero`), per the T1.2 follow-up. The
  Index is their only user and T2.2 adds the symbol adapter in the same package; `Fenwick` was
  exported only because it had no production caller yet. The contract commit leaves them (and the
  unexported Index state) out, since `unused` rejects them without callers; their form and the
  memory (16 KB + 8 MB at B = 4096) are in the `Index` godoc.
- **T1.6 (contract): locking.** `mu` (RW) → `slabMu` (RW: `Commit`/`Apply` write, `Summary`
  read) or `nextMu` (the slab being built), never `slabMu` and `nextMu` together; the subscription
  mutex last. Deltas arriving during a rebuild go straight into the new slab's plain bucket sums
  under `nextMu` (bounded memory) rather than into a queue drained at the swap; the rebuild's own
  additions take `nextMu` without `mu`. The T1.2 follow-up "expand outside `ix.mu`" cannot hold for
  `Commit`: the delta is known only after `run`, and must be applied before the read lock is
  released (step 4). The expansion runs under `mu` (R) but outside `slabMu`/`nextMu`, so only the
  2 KB additions are serialized.
- **T1.6 (contract): `Summary` while rebuilding.** The live slab keeps answering for its own root.
  Order: `r` inside the live root → `ErrBelowLeaf` if deeper than leaf depth, else the answer;
  otherwise `ErrBusy` if a running rebuild's root contains `r` or there is no live slab; otherwise
  `ErrOutsideRoot`. `Rebuild` returns `ErrBusy` when another runs, an error for
  `root.Bits + LeafBits > MaxBits`, and on an `iterate` error discards the new slab and keeps the
  live one (with none, the index returns to dropping deltas).
- **T1.6 (contract): `Reset` discards and rebuilds instead of zeroing.** `Reserve.Reset` holds no
  `r.multx` across its per-item transactions, so a `Put` can commit between the last per-item
  commit and `observer.Reset()`; zeroing the slab would lose it forever. `Index.Reset` drops the
  live slab (`Summary` → `ErrBusy`; deltas dropped unless a rebuild runs, whose snapshot plus
  delta stream stays exact) and the Indexer forces a rebuild (clears the last completed root and
  queues a wake-up). In production `observer.Reset` is practically unreachable: `node.go` calls
  `ResetReserve` right after `storer.New`, before `rangesync` exists.
- **T1.6 (contract): Indexer and its local interface.** `IndexerStore{ReserveIterateAddr,
  ReserveAddrIndexReady, StorageRadius}` with §5.4's signatures and `storer.ChunkID`;
  `NewIndexer(index, store, overlay, logger, IndexerOptions{ReadyPollInterval,
  RebuildRetryInterval})`, `Start()`, `Close()`, and the three `storer.ReserveObserver` methods
  (compile-time check). `SetReserveObserver` is **not** in the interface: its failure mode is still
  open (T1.4 → T1.5 round 2 follow-up), so the caller (T1.9) registers the Indexer and then calls
  `Start`, which queues the initial rebuild; the goroutine waits for `ReserveAddrIndexReady()` by
  polling (§5.4 has no notification), then reads `StorageRadius()` and rebuilds at
  `NewRange(overlay, radius)` with one `ReserveIterateAddr(root.Prefix, root.Bits, nil, …)` call.
  Beyond the task text: a failed rebuild is retried after `RebuildRetryInterval` (default 10 s);
  otherwise a failure with no later `RadiusChanged` leaves `rangesync` Busy forever. `Close`
  cancels a running rebuild through the iterate callback's error.
- **T1.6 (contract): test store.** The T1.0 follow-up asks for `transaction.NewStorage` over
  `leveldbstore` in snapshot tests, but `pkg/storer/internal/transaction` cannot be imported from
  `pkg/rangesync`. The local fake reserve keeps its address index in `pkg/storage/leveldbstore`
  directly (one leveldb batch per set-changing commit, wrapped in `Commit`;
  `ReserveIterateAddr` = one `Iterate` call), which has the same snapshot property the T1.0 spike
  established for the real stack; not `inmemstore`.
- **T1.6 contract review (M1): step-5 conversion window.** The new slab is converted with
  `NewFenwickFromBuckets` under `nextMu`, and a `converted` flag is set in the same critical
  section; a commit's delta to the new slab is a plain bucket addition before the flag and a
  `Fenwick.Add`/`Sub`/`Update` after it. Without the flag, a commit between the conversion and
  the swap would add a plain bucket delta into a Fenwick tree (permanent corruption). The swap
  under `mu` (write) is then O(1). Test: a commit between the end of the iteration and the swap
  is in the final index.
- **T1.6 contract review (M2; supersedes "`Reset` discards and rebuilds instead of zeroing" and
  the `Index.Reset` part of "files and API"):** `Reserve.Reset` wraps every per-item delete in
  `Commit` with an exact delta, so the index is exact after `Reset` and already signaled the root
  changes. `Index.Reset` is **removed** (it would only re-signal); `Indexer.Reset` is a no-op that
  queues and clears nothing. This also removes the race on the last completed root and the
  "does not block" inaccuracy.
- **T1.6 contract review (nits):** the new slab is allocated before `mu` is taken in write mode;
  each delta element is expanded once and the same `LtHash` is added into both slabs during a
  rebuild; `mu` (write) is released on every exit from `iterate` (first callback, return, error
  before the first callback, panic); `Apply` is for tests (no production caller); a failed
  rebuild logs a Warning first, then Debug until a rebuild completes, and a `RadiusChanged`
  during the retry wait wakes the goroutine at once; `Start` only after `SetReserveObserver`
  returned nil (T1.4 landed `reserve.SetObserver`, which T1.5 wraps, with an error return: `ErrAddrIndexDisabled`, `ErrNilObserver`,
  `ErrObserverRegistered`).
- **T1.6 contract review (L4): the fake reserve.** On-disk `leveldbstore` with a small
  `WriteBuffer` (as `TestIndexStoreIterateSnapshot`), so memtables flush during the iteration;
  exact deltas computed under a fake per-batch lock held across `Commit` (the role of
  `r.multx`), which `ReserveIterateAddr` never takes. Regression test: a rebuild that opens while
  a `Commit` holds that lock (and waits for `mu`) completes.

## Follow-ups
_(sessions append here)_
- **T1.1:** the `Want` limit (≤ 250 IDs, §6.2) and the per-peer delivery limit are not in
  `constants.go` yet; T1.10 adds them.
- **T1.1 → T1.6:** `Summary.Digest` must be special-cased to the zero value when `Count == 0`:
  keccak256 of the all-zero LtHash lanes is **not** zero, so `Digest()` of an empty bucket run
  cannot be used as is.
- **T1.1 → T1.6/T2.2:** map a range to Fenwick bucket indices with `root.BucketSpan(r, leafBits)`,
  not with `ChildIndex(r.Prefix, …)`: a non-canonical prefix carries bits past `r.Bits` into the
  index (root `ff…/4`, `r = {ff ff, 8}` gives 4095 instead of 3840 → out-of-range Fenwick index).
  Treat `ok == false` as an invalid range, not a panic.
- **T1.1 → T1.8:** peer `pb.Range`s must be strictly decoded and canonicalized before use (the
  strict `ParseRange` stays with T1.8, rules pending G2). Use `R.Bits >= leafDepth` (not `==`) for
  the items branch, and reject a session whose root is deeper than its leaf depth; otherwise
  `Child`/`Children` at depth > `fingerprint.MaxBits` (a panic) becomes reachable from peer input.
- **T1.0 → T1.4/T1.5/T1.6 tests:** `internal.NewInmemStorage` (used by the reserve unit tests)
  has neither property: `inmemstore.Iterate` holds the store lock for the whole iteration (all
  commits stall instead of seeing a snapshot) and its transactions write through immediately (not
  atomic, no rollback). Tests of rebuild or snapshot behaviour must use `transaction.NewStorage`
  over `leveldbstore` (see `newReserveDiskStorage` in
  `pkg/storer/internal/reserve/snapshot_test.go`); the T1.5 `pkg/storer/mock` must not be used to
  argue snapshot semantics.
- **T1.0 → T1.5:** document on `ReserveIterateAddr` that one call is one leveldb snapshot and
  that the `after` cursor starts a new one; implement it as a single `Iterate` call.
- **T1.0 → T1.6:** a rebuild keeps one leveldb iterator open for its whole duration (10–20 s
  at 4.2M elements), which pins that version's memtables and table files; compaction cannot
  delete obsolete files until it is released. Expected to be harmless; worth checking
  `alive_iterators` and disk usage in the T1.5/T1.6 measurements.
- **T1.0 (tooling):** `-race` could not run on the dev host (no C compiler, so no cgo).
  `TestIndexStoreIterateSnapshot` and `TestReserveRebuildSnapshot` should run under
  `make test-race` in CI or on a host with gcc. Also, `make lint` with the installed
  golangci-lint v2.11.3 (built with go1.26.1) fails to typecheck under go1.27 ("export data
  version 4"); `GOTOOLCHAIN=go1.26.1 make lint` works (matches `go 1.26` in `go.mod`).
  `make format` rewrites ~60 unrelated files with the installed gci; revert them and check only
  the touched files (`gofumpt -l`, `gci diff`).
- **(T6.0) Pin the metric definitions that the acceptance criteria depend on (T1.13).** Export
  the §10 metrics with namespace `bee` and subsystem `rangesync` (e.g.
  `bee_rangesync_bytes_metadata`), following `pkg/puller` / `pkg/pullsync`. `bytes_metadata{dir}`
  counts every `rangesync` byte except chunk payloads: `status`, `reconcile`, and `fetch` `Want`
  messages. `bytes_chunks{dir}` counts `Delivery` messages. `session_rounds` counts round trips
  inside one session (not neighborhood rounds). `index_memory_bytes` counts both slabs while a
  rebuild runs. Needed by AC-B1–B4, AC-V2, AC-R1/AC-R2 in `acceptance-criteria.md`.
- **(T6.0) Add a counter of chunks fetched by `rangesync` (T1.10 emits, T1.13 registers),
  proposed name `chunks_fetched_total`** (optionally `{result}`: stored, duplicate, rejected). §10
  has no chunk count. Without it, `cross_protocol_duplicates_total` and `bytes_chunks` can't be
  turned into ratios (AC-B3, AC-D2, AC-D3).
- **T1.7 → T1.10/T3.2/T4.4:** claim right before each `Want`, only the IDs that `Want` carries
  (≤ 250), never a whole decoded set or stripe ahead of time: claims held for IDs not yet
  requested count against both caps and expire unused. On `Result`: `Held` → skip the ID;
  `PeerFull` → leave it for another peer that has it; `Full` → stop claiming for now (no other
  peer helps); `Closed` → shutting down. End every `Granted` claim on every path:
  `Done(id)` after `Put` succeeds or returns `ErrOverwriteNewerChunk`, `Release(id, peer)` with
  the same peer otherwise. A `Release` after the claim expired is harmless (it only ends the
  claim if that peer still holds it), but bounding each `Fetch` by the claim deadline still
  avoids a duplicate request.
- **T1.7 → T4.4:** expiry is passive (no notification, no goroutine). Claim-and-stall must be
  detected by the fetch that times out (it knows the peer) and reported to `PeerScorer`; or T4.4
  adds an expiry hook. The rejected cache goes into `Set` as a second bounded structure; `New`
  then needs `RejectTTL` too (e.g. `New(claimDeadline, rejectTTL)`; the only call site will be
  the one T1.10 adds).
- **T1.7 → proposal §5.3:** update the memory table's "In-flight want set (≤ 10k IDs) ≈ 1.2 MB"
  row to the measured ≈ 2.7 MB (and the totals), or note it; editing the proposal is out of
  scope for T1.7.
- **T1.7 (tooling):** `-race` could not run on the dev host. `TestConcurrentClaims`,
  `TestConcurrentPeerCap`, `TestConcurrentChurn` and `TestConcurrentExpiry` in
  `pkg/rangesync/wantset` should run under `make test-race` in CI; here they ran with
  `-count=50 -cpu=1,2,4`.
- **T1.3 → T1.2/T1.6 (`LtHash` element cost):** the Fenwick target of a few µs per range needs a
  fast `AddHash`/`SubHash`. With a naive per-`uint16` lane loop as the 2 KB element, the same
  worst-case `Range` measured ~22–95 µs on this host (vs ~6 µs with four lanes per `uint64`,
  SWAR); the tree itself is ~2% of the profile. Generic method calls go through the instantiation
  dictionary and are not inlined, which cost ~1.4× against a hand-written loop with the naive
  element. Worth a `BenchmarkLtHashAddHash` in T1.2 and an `LtHash` adapter benchmark in T1.6.
- **T1.3 → T1.6 (`Rebuild`):** build the new slabs as plain bucket sums and convert them with
  `NewFenwickFromBuckets` (O(B)), not with one `Add` per element. Deltas that arrive during the
  rebuild (§5.2 step 3) can go to the plain buckets before the conversion.
- **T1.3 → T2.2 (bucket symbols):** apply an element's symbols with `Fenwick.Update(bucket, fn)`
  and an `Encode` restricted to the bucket's 32 symbols inside `fn`, rather than building a full
  `[BucketSymbols]CodedSymbol` delta and calling `Add`.
- **T1.3 (cache layout, measure in T1.6/T2.6):** the nodes of one query are 2 KB × power-of-two
  apart, so they map to the same cache sets. Not visible at the measured numbers; check again with
  the real `LtHash` and the 2.8 KB symbol arrays if `Summary` is slower than expected.
- **T1.3 → T1.6 (review round 1, concurrency):** add a stress test with concurrent `Commit`s
  (several writers applying deltas while `Summary` readers run, and a rebuild in between) and
  flag it for `-race` in CI (no C compiler on the dev host). The same exclusivity applies to the
  `Rebuild` follow-up above: deltas applied to the plain buckets during a rebuild race with the
  rebuild's own iterator writes to those buckets, so they need the same inner mutex (or a queue
  drained by the rebuild goroutine).
- **T1.3 → T1.6 (review round 1, bounds):** always check `BucketSpan`'s `ok`. Ignoring it
  yields `(0, 0)`, so `Range(0, 0)` returns a silently empty `Summary`. Assert
  `Len() == 1<<leafBits` for every slab when the Index is constructed or rebuilt.
- **T1.4 → T1.6 (`RadiusChanged` dedupe):** never drop a `RadiusChanged` call by comparing its
  argument with the current root; always queue (coalesced). The rebuild goroutine re-reads
  `StorageRadius()` when it starts, without holding `ix.mu`, and skips only if that equals the
  root of the last rebuild it started. Test: radius 5 → 6 → 5 with the 6-rebuild in flight ends
  with the index at 5; out-of-order `RadiusChanged(5)`/`(6)` with `Radius() == 6` ends at 6.
- **T1.4 → T1.6 (dropping deltas):** replace "drops deltas while `ReserveAddrIndexReady()` is
  false" with "drops deltas while no index has been built yet", decided from the observer's own
  state under the `ix.mu` read lock held across `run`; `Commit` must not call the storer
  (contract point 3). Waiting for `ReserveAddrIndexReady()` stays in the rebuild goroutine.
- **T1.4 → T1.5 (option plumbing):** pass `ReserveAddrIndex` into recovery (`sharkyRecovery` →
  `reserve.DeleteCorruptedChunkMetadata`, `pkg/storer/recover.go:106`, which must delete the
  `ChunkAddrItem` when the marker is present) and into `reserve.New`; the migrations
  (`removeRogueChunks.go:78`, `reserveRepair.go:183,191`) pass the option explicitly to the
  package-level removal functions. Registration (`SetReserveObserver`) is once-only and rejects
  nil; it needs an error return or a documented panic, since §5.4 gives it none.
- **T1.4 → T1.6 (round 2; supersedes the two T1.4 → T1.6 follow-ups above where they differ):**
  the drop state ends at rebuild step 1, switched to queuing in the same `ix.mu` write-locked
  section that opens the first snapshot iterator (not at the swap). Rebuild goroutine: take the
  wake-up token before reading `StorageRadius()`; skip only if it equals the root of the last
  **completed** rebuild (cleared on failure, initially none). Tests: a `Put` and an eviction
  committing after the first rebuild's iterator opened (and before the swap) appear in the
  final index; a failed rebuild followed by a wake-up with the same radius rebuilds.
- **T1.4 → T1.5 (round 2):** settle `SetReserveObserver`'s failure mode (error return vs
  documented panic for a second registration, nil, or a disabled index) **before** T1.6/T1.9
  declare their local interfaces over it. T1.5's "calls return an error when the address index
  is disabled" conflicts with §5.4's signatures: `SetReserveObserver(o)` and
  `ReserveAddrIndexReady() bool` have no error return.
- **T1.8 → T1.9/T1.13:** an `Items` stream without `Reply` can list any number of (fake,
  ascending) IDs, and each becomes a want; the session does not cap wants. Bound them in T1.9 (a
  per-session want cap, or against the peer's count for the range when it sent one) or rely on the
  want-set bound, fetch validation and the `PeerScorer`; add an adversarial test in T1.13.
- **T1.8 → T2.4:** the "fits in symbols" decision goes between `DecisionStripe` and
  `DecisionExchangeItems`; `Config` is the place for the extra inputs (both `Status`es for
  `cap(R)`, the hint LRU, the symbol source); `Scope.CheckMessage` stops rejecting `Symbols`/`More`.
  The deferral rule above also applies to symbol batches.
- **T1.8 → T3.1:** the hook is `Striper.Stripe(r, local, remote uint64) bool`, offered by
  initiators only for ranges above the session leaf depth; returning `false` makes the session
  split.
- **T1.8 → proposal (§6.4, §9):** update the text to the user's 2026-10-14 decision: "a
  `Fingerprint` child set answers, at most once and possibly in a later message, a range the
  receiver sent and has not had answered; it is exactly the `2^FanoutBits` children; ranges are
  split only while `R.Bits + FanoutBits ≤ LeafDepth`, then items are exchanged". Also §6.3's
  flowchart (leaf-depth items branch) and the "+4 bits" slack, which is now only for opening
  ranges.
- **T1.8 → T1.9:** add a session-level cap (a deadline, a message count or a total of IDs
  received): a malicious peer can keep a session open indefinitely with `Items` streams of fake,
  ascending IDs, since each page is valid on its own. Score the peer when the cap is hit with
  streams still open.
- **T1.2 → T1.6:** the `Group` adapter is a defined type over `LtHash` (e.g. `type ltHashNode
  LtHash` with `Add(o) → (*LtHash)(h).AddHash((*LtHash)(o))`, `Sub` → `SubHash`, `Zero` →
  `*h = ltHashNode{}`), converted with pointer conversions. Build the rebuild slab with plain
  per-bucket `Add(id)` and then `NewFenwickFromBuckets` (O(B) group ops), not B·log B Fenwick adds.
  Expansion (~4.7 µs) dominates `Apply`; if a commit touches many ids, expand outside `ix.mu`
  (`Expand(nil, id)`) and only `AddHash` under the lock.
- **T1.2 (pre-existing, out of scope):** `BenchmarkReservePutter` (`pkg/storer/reserve_test.go`)
  fails immediately ("chunk type unspecified: storage: invalid chunk"): it feeds
  `storagetest.BenchmarkChunkStoreWriteSequential` chunks that `ReservePutter` rejects. T1.2 added
  `BenchmarkReservePut` next to it instead of changing it.
- **T1.2 (perf note for T1.8/T1.9):** `Digest` costs ~7 µs (pure-Go legacy Keccak over 2 KB), about
  as much as a 24-node range query; a split answering 16 child fingerprints spends ~0.2 ms in
  summaries + digests. Fine for Phase 1; cache digests per range only if profiles say so.
- **T1.2 → proposal §7.9 (L1):** §7.9 and its sequence diagram say the audit expands
  `SHAKE128(anchor ‖ id)`; the implementation (`fingerprint.Expand(anchor, id)`) is
  `SHAKE128("rangesync/v1" ‖ anchor ‖ id)`. Amend §7.9 and the diagram to the domain-separated form
  (not edited by T1.2).
- **T1.2 → T5.1 (L2):** `Expand` with an empty salt (nil or non-nil) is the unsalted expansion, so an
  empty anchor would make the audit equal the plain fingerprint and silently defeat it. `audit.Compute`
  must reject `len(anchor) == 0` before computing (an error, the round's audit skipped with its
  metric). An all-zero 32-byte anchor is fine: it is still a distinct salt.
- **T1.4 → T1.5:** construct the reserve with `reserve.WithAddrIndex(opts.ReserveAddrIndex)`
  and pass the same value to `DeleteCorruptedChunkMetadata` (recovery) and
  `RemoveChunkWithItem` (migrations); `SetReserveObserver` can wrap `(*Reserve).SetObserver`
  and its three errors. Treat a `ChunkAddrValidItem` whose `Version` differs from
  `ChunkAddrIndexVersion` as no marker (refill).
- **T1.4 (CI, no cgo on the dev host):** run with `-race`: `TestAddrIndexRegistrationRace`,
  `TestAddrIndexRadiusRegistrationRace`, `TestAddrIndexSetObserver`, `TestAddrIndexEvict`,
  `TestAddrIndexReset`, `TestAddrIndexProperty` (`pkg/storer/internal/reserve`). Here they ran
  with `-count=10 -cpu=1,2,4` instead.
- **T1.4 review fixes (CI):** add `TestAddrIndexSetObserverWaits`, `TestAddrIndexSharedAddress`
  and `TestAddrIndexObserverContract` to the `-race` list above.
- **T1.6 → T1.5:** `*storer.DB` must satisfy `rangesync.IndexerStore` (§5.4's
  `ReserveIterateAddr` signature with `storer.ChunkID`). `ReserveIterateAddr` must open its
  snapshot without taking any reserve lock (`r.multx`, the observer registration lock): the
  rebuild holds `ix.mu` in write mode until the first callback, and a `Commit` holding `r.multx`
  waits for `ix.mu` in read mode, so a reserve lock there deadlocks.
- **T1.6 → T1.9:** wiring order `SetReserveObserver(indexer)` → `indexer.Start()`; shutdown
  `indexer.Close()` before `index.Close()`. `Local.Summary` maps `fingerprint.ErrBelowLeaf` to a
  `ReserveIterateAddr` scan, `ErrBusy` to `Busy`, and treats `ErrOutsideRoot` as the root having
  moved (end the session, not a peer violation). `Status` comes from `Index.RootSummary()` and
  `Index.LeafBits()`. Choose the production `RootChangeDebounce` (proposed:
  `StatusMinInterval`).
- **T1.6 → T1.13:** register `Index.Metrics()` (`index_rebuild_seconds`, `index_apply_seconds`,
  `index_memory_bytes`, `index_outside_root_total`).
- **T1.6 → T1.5 (contract review L1; supersedes the lock sentence of the T1.6 → T1.5 entry
  above):** `ReserveIterateAddr` opens its leveldb iterator before any blocking operation (no
  lock, channel or wait of any kind); seeks straight to the masked start of the range so the
  first callback (or the return on an empty range) comes after O(1) work; accepts `root.Prefix`
  of `ceil(bits/8)` bytes (empty at radius 0). T1.5 must document this on `ReserveIterateAddr`
  as the one exception to `ReserveObserver` contract point 4.
- **T1.6 → T1.9 (contract review L2, L3; supersede the `ErrBusy` mapping and the wiring order of
  the T1.6 → T1.9 entry above):** `ErrBusy` becomes `Busy` only when answering the session
  opening; a mid-session `ErrBusy` ends the session as an unscored `Local` error, like
  `ErrOutsideRoot`. Call `indexer.Start()` only after `SetReserveObserver` returned nil; on error
  do not `Start` (rangesync disabled, logged).
- **T1.6 → T1.13 (contract review L4):** rerun the fake-reserve scenario "a rebuild opening while
  a `Commit` holds the per-batch lock completes" against the real `*storer.DB` (`Put` holding
  `r.multx` while the rebuild opens its iterator).
- **T1.5 → T1.12 (keep mode):** pass `ReserveAddrIndexKeep: true` from the `bee db` commands that
  do not change the reserve (`info`, `compact`, the `validate*` commands, `export`); `import` and
  `repair-reserve` keep the default (`import` must invalidate the index; `repair-reserve` drops it
  itself). Without this, `bee db info` on a rangesync node deletes the marker and forces a full
  refill at the next start.
- **T1.5 → T1.8/T1.9 (N1):** `ReserveIterateAddr` returns `cb`'s error wrapped (via
  `leveldbstore`'s "iterate callback function errored: %w"): compare with `errors.Is`, never `==`,
  e.g. for a stop sentinel or `ErrInvalidMessage` raised inside `Local.IterateIDs`.
