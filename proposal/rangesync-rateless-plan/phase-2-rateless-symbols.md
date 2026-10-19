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
- **T2.1 contract review fixes (2026-10-15; supersede the T2.1 contract entries above where they
  differ).**
  - *Fixed sets (M1).* "Honest streams never trigger `ErrCorrupt`" holds only if `remote` and
    `local` are each the stream of one fixed set across all batches of the decoder. The Index
    applies deltas live, so an eviction between `More` batches can make an id recover twice
    (duplicate rule → `ErrCorrupt`) and an addition can keep the stream from ever being `Done`. The
    `Decoder` godoc now states the precondition; how streams get a snapshot is a follow-up for
    T2.2/T2.4/T4.3 (below). Rule (b) (symbol 0 empty while another symbol is not) is the
    churn-sensitive one; it is evaluated only after the peel queue of a batch drains.
  - *Which rule catches collisions (L1; corrects "it catches most false-pure symbols" and the
    range-rule reading of §6.4/§9).* A false pure symbol has an odd `Count`, so its `Sum` is the XOR
    of an odd number of ids of the range, which is again in the range: the range rule cannot catch
    a checksum collision, only a malicious peer's bogus symbols. Rule (a) (a pure symbol's id must
    map to its index) is what catches collisions. A collision rule (a) misses can still end in a
    wrong `Done`, so `TestDecoderTruncatedChecksum` bounds the wrong-`Done` rate (checksum
    truncated to ≥ 16 bits, batches ≥ `MinBatch`) instead of asserting it never happens.
  - *`Expected` rounding (L2).* Ties round away from zero (`math.Round`): `Expected(1, 2) = 1`,
    `Expected(5, 2) = 3`; both are in the vector grid. The Python vector generator must not use
    `round()` (half-to-even). `ParseSymbolData` takes the **received** `Symbols.Count`.
  - *Clamp and density (L3).* The implementation clamps when the quotient is `>= 0x1p63`
    (`float64(MaxInt64)` rounds to 2^63). `ρ(i)` only approximates the mapping probability (the
    `Mapper` godoc says so): `FloatMapper` puts an id at index 1 with probability 0.64 against
    `ρ(1) = 2/3`, so `TestFloatMapperIndices` uses ≥ 5 % relative tolerance at small `i`.
  - *Nits.* The godoc cites xoshiro256** 1.0 (Blackman–Vigna; constants 5/7/9, 17/45; output taken
    before the state advances). `Add`: `len(local) == 0` (not `local == nil`) means empty local
    symbols; `Add(Received(), nil, nil)` is a no-op returning nil, also once `Done`; a non-empty
    batch after `Done` is `ErrInvalidBatch`; a corrupt decoder returns `ErrCorrupt` first.
    `Result` is sorted in `ID.Compare` order. New `Recovered() int` (count without copying, for
    `symbols_per_difference` and the T2.4 hint).
