# `rangesync` testnet runbook (T6.3)

**Status:** prepared by T6.3 (documentation part). The comparison runs need people and testnet
hosts (gate G7) and have not run yet; T6.3 stays open until they have. Results go into the
"Decisions log" of [phase-6-acceptance-testing.md](phase-6-acceptance-testing.md) (template in
§10.3). Criteria: [acceptance-criteria.md](acceptance-criteria.md) (AC-*). Proposal: §8.3 (flag
matrix), §8.4 (`SyncRate`), §10 (metrics), §12 (rollout).

Code references are to `rangesync/phase-6` at d6fe508a. Every flag, metric, log line and API field
named here exists at that commit. What does not exist is marked **missing** and listed in the
phase-6 Follow-ups ("(T6.3 docs) …").

Contents: 0 decisions before the runs · 1 build and inventory · 2 modes and flags · 3 cohort design
· 4 procedures · 5 tooling · 6 dashboard · 7 criteria · 8 special items · 9 G6 confirmations ·
10 run plan · 11 known issues · A handed items · B metric coverage.

---

## 0. Decisions before the runs (G7)

| # | Decision | Options | Notes |
|---|---|---|---|
| G7-1 | The two production Mediums of T6.1 part 2 (§11, K1 and K2) | fix before the runs, or accept and run | If accepted: AC-V4 fails in the decreases the race hits (~3/8 in-process), and `bee_rangesync_peer_failures_total` fills with false positives whenever a neighborhood takes ≳ 10 uploads/s during a bootstrap or striped catch-up; run S10 at two loads (§8) to show it. If fixed: record the fix commits in G7-7. |
| G7-2 | Fetch cap for dual cohorts on constrained hosts | (a) provision ≈ 70 Mbit/s inbound per dual node; (b) a test build with a lower `DefaultFetchRate` (`pkg/rangesync/constants.go`); (c) OS shaping (`tc`) | There is **no flag** for rangesync's fetch rate (**missing**, §2.4). (b) is a code change: record the commit, and run it on every node (all cohorts) so the build stays the same everywhere. But (b) also caps the `rangesync`-only cohort, so the gating AC-V3 and AC-V4 would compare a capped rangesync with an uncapped puller, a bias against rangesync: run Runs 3 and 4 with the default build on every node (a recorded build switch), or record AC-V3 and AC-V4 of a capped run as non-gating. (c) on the dual hosts only slows both protocols there and leaves R and P untouched. |
| G7-3 | Topology | (a) one network, cohorts assigned per sibling pair of neighborhoods (§3); (b) one network per cohort (`--network-id`, same load and bootnode setup) | (a) needs one sibling pair per cohort, three in all: 2^(r−1) ≥ 3, so a storage radius r ≥ 3 (four pairs); (b) needs r ≥ 1 in each network. AC-N5 needs neighborhoods more than half full (§3 rule 8): seed 0.6 × 2²² chunks per neighborhood, ≈ 20M in all at r = 3 (§4.5). `--minimum-storage-radius` holds a radius (§4.9 Method B) only on a network where every full node uses it: salud marks a node unhealthy when its neighborhood's most common depth equals the network's and its own differs (`pkg/salud`), and an unhealthy node skips every redistribution round (and never logs `full sync done`). So (a) with Method B means a private network. |
| G7-4 | Does AC-V1 gate on testnet? | gate, or informational | The propagation probe now exists (§5.3, T6.0 follow-up). |
| G7-5 | Draft thresholds | confirm or replace AC-D2 1 %, AC-K2, AC-R4 5 %, AC-R7 1.0×, AC-R8 +64 MB, AC-N5 ±10 %, and the draft rules of §9 | acceptance-criteria.md §6 items 1 and 3. |
| G7-6 | Phase 5 in scope? | yes or no | No `audit_mismatch` metric exists at this commit, so AC-S6 is not measurable. |
| G7-7 | Build | one commit for every node | Same binary on every cohort; only the flags differ. |

---

## 1. Build and inventory

```bash
git checkout <commit>                      # the G7-7 commit; record it
make binary                                # dist/bee, CGO_ENABLED=0
dist/bee version
uname -m                                   # x86_64 on every host that runs rangesync (D22)
```

Inventory file, one row per node; the scripts of §5 read it (`#` lines are skipped):

```
# node,api_url,cohort,neighborhood
n01,http://10.0.0.11:1633,rangesync,01
n02,http://10.0.0.12:1633,dual,10
n03,http://10.0.0.13:1633,pullsync,00
```

- `cohort`: `pullsync`, `rangesync` or `dual` (§2).
- `neighborhood`: the node's neighborhood prefix at the start of the run, in bits
  (`storageRadius` bits of its overlay, from `/status` and `/addresses`).
- Prometheus labels, set in the scrape config: `node`, `cohort`, `neighborhood`, `group` (the
  sibling-pair prefix, one bit shorter, §3 rule 2), `arch`.

Bind the API where Prometheus can reach it, on a private interface only: it is unauthenticated
(`--api-addr=10.0.0.11:1633`). `/metrics` is served on the API address.

```yaml
scrape_configs:
  - job_name: bee
    scrape_interval: 15s
    static_configs:
      - targets: ['10.0.0.11:1633']
        labels: {node: n01, cohort: rangesync, neighborhood: '01', group: '0', arch: amd64}
  - job_name: runbook            # node_exporter --collector.textfile.directory=<dir> (§5.1)
    honor_labels: true           # keep the node/cohort/... labels the poller writes
    static_configs:
      - targets: ['10.0.0.11:9100']
```

---

## 2. Modes and flags

### 2.1 Flag matrix (§8.3; `cmd/bee/cmd/cmd.go`)

| Cohort | Flags (with `--full-node=true`) | Env | YAML |
|---|---|---|---|
| `pullsync`-only (defaults) | `--puller-enable=true --rangesync-enable=false` | `BEE_PULLER_ENABLE=true BEE_RANGESYNC_ENABLE=false` | `puller-enable: true`, `rangesync-enable: false` |
| `rangesync`-only | `--puller-enable=false --rangesync-enable=true` | `BEE_PULLER_ENABLE=false BEE_RANGESYNC_ENABLE=true` | `puller-enable: false`, `rangesync-enable: true` |
| dual | `--puller-enable=true --rangesync-enable=true` | both `true` | both `true` |
| refused | `--puller-enable=false --rangesync-enable=false` on a full node | | start fails: `full node needs at least one of --rangesync-enable and --puller-enable` |

- Light and boot nodes ignore `--rangesync-enable` and log the Warning `rangesync is only available
  on full nodes; ignoring --rangesync-enable`.
- `--puller-enable=false` stops pulling only. The `pullsync` handlers stay registered, so
  `pullsync` peers still pull from a `rangesync`-only node (AC-S3).
- Keep everything else equal across cohorts: `--reserve-capacity-doubling` (0), `--warmup-time`,
  `--minimum-storage-radius` (§4.9), staking, hardware class, `--verbosity`.

### 2.2 Check the mode of a running node

The metrics are registered only for the drivers that run (`pkg/node/node.go`):

| Mode | `bee_puller_pullsync_rate` | `bee_rangesync_sync_rate` |
|---|---|---|
| pullsync-only | present | absent |
| rangesync-only | absent | present |
| dual | present | present |

```bash
curl -s $API/metrics | grep -E '^bee_(puller_pullsync_rate|rangesync_sync_rate) '
```

Panel A2 (§6) checks the whole inventory.

### 2.3 Logs to know

| Logger (for `/loggers`) | Level | Message | Meaning |
|---|---|---|---|
| `node/storer` | Info | `filling the reserve address index, do not interrupt` / `filled the reserve address index` (`count`, `duration`) | first enabled start, or a refill (§4.2) |
| `node/storer` | Warning | `reserve address index fill took longer than expected` | fill > 60 s (AC-R6) |
| `node/storer` | Debug | `dropped the disabled reserve address index` (`count`) | the reclaim's drop ended; compaction follows, with no log at its end (§4.4) |
| `node/storer` | Info | `reserve radius decrease` (`radius`) | the reserve worker lowered the radius (`t_dec`, AC-V4) |
| `node` | Info | `node warmup complete. system is considered stable and ready.` | sync starts after this (`t_start`, AC-V3) |
| `node` | Info | `full sync done` | first time combined rate 0, salud healthy, reserve ≥ half capacity (its `duration` field is always 0: K11) |
| `node/storageincentives` | Info | `skipping round because node is not fully synced` | AC-N5: count per node |
| `node/rangesync` | Debug | `fingerprint index rebuilt` (`radius`) | index rebuild done (AC-R3) |
| `node/rangesync` | Debug | `fetch shortfall, peer scored`; `invalid message from peer, disconnecting` | S7 |
| `node/rangesync` | Debug | `overwrite newer chunk` (`peer_address`) | S15 |
| `node/rangesync-reconciler` | Debug | `peer failed` (`reason`) | the reason behind each `peer_failures_total` increment (S10) |
| `node/rangesync-reconciler` | Debug | `radius decreased`; `stripe reassigned` (`error`) | K1 |
| `node/rangesync-reconciler` | Debug | `peer does not support rangesync` (`peer_address`) | D12 check (§3) |
| `node/rangesync-reconciler` | Debug | `stripe claim ended: it stored nothing` | S4, S8 |

Debug on one logger at run time (no restart). `exp` is a base64url-encoded regular expression
over the logger IDs (`node/<name>[…]`); verbosity is one of `none error warning info debug all`:

```bash
b64() { printf %s "$1" | base64 -w0 | tr '+/' '-_'; }
curl -s -XPUT "$API/loggers/$(b64 '^node/rangesync')/debug"   # rangesync and rangesync-reconciler
curl -s -XPUT "$API/loggers/$(b64 '^node/storer\[')/debug"
curl -s -XPUT "$API/loggers/$(b64 '^node/rangesync')/info"    # back
```

### 2.4 Tuning knobs: none

There is no CLI flag for any rangesync constant. `rangesync.Options.FetchRate` exists, but
`pkg/node/syncer.go` (`rangesyncOptions`) leaves it at `DefaultFetchRate` (1000 chunks/s), so
"lower rangesync's fetch limit for dual cohorts" (T6.3 scope) needs G7-2. Holders serve at most
250 chunks/s per peer (`handleMaxChunksPerSecond`, `pkg/rangesync/fetch.go`), as pullsync does;
the puller's limit is also 1000/s (`maxChunksPerSecond`, `pkg/puller`).

---

## 3. Cohort design rules

1. **Whole neighborhoods (D12).** A `rangesync`-only node does not pull from `pullsync`-only
   peers. Every full node inside the radius of a `rangesync`-only node must run rangesync
   (`rangesync` or `dual`).

   | Full nodes in one neighborhood | Allowed? |
   |---|---|
   | pullsync-only only | yes (`pullsync` cohort) |
   | rangesync-only (+ dual) | yes (`rangesync` cohort) |
   | dual only | yes (`dual` cohort) |
   | dual + pullsync-only | yes (dual nodes pull through both), but not a clean cohort |
   | rangesync-only + pullsync-only | **no**: the rangesync-only node never gets what only the pullsync-only peers hold, never becomes fully synced, and skips redistribution, which would read as a rangesync failure |

2. **Sibling pairs.** A radius decrease merges two sibling neighborhoods. If the two halves are in
   different cohorts, the merged neighborhood breaks rule 1 and AC-V4 fails for a design reason.
   Assign cohorts per sibling pair (prefix `r − 1` bits, the `group` label), and per
   `r − k` bits if k decreases are planned. A radius increase (a split) keeps a cohort whole.
3. **amd64 only (D22).** Until T2.8 has checked the `FloatMapper` vectors on arm64, every host
   that runs rangesync is amd64 (`uname -m` = `x86_64`; `arch` label). Panel E2 must read
   `mapping_mismatch_total` 0 (AC-K4).
4. **Same build, same settings** on every node of every cohort (G7-7, §2.1).
5. **Neighborhood size.** At least one neighborhood per cohort with N ≥ 17 full nodes (AC-B1
   passed in-process at N ≥ 17 and missed at N = 5 and 9), one with N ≈ 18–24 if the hosts allow
   (the initiated-session cap slides there, S3, S19), and cohorts compared at similar N. Record N
   per neighborhood (`neighborhoodSize` in `/status`: connected peers within the radius, boot
   nodes included, plus itself).
6. **Dual-mode bandwidth.** A dual node pulls up to ~2000 chunks/s (1000 per protocol), about
   8.5 MB/s (≈ 70 Mbit/s) inbound during a catch-up, plus what it serves. If the hosts cannot
   take it, decide G7-2 and record the cap with the results.
7. **Redistribution.** For AC-N5, every full node of every cohort is staked the same way and has
   `--storage-incentives-enable=true` (the default).
