# Phase 2 — Rateless IBLT symbol streams

**Goal:** resolve small and medium differences from a single coded-symbol stream per range
instead of descending the fingerprint tree. This makes a session cheap enough (≈ 1.35–1.72
symbols × 82 B per difference) to run steady-state rounds in Phase 4.

**Proposal sections:** §1.1, §4.1 (coded symbol stream, symbol caches), §4.3.4–§4.3.7,
§5.2 (symbol parts of `Index`), §5.3, §6.2 (`Symbols`, `More`), §6.3, §6.4, §7.2, §7.3,
§7.7, §7.10, §9 (checksum collisions), §10, §13 (rateless tests), §14 Q2–Q4.

**Depends on:** Phase 1. T2.0 has no dependencies and starts in wave A; T2.1 starts as soon as
T2.0 is recorded (wave B).
All code lives in `pkg/rangesync/...`; no storer change in this phase.

---

## T2.0 Spike: CertainSync vs Rateless IBLT (open question 4)

**Scope:** research only, no production code. Read CertainSync (arXiv:2504.08314) and
compare with Rateless IBLT (Yang, Gilad, Alizadeh, SIGCOMM 2024, arXiv:2402.02668) on:
symbols per difference, decode-failure behaviour, compatibility with the cached-prefix design
(root 4096 / level-1 512 / bucket 32 Fenwick), linearity (needed for Fenwick range sums),
per-update cost, implementation size. Output: a short write-up appended to this file's
Decisions log with a recommendation. **T2.1 must not freeze the wire format before this is
recorded.** If CertainSync wins, stop and update T2.1–T2.4 here before anyone continues.

**Size:** S (timeboxed). **Parallel:** wave A. **Blocks:** T2.1.

---

## T2.1 `pkg/rangesync/fingerprint/rateless.go`: mapping, checksum, encoder, peeling decoder, wire codec

**Depends on:** T2.0 (its recommendation must be in the Decisions log first).

**Scope:** a self-contained in-tree implementation (§5.4 — no module dependency):

1. **Pluggable mapping** (§4.3.4, §5.2): a `Mapper` interface —
   `ID() uint32`, `Indices(id) IndexIterator` (increasing indices from 0),
   `Expected(count, i) int64` (= `round(count·ρ(i))`, used by the wire codec). **Nothing else in
   the package may compute indices or ρ directly**; `Encode`, the `Decoder`, the Index caches
   (T2.2) and the codec all take a `Mapper`. The Index holds one `Mapper`, set at construction.
2. **`FloatMapper` (ID `fingerprint.DefaultMapperID = 1`, from T1.1):** seed = `keccak256("rangesync/map" ‖ id)`;
   a pinned PRNG (spell the algorithm out in code, e.g. a keccak- or splitmix-based stream; don't
   rely on `math/rand` stream stability); next index
   `next = i + ⌈(1.5 + i) · ((1 − u)^(−1/2) − 1)⌉` in `float64`. Float results are not guaranteed
   bit-identical across architectures; that's why the mapper is injectable and its ID is in
   `Status`. Avoid `math.FMA`-sensitive expressions where a plain form exists, and write the
   formula so the compiler can't fuse operations differently per arch (separate statements,
   explicit `float64(...)` conversions — note this in a comment).
3. **Checksum:** `keccak256("rangesync/check" ‖ id)[:16]` (128 bits, §9 — no keyed checksums).
4. `CodedSymbol{Sum ID, Check [16]byte, Count int64}`, `Encode(m Mapper, syms, start, id, sign)`.
5. **Decoder** (§4.3.7): incremental `Add(start, remote, local)`, `Done()` = symbol 0 empty,
   `Result() (onlyLocal, onlyRemote)`. Pure symbol = `count = ±1 && check == h(sum)`.
   Corruption checks (§6.4): recovered id outside range R (pass R in), duplicate recovery,
   more than `2 × received` peel steps → return `ErrCorrupt`.
6. **Wire codec** for `Symbols.Data` (§6.2): `n × (Sum 64 B ‖ Check 16 B ‖ varint zigzag(count − m.Expected(Count, i)))`.
   Decoder of this blob is untrusted input → fuzz target.