- **T2.1 (implementation, 2026-10-15).**
  - *Vectors.* `testdata/rateless_vectors.json` (16 ids × first 64 indices + checksums, 195
    `Expected` grid points incl. ties, clamp and `EndIndex` ends) was generated by a pure-Python
    implementation written from the spec (by the first T2.1 session; that script is not in the tree)
    and is re-derived by a second, separately written pure-Python script (own Keccak-f[1600] checked against keccak256("") / ("abc"), own xoshiro256**, IEEE double
    ops one per statement, ties away from zero via floor/fraction, not `round()`): 0 mismatches. The
    Go code was not used to produce or check them. Mappings that end (next index past 2^53 − 1)
    are padded with `EndIndex` to 64 entries.
  - *Peel order.* The peel queue is a stack: a batch is peeled from its highest index down, so symbol
    0 (the whole difference, the likeliest false-pure symbol) is tried last. Consequence for
    `TestDecoderTruncatedChecksumCollision`: a 4-symbol batch with a colliding x^y^z may decode
    *correctly* (real pure symbols peel first), so the test asserts "`ErrCorrupt` or the correct
    result, never a wrong `Done`" over 20 collisions, plus a deterministic `scriptedMapper` case
    where rule (a) reports the collision. Symbol 0 alone is still a wrong `Done` (documented).
  - *Decode cost (FloatMapper, one symbol at a time, Done checked after each).* Mean
    symbols/difference: d=2 1.60, 3 1.72, 4 1.75, 5 1.76, 8 1.74, 10 1.72, 20 1.63, 50 1.51,
    100 1.46, 500 1.37, 10⁴ 1.35; d=1 is exactly 1 (symbol 0 alone; exempt from [1.3, 1.8]). The
    peak is ~0.04 above the paper's 1.72 (ρ(1) is 0.64 here, not 2/3). Per-decode SD is large at small
    d (1.0 at d=2), so the test uses 2,000–8,000 trials for d ≤ 10 (sample mean SE ≤ 0.012).
  - *16-bit checksum outcome rate* (batches 4, 16, 64, …, d = 2…300): 1,400 trials, 1,399
    correct, 1 `ErrCorrupt`, 0 wrong `Done` (test bound 0.5 %).
  - *Index overflow.* `ParseSymbolData` rejects a symbol whose index would be `MaxUint64`
    (= `EndIndex`), i.e. `start + n ≤ MaxUint64`, matching `AppendSymbolData`'s panic. `Decoder.Add`
    rejects a batch reaching `EndIndex` as `ErrInvalidBatch` (unreachable in practice).
  - *Costs (amd64, 4 cores).* `Checksum` 0.29 µs, 0 allocs; `Indices` to 4096 0.57 µs, 1 alloc (the
    boxed iterator); `Encode` to 4096 1.4 µs; decode d=1000 in one batch 2.5 ms, 1.7 MB; parse of
    4096 symbols 0.14 ms, 1 alloc.

