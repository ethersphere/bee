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
  of counts, and checksums, committed in `testdata/` and checked on amd64. Running them on other
  architectures is deferred to T2.8 (D22). We do **not** promise compatibility with the reference
  `riblt` library.
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

**Tests:** a seeded randomized consistency test (the shape of T1.6's `FuzzIndex`, but a plain
`Test` with fixed seeds; extending `FuzzIndex` itself is deferred to TF): after random
Put/replace/evict, all three caches
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

**Tests:** adversarial table tests: `More` past cap, past budget, `More` for an unknown range,
a message requesting > 64 scan-path ranges, endless `More`. Adding them as fuzz seeds is
deferred to TF ([followup-fuzzing.md](followup-fuzzing.md)).

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

**Tests:** the same linearity / decode-cost / codec suite as T2.1 (its fuzz targets excepted; TF),
run with `IntegerMapper`; the
symbols-per-difference distribution matches `FloatMapper` within noise; vectors in `testdata/`.

**Size:** M.

---

## T2.8 (deferred, before mainnet) Cross-architecture determinism vectors

**Depends on:** T2.1 (and T2.7 if it was done).

**Why deferred (D22):** no arm64 environment is set up yet. Until this task runs, testnet cohorts
use one architecture (amd64), so a `FloatMapper` difference between architectures can't show up as
failed decodes during acceptance testing.

**Scope:** run T2.1's `testdata/` vectors (and T2.7's, if present) on arm64 (an arm64 machine or
CI runner, or `GOARCH=arm64` under qemu-user). Record the result in the Decisions log. If
`FloatMapper` vectors differ, T2.7 becomes mandatory before any mixed-architecture or mainnet
deployment, and `IntegerMapper` should become the default before rollout.

**Size:** S (plus environment setup).

---

## Decisions log
- **Mapper (pre-implementation):** start with `FloatMapper` (ID 1) behind an injectable `Mapper`
  interface; peers exchange the mapper ID in `Status` and only sync with equal IDs; an integer
  mapper (T2.7) can replace it later.