**Tests (§13):**
- Linearity: `encode(A) − encode(B) == encode(A △ B)` for random sets.
- Determinism vectors for `FloatMapper`: id → first 64 indices, `Expected(count, i)` for a grid
  of counts, and checksums, committed in `testdata/`. Run them on amd64 and arm64
  (`GOARCH=arm64` on an arm64 machine or CI runner, or under qemu) and record the result. We do
  **not** promise compatibility with the reference `riblt` library.
- A test-only second `Mapper` (e.g. a trivial deterministic one) proves the package works with
  any `Mapper`: linearity, decoding and codec round-trips pass with both.
- Decode cost: mean symbols/difference within [1.3, 1.8] for differences 1 … 10⁴; `Done()`
  exactly when complete.
- The §4.3.7 three-element worked example as a literal test.
- Corruption: a test build with a truncated checksum (e.g. a `checksumBytes` var overridden in
  `export_test.go`), colliding ids → `ErrCorrupt`, never a wrong "done".
- Fuzz the wire codec and `Decoder.Add`.

**Size:** L. **Parallel:** wave B (after T2.0), pure library.

---

## T2.2 Symbol caches in the Index (bucket Fenwick, level-1, root, scan path)

**Depends on:** T1.6, T2.1.

**Scope:** extend `fingerprint.Index` (§5.2):
`bucketSym fenwick[[BucketSymbols]CodedSymbol]` (32), `level1Sym [16][512]`, `rootSym [4096]`.
`Apply` updates all three (§4.3.6: ~12 Fenwick nodes × ~6 symbols, ~16 root, ~12 level-1).
`Rebuild` builds them (expect 10–20 s CPU at 4.2M; parallelise the build by bucket if needed).
`Symbols(r, start, end) (syms, exhausted, err)` with the source selection of §4.3.5:
own root & `end ≤ 4096` → root cache; own level-1 & `end ≤ 512` → level-1 cache;
bucket-aligned & `end ≤ 32` → Fenwick; `count(r) ≤ ScanLimit` → scan path (an iterate callback the caller supplies;
`fingerprint` stays storer-free); else `exhausted = true`.

Add the group adapter for symbol arrays so they fit T1.3's generic Fenwick tree. All cache
updates go through the Index's `Mapper` (T2.1). Caches are only valid for the node's **own** root and level-1 ranges; a narrower session root
(edge peer) must use Fenwick/scan.

**Tests:** extend T1.6's consistency fuzz: after random Put/replace/evict, all three caches
equal a rebuild and equal `Encode` over the brute-force set, for every range and every
`[start, end)` slice; rebuild during concurrent writes. Memory at B = 4096 ≈ 11.5 + 0.7 +
0.36 MB (§5.3).

**Size:** L.

---

## T2.3 Index `Symbols` API + scan path over `ReserveIterateAddr`

**Depends on:** T2.2, T1.5.

**Scope:** in `pkg/rangesync` (no storer change — the storer only provides iteration): wire the
scan-path callback of `Index.Symbols` to `storer.ReserveAddrStore.ReserveIterateAddr`, and
expose `Symbols(r, start, end)` to the session layer through the same local-summary interface
T1.8 uses.

**Tests:** via the storer mock: symbols for root / level-1 / bucket / small range equal a
brute-force encode.

**Size:** S.

---

## T2.4 Session: `Symbols` / `More`, capacity, estimate, exhaustion, corruption fallback

**Depends on:** T2.3, T1.8, T1.9.

**Scope:** fill the "fits in symbols" seam in `pkg/rangesync/session.go` (§6.3):

- `cap(R)` for **both** peers from their `Status` and `Fingerprint` counts: 4096 if R is
  both peers' root, 512 if a level-1 range of both, unlimited if both hold ≤ `ScanLimit`
  in R, else 32.
- Estimate `d̂ = max(|cL − cR|, hint(R), 1)`; `hint` = last decoded diff size for
  (peer, range), kept in a small bounded in-memory LRU on the `Service` (not persisted;
  losing it is fine). First batch `n0 = max(MinBatch, ⌈1.6·d̂⌉ + 2)`.