- **T2.2 (implementation, 2026-10-16).**
  - *Snapshot (M1).* `Index.Symbols` returns freshly copied symbols the caller owns, never aliased
    to the live caches; holding one fixed set across the batches of a stream (Decoder
    precondition) is T2.4's stream state, not the Index's.
  - *API.* `Symbols(r, start, end, scan func(r Range, fn func(ID) error) error) (syms, exhausted,
    err)`: the scan iterate callback is a parameter (nil = no scan path), so `fingerprint` stays
    storer-free; T2.3 passes a `ReserveIterateAddr`-backed one. It is called without any Index
    lock and is not synchronized with the caches. New `IndexOptions.Mapper` (nil = `FloatMapper`)
    and `Index.Mapper()`; every cache index comes from it (each element's indices below 4096 are
    drawn once, then XORed into all three caches).
  - *Source selection.* As §4.3.5, with the cap of r = 4096 (own root) / 512 (own level-1) / 32
    (≤ LeafBits below the root) / 0 (below leaf): `end ≤ cap` → cache; else scan if `scan != nil`
    and `count(r) ≤ ScanLimit` (below leaf depth the containing bucket's count is the bound); else
    symbols `[min(start,cap), min(end,cap))` with `exhausted = true`. `exhausted` is also true for
    a cache window ending exactly at the cap when the scan path is unavailable. `end < start` →
    `ErrInvalidBatch`; `ErrBusy`/`ErrOutsideRoot` as `Summary`. The caller bounds `end − start`.
  - *Level-1 cache is independent of LeafBits* (a level-1 range may be below leaf depth in tests);
    Rebuild now also requires `root.Bits + FanoutBits ≤ MaxBits`.
  - *Tests.* `TestIndexSymbolsConsistency` (seeds 1–3, LeafBits 2–6, random put/replace/evict incl.
    out-of-root ids): every range at every level, the 16 level-1 ranges and below-leaf ranges, vs a
    rebuilt index and `Encode` over the brute-force set. "Every `[start, end)` slice" is every
    slice whose ends are boundary points {0,1,2,31,32,33,511,512,513,4095,4096} ≤ cap plus 8
    random slices per range (exhaustive pairs are 8M per root), and a window past the cap with
    and without scan. A mutation (dropping the root-cache update) fails it.
    `TestIndexSymbolsRebuildConcurrent` (≥ 20 commits overlapping a rebuild iteration).
    `TestIndexSymbolsMemory`: 11,534,336 + 720,896 + 360,448 B at B = 4096 (88-B symbols);
    `index_memory_bytes` includes them (AC-R1).
  - *Costs (this host, 4 cores, noisy).* `BenchmarkIndexApply` 8.6–14 µs → 21–22 µs, 0 → 1 alloc
    (48 B, the boxed `IndexIterator`); profile: SHAKE `Expand` ~43 %, symbol XORs + checksum +
    mapping ~25 %. Rebuild 15.3 µs/element (≈ 64 s at 4.2M here, single-threaded; `Expand` is
    55 %, the T2.2 share ~22 %). `Symbols`: root 4096 0.12 ms, level-1 17 µs, bucket run 2.7 µs.
    *`Indices` allocation does not matter* (mallocgc absent from the top of both profiles): keep
    the interface, no `AppendIndices`.

- **T2.3 (implementation, 2026-10-16).**
  - *Interface.* `Symbols(r, start, end) (syms, exhausted, err)` is added to `Local` (session.go),
    not to `Summarizer`: `*fingerprint.Index` keeps satisfying `Summarizer`, and only the
    storer-backed `local` adapter (local.go) can supply the scan path. Index errors are returned
    wrapped (`index symbols: %w`), as `Summary`.
  - *Scan path.* `local.scanIDs` = one `ReserveIterateAddr(r.Prefix, r.Bits, nil, …)` call mapping
    `ChunkID` → `fingerprint.NewID`; fn's error stops the iteration and is returned; never stops
    early otherwise. No context: the existing adapter (`scan`, `IterateIDs`) takes none.
  - *Tests.* `TestLocalSymbols` (mock reserve, 500 ids): root / level-1 / bucket / below-leaf
    range × windows inside and past each cache, all equal to a brute-force `Encode`; a mutation
    (scan yields nothing) fails it. `fakeLocal.Symbols` brute-encodes for session tests.
  - *Scan cap (review).* The `count ≤ ScanLimit` check precedes the unlocked scan, so concurrent
    puts can grow r: `scanIDs` stops after `2·ScanLimit` IDs with `errScanLimit` (returned wrapped,
    ends the session) rather than mapping to exhausted, since the scan's partial symbols are
    unusable. `TestLocalSymbolsExhausted`: a level-1 range over ScanLimit, window past its cache.

- **T2.4 (implementation, 2026-10-16).**
  - *Inputs.* `Config` gains `Status`/`PeerStatus` (both Statuses, for `cap(R)` and the mapping
    check), `Mapper` (nil = `FloatMapper`) and `Hints`. Symbol streams are used only when both
    Mappings are non-zero and equal `Mapper.ID()`; otherwise the session keeps the Phase-1 form
    and an incoming `Symbols` is a violation (or wraps `ErrMappingMismatch` when both Mappings are
    set and differ: Reconcile already refuses such peers, so it is not scored). `Reconcile` and the
    responder build the `Config` in `Service.sessionConfig`. `DecisionInput` gains `Cap` (0 = no
    symbols) and `Hint`; `DecisionSymbols` sits between `DecisionStripe` and `DecisionSplit`;
    d̂ (`DecisionInput.Estimate`) also drives the stripe test (the hint is ≤ a cache, far below
    `LargeDiff`, so this changes nothing).
  - *`cap(R)` as a per-peer minimum.* `Capacity = min(peer(local), peer(remote))`, a peer's value
    being unlimited if it holds ≤ `ScanLimit` in R, else 4096 / 512 / 32 for its root / level-1 /
    any range ≤ LeafBits below its root (0 deeper). It equals the task's table wherever that applies
    and is more precise elsewhere (e.g. R = root of one peer, level-1 of the other → 512; root of
    both with both small → unlimited instead of 4096). `CapUnlimited = MaxUint64`.
  - *Batch size.* `FirstBatch(d) = max(MinBatch, ⌈1.6·d⌉ + 2)` in integers (`(16d+9)/10 + 2`, d
    clamped so it cannot overflow). New `MaxBatchSymbols = 8192` (720 KiB at `MaxSymbolSize`, so a
    batch always fits one Message): a first batch above it is not chosen (split / items instead),
    and a `More` is served at most 8192 symbols at a time (the peer asks again). Batches are
    deferred whole, like splits.
  - *Snapshot (M1, T2.2 review).* Each side's stream (served and local) copies its cached prefix
    `[0, cacheCap)` (`Local.Symbols(R, 0, cap)`, cap from the own Status: 4096/512/32) when the
    stream starts. Symbols past the prefix come from a new `Local.Symbols(R, 0, end)` call (the T2.3
    scan path) and are used only if its first symbols equal everything taken so far; otherwise the
    stream ends there (served side: `Exhausted`; local side: the decoder gives up), and R is split or
    its items exchanged. So churn never mixes two sets in one decoder (no `ErrCorrupt`/non-`Done`
    for honest streams); the cost of churn is a split. We keep the symbols taken (≤ the symbols
    actually exchanged past the prefix) instead of the ≤ `ScanLimit` ID list, because an ID list of
    every bucket-range stream of a message (up to 1024 × 4096 IDs) would be ~256 MB, while the
    prefixes are ≤ 2.8 KB per bucket range. `TestSessionSymbolsSnapshot` (churn on either side
    between batches; a mutation that skips the prefix comparison makes it decode corrupt).
  - *Stream state and validation.* A served stream marks R unanswered when a batch is emitted, so
    the peer's answer (Items without Reply = its only-local IDs, a split after `Exhausted`, or Items
    with Reply after corruption / at the leaf) reuses the T1.8 answer rules and frees the snapshot.
    `More` is valid only for a served stream whose last batch is unanswered and not `Exhausted`, with
    `UpTo > sent`; `UpTo` is clamped to `cap(R)` (rejecting `UpTo > cap(R)`, the budget and the scan
    cap are T2.5). A first `Symbols` (Start 0) must answer an unanswered Fingerprint; later batches
    must answer a `More` this side sent, at `Start = Received()`, within its `UpTo`; an empty batch
    only with `Exhausted`. The decoded answer is always sent, even with no IDs (one empty page),
    which closes the peer's stream. `wants` = the decoder's `onlyRemote`, without a `HasID` check.
  - *Not decoded.* `More{UpTo: min(4 × received, cap(R))}`; `Exhausted`, a local stream that ended,
    or `UpTo ≤ received` → split if splittable, else `Items{R, Reply}`; the symbols are discarded.
  - *Corruption.* `ErrCorrupt` → `Items{R, Reply: true}` (the T1.8 exchange; no element is lost,
    `TestSessionSymbolsCorrupt`, and a mutation that drops the fallback loses them). Counted in
    `Session.Stats().DecodeCorruptions`, added to the new `decode_corruption_total` counter when the
    session ends (`TestReconcileDecodeCorruption`, over streamtest).
  - *Hints.* `hintCache` on the `Service`: `golang-lru/v2` (already a dependency), `HintCacheSize =
    4096` (peer, range) entries, set to `Recovered()` on every decoded stream. Only the decoding side
    records; the deciding side of the next session reads it, so it helps once both peers have
    initiated with each other.
  - *Metrics.* `parts_sent`/`parts_received` already had the `symbols`/`more` labels (T1.13
    `partType`); no change needed.
  - *Integration bounds.* `TestProtocolConverges` keeps the Meyer bounds plus an allowance for symbol
    streams: equal counts give d̂ = 1, so a stream grows 4 → 4096 in 5 Mores (+10 messages, +8 for
    level-1) and an exhausted root plus 16 level-1 streams waste ≤ 12,288 symbols. Measured: diff 1k
    14 messages (Meyer 11), 367 KB (Meyer 653 KB); diff 4k 22 messages, 1.42 MB (Meyer 0.87 MB).
    The literal worked examples are T2.6's.
  - *Review fixes.* (1) The stream snapshot takes at least symbol 0 when it starts, also for ranges
    without a cache (openings below the leaf depth). The review's "scan-path streams mix sets" was
    checked and does not reproduce: `window` keeps every symbol taken and compares each later
    window with all of them, so the first window fixes the set either way (the new
    scan-path churn cases pass with and without the change); the change only moves the snapshot
    to the decision. (2) A `Symbols{Start: 0}` for a range this side serves symbols for (crossed
    streams) is a violation. (3) The decoder's `cap(R)` is clamped to `SessionSymbolBudget`. (4) A
    `More` past `MaxBatchSymbols` is served in parts, one extra round trip each.