8. **Reserve fill.** A node is "fully synced" only with a reserve of at least half its capacity C
   (`ReserveSize()`, every chunk it holds; 2²¹ at the default C = 2²²; `pkg/node/node.go`, both
   `isFullySynced` functions), so AC-N5 and AC-V3's `t_synced` need neighborhoods that hold more
   than C/2. The reserve worker lowers the radius whenever the count *within the radius* is below
   C/2 (unless `--minimum-storage-radius` holds it, §4.9); the two counts differ only by chunks
   below the radius. A neighborhood holding between C/2 and C sits at radius r on its own; seeding
   exactly 2^(21+r) chunks would put it on the edge, so seed 0.6 C per neighborhood and load within
   the budget of §4.5.

Pre-flight check of rule 1 (`rs-coverage.sh`, §5.2):

```bash
./rs-coverage.sh inventory.csv      # exit 0: OK; prints VIOLATION lines otherwise
```

During the runs, also watch for the Debug line `peer does not support rangesync` naming a peer
inside the radius of a `rangesync`-only node.

---

## 4. Procedures

Values below are at the reference reserve (2²² ≈ 4.2M chunks; G4 and T6.2 measurements). Scale
them with the reserve size.

### 4.1 Start a node in a mode

```bash
bee start --full-node=true --data-dir=/var/lib/bee --api-addr=10.0.0.11:1633 \
  --puller-enable=false --rangesync-enable=true <the testnet's usual flags>
```

A new node in a chosen neighborhood (bootstrap, §4.8): a new, empty data dir and
`--target-neighborhood=<bits>` (mines the initial overlay into that neighborhood).

### 4.2 Upgrade: pullsync-only → dual → rangesync-only

Per sibling pair, so that rule 1 holds at every step:

1. Run 0 (§10) gives each node's before values for panels G4–G7. For AC-R5, on 2–3 nodes and in
   an upload pause (§4.6, so the other indexes do not grow meanwhile), record
   `du -sb <data-dir>/localstore/indexstore` and `bee_localstore_reserve_size` right before the
   enabled restart (B0, n0), right after the fill (B1, uncompacted) and right after one more
   restart (B2). AC-R5 reads (B2 − B0) ÷ n0 and reports (B1 − B0) ÷ n0.
2. Restart every node of the pair with the **dual** flags, one at a time. Dual + pullsync-only is
   allowed (rule 1).
3. Restart each node with the **rangesync-only** flags (only for the `rangesync` cohort), once every
   node of the pair runs dual.

What the first enabled start does (the order of the log):

| Step | Signal | Expected at 4.2M |
|---|---|---|
| address-index fill, blocking in `storer.New` | Info `filling the reserve address index, do not interrupt` → `filled the reserve address index` (`count`, `duration`) | ≈ 41–44 s (G4); ≈ 65–75 s with leftovers of an earlier enabled period. A stop during the fill makes the next start fill again. |
| fingerprint index build | `bee_rangesync_index_rebuild_seconds` (`_count` +1), Debug `fingerprint index rebuilt`; sessions get Busy meanwhile | 26.5 s (accepted at G4) |
| start-up guard | `bee_rangesync_sync_rate` = 1 (`StartupGuardRate`) until every eligible neighbor exchanged a Status, or 120 s | ≤ 120 s |
| disk | `du` of `localstore/indexstore` grows | +350–600 MiB (§4.4) |

A dual → rangesync-only restart after a clean stop refills nothing (the index marker matches; a
crash leaves no marker, and the next start refills).

### 4.3 Rollback to pullsync (`--rangesync-enable=false --puller-enable=true`, AC-S4)

Per sibling pair, keeping rule 1:

1. rangesync-only → dual (`--puller-enable=true`, rangesync stays on), every node of the pair.
2. dual → pullsync-only (`--rangesync-enable=false --puller-enable=true`), one at a time.

At step 2 the disabled start deletes the index marker, drops the address-index entries in the
background, then compacts their range (§4.4 for the times and the completion signal). After it,
`bee_rangesync_*` is absent from `/metrics`.

The puller resumes from its stored per-peer intervals. A node that never ran the puller (or ran it
with other peers) pulls every peer's history from the start: offers for its whole reserve (the
Want bitvector skips the chunks it holds, so few chunks move), and `bee_puller_pullsync_rate` > 0
until it is done, which holds the radius and `isFullySynced` meanwhile.

Emergency: restart every node of the cohort at once with `--rangesync-enable=false
--puller-enable=true`. Rule 1 holds again once all have restarted.

AC-S4 passes when: the node runs pullsync-only (§2.2); `localstore/indexstore` bytes per reserve
chunk, measured in an upload pause right before the disabled restart and again once the drop and
the compaction are done (§4.4), are back to the node's per-chunk value before the upgrade (B0 ÷ n0
of §4.2) within 10 % (draft, T6.3); and `isFullySynced` returns after the puller's catch-up.
Compare per chunk: the reserve has grown since the upgrade.

### 4.4 Downgrade to a build without rangesync

From the corrected T6.2 paragraph ("(T6.2 fix round 1) T6.3, runbook …").

- **What stays on disk.** A build without rangesync starts and runs normally with the leftover
  `chunkAddr` entries and the `addrIndexValid` marker; it never reads or changes them. Their size:
  §5.1 estimates ~80 B per chunk (≈ 350 MB at 4.2M); T6.2 measured 85 B per chunk at 4000 chunks
  after a restart; G4 measured ≈ 600 MiB (≈ 150 B per entry) at 4.2M right after a fill,
  uncompacted. **Plan for 350–600 MiB on a full reserve.**
- **Reclaim before downgrading.** Start once with a rangesync-aware build and
  `--rangesync-enable=false`. Keep it running until the background drop and the compaction after it
  have both finished: at 4.2M on G4's host ≈ 12–14 s of drop, then ≈ 19–21 s of compaction; allow
  several times that on a loaded node. Check that `localstore/indexstore` got smaller (`du`) before
  stopping.
- **Stopped early:**
  - during the drop: the next start with `--rangesync-enable=false` resumes the drop, then
    compacts;
  - during the compaction: every entry is already deleted, so the next disabled start finds none
    and runs neither drop nor compaction. The space comes back only as leveldb rewrites those tables
    in its own compactions. `bee db compact` compacts sharky, not leveldb;
  - to force the reclaim: start once with `--rangesync-enable=true` (a refill, ≈ 41–44 s at 4.2M),
    then again with `--rangesync-enable=false`, and let it finish.
- **Completion signal.** Only the Debug log `dropped the disabled reserve address index` (with
  `count`), written when the drop ends, before the compaction. Nothing reports the end of the
  compaction (**missing**, T6.2 follow-up); a compaction failure is logged at Debug. Set the
  storer logger to debug (§2.3), wait at least the compaction time after that line, or watch the
  directory size:

  ```bash
  watch -n5 du -sb /var/lib/bee/localstore/indexstore
  ```
- **Upgrading again with the leftovers** costs one refill (≈ 65–75 s at 4.2M). No stale entry is
  served.

### 4.5 Load and the reserve budget

Uploads go through an uploader node outside the measured neighborhoods (a node's own uploads sit
in its upload store, which `HEAD /chunks` sees). `rs_probe.py load` (§5.3) posts random 4 KB
chunks (`POST /chunks`, `Swarm-Deferred-Upload: false`) and counts the successful ones per
inventory neighborhood (`runbook_uploads_total{neighborhood}`). Random addresses spread uniformly,
so **each neighborhood gets the total rate ÷ 2^r** (r = storage radius): 15/s per neighborhood at
r = 3 takes 120/s in total; past ~100/s use several uploaders, each with its own `--name`.

- **Reference load** (AC-B1, AC-K2, AC-V2): median `pairwise_diff` ≥ 20 per steady-state session.
  Start at 15 uploads/s per neighborhood (N ≤ 9) or 30/s (N ≥ 17, T6.1's `accRate`), check panel
  C5, raise by 5/s until it holds.
- **Low load** (AC-B1b): ≤ 1 upload/s per neighborhood (median `pairwise_diff` ≤ 1).

**Reserve budget per neighborhood** (C = 2²² chunks). Without one, 15 uploads/s add 1.3M chunks a
day per neighborhood: a reserve seeded just above C/2 overflows within about a day and a half,
the radius rises network-wide, N halves (rule 5) and eviction starts.

| Item | Rule | At 15 uploads/s per neighborhood (30/s) |
|---|---|---|
| Seed (Run S) | 0.6 C per neighborhood (rule 8): 0.3 C on batch `B_keep` (TTL past the campaign), 0.3 C on batch `B_drop` (TTL ending with the drain before Run 4) | 2.5M per neighborhood, × 2^r in all (20M at r = 3) |
| Load | immutable load batches in rotation: a new one every 6 h, each with a TTL of ~12 h, so at most 12 h of uploads are live | ≤ 0.65M = 0.15 C (1.3M = 0.31 C) |
| Peak | seed + live load ≤ 0.95 C | 0.75 C (0.91 C) |
| Drain before Run 4 | 12 h at the low load (the load batches expire), then `B_drop` expires | ≈ 0.3 C < C/2; the merged pair holds ≈ 0.6 C after the decrease |
| S1 (Run 6) | one mutable batch of depth 17: past its 2^(17−r) slots per neighborhood every upload overwrites | + 2^(17−r) per neighborhood |

- **Batches.** `POST /stamps/<amount>/<depth>` (response `batchID`); wait until `GET /stamps/<id>`
  says `usable`, and read its `batchTTL`. Load batches are immutable (the API default): the node's
  stamper never reuses a slot of an immutable batch (it answers "bucket full" instead), so the load
  makes no overwrites. A bucket holds 2^(depth−16) chunks: give each load batch at least 3 × its
  mean per bucket, 2^(depth−16) ≥ 3 × the batch's uploads ÷ 65536 (6 h at 120/s = 2.6M uploads:
  depth 23). A mutable batch (header `Immutable: false`) is for S1 only.
- **Rotation.** `--batch @batch.txt` re-reads the batch ID every 10 s: write each new load batch's
  ID into the file every 6 h.