- **T2.0 recommendation (gate G3): Rateless IBLT, not CertainSync.** Both papers read in full
  from arxiv.org: Yang/Gilad/Alizadeh arXiv:2402.02668v3 ("RIBLT") and Keniagin/Yaakobi/
  Rottenstreich arXiv:2504.08314v1 ("CS"). *[p]* = the paper says so, *[ours]* = our computation
  or inference. Our elements are 64-byte `id`s, so the universe is n = 2⁵¹².

  | | Rateless IBLT | CertainSync, EGH construction (the only one CS §6.2.3 keeps for large n) |
  |---|---|---|
  | Symbols per difference | 1.72 at d = 4, → 1.35 for d > 128 (§5.1, Fig. 5, Cor. 5.2), independent of n *[p]* | Guarantee needs m_d = Σ of the first k primes with Π_k ≥ nᵈ (Def. 4.2, Thm 4.3), O(d² log²n / (log d + log log n)) (Table 1) *[p]*. At n = 2⁵¹²: d = 1 → 13,101 cells, d = 4 → 157,589, d = 22 → 3.7 M *[ours]*. Average case (random sets, n = 10⁶, 8-B elements): close to RIBLT up to d ≈ 100, ~4× RIBLT at d = 1000 (§5.1, Fig. 6), worst scaling above 1000 (§5.3, Fig. 8b) *[p]* |
  | Decode failure | Peeling stalls → ask for more symbols, nothing received is wasted; done = symbol 0 decoded, because ρ(0) = 1 (§4.1.2) *[p]*. Success only w.h.p. (Thm 5.1), no worst-case bound | Same stall-and-ask loop (Fig. 4b; Alg. 5 returns FAIL while cells remain) *[p]*. After m_d cells success is certain for *any* d-set (Def. 3.1), but the receiver doesn't know d, so in practice it also stops on "listing succeeded" *[ours]* |
  | False "pure" symbols | Checksum collisions; §4.3 proposes a keyed 64-bit SipHash *[p]*. We use a fixed 128-bit checksum + `ErrCorrupt` fallback (§9) | "Probability 1" assumes pure cells are recognised exactly (§2.2, Alg. 5); with a hashed checksum the same collision risk remains *[ours]*, so §9's mitigations are needed either way |
  | Cached prefixes 4096 / 512 / 32 | ≈ 2,900 / 360 / 22 differences (proposal §4.3.5) | Prefix property holds (Def. 4.1: one block per prime 2, 3, 5, …), but the caches hold 45 / 18 / 5 prime blocks (Π ≈ 2²⁶⁴ / 2⁷⁷ / 2¹¹) → guaranteed d = **0** at every level for n = 2⁵¹² *[ours]*; on average fewer differences than RIBLT at the root (Fig. 6) |
  | Linearity (Fenwick sums) | Yes (§4.1.1: deterministic per-element mapping rule) | Yes: fixed 0/1 mapping matrix, XOR + count, subtraction (Alg. 3, 4) |
  | Per-update cost | ~15.8 / 11.6 / 6.2 indices below 4096 / 512 / 32, generated in O(log m) (§4.1.2, proposal §4.3.4) | One cell per prime block: 45 / 18 / 5, plus 45 reductions of a 512-bit integer mod p → ~2× the root + level-1 XORs, bucket about equal *[ours]* |
  | Implementation size | 353-line self-contained Go library (§6) *[p]* | Mapping is a few lines (`(y+1) mod p` per prime); reference Python/Go repo not reviewed. About equal |
  | Other variants | — | OLS needs ⌈√n⌉ = 2²⁵⁶ cells per step (Def. 4.4). Extended Hamming stops at d = 3 (Thm 4.7). UniverseReduceSync (§6.2–6.3) hashes ids with a per-round salt: certainty is lost (Table 3: collisions, ≥ 1 rounds) and a per-round salt remaps every element, so the caches would have to be rebuilt each round (the same objection as keyed checksums, proposal §9) |

  CS itself concludes "Rateless IBLT consistently demonstrating superior overall performance"
  (CS §5, Findings), measured with 24-byte cells and n = 10⁶ (Figs. 6, 8). CertainSync's one distinctive
  property, a worst-case listing guarantee, grows with log n and is out of reach for 512-bit ids:
  guaranteeing a single difference takes 13,101 cells, 3× the whole root cache. Its average cost is
  no better than Rateless IBLT for any d and worse from d ≈ 1000, and the variant that shrinks the
  universe gives up both the guarantee and the caches.

  **Downstream changes: none.** T2.1–T2.4 proceed as written, and T2.1 may freeze the §6.2
  `Symbols`/`More` format as specified (`Data = n × (Sum 64 B ‖ Check 16 B ‖ varint zigzag(count −
  Expected))`, ≈ 82 B per symbol, with the count compressed as in RIBLT §6). For reference, had
  CertainSync (EGH) won, the wire format would still not change (EGH fits `Mapper`: `Indices` = block
  offset + `(y+1) mod p` per prime, `Expected = round(count/p)`), but: `Decoder.Done()` ("symbol 0
  empty") would need a stop rule defined by the `Mapper` (e.g. the 2-cell first block is empty); the cap
  table and sizing in §4.3.5 / §6.3 / T2.4 (`≈ symbols/1.4`, `n0 = ⌈1.6·d̂⌉ + 2`) would need EGH
  numbers; T2.1's decode-cost band [1.3, 1.8] would not hold; and T2.2's `Apply` would do ~2× the
  XORs.

_(sessions append here — T2.0 recommendation; T2.6 cache sizes; T2.8 cross-arch vector result)_
- **Gate G3 (2026-10-14): Rateless IBLT accepted.** T2.1–T2.4 proceed as written; the §6.2
  `Symbols`/`More` encoding is unchanged. The stamp-hash grinding risk from the T2.0 follow-ups
  is tracked there (§9 row + adversarial test in T2.5).
- **T2.1 (contract): API.** All in `fingerprint/rateless.go`. `Mapper{ID() uint32;
  Indices(id) IndexIterator; Expected(count, i uint64) int64}` and `IndexIterator{Next() uint64}`
  as in §5.2; `FloatMapper` (zero value usable, `NewFloatMapper()`, `ID() = DefaultMapperID`);
  `CodedSymbol{Sum ID; Check [ChecksumSize]byte; Count int64}`; `Checksum(id) [16]byte`;
  `Encode(m, syms, start, id, sign)`; `NewDecoder(m, r Range)`, `Add(start, remote, local)`,
  `Received()`, `Done()`, `Result()`; codec `AppendSymbolData(dst, m, count, start, syms)` /
  `ParseSymbolData(m, count, start, data)`; sentinels `ErrCorrupt`, `ErrInvalidBatch`,
  `ErrInvalidSymbolData`; constants `ChecksumSize = 16`, `MinSymbolSize = 81`,
  `MaxSymbolSize = 90` (for T2.4/T2.5 batch sizing against `MaxMessageBytes`), `EndIndex`.
  Format constants sit next to their types in `rateless.go` (as `IDSize`, `LtHashSize` do), not in
  `constants.go`.
- **T2.1 (contract): group.** `*CodedSymbol` has `Add`/`Sub` (XOR on `Sum`/`Check`, wrapping
  `int64` add/sub on `Count`) and `Zero`, so it satisfies T1.3's `Group` (checked at compile time
  with `var _ = NewFenwick[CodedSymbol, *CodedSymbol]`). The `[BucketSymbols]CodedSymbol` array
  adapter is T2.2's (D20).
