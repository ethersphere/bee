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
  - `Commit` takes `mu.RLock()` around `run` and applies the returned delta — but **drops deltas
    while `ReserveAddrIndexReady()` is false** (the first rebuild covers them);
  - `RadiusChanged` **only queues an asynchronous rebuild and returns** (no-op if the radius
    equals the current root's). It's called from the reserve worker and from `unreserve`; a
    synchronous rebuild would stall eviction or deadlock against writers waiting for `ix.mu`. A
    single rebuild goroutine coalesces queued requests;
  - `Reset` clears.
  - **Start-up order:** `SetReserveObserver` first, then wait for `ReserveAddrIndexReady()`, then
    the initial rebuild from the persisted radius (`StorageRadius()`), so no commit is missed. The
    start-up `SetRadius` in `startReserveWorkers` then only triggers a second rebuild if the radius
    actually changed.
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
immediately while the rebuild runs; deltas before the address index is ready are dropped and the
first rebuild still equals brute force; a chunk evicted before the fill reached it produces no
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