- **Denominators.** AC-B1, AC-D3 and AC-D5 divide by the uploads into the neighborhood
  (`runbook_uploads_total`; AC-B1's own "÷ chunks uploaded"), not by reserve growth, which expiries
  and overwrites distort. Windows: ≤ 6 h, no radius change, and inside one load batch's active 6 h,
  so that no batch that took uploads inside the window expires in it.
- **Overwrite check** (every AC window, panel D0): a node's reserve growth plus its removals equals
  the uploads into its neighborhood within ~1 %. A shortfall means overwrites (a mutable batch, a
  reused stamp) or chunks the node never got; the Debug line `replacing chunk stamp index` (logger
  `node/storer/reserve`) names each overwrite.

```bash
curl -s -XPOST "$UPLOADER/stamps/<amount>/<depth>" | jq -r .batchID > batch.txt    # every 6 h
python3 rs_probe.py load --uploader $UPLOADER --batch @batch.txt --rate 120 --duration 86400 \
  --inventory inventory.csv --name gen1 --out load.csv \
  --textfile /var/lib/node_exporter/textfile/runbook_uploads_gen1.prom
```

### 4.6 Upload pause (AC-C2, AC-V5)

Stop the load generator for 30 min. Check that the neighborhood is otherwise quiet
(`rate(bee_pushsync_storer[5m])` ≈ 0 on its nodes; a public testnet may carry other uploads).
`t_pause` = the generator's stop time.

### 4.7 Reserve-equality probe (stands in for AC-C1; extends AC-N5's sample equality)

`/rchash/<depth>/<anchor1>/<anchor2>` computes a reserve sample over the chunks older than the
previous round's start (`getPreviousRoundTime`), so nodes that run it in the same round sample the
same set if their reserves agree. Run it on every node of a neighborhood within one round (a round
is 152 blocks), with the same anchors and `depth` = the nodes' `committedDepth` (`/status`):

The sample skips every chunk whose proximity to the anchor is below `depth`
(`pkg/storer/sample.go`), so the anchor must start with the neighborhood's prefix: a random anchor
falls outside the neighborhood in 2^r − 1 of 2^r cases, the sample comes up short, the proofs fail
and the API answers 500 on every node alike.

```bash
# PREFIX = the neighborhood's first DEPTH bits, e.g. 011; DEPTH = committedDepth
anchor() { python3 -c 'import os,sys; p=sys.argv[1]; n=int.from_bytes(os.urandom(32),"big"); L=len(p); n=(n>>L|int(p,2)<<(256-L)) if L else n; print(format(n,"064x"))' "$1"; }
A=$(anchor "$PREFIX")
for api in $NBHD_APIS; do
  h=$(curl -fsS "$api/rchash/$DEPTH/$A/$A" | jq -er .hash) || h=FAILED
  echo "$api $h"
done
```

Needs the storage-incentives agent (`--storage-incentives-enable=true`, the default). Pass: no
`FAILED` line, and one hash for the whole neighborhood. It reads the whole reserve (minutes, I/O
heavy) and is recorded in `bee_localstore_reserve_sample_duration_seconds`: keep it out of the AC-N5 windows and
out of the nodes' own sample phases.

### 4.8 Bootstrap event (AC-V3, AC-B2, AC-N1, AC-N2, AC-R7)

1. Pick one neighborhood per cohort with similar N and reserve size.
2. Start one new node in each at the same time (§4.1: empty data dir,
   `--target-neighborhood=<prefix>`, the cohort's flags), under the reference load.
3. `t_start` = Info `node warmup complete…` on the joiner (sync starts after warm-up; also record the
   process start).
4. `t_union` = first time the joiner's `bee_localstore_reserve_size` is within 0.1 % of the median of
   the other nodes of its neighborhood (≈ 4200 chunks ≈ 4 s of fetch at 4.2M). Confirm with §4.7
   afterwards.
5. `t_synced` = first `runbook_is_fully_synced` = 1 and the Info `full sync done`;
   `t_rate0` = `bee_rangesync_sync_rate` (R, D) or `bee_puller_pullsync_rate` (P, D) back to 0.
6. Observe until `t_synced` + 2 rounds. At 4.2M and 1000 chunks/s a bootstrap takes ~70 min; allow 3 h.

### 4.9 Radius-decrease event (AC-V4, AC-R2, AC-N1)

The reserve worker lowers the radius at its tick (every 15 min, `reserveWakeUpDuration`) when the
chunks within the radius are below half the capacity, the combined `SyncRate` is 0 and the radius is
above `--minimum-storage-radius` (`pkg/storer/reserve.go`).

Both methods first bring the count within the radius below half the capacity: the drain of §4.5
(12 h of low load, then `B_drop` expires; ≈ 0.3 C left, ≈ 0.6 C in the merged pair after the
decrease). With uniform uploads the expiry empties every neighborhood alike. Do it after Runs 2
and 3: from then on the neighborhoods below half are not "fully synced" (§3 rule 8).

- **Method A (natural radius):** the reserve worker of every node of every neighborhood below half
  lowers the radius at its next tick. Every pair decreases (within 15 min of each other), so pick
  the measured pairs by cohort and record each node's `t_dec`.
- **Method B (deterministic):** the whole network runs at `--minimum-storage-radius=r` from the start
  (it only holds the radius; the neighborhoods come from the seed, §3 rule 8), so the drain
  lowers no radius. For the event,
  restart every node of one sibling pair per cohort, at the same time, with
  `--minimum-storage-radius=r-1`. The stored radius stays r at start (`waitNetworkRFunc` keeps a local
  radius above the minimum), and the first worker tick, ~15 min after the warm-up, lowers it to
  r − 1. Try it on one node first. The restart adds the start-up guard (≤ 120 s) to both cohorts;
  the decrease comes long after it. While a node sits at its minimum, the radius pushsync uses
  follows the network's (`waitNetworkRFunc` returns max(network, minimum)), so live pushes of the
  sibling half keep going to the sibling's nodes: the decreaser gets the new half by sync only,
  which is what AC-V4 measures. After the event the merged pair's most common depth (r − 1)
  differs from the network's (r), so salud keeps the decreasers healthy.
- `t_dec` = Info `reserve radius decrease` per node. `t_union_half`: every node of the merged pair
  decreases, so none is a complete reference (their reserve sizes stay within ~0.1 % of each other
  throughout). Use an absolute target per node X of the pair of neighborhoods A and B, with T0 =
  just before the pair's first `t_dec` (Unix seconds; the `@` modifier needs Prometheus ≥ 2.33; do
  not restart the load generator between T0 and t):

  ```promql
  # T(t): both halves' medians at T0, plus the uploads into the pair since T0, minus X's expiries
    scalar(quantile(0.5, bee_localstore_reserve_size{neighborhood="A"} @ T0))
  + scalar(quantile(0.5, bee_localstore_reserve_size{neighborhood="B"} @ T0))
  + scalar(sum(runbook_uploads_total{neighborhood=~"A|B"}) - sum(runbook_uploads_total{neighborhood=~"A|B"} @ T0))
  - (bee_localstore_expired_count{node="X"} - bee_localstore_expired_count{node="X"} @ T0)
  ```

  `t_union_half` = the first time `bee_localstore_reserve_size{node="X"}` ≥ 0.999 × T(t). Confirm
  it with a plateau (from then on `deriv(bee_localstore_reserve_size{node="X"}[5m])` stays below
  1.5 × `sum(rate(runbook_uploads_total{neighborhood=~"A|B"}[5m]))`) and with §4.7 for the merged
  pair. Observe ≥ 1 h, plus 20 min for the rate windows.
- Also record: `bee_rangesync_stripes_total{result="reassigned"}` and the Debug lines
  `radius decreased` / `stripe reassigned` (K1), the number of covering peers (K5).

### 4.10 Batch-expiry event (S6, S7)

`B_drop`'s expiry at the end of the drain (§4.5) is this event. On its own: create a batch whose
amount gives it ~1 h of TTL (`batchTTL` in `GET /stamps/<id>`), upload 10–20 % of a
neighborhood's reserve into it at the reference rate (within the budget of §4.5), let it expire.
Watch ±30 min around the expiry (`bee_localstore_expired_count` jumps): panels F1–F3 and D5. If
the expiry takes the count within the radius below half the capacity, the reserve worker lowers
the radius at its next tick: then it is also a §4.9 (Method A) event, and is recorded as one.

### 4.11 Node-away catch-up (S9)

Under the reference load, stop one `rangesync`-only node for 5 min and, later, one for 15 min (at
15 uploads/s per neighborhood ≈ 4500 and ≈ 13500 missed IDs: decoded in sessions, below
`LargeDiff` 65536). Restart with the same flags. Time from the warm-up to `t_union`; read panels
B4, B6, C4, E3, E4.

---

## 5. Tooling

All three scripts were tested against a mock of the API (curl, jq ≥ 1.6, Python 3 stdlib), not
against a live node: run them on one node first.

### 5.1 `rs-poll.sh`: `/status`, `/redistributionstate`, address-index size

There is no Prometheus metric for `isFullySynced`, `isFrozen` or rounds skipped as "not fully
synced" (**missing**, T6.0 follow-up). This poller runs next to each node, appends a CSV row every
30 s and, with a node_exporter textfile directory, exports `runbook_*` gauges (not bee metrics).
`isFullySynced` in `/redistributionstate` is the value the agent set at the last phase change
(about every 38 blocks), so 30 s polling resolves it.

```bash
#!/usr/bin/env bash
# rs-poll.sh: poll one bee node every 30 s; append a CSV row and (optionally)
# write a node_exporter textfile. Needs curl, jq, du.
# usage: rs-poll.sh <api-url> <node> <cohort> <neighborhood> <data-dir> <out-dir> [textfile-dir]
set -u
API=$1 NODE=$2 COHORT=$3 NBHD=$4 DATADIR=$5 OUT=$6 TXT=${7:-}
mkdir -p "$OUT"
CSV="$OUT/$NODE.csv"
[ -s "$CSV" ] || echo 'ts,node,cohort,neighborhood,storageRadius,committedDepth,reserveSize,reserveSizeWithinRadius,pullsyncRate,connectedPeers,neighborhoodSize,isWarmingUp,isFullySynced,isFrozen,isHealthy,round,phase,lastPlayedRound,lastFrozenRound,lastSelectedRound,lastSampleDurationSeconds,indexstoreBytes' >"$CSV"
while :; do
  ts=$(date -u +%s)
  st=$(curl -fsS --max-time 10 "$API/status" 2>/dev/null) || st='{}'
  rd=$(curl -fsS --max-time 10 "$API/redistributionstate" 2>/dev/null) || rd='{}'
  idx=$(du -sb "$DATADIR/localstore/indexstore" 2>/dev/null | cut -f1)
  jq -nr --argjson s "$st" --argjson r "$rd" --arg ts "$ts" --arg n "$NODE" --arg c "$COHORT" \
    --arg h "$NBHD" --arg idx "${idx:-}" '
    [$ts, $n, $c, $h, $s.storageRadius, $s.committedDepth, $s.reserveSize,
     $s.reserveSizeWithinRadius, $s.pullsyncRate, $s.connectedPeers, $s.neighborhoodSize,
     $s.isWarmingUp, $r.isFullySynced, $r.isFrozen, $r.isHealthy, $r.round, $r.phase,
     $r.lastPlayedRound, $r.lastFrozenRound, $r.lastSelectedRound,
     $r.lastSampleDurationSeconds, $idx] | @csv' >>"$CSV"
  if [ -n "$TXT" ]; then
    jq -nr --argjson s "$st" --argjson r "$rd" --arg idx "${idx:-}" \
      --arg l "node=\"$NODE\",cohort=\"$COHORT\",neighborhood=\"$NBHD\"" '
      def g(name; v): if v == null or v == "" then empty
        else "runbook_\(name){\($l)} \(if v == true then 1 elif v == false then 0 else v end)" end;
      g("is_fully_synced"; $r.isFullySynced), g("is_frozen"; $r.isFrozen),
      g("is_healthy"; $r.isHealthy), g("last_played_round"; $r.lastPlayedRound),
      g("last_frozen_round"; $r.lastFrozenRound), g("last_selected_round"; $r.lastSelectedRound),
      g("last_sample_duration_seconds"; $r.lastSampleDurationSeconds),
      g("status_pullsync_rate"; $s.pullsyncRate), g("status_storage_radius"; $s.storageRadius),
      g("status_committed_depth"; $s.committedDepth), g("status_reserve_size"; $s.reserveSize),
      g("status_connected_peers"; $s.connectedPeers),
      g("status_neighborhood_size"; $s.neighborhoodSize),
      g("indexstore_bytes"; $idx)' >"$TXT/runbook_$NODE.prom.$$" &&
      mv "$TXT/runbook_$NODE.prom.$$" "$TXT/runbook_$NODE.prom"
  fi
  sleep 30
done
```

Count the skipped rounds per reason from the logs (not fully synced, unhealthy and frozen are
separate lines): `grep -oE 'skipping round because node is [a-z ]+' bee.log | sort | uniq -c`.

### 5.2 `rs-coverage.sh`: the D12 check

```bash
#!/usr/bin/env bash
# rs-coverage.sh: D12 check. Every full-node peer inside the radius of a
# rangesync-only node must run rangesync (cohort rangesync or dual).
# usage: rs-coverage.sh <inventory.csv>   (rows: node,api_url,cohort,neighborhood)
set -u -o pipefail
INV=$1
declare -A cohort_of
bad=0
while IFS=, read -r node api cohort _; do
  case $node in '#'*|'') continue ;; esac
  ov=$(curl -fsS --max-time 10 "$api/addresses" | jq -er .overlay) || { echo "UNREACHABLE $node"; bad=1; continue; }
  cohort_of[$ov]=$cohort
done <"$INV"
while IFS=, read -r node api cohort _; do
  case $node in '#'*|'') continue ;; esac
  [ "$cohort" = rangesync ] || continue
  r=$(curl -fsS --max-time 10 "$api/status" | jq -e .storageRadius) || { echo "UNREACHABLE $node"; bad=1; continue; }
  while read -r ov; do
    c=${cohort_of[$ov]:-unknown}
    case $c in rangesync|dual) ;; *) echo "VIOLATION $node (radius $r): in-radius full peer $ov is $c"; bad=1 ;; esac
  done < <(curl -fsS --max-time 10 "$api/status/peers" |
    jq -r --argjson r "$r" '.snapshots[] | select(.beeMode == "full" and .proximity >= $r) | .overlay')
done <"$INV"
[ $bad = 0 ] && echo "OK: every rangesync-only node's in-radius full peers run rangesync"
exit $bad
```

A full peer that is not in the inventory reads `unknown` (a node outside the experiment): it breaks
rule 1 as well.

### 5.3 `rs_probe.py`: load generator and AC-V1 propagation probe

In load mode, with `--inventory` and `--textfile`, it counts the successful uploads per inventory
neighborhood (a reference counts toward every inventory prefix it starts with) and writes
`runbook_uploads_total{uploader,neighborhood}` every 10 s for node_exporter's textfile collector
(scrape it with `honor_labels: true`, as in §1); the panels sum it by `neighborhood`. A restarted
generator starts again from 0, which `increase()` and `rate()` treat as a counter reset.

The probe defines the T6.0 follow-up "a propagation probe for AC-V1 on testnet". It uploads a
random chunk with `Swarm-Deferred-Upload: false` (the call returns once pushsync stored it), finds
the inventory nodes of the neighborhood the address falls in (prefix match on `neighborhood`), and
polls `HEAD /chunks/<ref>` on each (a local check: `ChunkStore().Has`, no retrieval) until all have
it or 120 s pass. It also reads `bee_rangesync_round_interval_seconds` from each node at the upload,
for AC-V1's bound. Output rows: `ref,t0,node,cohort,neighborhood,latency_s,round_interval_max`;
an empty latency means not seen within the timeout. After a radius decrease, set `neighborhood`
to the new (shorter) prefix in the inventory.

```python
#!/usr/bin/env python3
"""rs_probe.py: upload random single chunks; load mode or AC-V1 propagation probe.

load:  rs_probe.py load --uploader URL --batch ID|@FILE --rate R --duration S --out FILE
                      [--inventory FILE --textfile PATH --name NAME]
probe: rs_probe.py probe --uploader URL --batch ID|@FILE --inventory FILE --count N --out FILE

inventory: CSV rows "node,api_url,cohort,neighborhood" (neighborhood = the
binary prefix of the node's storage radius, e.g. 01). --batch @FILE re-reads
the batch ID from FILE every 10 s (for rotating batches). With --inventory and
--textfile, load mode writes runbook_uploads_total{uploader,neighborhood}
(successful uploads per inventory prefix) to PATH every 10 s, in the
node_exporter textfile format. Python 3 stdlib only.
"""
import argparse
import concurrent.futures as cf
import csv
import json
import os
import threading
import struct
import time
import urllib.error
import urllib.request


_batch = {'id': None, 'at': 0.0}


def batch_id(spec):
    if not spec.startswith('@'):
        return spec
    if time.time() - _batch['at'] > 10:
        with open(spec[1:]) as f:
            _batch['id'], _batch['at'] = f.read().strip(), time.time()
    return _batch['id']


def upload(uploader, spec):
    batch = batch_id(spec)
    data = os.urandom(4096)
    req = urllib.request.Request(
        uploader.rstrip('/') + '/chunks', data=struct.pack('<Q', len(data)) + data,
        method='POST', headers={'Swarm-Postage-Batch-Id': batch,
                                'Swarm-Deferred-Upload': 'false',
                                'Content-Type': 'application/octet-stream'})
    with urllib.request.urlopen(req, timeout=60) as r:
        return json.load(r)['reference']


def has_chunk(api, ref):
    req = urllib.request.Request(api.rstrip('/') + '/chunks/' + ref, method='HEAD')
    try:
        with urllib.request.urlopen(req, timeout=5) as r:
            return r.status == 200
    except (urllib.error.URLError, OSError):
        return False


def round_interval(api):
    try:
        with urllib.request.urlopen(api.rstrip('/') + '/metrics', timeout=5) as r:
            for line in r.read().decode().splitlines():
                if line.startswith('bee_rangesync_round_interval_seconds '):
                    return float(line.split()[1])
    except (urllib.error.URLError, OSError, ValueError):
        pass
    return None


def bits(ref):
    return ''.join(format(int(c, 16), '04b') for c in ref)


def read_inventory(path):
    with open(path, newline='') as f:
        return [r for r in csv.DictReader(f, fieldnames=['node', 'api', 'cohort', 'nbhd'])
                if not r['node'].startswith('#')]


def write_textfile(path, name, counts):
    tmp = f'{path}.{os.getpid()}'
    with open(tmp, 'w') as f:
        f.write('# TYPE runbook_uploads_total counter\n')
        for p, c in sorted(counts.items()):
            f.write(f'runbook_uploads_total{{uploader="{name}",neighborhood="{p}"}} {c}\n')
    os.replace(tmp, path)


def cmd_load(a):
    prefixes = sorted({r['nbhd'] for r in read_inventory(a.inventory)}) if a.inventory else []
    counts, lock = {p: 0 for p in prefixes}, threading.Lock()
    end, step, n, last = time.time() + a.duration, 1.0 / a.rate, 0, 0.0
    with open(a.out, 'a', newline='') as f, cf.ThreadPoolExecutor(a.workers) as ex:
        w = csv.writer(f)

        def one(t):
            try:
                ref = upload(a.uploader, a.batch)
                with lock:
                    for p in prefixes:
                        if bits(ref).startswith(p):
                            counts[p] += 1
                return [f'{t:.3f}', f'{time.time():.3f}', ref, '']
            except Exception as e:  # record and go on
                return [f'{t:.3f}', f'{time.time():.3f}', '', repr(e)]

        futs, start = [], time.time()
        while time.time() < end:
            t = start + n * step
            time.sleep(max(0.0, t - time.time()))
            futs.append(ex.submit(one, t))
            n += 1
            done = [x for x in futs if x.done()]
            for x in done:
                w.writerow(x.result())
                futs.remove(x)
            if a.textfile and time.time() - last > 10:
                with lock:
                    write_textfile(a.textfile, a.name, counts)
                last = time.time()
        for x in futs:
            w.writerow(x.result())
    if a.textfile:
        write_textfile(a.textfile, a.name, counts)


def cmd_probe(a):
    inv = read_inventory(a.inventory)
    with open(a.out, 'a', newline='') as f, cf.ThreadPoolExecutor(32) as ex:
        w = csv.writer(f)
        for _ in range(a.count):
            ref = upload(a.uploader, a.batch)
            t0 = time.time()
            nodes = [r for r in inv if bits(ref).startswith(r['nbhd'])]
            if not nodes:
                w.writerow([ref, f'{t0:.3f}', '', '', '', '', 'no inventory node in its neighborhood'])
                continue
            ri = [x for x in ex.map(lambda r: round_interval(r['api']), nodes) if x is not None]
            ri_max = max(ri) if ri else ''
            seen = {}
            while len(seen) < len(nodes) and time.time() - t0 < a.timeout:
                todo = [r for r in nodes if r['node'] not in seen]
                for r, ok in zip(todo, ex.map(lambda r: has_chunk(r['api'], ref), todo)):
                    if ok:
                        seen[r['node']] = time.time() - t0
                time.sleep(a.poll)
            for r in nodes:
                lat = seen.get(r['node'])
                w.writerow([ref, f'{t0:.3f}', r['node'], r['cohort'], r['nbhd'],
                            '' if lat is None else f'{lat:.2f}', ri_max])
            f.flush()
            time.sleep(a.gap)


p = argparse.ArgumentParser()
p.add_argument('mode', choices=['load', 'probe'])
p.add_argument('--uploader', required=True)
p.add_argument('--batch', required=True)
p.add_argument('--out', required=True)
p.add_argument('--rate', type=float, default=10.0)
p.add_argument('--duration', type=float, default=600.0)
p.add_argument('--workers', type=int, default=64)
p.add_argument('--inventory')
p.add_argument('--count', type=int, default=100)
p.add_argument('--timeout', type=float, default=120.0)
p.add_argument('--poll', type=float, default=0.5)
p.add_argument('--gap', type=float, default=5.0)
p.add_argument('--textfile')
p.add_argument('--name', default=os.uname().nodename)
args = p.parse_args()
cmd_load(args) if args.mode == 'load' else cmd_probe(args)
```

AC-V1 summary per cohort (bound per probe = min(2 × `round_interval_max`, 15 s), T6.1's two
bounds; 15 s alone for pullsync nodes, which have no round interval):

```bash
awk -F, '$3!=""{k=$1; c[k]=$4; ri[k]=$7; if($6=="") miss[k]=1; else if($6+0>m[k]) m[k]=$6+0}
END{for(k in c){x=c[k]; n[x]++; b=(ri[k]==""?15:(2*ri[k]<15?2*ri[k]:15)); if(!miss[k] && m[k]<=b) ok[x]++; if(m[k]>mx[x]) mx[x]=m[k]}
for(x in n) printf "%s probes=%d within_bound=%d max_s=%.2f\n", x, n[x], ok[x], mx[x]}' probe.csv
```

---

## 6. Dashboard: panels and PromQL

**Choice:** a panel list with PromQL here, not a Grafana JSON (Decisions log, "(T6.3 docs)
Dashboard as a PromQL panel list"). Each panel names the criterion it serves. Conventions:

- Labels as in §1 (`node`, `cohort`, `neighborhood`, `group`). `runbook_*` series come from §5.1.
- `$__range` = the segment being judged (Grafana's time range). Windows: `[5m]` for rates,
  `[10m]` where §9 reads "per 10-minute window", `[30m]` for slow ratios.
- Counters keep their exported names: `bee_rangesync_parts_sent`, `bee_rangesync_bytes_metadata`,
  `bee_rangesync_want_claims` and every `bee_pullsync_*` counter have no `_total` suffix.
- Histogram bucket edges limit what a quantile can resolve; where a threshold sits inside a bucket,
  the panel reads the bucket fraction instead (noted per panel).
- A metric with labels (`sessions_total`, `stripes_total`, `streams_exhausted_total`,
  `want_claims`, `fetch_skipped_total`, `parts_*`, `bytes_*`) has no series for a label value until
  its first event: an empty result reads 0, and every filter or join built on one needs an
  `or … * 0` fallback (STEADY in §9 has one). Histograms and plain counters exist from the start.
- PromQL that contains `|` is in code blocks below the tables (a `\|` inside a table cell is not
  copyable PromQL).

### Row A: inventory and mode

| # | Panel | PromQL | Expect |
|---|---|---|---|
| A1 | Build | `count by (cohort, version) (bee_info)` | one version |
| A2 | Mode | `count by (cohort) (bee_puller_pullsync_rate)` and `count by (cohort) (bee_rangesync_sync_rate)` | §2.2 |
| A3 | Radius, depth, mixed radii | `bee_localstore_storage_radius`; `runbook_status_committed_depth`; `max by (neighborhood) (bee_localstore_storage_radius) - min by (neighborhood) (bee_localstore_storage_radius)` | > 0 = mixed radii (K3, S3) |
| A4 | Neighborhood size | `runbook_status_neighborhood_size`; `runbook_status_connected_peers` | rule 5 |
| A5 | Reconciler peers by state | `sum by (cohort, state) (bee_rangesync_peers)`; per node: A5-T below | `state`: probing, unsupported, idle, reconciling, fetching, striped, removed |
| A6 | Initiated-session cap | `bee_rangesync_initiated_session_cap` | 8–16; S3 |

```promql
# A5-T: peers per node that do not sync with it
bee_rangesync_peers{state=~"unsupported|striped|removed"}
```

### Row B: sync progress and convergence

| # | Panel | PromQL | Serves |
|---|---|---|---|
| B1 | Catch-up rates | `bee_rangesync_sync_rate`; `bee_puller_pullsync_rate`; `runbook_status_pullsync_rate` (combined, `/status`) | AC-N1, AC-V3, AC-V4, S17 |
| B2 | Reserve size and spread | `bee_localstore_reserve_size`; `max by (neighborhood) (bee_localstore_reserve_size) - min by (neighborhood) (bee_localstore_reserve_size)` | `t_union` (§4.8, §4.9) |
| B3 | Chunk inflow by path (chunks/s) | `rate(bee_rangesync_fetch_delivered_total[1m])`; `rate(bee_pullsync_chunks_delivered[1m])`; `sum by (node, type) (rate(bee_puller_synced_chunks[1m]))`; `rate(bee_pushsync_storer[1m])` | AC-V3, AC-V4 |
| B4 | Sessions by result | `sum by (cohort, result) (rate(bee_rangesync_sessions_total[5m]))` (`result`: ok, busy, session_limit, stale, error; initiated round sessions); served Busy `sum by (cohort) (rate(bee_rangesync_session_busy_total[5m]))` | AC-C2, AC-V5, S3 |
| B5 | Round interval | `bee_rangesync_round_interval_seconds`; `min by (neighborhood) (…)`, `max by (neighborhood) (…)` | AC-V1, §9 |
| B6 | Messages per session | median ≤ 2: `sum by (cohort) (rate(bee_rangesync_session_rounds_bucket{le="2"}[30m])) / sum by (cohort) (rate(bee_rangesync_session_rounds_count[30m]))` ≥ 0.5; ≤ 4: same with `le="4"` | AC-V2. Buckets 1, 2, 4, 8, …: "p95 ≤ 3" is not resolvable; the panel reads p95 ≤ 4 |
| B7 | Stripes | `sum by (cohort, result) (increase(bee_rangesync_stripes_total[10m]))` (`result`: done, reassigned, timeout, dropped, idle); `sum by (cohort, le) (increase(bee_rangesync_verification_diff_bucket[10m]))` | AC-V3, AC-V4, K1, K2, S8 |
| B8 | Puller workers | `bee_puller_worker`; `rate(bee_puller_worker_iterations[5m])`; `rate(bee_puller_worker_errors[5m])`; `increase(bee_puller_max_uint_errors[1h])` | comparison |

### Row C: bandwidth

| # | Panel | PromQL | Serves |
|---|---|---|---|
| C1 | rangesync bytes | `sum by (cohort, dir) (rate(bee_rangesync_bytes_metadata[5m]))`; `sum by (cohort, dir) (rate(bee_rangesync_bytes_chunks[5m]))` | AC-B1, AC-B2, AC-V5 |
| C2 | pullsync metadata (baseline) | `102 * sum by (cohort) (rate(bee_pullsync_chunks_offered[5m]))`; responder side `rate(bee_pullsync_chunks_sent_offered[5m])`, `rate(bee_pullsync_chunks_sent_wanted[5m])`, `rate(bee_pullsync_chunks_sent[5m])`; `rate(bee_pullsync_chunks_wanted[5m])` | AC-B1 (102 B per offer, AC table §1), AC-S3 |
| C3 | Metadata per chunk uploaded into the neighborhood | CQ-R (rangesync's bytes) and CQ-P (pullsync's offers × 102 B) below; a dual neighborhood reports both, apart and summed | AC-B1 (R ÷ P), AC-B1b. Windows per §4.5 (≤ 6 h, no radius change) |
| C4 | Parts and Mores | `sum by (cohort, type) (rate(bee_rangesync_parts_sent[5m]))`; `sum by (cohort, type) (rate(bee_rangesync_parts_received[5m]))` (`type`: fingerprint, items, symbols, more); `sum by (cohort) (rate(bee_rangesync_more_requests_total[5m]))` | AC-K5, §9 |
| C5 | Reference load | `histogram_quantile(0.5, sum by (cohort, neighborhood, le) (rate(bee_rangesync_pairwise_diff_bucket[30m])))`; bucket check: `sum by (cohort, neighborhood) (rate(bee_rangesync_pairwise_diff_bucket{le="15"}[30m])) / sum by (cohort, neighborhood) (rate(bee_rangesync_pairwise_diff_count[30m]))` < 0.5 → median ≥ 16; with `le="31"` < 0.5 → median ≥ 32 | §4.5. Buckets 0, 1, 3, 7, 15, 31, …: "≥ 20" lies inside 16–31; a median in that bucket reads "at or near the reference load" (as in T6.1) |

```promql
# CQ-R: rangesync metadata per chunk uploaded into the neighborhood (AC-B1, the R cohort)
  sum by (cohort, neighborhood) (rate(bee_rangesync_bytes_metadata{dir="in"}[30m]))
/ on (neighborhood) group_left ()
  sum by (neighborhood) (rate(runbook_uploads_total[30m]))

# CQ-P: pullsync metadata per chunk uploaded (102 B per offer, acceptance-criteria.md §1)
  102 * sum by (cohort, neighborhood) (rate(bee_pullsync_chunks_offered[30m]))
/ on (neighborhood) group_left ()
  sum by (neighborhood) (rate(runbook_uploads_total[30m]))
```

### Row D: duplicates

Windows for D3 and D4 follow §4.5 (≤ 6 h, no radius change, inside one load batch's active
period). D0 is the overwrite check of §4.5; U is what a node should add by sync in the window:
the uploads into its neighborhood minus what pushsync stored on it (approximate at the window's
edges, where uploads from before the window are still syncing).

```promql
# D0, per node: ≈ 0 (within 1 % of the uploads); negative = overwrites or chunks never received
  sum by (node, neighborhood) (
      delta(bee_localstore_reserve_size[$__range])
    + increase(bee_localstore_evicted_count[$__range])
    + increase(bee_localstore_expired_count[$__range]))
- on (neighborhood) group_left ()
  sum by (neighborhood) (increase(runbook_uploads_total[$__range]))

# U, per node
  sum by (neighborhood) (increase(runbook_uploads_total[$__range]))
- on (neighborhood) group_right ()
  sum by (node, neighborhood) (increase(bee_pushsync_storer[$__range]))

# DUP-D, per dual node: deliveries - U (the corrected T6.1 formula); the ratio is DUP-D / on (node, neighborhood) U
  sum by (node, neighborhood) (
      increase(bee_pullsync_chunks_delivered[$__range])
    + increase(bee_rangesync_fetch_delivered_total[$__range])
    + increase(bee_rangesync_cross_protocol_duplicates_total[$__range])
    + increase(bee_pushsync_storer[$__range]))
- on (neighborhood) group_left ()
  sum by (neighborhood) (increase(runbook_uploads_total[$__range]))

# DUP-P, per pullsync node: the same with pullsync's deliveries only
  sum by (node, neighborhood) (
      increase(bee_pullsync_chunks_delivered[$__range])
    + increase(bee_pushsync_storer[$__range]))
- on (neighborhood) group_left ()
  sum by (neighborhood) (increase(runbook_uploads_total[$__range]))
```

| # | Panel | PromQL | Serves |
|---|---|---|---|
| D1 | rangesync deliveries | stored `rate(bee_rangesync_fetch_delivered_total[5m])`; already held `rate(bee_rangesync_cross_protocol_duplicates_total[5m])`; stamp failed `rate(bee_rangesync_fetch_rejected_total[5m])` | AC-D2, AC-D3 |
| D2 | AC-D2 (rangesync-only) | `sum by (cohort) (increase(bee_rangesync_cross_protocol_duplicates_total[$__range])) / (sum by (cohort) (increase(bee_rangesync_fetch_delivered_total[$__range])) + sum by (cohort) (increase(bee_rangesync_cross_protocol_duplicates_total[$__range])))` | ≤ 0.01 (draft). `chunks_fetched_total` was never added; stored + already held stands in for it |
| D3 | AC-D3 (dual) | DUP-D ÷ U (above) | the corrected T6.1 formula ("(T6.1 part 1 fix round 1 → T6.3, T6.4)"); rejected stamps are in neither term. Report next to D1's metric-only value, a lower bound (it misses rangesync-first, pullsync-in-the-same-page) |
| D4 | AC-D5 (pullsync) | DUP-P ÷ U (above) | informational |
| D5 | Want set | `sum by (cohort, result) (rate(bee_rangesync_want_claims[5m]))` (`result`: granted, denied, peer_full, full, rejected, closed, expired, late ⊂ expired); `bee_rangesync_rejected_cache_size`; `sum by (cohort, reason) (rate(bee_rangesync_fetch_skipped_total[5m]))` (`reason`: out_of_radius, in_reserve, outside_peer_root, no_credit, held, rejected, full, peer_full, closed) | AC-D2 (`late` 0), AC-C3, S5, S6 |

### Row E: decoding

| # | Panel | PromQL | Serves |
|---|---|---|---|
| E1 | Symbols per difference | `histogram_quantile(0.5, sum by (cohort, le) (rate(bee_rangesync_symbols_per_difference_bucket[30m])))`, same at 0.95 | AC-K2 (buckets 1.0–5.0 by 0.25: resolvable) |
| E2 | Corruption, mapping | `sum by (cohort, node) (increase(bee_rangesync_decode_corruption_total[$__range]))`; `sum by (cohort, node) (increase(bee_rangesync_mapping_mismatch_total[$__range]))` | AC-K3, AC-K4: 0 |
| E3 | Exhausted streams | `sum by (cohort, neighborhood, level) (increase(bee_rangesync_streams_exhausted_total[1h]))` (`level`: root, level1, bucket, deep) | AC-K5, §9 |
| E4 | Pairwise differences | heatmaps `sum by (le) (increase(bee_rangesync_pairwise_diff_bucket[10m]))` and `…pairwise_diff_half_bucket…`; max per 10-min window and crossings: §9 Q1, Q2 | AC-C2, AC-K5, §9 |

### Row F: peer quality and protection

| # | Panel | PromQL | Serves |
|---|---|---|---|
| F1 | Peer failures | `sum by (cohort, node) (increase(bee_rangesync_peer_failures_total[10m]))` (no reason label: the reason is in the Debug `peer failed` line, **missing** as a label) | S7, S10, K2 |
| F2 | Disconnects and drops | `increase(bee_rangesync_invalid_message_total[10m])`; `increase(bee_rangesync_stream_limit_exceeded_total[10m])`; `increase(bee_rangesync_status_rate_limited_total[10m])` | S7 |
| F3 | Fetch credit | `sum by (cohort, node) (increase(bee_rangesync_fetch_skipped_total{reason="no_credit"}[10m]))` | S5: honest neighbors 0 |
| F4 | Outside the root | `increase(bee_rangesync_index_outside_root_total[1h])` | edge peers, radius changes |
| F5 | Busy breakdown | §8, S3 | S3 |

### Row G: resources

| # | Panel | PromQL | Serves |
|---|---|---|---|
| G1 | Index memory | `bee_rangesync_index_memory_bytes`; `max_over_time(bee_rangesync_index_memory_bytes[$__range])` | AC-R1, AC-R2 |
| G2 | Index rebuild | per rebuild: `increase(bee_rangesync_index_rebuild_seconds_sum[$__range]) / increase(bee_rangesync_index_rebuild_seconds_count[$__range])` (exact when one rebuild falls in the range) | AC-R3 (buckets 20 and 40 bracket 30 s: use the mean) |
| G3 | Index apply | `histogram_quantile(0.99, sum by (cohort, le) (rate(bee_rangesync_index_apply_seconds_bucket[30m])))` | AC-R4 (report) |
| G4 | Reserve `Put` latency, mean | `sum by (cohort, node) (rate(bee_localstore_method_calls_duration_sum{component="reserve",method="Put"}[30m])) / sum by (cohort, node) (rate(bee_localstore_method_calls_duration_count{component="reserve",method="Put"}[30m]))` | AC-R4. Default buckets (first edge 5 ms) cannot resolve a median: compare means |
| G5 | CPU | `avg by (cohort) (rate(bee_process_cpu_seconds_total[30m]))`; per node | AC-R7 |
| G6 | Memory | `avg by (cohort) (bee_process_resident_memory_bytes)`; `avg by (cohort) (go_memstats_heap_inuse_bytes)` | AC-R8 |
| G7 | Goroutines | `avg by (cohort) (go_goroutines)` | AC-R9 |
| G8 | Address-index disk | `runbook_indexstore_bytes`; per chunk `runbook_indexstore_bytes / on(node) bee_localstore_reserve_size` | AC-R5 |

### Row H: redistribution

| # | Panel | PromQL / source | Serves |
|---|---|---|---|
| H1 | Fully synced | `avg by (cohort) (avg_over_time(runbook_is_fully_synced[$__range]))`; per node | AC-N5 (a), AC-V3 |
| H2 | Played ÷ selected | `sum by (cohort) (increase(bee_storageincentives_commit_phases[$__range])) / sum by (cohort) (increase(bee_storageincentives_neighborhood_selected[$__range]))` | AC-N5 (b) |
| H3 | Commit, reveal, claim | commits `increase(bee_storageincentives_commit_phases[$__range])` − `increase(bee_storageincentives_commit_phase_errors[$__range])`; reveals `…reveal_phases…` − `…reveal_phase_errors…`; `…claim_phases…`, `…winner…`, `…claim_phase_errors…`, `…is_playing_errors…`, `…insufficient_funds_to_play…` | AC-N5 (c) |
| H4 | Frozen | `max by (cohort, node) (max_over_time(runbook_is_frozen[$__range]))`; `changes(runbook_last_frozen_round[$__range])` | AC-N5 (d): 0 |
| H5 | Sample duration | `bee_storageincentives_reserve_sample_duration` (last sample, s); `runbook_last_sample_duration_seconds`; mean `sum by (cohort) (rate(bee_localstore_reserve_sample_duration_seconds_sum{status="success"}[6h])) / sum by (cohort) (rate(bee_localstore_reserve_sample_duration_seconds_count{status="success"}[6h]))` | AC-N5 (e) |
| H6 | Phase, round | `bee_storageincentives_current_phase`; `bee_storageincentives_round`; skipped-round log count (§5.1) | context |

---

## 7. What to watch, per criterion

Every row of acceptance-criteria.md whose "Measured by" names T6.3. Segments: §10. R = `rangesync`
cohort, P = `pullsync` cohort, D = dual. "Before/after" = the same node in Run 0 (pullsync-only)
and in Run 2, which removes host differences.

| Row | Segment | Read | Pass | Notes |
|---|---|---|---|---|
| AC-C2 quiescence | upload pause (§4.6) | E4: sessions with n_Δ > 0 after `t_pause` + 120 s, `increase(bee_rangesync_pairwise_diff_count[w]) - ignoring(le) increase(bee_rangesync_pairwise_diff_bucket{le="0"}[w])`; B4 `result="ok"`; §4.7 | no session with n_Δ > 0 starts after `t_pause` + 120 s on any R/D node; one `/rchash` per neighborhood | With mixed radii nested-root sessions continue with n_Δ = 0 (K3): gate on n_Δ, report the session count |
| AC-V1 2 × RoundInterval | Run 2, probe every 30 s | §5.3 summary | every probe ≤ min(2 × `round_interval_max`, 15 s) on R and D | gating per G7-4; P reported as the baseline; the sparse-load knife edge at 2 × 5 s (K8, T6.4 wording) |
| AC-V2 round trips | Run 2 steady windows (B7 shows no stripes) | B6 | fraction ≤ 2 ≥ 0.5; fraction ≤ 4 ≥ 0.95 | `session_rounds` = Messages this side handled per completed session (≈ round trips); no label separates catch-ups, so use steady windows |
| AC-V3 bootstrap | §4.8 | `t_union − t_start`, R vs P; also `t_synced`, `t_rate0` | R ÷ P ≤ 1.0 on `t_union` (margin: G7-5) | compare the time to the union, not to `sync_rate` 0: both rates read 0 10–20 min after their last chunk (K8) |
| AC-V4 radius decrease | §4.9 | `t_union_half − t_dec`, R vs P; B7 `reassigned` | R ÷ P ≤ 1.0 | expected misses: the moved-root race (K1, ~1.25× with 3 covering peers) and uneven stripes (K5); record which applied |
| AC-V5 idle | upload pause, after AC-C2 | B4; §9 Q8 | `increase(bee_rangesync_sessions_total{result="ok"}[10m])` = 0 on single-radius neighborhoods; ≈ 100 B per rangesync peer per minute (one Status per `StatusMaxInterval` to every connected peer that speaks rangesync, not only to neighbors), flag > 150 B (draft, T6.3) | fails with mixed radii (K3: ~3–8 sessions per minute on a node with nested-root peers) |
| AC-B1 metadata ≤ 50 % | Run 2 at the reference load; Run 0 for the same neighborhood's P baseline | C3: CQ-R of the R neighborhood ÷ CQ-P of the same neighborhood in Run 0 (or of a P neighborhood of the same N) | ≤ 0.50 at N ≥ 17, R cohort | D (informational): its CQ-R and CQ-R + CQ-P, each ÷ P; a dual node pays both protocols, so its sum cannot meet 0.50. In-process: N = 17 0.35–0.40 pass; N = 5 and 9 0.57–0.73 miss (re-basing is T6.4's, K6). On testnet each node also sends Statuses to its ~100+ non-neighbors, included here (it was outside the in-process clique) |
| AC-B1b low load | low-load segment | C3 | report | |
| AC-B2 bootstrap metadata | §4.8 | joiner's `bee_rangesync_bytes_metadata{dir="in"}` at `t_union` (a new node starts at 0) minus the neighbors' mean `increase(…{dir="in"}[t_start, t_union])` | IDs ≤ 1.15 × 70 B × the joiner's `bee_localstore_reserve_size` at `t_union` | the 1 MB verification part cannot be separated under load (round sessions add ~7 KB/s per node at N = 9): report "not separable"; T6.1 measured 147–196 B |
| AC-D2 | Run 2, R | D2 | ≤ 1 % (draft); `increase(bee_rangesync_want_claims{result="late"}[$__range])` = 0 (draft, T6.3) | the counter also catches pushsync racing a fetch (an upper bound) |
| AC-D3 | Run 2, D | D3 (and D1's metric-only lower bound) | report | windows per §4.5 |
| AC-D5 | Run 2 and §4.8, P | D4 | report | windows per §4.5 |
| AC-K2 | Run 2 | E1 | median ≤ 2.0, p95 ≤ 4.0, unfiltered | "≥ 20 differences" cannot be filtered (no size label): indicative only (K7) |
| AC-K3 | every segment | E2 | 0 on every node | |
| AC-K4 | every segment | E2 | 0 on every node | D22 |
| AC-K5 | every segment | C4, E3, E4 | report; the Q9 part per §9 | |
| AC-R1 index memory | Run 2 | G1 | ≤ 22 MB steady (in-process 21.0 MB, the slab at `LeafBits` 12) | the metric counts slabs only; session buffers show in AC-R8 |
| AC-R2 rebuild memory | §4.9 and every start | G1 max over the rebuild | ≤ 2 × the steady value (in-process 42.0 MB), for ≤ the rebuild's duration (G2) | a 15-s scrape catches a ~26 s rebuild at 4.2M |
| AC-R3 rebuild time | every start and §4.9 | G2 | ≤ 30 s at ~4.2M | the storer-iterator rebuild on a real disk (phase-4: "worth reading off T6.3") |
| AC-R4 `Put` latency | Run 2 vs Run 0, same node | G4; G3 | mean ≤ +5 % (draft); report `index_apply_seconds` p99 | median not resolvable (G4) |
| AC-R5 address-index disk | §4.2, in an upload pause | (B2 − B0) ÷ n0 (§4.2 step 1) | ≤ 100 B per chunk | also report (B1 − B0) ÷ n0, right after the fill (G4: ≈ 150 B uncompacted). Around the restart only: under load the other indexes grow by hundreds of MB in half a day |
| AC-R7 CPU | Run 2 (before/after; R vs P cohorts); §4.8 (joiner vs joiner) | G5 | ≤ 1.0 × (draft) | in-process it ran on in-memory storers; this is the on-disk measurement |
| AC-R8 RSS | Run 2, before/after | G6 | Δ ≤ +64 MB (draft) | |
| AC-R9 goroutines | Run 2, before/after | G7 | R ≤ P | |
| AC-N1 sync rate | §4.8, §4.9, Run 2 | B1 | > 0 from `t_start` to `t_union` (`min_over_time`); back to 0 within 20 min after `t_union` and no later than the puller's; `max_over_time(bee_rangesync_sync_rate[steady])` = 0 | window wording: `pkg/rate` reads 0 10–20 min after the last chunk (K8). After `t_dec` it reads 0 until the next round (0.5–5.1 s, K4). S1 pins it on purpose |
| AC-N2 no early decrease | §4.8 | A3 on the joiner; Info `reserve radius decrease` | no decrease on the joiner before `t_union` | the worker ticks every 15 min, the guard lasts ≤ 120 s |
| AC-N5 redistribution | Run 2, ≥ 10 selections per cohort | H1–H5; skipped-round log count | (a) R ≥ P; (b) R ≥ P; (c) R ≤ P; (d) 0 freezes; (e) R within ±10 % of P (draft) | the joiners of §4.8 add a per-node view; §4.7 runs separately |
| AC-S4 rollback | §4.3, §4.4 drills, in an upload pause | §2.2; `du` and n per chunk; H1 | §4.3's pass line | per chunk, not absolute: the reserve has grown since the upgrade |
| AC-S6 audit | — | — | not measurable | no Phase 5 (G7-6) |

Also readable on the way (not T6.3 rows): AC-N4 on D nodes (S17), AC-B3
(`bytes_chunks{dir="in"}` ÷ the joiner's stored chunks), AC-N3 (one decrease per tick, none while
B1 > 0).

---

## 8. Special items handed to T6.3

Source entries are in the phase files (Appendix A maps each one). "Not runnable" = needs a
modified (adversarial) node; T6.4 lists those as not exercised.

| # | Item | Procedure | Read | Expected / decision |
|---|---|---|---|---|
| S1 | **Sustained overwrite stream** (phase-4 "(T4.6 part 3 fix round 2 → T6.3) A sustained overwrite stream") | One mutable batch, depth 17 (the API's minimum; 2 slots per bucket), uploads at stepped rates per neighborhood (the total is the rate × 2^r, §4.5): 10, 25, 50, 100, 200 IDs/s, each held 30 min (≥ one 10-min window aligned to Unix time, plus the rate's lag), then 30 min at 0. Overwrites start once the neighborhood's 2^(17−r) slots are used. Note N and A6 per neighborhood | §9 Q1/Q2 per step; B1 (`sync_rate`), A3 (radius), H1 (`isFullySynced`), E4 | Crossing (G6): ~50–100 IDs/s per neighborhood at N = 17 with 8 slots, higher with the per-node cap, lower at N > 17. The lowest step where Q1 > 0 in steady windows and B1 stays > 0 is the crossing; record it. It decides how urgent the load-relative catch-up rule is (before mainnet). The pin costs an attacker only pushsync bandwidth (phase-4 note 7) |
| S2 | Ordinary-load `CatchUpMinDiff` check (Q9; phase-3 T3.5; phase-4 Q9 entries) | Run 2 | §9 Q1, Q2 | §9 rule |
| S3 | **Initiated-session cap and the mixed-radius Busy breakdown** (phase-4 "(T4.7 part 2 → T6.3)", "(T4.7 part 2 fix round 1 → T6.3)", T4.7 part 2 gauge) | Run 2 per neighborhood size N; neighborhoods with mixed radii (after §4.9, or naturally) | S3-Q below the table, with A6 (cap), A3 (radius), A4 (N) | Harness, one radius: 1–3 % at N = 18, 3–6 % at N = 21, 5–10 % at N = 25 (~8 % at 33). If the deeper nodes' sessions draw most Busy answers: size the cap by the targets' peer counts. If Busy at N ≈ 20–24 runs above N = 25's: a steeper slope (two slots per peer past the tokens). AC-B1 at these N (S19) |
| S4 | **Claims' first-chunk window and queue wait** (phase-6 "(T4.7 part 3 → T6.1/T6.3)"; phase-4 T4.7 part 3/4 entries; catch-up first chunk, T4.5 part 2c) | §4.8, §4.9, §4.11 with Debug on `^node/rangesync` | B1 reading 0 while B2 shows the node behind; Debug `stripe claim ended: it stored nothing`; `stripes_total{result="idle"}` | Partial: **missing** a metric (or log) for a claim's queue wait and first-chunk latency, and for a catch-up fetch's first chunk (2-s `catchUpGrace`); report B1 dips with B2 lagging as the symptom |
| S5 | **Credit parameters** (phase-4 "(T4.7 part 4 → T6) Credit parameters…", newcomer, pool, refund entries) | Run 2, §4.8 (a newcomer), §4.11 | F3 per node; on the joiner the first minutes of D5 `no_credit` | Honest neighbors: 0 `no_credit` in steady state. A newcomer's second concurrent fetch is refused for ≈ 1 s (expected, small). Record how long the joiner waits for its first chunk. "How often honest peers hit 0" needs a per-peer view: **missing** (phase-4 follow-up). The refund (2) is decided on these data |
| S6 | **Chain-state rejections, Q7 under batch expiry** (phase-4 "(T4.7 part 4 fix round 1 → T6) Chain-state rejections…", T4.6 Q7, the honest peer tripped at a batch expiry) | §4.10 | D1 `fetch_rejected_total`; D5 `rejected`; F1–F3 around the expiry; `increase(bee_localstore_expired_count[1h])` vs `maxRemovals` 65536 | Honest neighbors: no `no_credit`, no disconnect. A large expiry with a radius eviction at the same time can flush the removal record (> 65536); report. A metric for excused/charged rejections is **missing** (phase-4 follow-up) |
| S7 | **Shortfall offenses and violations** (phase-4 "(T4.5 part 2b → T6)", "(T4.5 part 2b fix round 1 → T6)") | §4.10 and §4.9 (radius changes), Debug on `^node/rangesync` | Debug `fetch shortfall, peer scored`; F2 `invalid_message_total` with the Debug `invalid message … requested ids not delivered` | Honest peers: none. One around an expiry or a radius change calls for a wider excuse |
| S8 | **Stall chains past the own-job grace; an honest claim with a stalling stripe peer** (phase-4 "(T4.7 part 3 fix round 1 → T6.3) Stall chains…", "(T4.7 part 3 → T6.3) An honest claim…") | Optional, on a non-staking node: during §4.8, `kill -STOP` one of the joiner's stripe peers for 20 min (> `StripeTimeout` 15 min), then `kill -CONT`. Which peers are stripe peers: `PlanStripes` makes 2^s ≥ k stripes and gives stripe i to peer i mod k, so every full peer whose root covers the joiner's serves at least one; its `rate(bee_rangesync_bytes_chunks{dir="out"}[1m])` rises after `t_start`. No log or metric names a stripe's peer before it fails (the Debug `stripe` line with `peer_address` comes only with an error) | B7 `timeout`, `reassigned`; B1 on the joiner; B2 | A stall chain reads 0 from 19 min (`ownJobGrace`) after the last own-job chunk. A stopped peer may also be dropped by the transport; record what happened. A verifier picked for most stripes stalling ((stripes − 1) × 15 min) is not reproducible this way |
| S9 | **Large session decode on the real clock** (phase-6 "(T6.1 part 2 fix round 1 → T6.3 or a later harness run)") | §4.11 | B4, B6, C4 (Mores), E3 (exhausted by level), E4 (`pairwise_diff_half` ≥ 1024 buckets); time to `t_union` | sessions decode thousands of IDs: `SessionSymbolBudget` (16384) and `More` requests at work; one session's fetch of thousands of IDs at 250 chunks/s per holder |
| S10 | **Stripe verification false positives** (K2; phase-6 "(T6.1 part 2 → T4/T6.4, finding)", "→ before T6.3") | §4.8 twice if G7-1 accepts K2: once at < 10 uploads/s per neighborhood, once at the reference load | F1 on the joiner; B7 `verification_diff`; Debug `peer failed` reasons | expected: none at < 10/s, honest stripe peers scored (`stripe incomplete at verification`) at ≥ 15/s |
| S11 | Moved-root race at scale (K1) | §4.9 | B7 `reassigned` within seconds of `t_dec`; Debug `stripe reassigned … stream reset` right after `radius decreased` | count the decreases it hits |
| S12 | In-sync nested roots (K3; phase-6 "(T6.1 part 2 → T4/T6.3, Low)") | Run 2 neighborhoods with mixed radii | B4 `result="ok"` per node during idle; E4 n_Δ = 0 sessions | report `sessions_total` per cohort with mixed radii |
| S13 | AC-V1 propagation probe (T6.0) | §5.3 | §7 AC-V1 | |
| S14 | `isFullySynced` polling (T6.0) | §5.1 | H1 | |
| S15 | **Older-version refunds** (phase-4 "(T4.7 part 4 fix round 2 → T6) Refunds for older versions are uncapped") | S1 (overwrites), Debug on `^node/rangesync` | count Debug `overwrite newer chunk` lines per `peer_address` per hour | the honest rate sets a future per-peer refund cap; a metric is **missing** (phase-4 follow-up) |
| S16 | Index rebuild on a real node (phase-4, pre-T4.3 check) | every start | G2 | AC-R3 |
| S17 | Combined `SyncRate` with the real puller (phase-4 "(T4.5 part 1) Dual mode with the real puller"; AC-N4) | D nodes, §4.8 and Run 2 | B1: `runbook_status_pullsync_rate` vs `bee_puller_pullsync_rate + bee_rangesync_sync_rate` | equal (sampling skew aside); 0 only when both are 0 |
| S18 | Address index on a real disk, downgrade sizes (T6.2) | §4.2, §4.4 | G8, `du` | AC-R5; 350–600 MiB at 4.2M |
| S19 | AC-B1 at N ≥ 17 and N ≈ 20 (phase-4 "(T4.7 part 2 fix round 1 → T6.0, AC-B1)") | Run 2 | C3 per neighborhood with A4 | in-process with the continuous cap: N = 18 45.8–46.4 %, 21 46.0–48.5 %, 25 42.8–44.4 % |
| S20 | AC-V4 and the stripe plan at scale (phase-6 "(T6.1 part 2 → T4/T6.4) Stripe plans…") | §4.9 | covering peers (k), B7 per peer | with k not a power of 2 one peer serves 2 stripes; at 2^22 with ≥ 4 peers the node's 1000/s limit binds first. Decide whether it matters |
| S21 | Bootstrap tail (phase-6 "(T6.1 part 2) … The tails of the two claimed transfers") | §4.8 under the reference load | B7, D5 on the joiner after `t_union` | the in-process static network had no tail; record its size and duration |

```promql
# S3-Q, opener side: Busy and session-limit answers per completed initiated session
  sum by (node) (rate(bee_rangesync_sessions_total{result=~"busy|session_limit"}[30m]))
/ sum by (node) (rate(bee_rangesync_sessions_total{result="ok"}[30m]))

# S3-Q, serving side (approximate: session_rounds counts every completed session of the node)
  rate(bee_rangesync_session_busy_total[30m])
/ (rate(bee_rangesync_session_busy_total[30m]) + rate(bee_rangesync_session_rounds_count[30m]))
```

Not runnable with stock builds (record as not exercised):

- a claimant that drips (one stored chunk per `claimIdleTimeout` holds the stripe worker; ≈ 720 chunks a day per neighborhood);
- the one-chunk-then-reset pin (one real chunk per `SyncRateWindow`, 2 IDs per cycle);
- a Sybil majority triggering `IsBootstrap` (threshold 0 on served sessions; `SyncRate` held one `ownJobGrace`);
- a flood of claims ahead of a no-credit claimant (≤ 10 × (n + 1) minutes);
- fresh junk below half a fetch, unknown-batch junk.

---

## 9. G6 confirmations: `CatchUpMinDiff`, caches, `RoundInterval`

From phase-4's "G6 decision sheet (final)", last column. "Steady window" = a 10-minute window in
which the neighborhood ran no stripes and changed no radius:

```promql
# STEADY: a series per neighborhood, present in steady windows only. stripes_total has no series
# before a node's first stripe (none at all since the Run 1 restarts, typically): the "or" adds 0s.
(
    sum by (neighborhood) (increase(bee_rangesync_stripes_total[10m]))
  or
    sum by (neighborhood) (bee_localstore_storage_radius) * 0
) == 0
and on (neighborhood)
  (sum by (neighborhood) (changes(bee_localstore_storage_radius[10m])) == 0)
```

`pairwise_diff_half` buckets: 0, 1, 4, 16, 64, 256, 1024, 4096, 16384, 65536, 262144. A side is a
catch-up at ≥ 256 wants (`CatchUpMinDiff`).

**Queries**

- **Q1, crossings per node and window** (sessions whose side listed more than 256 missing IDs):
  `increase(bee_rangesync_pairwise_diff_half_count[10m]) - ignoring(le) increase(bee_rangesync_pairwise_diff_half_bucket{le="256"}[10m])`,
  then `and on (neighborhood) STEADY`. The `(64, 256]` bucket also holds exactly 256, which is a catch-up:
  read it as "at the threshold".
- **Q2, the max per 10-minute window** (the upper edge of the highest non-empty bucket):
  `histogram_quantile(1, sum by (cohort, neighborhood, node, le) (increase(bee_rangesync_pairwise_diff_half_bucket[10m])))`.
- **Q3, exhausted streams**: E3, split by A3 (mixed radii or not), in steady windows.
- **Q4, catch-up sizes**: `sum by (cohort, neighborhood, le) (increase(bee_rangesync_pairwise_diff_half_bucket{le=~"1024|4096|16384|65536|\\+Inf"}[1d]))`,
  outside §4.8/§4.9 events. The root splits past ~2558 missing (bucket `(1024, 4096]` and up), the
  level-1 ranges past ~5000 (`(4096, 16384]` and up).
- **Q5, Mores** per completed session: `sum by (cohort) (rate(bee_rangesync_more_requests_total[1h])) / sum by (cohort) (rate(bee_rangesync_session_rounds_count[1h]))`;
  per upload (G6's measure): `sum by (neighborhood) (rate(bee_rangesync_more_requests_total[1h])) / on (neighborhood) sum by (neighborhood) (rate(runbook_uploads_total[1h]))`, in steady windows.
- **Q6, round interval**: `quantile_over_time(0.5, bee_rangesync_round_interval_seconds[1h])`;
  share of time at the minimum `avg_over_time((bee_rangesync_round_interval_seconds <= bool 5)[1h:30s])`.
- **Q7, convergence**: the AC-V1 probe (§5.3).
- **Q8, idle heartbeats** (B per rangesync peer per minute, in the upload pause):
  `60 * rate(bee_rangesync_bytes_metadata{dir="out"}[10m]) / on(node) sum by (node) (bee_rangesync_peers{state!="unsupported"})`.

**Decision rules**

| Constant (G6 value) | Keep if | Otherwise |
|---|---|---|
| `CatchUpMinDiff` 256 | Q1 = 0 in every steady window of every R and D neighborhood at the ordinary and reference loads, and Q2 ≤ 256 (G6's wants max at the reference load, 35–82, reads 64 or 256 here) | Do **not** raise it to a fixed 1024 (a fixed value only moves the crossing with the load). Record the load per neighborhood, N, A6 and Q2 at each crossing and hand them to the load-relative catch-up rule (phase-4 "(T4.6 part 3 fix round 2 → T4.x or T6, before mainnet)"). S1 measures the crossing load itself. `pairwise_diff` (n_Δ, both sides) p99 against 255, T3.5's first rule, is reported only: Q9 reads `pairwise_diff_half` |
| Caches `RootSymbols` 4096, `Level1Symbols` 512 | Q3 `level="root"` and `"level1"` stay 0 in steady windows (G6 bound: first batch ≤ 777 symbols at 100 uploads/s); they may rise only with Q4 catch-ups past the split sizes | Root exhaustion in steady windows contradicts the G6 bound: investigate. If Q4 shows catch-ups past ~2500 or ~5000 often (draft trigger: more than one per node per hour outside bootstraps and radius changes), doubling `RootSymbols` (+0.36 MB, first split to ~5100) or `Level1Symbols` (+0.7 MB, second split to ~10,000) is the cheap lever: a team decision |
| `BucketSymbols` 32 | Q3 `level="bucket"` stays 0 in steady windows of single-radius neighborhoods; in mixed-radius neighborhoods, below the draft trigger: exhausted bucket-level streams in ≤ 10 % of the deeper nodes' completed sessions (`sessions_total{result="ok"}`) | Above it: the "caches for mixed radii" change (a cache for the root's half ranges, 2 × 4096 symbols, ~0.7 MB; phase-4 "(T4.6 part 3 → T6, a radius-aware harness)"), which G6 listed as a change after G6 |
| `RoundInterval` 5–30 s | AC-V1 holds (Q7 ≤ min(2 × the largest round interval, 15 s)), or misses only at sparse/idle loads with 2 × 5 s < latency ≤ 15 s (G6's known case: 10.75–10.95 s; a T6.4 wording item); Q8 ≈ 100 B per rangesync peer per minute | Probes past 15 s in steady state at the reference load: try a lower `MinRoundInterval` (G6: 2.5 s gave −1.4 s of mean convergence at N = 9 for +50 % rounds). A higher maximum (60 s) saves only heartbeats |
| request-driven `More`; hint(R) (keep both) | Q5 per upload in steady windows at most 2 × the in-process figure at the same N (draft, T6.3), which with `GrowthFactor` 2, `BatchFactor` 1.6 and the hint is 0.36 at N = 9 and 1.19–1.28 at N = 17 (phase-4, T4.6 part 3 fix round 1 and T4.7 part 2) | Near the no-hint figure (4.0 per upload at N = 17) the hints are not working: check the hint LRU (phase-4 follow-ups on flushing it) before the constants. After §4.11 the stale catch-up hint shows once per pair as one large `Symbols` batch (≤ 4095 symbols, ~336 KB): report it |
| `GrowthFactor` 2 (landed in T4.7) | Q5 and the share (C3) within the in-process range | report; all nodes run the same build, so the mixed GF2/GF4 case does not arise |
| `LargeDiff` 65536 | bootstrap stripes complete (B7 `done`, no `timeout`), AC-B2 holds | report stripe counts and sizes (2^22 ÷ peers: 2^17–2^19 IDs with 5–32 peers) |
| Initiated-session cap | S3 | S3's two rules |

Draft triggers above are T6.3's, for the team at G7-5.

---

## 10. Comparison-run plan

### 10.1 Runs

| Run | What | Duration | Covers |
|---|---|---|---|
| S | Seed per §4.5 (only where neighborhoods do not already hold 0.6 C): `B_keep` and `B_drop`, 0.3 C each per neighborhood, pullsync-only, several uploaders | ≈ 20M chunks at r = 3: ≈ 14 h at 400/s | the precondition of AC-N5 and `t_synced` (§3 rule 8) |
| 0 | Every node pullsync-only (defaults), same build, the load of Run 2 | ≥ 24 h | the before/after baseline per node: AC-B1 (same neighborhood), AC-R4, AC-R7–R9, AC-N5 history |
| 1 | Upgrade per §4.2 (P pairs stay as they are) | ~½ day | AC-R3, AC-R5, AC-R6 (fill times), S16, S18 |
| 2 | Steady state at the reference load; one 30-min upload pause every 6 h (§4.6); one 2-h low-load segment; probe every 30 s; §4.7 once per day | ≥ max(24 h, the rounds for 10 selections per cohort: ~10 × 2^`committedDepth` rounds of 152 blocks) | AC-C2, AC-V1, AC-V2, AC-V5, AC-B1, AC-B1b, AC-D2, AC-D3, AC-D5, AC-K2–K5, AC-R1, AC-R4, AC-R7–R9, AC-N1, AC-N5; S2, S3, S12, S17, S19, §9 |
| 3 | Bootstrap (§4.8), one joiner per cohort at once | 3 h + 2 rounds; twice if S10 runs | AC-V3, AC-B2, AC-N1, AC-N2, AC-R7 (bootstrap); S4, S5, S10, S21 |
| 4 | Radius decrease (§4.9), one sibling pair per cohort | ≥ 1.5 h after the restarts | AC-V4, AC-R2, AC-N1; S11, S20, S7 |
| 5 | Node-away catch-up (§4.11): 5 min, then 15 min | 2 h | S9, S4 |
| 6 | Sustained overwrite stream (S1) | 3 h (5 steps × 30 min + cool-down) | S1, S15 |
| D, 7 | Drain (§4.5): 12 h at the low load; `B_drop` expires at its end, observed as §4.10 | 12 h + 2 h | S6, S7; the precondition of Run 4 |
| 8 | Optional stall (S8) | during a Run 3 repeat | S8 |
| 9 | Rollback and downgrade drills on 2 R nodes (§4.3, §4.4) | 2 h | AC-S4 |

Order: S, 0, 1, 2, 3, 5, 6, D/7, 4; 8 and 9 where they fit. About 5–7 days of wall clock with
Runs 0 and 2 at 24 h, plus Run S. The load batches rotate throughout (§4.5). Runs 3 and 5 need the
reference load running; Run 6 changes the load, so keep it apart from the AC windows. Run 4 needs
the drain first and ends AC-N5 for the neighborhoods it leaves below half: run it last.

### 10.2 Every run

- Poller (§5.1) on every node; Prometheus retention ≥ the whole campaign; logs kept (Debug where
  §2.3 says).
- `rs-coverage.sh` before each run and after each restart wave.
- Note the time of every event (restarts, pauses, steps, expiries) in UTC: the windows of §7 are
  cut from them.

### 10.3 What to record in the phase-6 Decisions log

One entry per run, "(T6.3 run <n>, <date>)":

```
- **(T6.3 run <n>, <UTC dates>) <title>.**
  - Setup: build <commit>, network <id>, hosts (CPU, RAM, disk, bandwidth), amd64 check, flags per
    cohort, fetch cap (G7-2) or none, neighborhoods per cohort with N, radius, reserve size,
    `--minimum-storage-radius`, load per neighborhood (uploads/s), batch depth and TTL.
  - Events: <UTC times>.
  - Results per row: | Row | R | D | P | threshold | verdict | (pass / fail / needs work / not measurable).
  - Special items: S<n>: numbers, decision taken or proposed.
  - G6 confirmations (§9): the Q1/Q2 values per neighborhood and the decision per constant.
  - Incidents and deviations from this runbook; known issues (§11) that showed, with their counts.
```

---

## 11. Known issues before the runs

How each one shows on the dashboard, so that it is not read as a new failure.

| # | Issue (source) | Rank | What it looks like |
|---|---|---|---|
| K1 | **Stripe sessions race the moved-root Status** after a radius decrease: stripe sessions can open before the stripe peer has the decreaser's new root; the peer resets the stream, the stripe is reassigned, one peer serves 3 of 4 stripes at 250 chunks/s (phase-6 "(T6.1 part 2 fix round 1 → before T6.3) Two findings rank Medium") | **Medium**, unfixed; G7-1 | B7 `stripes_total{result="reassigned"}` +1–2 within seconds of `t_dec`; Debug `stripe reassigned … reconcile: read: … stream reset` right after `radius decreased`; the new half ≈ 1.25× the puller's time with 3 covering peers: AC-V4 fails in the decreases it hits (~3/8 in-process) |
| K2 | **Stripe verification scores honest deliverers under uploads**: chunks uploaded into a stripe after its listing count against the deliverer; the tolerance (max(4, 1 % of the stripe)) holds only below ~10 uploads/s per neighborhood, whatever the reserve size (same entry) | **Medium**, unfixed; G7-1 | F1 `peer_failures_total` rising on bootstrapping or striped nodes at the reference load (15/s); B7 `verification_diff` in the 16–64 buckets; Debug `peer failed … stripe incomplete at verification`; honest peers' scores rise above `ScoreThreshold` 3 (deprioritized, no disconnect, −1 per 10 min). Makes F1 useless for telling withholding from honest service while it lasts |
| K3 | In-sync peers with nested roots keep running sessions (mixed radii) | Low | B4 `result="ok"` ~3–8 per minute on a node with nested-root peers while idle (in-process: 8.0 for a radius-1 node with 4 radius-2 peers, 3.4 for the decreaser), each with n_Δ = 0 (E4 bucket 0), ~50 B each; AC-V5 fails with mixed radii |
| K4 | The reconciler's rate lags a radius decrease: 0 until the round that tracks the new root | latent | B1 R reads 0 for 0.5–5.1 s after `t_dec` while P's rises within ~0.1 s; harmless at the 15-min worker tick |
| K5 | Stripe plans load peers unevenly when their count is not a power of 2 | Low | in §4.9 with 3 (5–7, 9–15) covering peers, one peer serves 2 stripes (B7 per peer): the new half takes up to 1.5× (3 peers) or 1.1–1.9× (9–15 peers) of an even split, unless the node's 1000/s limit binds first |
| K6 | AC-B1 misses at small N (in-process N = 5 and 9: 0.57–0.73) | criteria (T6.4) | C3 ratio above 0.5 in N < 17 neighborhoods |
| K7 | AC-K2 cannot be filtered to ≥ 20 differences | criteria | E1 is over all decoded streams (in-process medians ≤ 2–3, p95 ≤ 3–4.75, a tail past 5) |
| K8 | Rate windows: `pkg/rate` sums the current and previous 10-min buckets aligned to Unix time, so a rate reads 0 10–20 min after its last chunk; two nodes finishing in one bucket read 0 at the same instant. AC-V1 at the 5-s minimum is a knife edge at sparse loads | criteria (T6.4) | B1 drops to 0 ~16 min after the last catch-up chunk for both protocols; probes at 10–11 s in quiet periods |
| K9 | `cross_protocol_duplicates_total` undercounts on dual nodes (rangesync stores first while pullsync's delivery is in the same page) | known | D1 below D3; use D3 |
| K10 | Sustained-overwrite pin: past ~50–100 IDs/s per neighborhood steady-state sides count as catch-ups and keep `SyncRate` > 0 | known, fix before mainnet | B1 > 0 and H1 0 during S1's high steps; lifts within the window after the stream stops |
| K11 | `bee_init_full_sync_duration_minutes` always observes 0 (`pkg/node/node.go`: `pullSyncStartTime` is set to the same `t` it is subtracted from); outside this plan | pre-existing | use the timestamp of the Info `full sync done` line; its `duration` field is 0 as well |
| K12 | No completion signal for the reclaim's compaction (§4.4) | known | watch `du` |
| K13 | Rebuild memory is 2 × the slab for the rebuild's duration (§5.3 of the proposal counts one) | known (AC-R2) | G1 at ~42 MB for ~26 s at each rebuild |
| K14 | A newcomer's second concurrent fetch is refused until its first Want settles | Low | a few `no_credit` (F3) on a joiner's first seconds |
| K15 | Residuals not exercised by honest runs (§8, not runnable) | Low–Medium | none expected on an honest testnet |
| K16 | Load-induced test flakes (`TestSyncRateHonestChurn/radius_half`, `TestRangesyncToggle` "peer busy") | test-only | none on testnet |

---

## Appendix A: handed items → where covered

| Source entry (phase file) | Covered in |
|---|---|
| phase-6 "(T6.0) T6.3: no Prometheus metric exposes `isFullySynced`…" | §5.1, H1, S14 |
| phase-6 "(T6.0) T6.3: define a propagation probe for AC-V1" | §5.3, S13 |
| phase-6 "(T6.2 fix round 1) T6.3, runbook …, corrected" (supersedes "(T6.2) T6.3, runbook") | §4.4 |
| phase-6 "(T6.1 part 1 → T6.3, T6.4) `cross_protocol_duplicates_total` undercounts" and "(T6.1 part 1 fix round 1 → T6.3, T6.4) The dual cohort's duplicate formula, corrected" | D3, K9 |
| phase-6 "(T6.1 part 1 fix round 1) Biases…" (AC-R7 on disk; heartbeats to non-neighbors) | AC-R7, AC-B1 rows |
| phase-6 "(T6.1 part 1) … Index memory, for AC-R1 and AC-R2 (T6.3 rows)" | AC-R1, AC-R2 |
| phase-6 "(T6.1 part 2) … The tails of the two claimed transfers" | S21 |
| phase-6 "(T4.7 part 3 → T6.1/T6.3) Claims' first-chunk window" | S4 (partial) |
| phase-6 "(T6.1 part 2) AC-V4 … T6.3 measures it at scale"; "(T6.1 part 2 → T4/T6.4) Stripe plans…" | AC-V4, S20, K5 |
| phase-6 "(T6.1 part 2 → T4/T6.4, finding) Stripe verification scores honest deliverers…" | S10, K2 |
| phase-6 "(T6.1 part 2 → T4/T6.3, Low) In-sync peers with nested roots…" | S12, K3, AC-V5 |
| phase-6 "(T6.1 part 2 → T6.4) Criteria wording" | AC-V3, AC-V4, AC-N1, K8 |
| phase-6 "(T6.1 part 2 fix round 1 → before T6.3) Two findings rank Medium" | §0 G7-1, K1, K2 |
| phase-6 "(T6.1 part 2 fix round 1 → T6.3 or a later harness run) A large session decode…" | §4.11, S9 |
| phase-3 "(T3.5) Open question 9, `CatchUpMinDiff = 256`" | §9, S2 |
| phase-4 "(Orchestrator, pre-T4.3 check) Index rebuild time accepted at 26.5 s" | AC-R3, S16 |
| phase-4 caches: "T6.3 can show how often catch-ups of that size happen"; G6 sheet row Q2 | §9 Q3, Q4 |
| phase-4 Q9: "T6.3 confirms against testnet's real rate"; "keep 256, confirmed in T6.3 against `pairwise_diff_half`"; "(T4.6 part 3 fix round 1 → T6.3, Q9) `CatchUpMinDiff` crossings" | §9, S2 |
| phase-4 "First-chunk latency of catch-up fetches … moved to T6.3"; "(T4.5 part 2c fix round 2 → T4.6) Measure the first-chunk latency…" | S4 (partial) |
| phase-4 "Q7 under expired-batch skew … Moved to T6.3" | S6 |
| phase-4 AC-B1 caveats "T6.1 and T6.3 measure the real one"; G6 sheet row AC-B1 | AC-B1, S19 |
| phase-4 "G6 decision sheet (final)", last column | §9 |
| phase-4 "(T4.6 part 3 fix round 2 → T6.3) A sustained overwrite stream" (and the pin paragraphs it ends) | S1, K10 |
| phase-4 "(T4.7 part 2 fix round 1) The `initiated_session_cap` gauge"; "(T4.7 part 2 → T6.3) The per-node cap on testnet"; "(T4.7 part 2 fix round 1 → T6.3) Mixed radii: a Busy breakdown…" | S3, A6 |
| phase-4 "(T4.7 part 2 fix round 1 → T6.0, AC-B1) N = 18–24 pass by 1.5–4 points" | S19 |
| phase-4 Sybil-made bootstrap (T4.7 part 2 "Pre-existing, widened by part 1 → T6.3"; "(T4.7 part 3 → T6.3) The Sybil-made bootstrap…") | §8 not runnable |
| phase-4 "(T4.7 part 3 fix round 1 → T6.3) Stall chains past the own-job grace"; "(T4.7 part 3 → T6.3) An honest claim with a stalling stripe peer…" | S8 |
| phase-4 "(T4.7 part 3 → T6.3) A claimant that drips…" (and its correction) | §8 not runnable |
| phase-4 "(T4.7 part 4 → T6.3) The one-chunk-then-reset pin stays as a drip" | §8 not runnable |
| phase-4 "(T4.7 part 4 → T6.3) Claims of an honest claimant this node never fetched from" (and fix-round corrections: measure the wait behind a flood) | S4 (partial), §8 not runnable (flood) |
| phase-4 "(T4.7 part 4 → T6) Credit parameters…", "A newcomer's second concurrent fetch…", "Junk up to the chunks a peer stores", "The pool can be drained" | S5 |
| phase-4 "(T4.7 part 4 fix round 1 → T6) Chain-state rejections to read off testnet" | S6 |
| phase-4 "(T4.7 part 4 fix round 2 → T6) Refunds for older versions are uncapped" | S15 |
| phase-4 "(T4.5 part 2b → T6) Fetch shortfall violations"; "(T4.5 part 2b fix round 1 → T6) Shortfall offenses"; "(T4.5 part 2b) The tolerance against honest churn, and what it leaves" (a batch expiry or a radius increase trips an honest peer: "T6 should read it off testnet") | S7, S6 |
| phase-4 "(T6.0) `round_interval_seconds` gauge … T6.3 ('confirm or adjust `RoundInterval` bounds')" | B5, §9 |
| phase-4 "(T4.2) `peers{state}` gauge … would show testnet operators (T6)" | A5 |
| phase-4 "(T4.5 part 1) Dual mode with the real puller" | S17 |
| phase-6 T6.3 scope: cohort design (D12), D22, dual-mode bandwidth, fetch limit | §3, §0 G7-2, §2.4 |

## Appendix B: metric coverage

Every `bee_rangesync_*` metric at d6fe508a and its panel: `sessions_total` B4 · `session_rounds`
B6 · `session_busy_total` B4 · `parts_sent`, `parts_received` C4 · `bytes_metadata`,
`bytes_chunks` C1 · `pairwise_diff` C5, E4 · `pairwise_diff_half` E4, §9 · `symbols_per_difference`
E1 · `more_requests_total` C4 · `streams_exhausted_total` E3 · `decode_corruption_total` E2 ·
`mapping_mismatch_total` E2 · `want_claims` D5 · `rejected_cache_size` D5 · `fetch_skipped_total`
D5, F3 · `fetch_delivered_total` D1 · `fetch_rejected_total` D1 · `cross_protocol_duplicates_total`
D1–D3 · `invalid_message_total`, `stream_limit_exceeded_total`, `status_rate_limited_total` F2 ·
`index_rebuild_seconds` G2 · `index_apply_seconds` G3 · `index_memory_bytes` G1 ·
`index_outside_root_total` F4 · `sync_rate` B1 · `peer_failures_total` F1 · `stripes_total`,
`verification_diff` B7 · `round_interval_seconds` B5 · `peers` A5 · `initiated_session_cap` A6.
§10's `audit_mismatch{kind}` does not exist (Phase 5).