- **T2.5 Symbol-stream limits.**
  - *More.* `UpTo > cap(R)` is now a violation (was clamped). The decoding side computes `cap(R)`
    from the count it **sent** in its Fingerprint (kept in `unanswered`), not a fresh `Summary`, so
    an honest `More` never exceeds the peer's `cap(R)` under churn (and one `Summary` per first batch
    is saved). An unknown range / exhausted stream was already a violation (T2.4).
  - *Budget.* `SessionSymbolBudget` counts all streams of a session, each direction: sent symbols
    are capped (the batch reaching it is `Exhausted`, possibly empty; `decide` gives no symbol
    stream once the sent **or received** count is spent → split/exchange); received symbols past it are a violation (an honest
    peer never sends more), and this side's `More` never asks past the remaining budget.
  - *Peer `Symbols.Count`.* Must be in `[1, itemLimit]` (the `Items` limit), equal in every batch of
    a stream, give `cap(R) > 0`, and the stream must stay within `cap(R)`.
  - *MaxScansPerMessage* is enforced on the serving side, never as a violation (an honest
    initiator can legitimately send > 64 scan-path Fingerprints in one Message, several splits, so
    rejecting would score honest peers): (a) **stream starts** — `decide` starts at most 64 symbol
    streams per received Message for uncached ranges (`cacheCapacity == 0`, whose symbol-0
    snapshot scans); further such Fingerprints get no `Cap` and take the Fingerprint/items path,
    which honest peers accept; (b) **deferral** — at most 64 batches that read past the snapshot
    (`symStream.scans`) per outgoing Message, the rest go out in later Messages. (Review fix: the
    start scans were uncounted, up to `MaxParts` per Message.)
  - *Per-peer session rate.* `ratelimit.Limiter` keyed by peer, `PeerSessionBurst = 4`, one more per
    `PeerSessionInterval = MinRoundInterval / 2`; over it the responder answers `Busy` (not scored),
    checked after every other Busy check (review fix: no token is spent on another Busy). Cleared on disconnect like the delivery limiter.
  - *Tests.* `TestSessionSymbolLimits` (More to/past cap, budget across streams, endless More,
    unknown range, Count lies, first batch past cap, no capacity, received budget),
    `TestSessionMaxScansPerMessage`, `TestSessionReceivedBudgetNoSymbols`, `TestSessionMappingGrinding` (the T2.0 suggestion: ground x, y
    with equal first-32 index sets, bucket cache 32 → exhausted → split → children decode),
    `TestReconcilePeerSessionRate`, `TestReconcilePeerSessionRateAfterBusy`. Violations are session-level; the reset + score path is T1.9's,
    shared by every `ErrInvalidMessage`.