- **T2.1 (contract): `Checksum` is exported for T2.2.** One id touches ~16 root, ~12 level-1 and
  ~13 Fenwick nodes × ~6 symbols; calling `Encode` per target would recompute the checksum and the
  mapping each time. T2.2 instead builds `CodedSymbol{id, Checksum(id), ±1}` once, collects
  `m.Indices(id)` once (still through the `Mapper`, as the task requires) and `Add`s/`Sub`s it at
  each index. No other helper is published.
- **T2.1 (contract): mapping sequence end.** Indices are strictly increasing from 0; a mapping may
  end, after which `Next` returns `EndIndex = MaxUint64` forever. `FloatMapper` ends when
  `i + gap > 2^53 − 1` (the float64-exact range), so no float → integer conversion is ever out of
  range (those are implementation-defined in Go). Indices that large are never reached in practice
  (a session sees ≤ `SessionSymbolBudget` symbols).
- **T2.1 (contract): `FloatMapper` pinned.** PRNG: xoshiro256** (spelled out in code), state = the
  32-byte seed `keccak256("rangesync/map" ‖ id)` as four little-endian `uint64` words, so the whole
  256-bit seed matters (a 64-bit seed would let an uploader find two ids with identical mappings in
  ~2^32 work, cf. the T2.0 grinding follow-up). `w = float64((x>>11)+1)·2^-53` is `1 − u` in
  `(0, 1]`; `t = 1/math.Sqrt(w)` (correctly rounded on every arch; `math.Pow` is not);
  `gap = max(1, ⌈(1.5 + i)·(t − 1)⌉)`. **Deviation from the task formula:** the `max(1, …)`; with
  `u = 0` the formula gives gap 0, a repeated index, which XORs the id out of that symbol again.
  `Expected(count, i) = Round(float64(count)·2 / (float64(i) + 2))` (one rounded division, then
  `math.Round`, ties away from zero), clamped to `MaxInt64`; this is the pinned form of
  `round(count/(1 + i/2))`, which rounds differently. The determinism rule (separate statements,
  explicit `float64(...)` conversions, no product feeding an addition, range-checked conversions)
  is in the `FloatMapper` godoc.
