# CAC stamp carriers — PoC design

**Date:** 2026-08-18
**Status:** agreed design, PoC scope. Self-contained: implementable from `master`, no dependency
on any research branch.

## 1. Problem

Erasure coding rebuilds a lost chunk's **bytes**, but not its **postage stamp**. The stamp
(113 B: `batchID(32) ‖ index(8) ‖ timestamp(8) ‖ signature(65)`) is stored next to the chunk by
its storers and travels on pushsync/pullsync — it is not derivable from the chunk and its
signature cannot be forged. A rebuilt chunk therefore cannot re-enter the reserve or be
pushsynced. This design makes the **original** stamp of every erasure-coded chunk recoverable.

## 2. Agreed constraints

Fixed in team discussion; the design must satisfy all of them:

1. **Original stamps only** — recovery returns the byte-identical stamp, never a fresh one.
2. **Full coverage** — data, parity and intermediate chunks (every stamped child of every
   parent). The root chunk and the carriers themselves are excluded: they are never
   RS-reconstructed, so their durability path stays what it is today (dispersed replicas for
   the root).
3. **The carrier set is itself erasure coded** — losing any 2 of the carrier group loses nothing.
4. **No unprotected intermediary** — carrier references live directly in the parent; there is no
   single "carrier subtrie root" chunk whose loss would orphan the group.
5. **No mining** — all placement is by uniform hash addresses, derived once, never retried.
   Neighborhood co-location is accepted as probabilistic (same model as the file's own data and
   parity chunks) and absorbed by constraint 3.
6. **Graceful degradation** — every failure mode ends at "chunk recovered, unstamped", never
   worse than today.
7. **Span-neutral** — carriers do not count toward the parent's span (agreed: same treatment as
   parity references).

PoC simplifications: **no backwards compatibility** (the new layout is emitted whenever the
redundancy level is not `NONE`; old joiners cannot read PoC files, and no flag gates it) and
**no reserve re-entry** (recovery ends at a validated `(chunk, stamp)` pair; feeding it back
into the reserve/pushsync uses existing paths and is out of PoC scope).

## 3. Parent layout

Carriers are ordinary CACs referenced from the parent, appended after the parity references,
**outside** the main RS scope, forming their own small RS group:

```
parent payload = [ m data refs ][ k parity refs ][ c carrier refs ][ 2 carrier-parity refs ]
                 └───── main RS scope ─────────┘ └── carrier RS(c,2): any c of c+2 ──────┘
```

Reference sizes: data refs are `refLen` bytes (32 plain, 64 encrypted); parity, carrier and
carrier-parity refs are always 32 B (their chunks are not encrypted — see §7).

### 3.1 Composition per level (verified by fixed-point solve against `level.go` tables)

`m` is the largest data count satisfying the slot budget, with `k = k(m)` from the existing
erasure tables and `c = ⌈(m + k) / 48⌉` (48 = stamps per carrier chunk, §4).

Unencrypted — budget `m + k + c + 2 ≤ 128`:

| Level | m | k | carriers | total slots | spare | data slots today |
| --- | --- | --- | --- | --- | --- | --- |
| MEDIUM | 114 | 9 | 3 + 2 | 128 | 0 | 119 |
| STRONG | 103 | 20 | 3 + 2 | 128 | 0 | 107 |
| INSANE | 92 | 30 | 3 + 2 | 127 | 1 | 97 |
| PARANOID | 36 | 87 | 3 + 2 | 128 | 0 | 39 |

Encrypted — budget `2m + k + c + 2 ≤ 128` (64 B data refs, 32 B everything else):

| Level | m | k | carriers | 32B words used | spare | data slots today |
| --- | --- | --- | --- | --- | --- | --- |
| MEDIUM | 57 | 9 | 2 + 2 | 127 | 1 | 59 |
| STRONG | 51 | 20 | 2 + 2 | 126 | 2 | 53 |
| INSANE | 46 | 31 | 2 + 2 | 127 | 1 | 48 |
| PARANOID | 18 | 87 | 3 + 2 | 128 | 0 | 20 |

Encrypted parents have half the children, so half the stamps — two carriers suffice everywhere
except PARANOID. The spare words (INSANE plain, and most encrypted levels) stay empty (precedent: encrypted
parents are not completely full today either).

**Partial parents** (the last, not-full parent at each level) derive the same way from their
actual data count `m′`: `k′ = k(m′)` from the table, `c′ = ⌈(m′ + k′) / 48⌉`, layout
`[m′ data][k′ parity][c′ carriers][2 carrier-parities]`. Everything is a function of the span
and the redundancy level — no count is stored anywhere. Note `c′` is derived from `m′ + k′`
(data **and** parity), never from `m′` alone.

The existing erasure tables need no regeneration — every `(m, k)` pair above is already inside
them. Only the max-shard derivations (`GetMaxShards` / `GetMaxEncShards`) are replaced by this
~10-line fixed-point loop, exposed as one function returning `(m, k, c)` per
`(level, encrypted)`.

## 4. Carrier payload format

One upload uses one postage batch, so the 32-byte batch ID is hoisted into the header and each
entry carries only the per-chunk remainder of the stamp:

```
header:  count(2, BE) ‖ batchID(32)                          = 34 B
entry:   childIndex(2, BE) ‖ index(8) ‖ timestamp(8) ‖ signature(65) = 83 B
```