- **T2.6 (2026-10-18): measurements (this 4-core host, `-benchtime` as stated); no protocol
  constant changed — G6 decides.** Benchmarks in `fingerprint/symbols_bench_test.go`,
  worked examples in `rangesync/worked_examples_test.go`.
  - *Decoder threshold* (symbols until `Done`, FloatMapper, 200 draws; p50/p90/p99): d=3 5/8/14,
    d=10 16/24/35, d=25 39/49/68, d=45 68/79/103, d=100 144/160/179, d=250 353/378/396,
    d=1000 1373/1418/1439. The ratio is ~1.5 at d ≈ 25–45 and falls to 1.37 at d=1000; the
    proposal's "~62 symbols for 45" (1.38) is optimistic for small d.
  - *`BucketSymbols` 16 vs 32 (open question 2), B = 4096:* bucket tree 5.5 MiB vs 11 MiB; element
    update 6.1 vs 6.7 µs (dominated by `Encode`); worst bucket-run read (23 nodes) 2.8 vs 5.5 µs.
    Fraction of bucket-run streams decoding within 16 / 32 symbols (2000 draws): d=4 0.99/1.00,
    d=8 0.80/0.99, d=11 0.34/0.98, d=16 0.00/0.87, d=20 0/0.59, d=24 0/0.14. So 16 loses
    one descent already at d ≈ 8–11 (the §5.3 "~11" is the median, not a safe bound).
    **Recommendation: keep 32** — 5.5 MiB is the only gain, and below the root/level-1 caches
    bucket streams only serve ranges the split rule already picked for large local differences.
  - *Root 4096 / level-1 512:* unchanged; the worked examples and §7.3 (16 × 402 symbols,
    4 Messages, 527 KB) behave as specified. Final sizing waits on `pairwise_diff` (G6).
  - *`Index.Apply` (open question 1 follow-through):* populated index (2^18 elements, LeafBits 12)
    8.3 µs per put, 15.9 µs per put + eviction: ≈ 60–120k puts/s per core, one to two orders above
    the reserve's put rate; not a bottleneck.
  - *`Rebuild`:* 2^20 elements 6.6 s (6.3 µs/element, 20 MiB index); **4.2M: 26.5 s** (6.3 µs/element, one allocation per element from `Mapper.Indices`, in-memory IDs: no storer iteration), still over the T2.2 follow-up target of 20 s.
    `index_memory_bytes` already counts the three T2.2 symbol caches (`slabBytes`), as T6.0 asks.
  - *Worked examples (§7.10, §7.2, §7.3):* counted as Messages with parts (the closing empty
    Message excluded, as the proposal does). Example 1: 3 Messages, 453 B (stated ≈ 0.5 KB) ✓.
    §7.3: 4 Messages, 527 KB ✓. **Deviations, reported not loosened:** (a) Example 2: 68 symbols
    decode 45 differences in only ~50% of draws (6/10 seeds; 50/100 in a sweep); otherwise one
    More to 272 → 5 Messages, 22.5 KB. (b) Example 3: 7 Messages (3 + 2 More/Symbols round
    trips; the proposal's own diagram has 7), not 5; bytes 6.1 KB ✓. (c) §7.2: counts N+1 vs N+2
    give d̂ = 1, not 3 (proposal arithmetic error), so n0 = 4: 453 B in ~55%, else one More
    (5 Messages, 1.4 KB); the stated 0.7 KB assumes n0 = 7.
  - *Low estimates (T2.4 follow-up) and open question 3:* sweep of 100 draws per example.
    GrowthFactor 2/3/4: Example 2 fail path 11.3/16.8/22.3 KB; Example 3 11/9/7 Messages at
    ≈ 6.2/≈ 3.9–9.7/6.1 KB. A larger factor saves round trips on hidden differences and costs
    overshoot on near misses; 4 stays. **The better lever is `BatchFactor`:** 2.0 instead of 1.6
    makes Example 2 decode in the first batch in 96% of draws (7.0 KB vs a 14 KB mean), with
    Examples 1/3/§7.2 unchanged. Recommend G6 consider `BatchFactor` 2.0 (a constant, not
    wire). A responder-side hint (size of the peer's Items answer) only helps sessions after the
    first; it does not change the d̂ = 1 cases above (the initiator's hint already covers the
    repeat case), so not added. Streamed batches (Q3): a stop-driven stream overshoots by one
    RTT at line rate (≈ 1,200 symbols ≈ 100 KB at 1 MB/s and 100 ms) against the ≤ 2 extra
    round trips measured here; **keep request-driven `More`** (wire frozen at G2 regardless).
  - *Metrics:* `symbols_per_difference` (histogram, symbols received incl. overshoot ÷ recovered,
    once per decoded stream with ≥ 1 difference, T6.0 definition), `more_requests_total`
    (More parts sent), `streams_exhausted_total{level=root|level1|bucket|deep}`; reuse
    `decode_corruption_total`. Fed from `SessionStats` at session end.

## Follow-ups
_(sessions append here)_
- **T2.4 (from T2.3).** `Local.Symbols`' scan (`scanIDs` → `ReserveIterateAddr`) takes no context
  and cannot be canceled; consider session-lifetime cancellation when the stream state snapshots.
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
- **(T2.1, contract review M1) Symbol streams must come from one snapshot on each side — decide
  before T2.2.** The `Decoder` requires `remote` and `local` to be streams of fixed sets across all
  batches, but the Index applies deltas live between `More` batches. Options: (1) at stream start,
  copy symbols `[0, cap(R))` for both the served stream and the local side (root ≈ 360 KB,
  level-1 ≈ 45 KB, bucket ≈ 2.8 KB per stream) and, for scan-path streams, keep the ≤ `ScanLimit`
  ID list the stream was built from; or (2) treat `ErrCorrupt` on large ranges as a restart/split
  of R rather than the §9 `Items` listing (and accept non-`Done` streams ending in `Exhausted`).
  This changes T2.2's `Index.Symbols` copy semantics (T2.2), T2.4's stream state, and T4.3's
  session memory budget.
- **(T2.1, contract review L1) Correct §6.4/§9.** They present "recovered id outside R" as a
  checksum-collision check. It is not: a false pure symbol XORs an odd number of in-range ids,
  which stays in range. The range check only detects a malicious peer; collisions are caught by
  the "pure symbol's id maps to its index" rule (T2.1 rule (a)), and the remaining ones by
  undeliverable fetches and later rounds.
- **(T2.1, contract review) `Indices` allocation, resolved: keep the interface.** The reviewer
  agrees the allocation is < 1 % of a rebuild and the iterator's resumability is needed (the
  `Decoder` resumes a recovered id's mapping batch by batch). If T2.2's benchmarks disagree, add an
  optional fast-path interface (e.g. `AppendIndices`) that callers detect with a type assertion,
  without changing `Mapper`.
- **(T2.1) Decoder memory per recovered id.** Each recovered id keeps its `IndexIterator`
  (~48 B boxed), a ~112-B entry and a map key so later batches can remove it; with `SessionSymbolBudget` this is
  bounded (≤ 2 × 16384 ids), but T4.3's session memory budget should count it.
- **(T2.2) Rebuild is over the 10–20 s target on this host — due before T4.3 / phase-4 exit.**
  ≈ 15 µs/element single-threaded (≈ 64 s at 4.2M), dominated by the pre-existing SHAKE `Expand`
  (T1.6). On a first start (no live slab) `Symbols` and `Summary` return `ErrBusy` for the whole
  rebuild, so this is the node's sync-unavailable window. *Acceptance:* a by-bucket parallel build
  (expansion workers fed from the iterate callback) and/or batched `addNext` (one `nextMu`
  section per batch of elements), with rebuild ≤ 20 s at 4.2M elements on the reference host
  (T2.6), the T2.2 consistency and concurrent-rebuild tests unchanged and passing.
- **(T2.2 review) T2.4 must snapshot the stream before its first batch.** `Index.Symbols` may serve
  the same r from a cache on one call and the scan path on the next (window past the cache, or
  `count(r)` crossing `ScanLimit`), and cache and scan results can reflect different element sets.
  T2.4's served and local streams must each be taken from one snapshot (cached prefix copied, or
  the scanned ID list kept) and never combine separate `Symbols` calls.
- **(T2.4) Symbol-stream limits are T2.5's.** T2.4 clamps `More.UpTo` to `cap(R)` and serves at
  most `MaxBatchSymbols` per batch, but does not enforce `SessionSymbolBudget` (sent or received),
  `MaxScansPerMessage`, nor reject `UpTo > cap(R)`; the decoder's memory is bounded only by the
  `More`s this side sends (≤ 4× per round up to `cap(R)`; unlimited ranges have no cap yet).
- **(T2.4) Low estimates cost round trips.** With equal counts (d̂ = 1, stale stamps, paired
  differences) the root stream needs up to 5 `More`s; hints only help the side that decoded last
  time. T2.6 should measure whether a larger growth factor or a responder-side hint (record the
  size of the peer's `Items` answer on the serving side) pays off.
- **(T2.4) Scan without context (from T2.3), still open.** The snapshot re-reads the scan path per
  batch past the cached prefix, so a slow scan blocks `Handle`; session-lifetime cancellation of
  `Local.Symbols` would need a context in `Local` (T4.3 session budget).
- **(T2.4) `local.Symbols` vs `cacheCapacity`.** The prefix length comes from the node's `Status`
  root, which can be stale if the Index root moves mid-session; the Index then returns a shorter
  window and the stream just ends earlier (`Exhausted`). Fine for correctness; T3.4 (root change)
  should keep it in mind.
- **(T2.4 review) Peer-claimed `Symbols.Count` is unchecked — T2.5 must bound it.** A peer claiming
  ≤ `ScanLimit` makes `cap(R)` unlimited, and every `More` past the cached prefix rescans `[0, end)`
  (quadratic in the stream length). T2.4 clamps the decoder's `cap(R)` to `SessionSymbolBudget`;
  T2.5 should also validate `Count` against the peer's Fingerprint count / `itemLimit` and enforce
  the budget across all streams of a session.
- **(T2.4 re-review, Low) No test guards churn between `decide` and the first emit.** The scan-path churn tests pass with or without the symbol-0 snapshot (b72b11cf); a test that changes the set between `decide`'s Summary and the first window would. Add with T2.5 or T2.6.
- **(T2.5) Session rate limit resets on reconnect.** The per-peer session limiter is cleared on
  disconnect (as the delivery limiter), so a peer can reconnect for a fresh burst; reconnects cost a
  handshake. Revisit if Status/session spam is observed (T4.x).
- **(T2.5) Churn-before-first-emit test still missing.** `decide` and the first emit run in the same
  `Handle` call, so it needs a `Local` hook that mutates between `Summary` and `Symbols`; not cheap
  enough for T2.5. Carry to T2.6. *Done in T2.6:* `TestSessionSymbolsChurnBeforeFirstEmit` (a
  `fakeLocal.beforeSymbols` hook); it fails with the symbol-0 snapshot of b72b11cf reverted.
- **(T2.5 review) Decide-time scans are now under `MaxScansPerMessage`** (stream starts counted,
  excess ranges take the Fingerprint/items path); the earlier follow-up is resolved.
- **(T2.5 review, Low) Session limiter entry can outlive a disconnect.** `ratelimit.Limiter` never
  prunes idle keys. The `Allow` now runs after the "no status of the peer" Busy (the Status is
  deleted on disconnect), so only a handler racing `disconnected` between that check and `Allow`
  recreates an entry (one small entry per such race). Revisit with a pruning limiter if needed.
- **(T2.5 verify) Flaky `TestIndexSubscribeRootChange/debounced`** (fingerprint, T1.6): failed once with "unexpected signal" under `-count=3 -cpu=1,2,4`, then passed 30 reruns. Timing-sensitive; untouched by T2.5. Fix the wait when next in that package.
- **(T2.6) Rebuild at 4.2M is 26.5 s on this host, over the 20 s target (T2.2 follow-up, still
  open).** Single-threaded, in-memory IDs; with the storer iterator it will be slower. Due with the
  by-bucket parallel build before T4.3 / phase-4 exit.
- **(T2.6) `BatchFactor` 2.0 for G6.** Measured in the Decisions log: at d ≈ 45 the first batch
  decodes ~50% of the time with 1.6 and ~96% with 2.0. A constant change (not wire); G6 decides with
  `pairwise_diff` data.
- **(T2.6) Proposal text corrections.** §7.2 states d̂ = 3 for counts N+1/N+2 (it is 1, so n0 = 4
  and ~0.46 KB); Example 3 has 7 Messages with parts, not the plan's 5; Example 2's "~62 symbols on
  average" is ~68 (median) with FloatMapper. The literal tests assert the implementation's outcomes
  next to the proposal's.
- **(T2.6 review, Low) Small cleanups, not blocking.** (1) `session_symbols.go` labels ranges with 0 < depth < FanoutBits as `bucket` in `streams_exhausted_total`; add a comment or a `default` case. (2) `integration_test.go` checks `symbols_per_difference` loosely (count ≤ symbol parts, sum ≥ n); assert the numerator. (3) `BenchmarkIndexRebuildScale/n=4194304` runs under `-bench .` (26 s+, ~128 MiB); gate it or say so in its doc. (4) Ex3's `want` bytes omit the `fp` term the other cases include. The review confirmed the §7.2/Ex2/Ex3 gaps are proposal errors (§7.2 |cA−cB| = 1, not 3), not code bugs: correct the proposal text when it is next edited.
- **(Orchestrator, 2026-10-19) Rebuild 26.5 s accepted.** The two rebuild follow-ups above (T2.2,
  T2.6) are closed: the user accepted 26.5 s at 4.2M at the pre-T4.3 check (phase-4 Decisions log).