- `n0 ≤ cap(R)` → emit `Symbols{R, 0…n0}`; else split.
- Incoming `Symbols`: feed decoder; decoded → wants `onlyRemote`, emit `Items{R, onlyLocal}`;
  not decoded & not exhausted → `More{R, UpTo: min(4 × received, cap(R))}`; not decoded &
  exhausted → split R, discard symbols.
- Incoming `More`: continue from `sent`, set `Exhausted` at `cap(R)`.
- `ErrCorrupt` from the decoder → fall back to an exact `Items{R, Reply: true}` listing for R
  (§9), increment `decode_corruption_total`.
- Remove the Phase-1 rejection of `Symbols`/`More`.
- Sessions only run when both peers' `Status.Mapping` match (T1.9 already enforces it); assert it
  again before decoding, since a mismatch would silently fail to peel.

**Tests:** property test from T1.8 now hits the symbol path; stale-stamp divergence (§7.7):
same address, two stamps → both sides learn the other's id and fetch; `Exhausted` path splits;
corruption path falls back without losing elements.

**Size:** L.

---

## T2.5 Symbol-stream limits

**Depends on:** T2.4.

**Scope (§6.4, §9):** `More` valid only for a range already streaming in this session with
`UpTo ≤ cap(R)`; `SessionSymbolBudget = 16384` per session (past it → `Exhausted` for every
range); `MaxScansPerMessage = 64` ranges per message may use the scan path; per-peer session
rate limit. Violations reset the stream and penalise the peer (same mechanism as T1.9).

**Tests:** adversarial cases added to the T1.13 fuzzers: `More` past cap, past budget,
`More` for an unknown range, a message requesting > 64 scan-path ranges, endless `More`.

**Size:** S–M.

---

## T2.6 Benchmarks, worked-example tests, symbol metrics

**Depends on:** T2.4.

**Scope**

- Benchmarks (in `pkg/rangesync/fingerprint`): `BucketSymbols` 16 vs 32 (memory vs extra descent for bucket runs > ~11
  diffs, §5.3); `Index.Apply` at reserve put throughput; `Rebuild` at 4.2M elements.
  Record results + chosen values in the Decisions log (open question 2). Measure
  request-driven vs streamed batches (open question 3) on the worked examples.
- Literal tests for §7.10 examples 1–3 and §7.2/§7.3: assert message counts (3, 3, 5
  messages) and metadata bytes within ±20% of the stated numbers.
- Metrics: `symbols_per_difference`, `more_requests_total`,
  `streams_exhausted_total{level}`, `decode_corruption_total`.

**Size:** M.

---

## T2.7 (optional, before mainnet) Integer `Mapper`

**Depends on:** T2.1.

**Scope:** an `IntegerMapper` with a new ID (`2`) implementing the same distribution
(`ρ(i) = 1/(1 + i/2)`, gap formula of §4.3.4) in fixed-point / integer arithmetic only, so results
are identical on every architecture by construction. It is a drop-in replacement: no change outside
the new type and its ID constant. The mapper is chosen through a `Mapper` field in
`rangesync.Options` (passed to the Index at construction, default `FloatMapper`); tests use that
field. **No node flag** is added: changing the default mapper is a network-wide switch (peers with
different IDs don't sync), so exposing it is part of a later rollout plan, not this task.

**Tests:** the same linearity / decode-cost / codec suite as T2.1, run with `IntegerMapper`; the
symbols-per-difference distribution matches `FloatMapper` within noise; vectors in `testdata/`.

**Size:** M.

---

## Decisions log
- **Mapper (pre-implementation):** start with `FloatMapper` (ID 1) behind an injectable `Mapper`
  interface; peers exchange the mapper ID in `Status` and only sync with equal IDs; an integer
  mapper (T2.7) can replace it later.

_(sessions append here — T2.0 recommendation; T2.1 cross-arch vector result; T2.6 cache sizes)_

## Follow-ups
_(sessions append here)_