- `childIndex` is the child's slot in `[data ‖ parity]`, 0-based (`0 … m+k−1`).
- Capacity: `⌊(4096 − 34) / 83⌋ = 48` entries per carrier.
- Carrier `j` holds the entries for slots `[48j, 48j+48)`, sorted ascending; the stamp for
  slot `i` is in carrier `i ÷ 48` (the explicit `childIndex` makes the mapping verifiable and
  robust for partial parents).
- Payloads are zero-padded to 4096 B so the carrier RS shards are uniform; `count` bounds the
  valid entries. Reassembling a full stamp = `batchID ‖ entry[2:]`.
- Carriers are stored as ordinary CACs (BMT address over the padded payload, span = 4096 —
  the padded content length). The joiner never descends into them (§6), so their span is inert.

### 4.1 Carrier RS group

`klauspost/reedsolomon` (already vendored) with `(c, 2)` over the padded 4096 B payloads
produces 2 carrier-parity shards, stored as CACs the same way. Any `c` of the `c + 2` chunks
recover all payloads. For `c = 1` (small partial parents) the group is `(1, 2)` — effectively
three copies, which the library supports.

## 5. Encode path (hashtrie), per parent — the order that avoids circularity

1. Data children are written and stamped; their stamps are collected as they pass through the
   pipeline.
2. The level fills to `m` (or the file ends → partial parent) → **main RS encode** runs →
   `k` parity chunks are created, stored and stamped; their stamps are collected too.
3. Every stamp of the level now exists. Pack all `m + k` stamps into `c` carrier payloads (§4).
4. **Carrier RS encode** `(c, 2)` → 2 carrier-parity payloads.
5. Store all `c + 2` as CACs; append their 32 B references to the level buffer after the parity
   references.
6. Hash the parent over `[data][parity][carriers][carrier-parities]`.

Nothing in steps 3–5 feeds back into steps 1–2, so the parity-stamp circularity of
carrier-as-RS-input designs cannot occur.

## 6. Decode path (joiner)

The joiner learns a third reference category, treated exactly like parity — skipped, not
descended into, span-neutral:

- `chunkToSpan` already yields `(level, span)`; from those, the §3.1 fixed-point yields
  `(m, k, c)` — including for partial parents.
- Shard count: `shardCnt = (payloadLen − (k + c + 2) × 32) / refLen` (generalizes today's
  `(payloadLen − parities × HashSize) / refLength` in `getShards`).
- `file.ChunkAddresses`, `file.ReferenceCount`, `subtrieSection`, `readAtOffset` and
  `processChunkAddresses` skip carrier references the same way they skip parity references.
- A full read of a PoC file returns byte-identical content; downloads never see stamp bytes.

## 7. Stamp recovery

- **Rebuilt data/parity chunk** (main RS decode produced slot `i`): fetch carrier `i ÷ 48` —
  one reference away in the parent already being read — unpack, take the entry with
  `childIndex = i`, reassemble the stamp.
- **Missing carrier:** fetch the remaining group members; any `c` of `c + 2` → carrier RS
  decode → all payloads.
- **Validation before use:** `stamp.ValidBinding(chunkAddr)` — the signature must recover the
  batch owner over `hash(chunkAddr ‖ batchID ‖ index ‖ timestamp)`. A tampered or mismatched
  entry is discarded.
- **Result:** a validated `(chunk, stamp)` pair handed to the caller (PoC: asserted
  byte-identical to the original; production wiring into reserve/pushsync is out of scope).
- **Degradation:** carrier group unrecoverable (≥ 3 of 5, resp. ≥ 3 of 4 lost) or validation
  fails → the rebuilt chunk is returned unstamped, exactly today's behavior.

Security properties: carriers are content-addressed, so their content cannot be tampered with
in place; each inner stamp is individually signed by the batch owner and bound to its chunk
address, so a wrong carrier can only yield "unstamped", never a forged attribution. One honest
disclosure: for encrypted uploads the carrier payload publicly groups the stamps of sibling
chunks, revealing which chunks share a parent (pushsync already reveals individual
stamp↔chunk pairs; the grouping is new). Accepted for the PoC.

## 8. PoC success criteria

All at MEDIUM unless noted; encrypted cases at encrypted MEDIUM (57+9+2+2).

1. **Format:** upload with redundancy → parent layout matches §3.1; full read returns
   byte-identical content (plain and encrypted; full and partial parents; multi-level file).
2. **Data-chunk recovery:** delete a data chunk → RS rebuild → original stamp recovered,
   byte-identical, `ValidBinding` passes.
3. **Parity-chunk recovery:** delete a parity chunk → its stamp recovered (the case in-scope
   carrier designs could not cover).
4. **Carrier durability:** delete 2 of the 5 (plain) / 2 of the 4 (encrypted) carrier-group
   chunks → stamps still recoverable.
5. **Degradation:** delete 3 of the group → read still succeeds, chunk recovery still works,
   stamps cleanly absent.
6. **Intermediate chunks:** stamps of intermediate (non-leaf) children recoverable via their
   parent's carriers.

## 9. Out of scope (PoC)

- Backwards compatibility, format versioning, rollout gating.
- Reserve re-entry / pushsync of recovered `(chunk, stamp)` pairs.
- Stamps of the root chunk and of the carriers themselves (not RS-reconstructible by
  construction; root durability remains the dispersed-replica mechanism).
- Multi-batch uploads (batch hoisting assumes one batch per upload — true today).
- Table regeneration or protocol/on-chain changes (none are needed).