- **T2.1 (contract): wire codec.** Per symbol at absolute index `i = start + k`:
  `Sum 64 B ‖ Check 16 B ‖ binary.AppendVarint(Count − m.Expected(count, i))` (Go's signed varint
  *is* zigzag), the difference in wrapping `int64` arithmetic so any `Count` round-trips whatever
  `Symbols.Count` says (a lying `Count` only lengthens the sender's own encoding, so it is not
  validated). Strict: whole symbols only, shortest varint only, `start + n` must not overflow; so
  `ParseSymbolData` accepts exactly what `AppendSymbolData` produces (the fuzz target asserts
  re-encoding is byte-identical). Empty data = zero symbols (whether an empty batch is allowed is
  T2.4's call).
- **T2.1 (contract): Decoder semantics.** It works on `d_i = local_i − remote_i` (§4.3.7):
  `+1` → only local, `−1` → only remote. `Add` requires `start == Received()` and
  `len(local) == len(remote)` (`local == nil` = empty local symbols) and returns `ErrInvalidBatch`
  otherwise, without changing state; the session still checks `Symbols.Start` itself (a wrong
  `Start` is a peer violation, `ErrInvalidBatch` is a caller bug). `Add` after `Done` is
  `ErrInvalidBatch` (overshoot inside one batch is fine: a batch is added whole before peeling).
  `Result()` returns copies **sorted ascending**, so T2.4 can emit `Items{R, onlyLocal}` directly
  under T1.8's ascending-order paging rule. "Peel step" = one recovered id; the cap is
  recoveries > `2 × Received()` → corrupt (honest decoding recovers at most one id per symbol).
- **T2.1 (contract): two corruption rules beyond §6.4's three.** (a) a pure symbol at index `j`
  whose id does not map to `j` → `ErrCorrupt`: removing that id would not clear symbol `j`, so
  peeling could not continue correctly anyway, and it catches most false-pure symbols of the
  truncated-checksum test; (b) symbol 0 empty while another received symbol is not →
  `ErrCorrupt`: when the whole difference is recovered every symbol is empty, so this blocks a
  wrong "done" from a coincidental empty symbol 0. Both are sound for honest streams. `ErrCorrupt`
  is sticky; a corrupt decoder never reports `Done`.

## Follow-ups
_(sessions append here)_
- **(T2.0) Mapping grinding, not covered by either paper or §9 *[ours]*.** Rateless IBLT has no
  worst-case guarantee, so an uploader could grind `stampHash` (e.g. via the stamp timestamp) for
  two ids x, y that map to exactly the same indices within a cached prefix. The chance that two
  random ids do is ∏(ρ² + (1 − ρ)²) = 2⁻¹²·⁹ / 2⁻²⁸·⁶ / 2⁻⁴⁰·⁶ for the first 32 / 512 / 4096 symbols.
  If x is held only on A and y only on B, that prefix can't peel them (count 0) → `Exhausted` →
  split → scan path (unlimited; the mappings diverge beyond the prefix) resolves it. So the impact
  is extra cost in one bucket, not divergence. Suggest a §9 row and a T2.5 adversarial test (x and
  y with equal first-32 index sets on A and B in one bucket → the session converges via split
  and scan).
- **(T2.0)** `Decoder.Done()` relies on ρ(0) = 1. If a `Mapper` without an all-elements symbol 0
  is ever added, the stop rule has to move into the `Mapper` interface. This is not needed for
  T2.1 / T2.7, which use the same ρ.
- **(T6.0) Metric definitions for the decoding criteria (T2.6, with T2.2/T2.4).**
  `symbols_per_difference` = symbols received for a range, overshoot included, ÷ differences
  recovered in that range, observed once per decoded range with ≥ 1 difference. AC-K2 thresholds
  assume this definition. `parts_sent{type}` / `parts_received{type}` (T1.13) need `symbols` and
  `more` values once T2.4 accepts those parts. `index_memory_bytes` must include the three symbol
  caches added by T2.2 (AC-R1).
- **(T2.1) `Mapper.Indices` allocates.** It returns an `IndexIterator` interface (§5.2), so each
  call boxes the iterator on the heap: one allocation per element, ~4.2M per T2.2 rebuild. If
  T2.2's `BenchmarkIndexApply`/rebuild shows it matters, an allocation-free form (e.g.
  `AppendIndices(dst []uint64, id, end) []uint64` on `Mapper`) would be a contract change to
  `Mapper` and T2.7; not added now.
