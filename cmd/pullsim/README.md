# pullsim — in-memory pull-sync network simulator

`pullsim` spins up N synthetic Swarm nodes in a single process, connected in
memory, each running the **real** `pkg/pullsync` Syncer and `pkg/puller` Puller
against an in-memory reserve shim. It streams chunk propagation and live
per-directed-edge protocol state to a browser UI so you can watch the pull-sync
protocol behave — coalescing waits, offers/wants/deliveries, cursors, and the
radius-driven bin selection — without running full Bee nodes.

Everything is self-contained under `cmd/pullsim/`; no production package is
modified.

## Run

```bash
go run ./cmd/pullsim                 # defaults: :8080, 20 nodes, full mesh
go run ./cmd/pullsim -nodes 20 -topology ring -degree 6
```

Then open <http://localhost:8080>. Inject a single chunk (count=1) to trace a
green wavefront across the graph; move the radius slider live; rebuild the
network from the sidebar.

### Flags

`-listen :8080 -nodes 20 -bins 8 -topology full|ring|k-nearest|random|kademlia -degree 6`
`-radius 0 -latency 5ms -maxpage 64 -clusters 1 -seed 0 -settle 3s -v`
`-churn 0 -churn-after 30s -churn-radius 0`

`-churn` and its two companions script a departure scenario in server mode; see
[Churn and radius backfill](#churn-and-radius-backfill). They are rejected in
sweep mode.

`-settle` is the batch quiescence window and applies to **both** the server and
the sweep. In sweep mode `-bench-settle` overrides it, but only if you actually
pass it; leaving it alone keeps the two modes in step. Sweep-only flags are
listed under [Sweep mode](#sweep-mode).

### Propagation timing

Every `Inject` is tracked as a **batch**. A batch is done when no chunk of it
has been stored anywhere for the settle window (`-settle`, 3s by default); the
reported span runs from the first inject to the last chunk stored, so the quiet
window itself is never counted. The quiescence clock only starts at the first
put, so a drip slower than the settle window cannot settle before its own first
chunk exists. The sidebar's Propagation panel lists recent batches with span,
replica count, and nodes reached. A running row's span freezes between arrivals
— it is "first inject to last put so far", not a wall clock.

Because completion is detected by quiescence, a hop slower than the settle
window closes the batch early. That is made self-evidencing rather than
inferred: any replica that arrives after a batch settled is counted as a **late
replica** and reported (`lateReplicas` in the CSV and on the batch row in the
UI). `lateReplicas > 0` is direct proof the window truncated the measurement —
raise `-settle` and re-run. In the sweep such a cell is marked `truncated` and
its timing columns are blanked.

### Sweep mode

```bash
go run ./cmd/pullsim -bench \
  -bench-nodes 10,20,30,40,50 -bench-chunks 1,10,100 -bench-reps 3 \
  -topology k-nearest -degree 6 -bench-out sweep.csv
```

Runs each (nodes x chunks) cell headlessly and emits one CSV row per
repetition, with every config column — including `settleMs` and `warmupMs` —
so runs stay comparable. No HTTP server is started. Sweep flags:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-bench-nodes` | `10,20,30,40,50` | comma-separated node counts |
| `-bench-chunks` | `1,10,100` | comma-separated batch sizes |
| `-bench-reps` | 3 | repetitions per cell |
| `-bench-warmup` | 5s | settling time after start before injecting |
| `-bench-settle` | 3s | overrides `-settle`, sweep only, only if passed |
| `-bench-minpo` | 0 | proximity order the injected chunks are mined to |
| `-bench-timeout` | 120s | per-cell hard cap |
| `-bench-out` | "" | CSV path; empty means stdout |

`-bench-warmup` is not optional: puller startup plus the first pullsync
handshake round costs more than most propagation spans, so injecting
immediately would measure startup and present as a spurious node-count
dependence.

**Every cell emits a row, and `status` says whether its timing means
anything.** `ok` is the only status whose timing is a measurement; for every
other status `spanMs`, `tailMs` and the three `perDelivery*` columns come out
**empty** rather than `0`, because a `0` there sorts as the fastest cell in the
sweep. `replicas`, `nodesReached` and `lateReplicas` stay real for every status
— they are what says *why* the cell is untrustworthy.

| status | meaning |
| --- | --- |
| `ok` | settled, replicated, nothing arrived afterwards |
| `truncated` | settled, but replicas kept arriving — raise `-settle` |
| `no-replicas` | nothing was ever replicated; the span is just the origin's own inject |
| `not-settled` | the wait returned without the batch settling |
| `timeout` | the cell exceeded `-bench-timeout` |
| `error` | the wait failed for another reason (batch evicted, network closed) |

**Measurement validity — what the default grid actually measures.** In a full
mesh (`-topology full`, the default) with `-radius 0`, every node is a direct
peer of the origin, so propagation is a single hop by construction: growing
`-bench-nodes` only widens fan-out, it never adds hops. `spanMs` therefore
collapses to pullsync's ~1s `pageTimeout` floor (see
`pkg/pullsync/pullsync.go`) regardless of N — exactly what the default sweep
shows — and is structurally meaningless as a probe of how propagation time
scales with cluster size. The sweep prints a warning on stderr when it is run
in that configuration.

To actually measure that, force multiple hops with a bounded-degree topology:

- `-topology ring` maximizes diameter for a given N (roughly `N/degree` hops),
  so it gives the steepest, easiest-to-read size dependence.
- `-topology kademlia` is the realistic one — it is what Bee actually builds.
  Each node keeps up to `-degree` peers in every proximity order bin below the
  storage radius, and links every peer at or above it, so the neighborhood is
  fully connected while the sub-radius bins stay sparse.

  Expect it to produce a **flat** span, and understand why before reading that
  as a bug. Pull-sync only replicates a chunk into nodes whose storage radius
  covers it — that is, the origin's neighborhood — and kademlia connects that
  neighborhood fully. So replication is one hop by construction, no matter how
  large the network is. Measured with `-clusters 3 -radius 5 -bench-minpo 6`,
  N of 9/18/36 gives `spanMs` 1007/1007/1007 with `replicas` 2/5/11: growing
  the network grows how many nodes hold the chunk, not how long it takes to get
  there. That flatness is the property kademlia exists to provide.

  Use the ring instead if what you want is a hop-count curve; the ring is not a
  model of any real Swarm deployment, but it is the configuration in which hop
  count and node count actually covary.
- `-topology k-nearest -degree 6` is a middle ground between hop count and
  goroutine cost, but note it only links *close* peers, so it is not a model of
  any real routing table.

Expect spans to quantize to roughly one second per hop.

`-topology kademlia` needs a non-zero `-radius` to mean anything: at radius 0
every bin falls inside the neighborhood, every peer gets linked, and the graph
degenerates to a full mesh. The sweep warns about this too. Note also that the
graph is built once, from the configured radius — moving the radius slider in
the UI changes sync behaviour but does not rewire the topology, so reshaping it
needs a rebuild.

It also needs `-clusters` above 1. Addresses are otherwise spread uniformly over
the whole 32-byte space, so at any useful radius almost no pair of nodes is
close enough to be in each other's neighborhood: nothing replicates, and every
sweep cell comes back `no-replicas`. `-clusters 3` concentrates the addresses
into three neighborhoods, which is what makes a non-zero radius meaningful.

A non-zero `-radius` is also worth adding — at radius 0 every node stores
everything regardless of topology, which flattens the very effect you're trying
to observe. **If you set `-radius R`, set `-bench-minpo` to at least `R`.** The
sweep mines its chunks at `-bench-minpo` proximity to the origin; at
`-bench-minpo 0` a receiving node only stores an offered chunk with probability
about `2^-R`, so most cells replicate nothing at all and come back
`no-replicas`. Consider `-clusters` above 1 as well, so the origin's peers
actually share its neighborhood.

Keep `-settle` comfortably above 1s (the 3s default is right); below that the
batch is judged settled before the first replica arrives, which shows up as
`no-replicas` rows, and marginal windows show up as `truncated`.

## Churn and radius backfill

Nodes can leave a running network, and the radius change you make in response
drives the survivors to pull the chunks they have newly become responsible for.

```bash
go run ./cmd/pullsim -topology kademlia -clusters 3 -radius 5 -nodes 18
```

Inject at `-minpo 5`, kill a few nodes from the Churn panel, then drop the
radius to 4 and watch the Heal panel. From a shell, `-churn N -churn-after 10s
-churn-radius R` runs the same scenario without the UI.

**This does nothing at `-radius 0`**, the default. There every node stores
everything, so no node has a deficit, and churn has no radius effect to reveal —
only the data it destroys. The configuration above is the one where the radius
is doing real work, and it is the same one the sweep section recommends, for the
same reason.

### What a departure is

A hard departure. The node's puller, transport, syncer and reserve are closed,
and its chunks go with it unless a survivor already held a replica; `lost`
reports how many did not. Survivors then re-wire: the adjacency is recomputed
over the surviving addresses and applied as a delta, modelling kademlia
re-bootstrapping into bins the departures emptied. Departed nodes keep their
index and their position on the ring, so the hole stays visible and nothing
renumbers mid-run.

### What heals, and why

Nothing in the sync path is simulated — the effect is entirely production code.
The pullsync client gates every `want` on `IsWithinStorageRadius`
(`pkg/pullsync/pullsync.go`), so lowering the radius widens what a node will
accept; and the puller, on a radius *decrease* only, disconnects its sync peers
and calls `resetIntervals` (`pkg/puller/puller.go`), so history is offered
again. A drop from R to R-1 therefore opens bin R-1 against neighbors, and the
chunks the node previously ignored arrive.

A **heal episode** opens on each decrease. It snapshots every survivor's
deficit — chunks that exist somewhere in the network, fall within that node's
new radius, and are absent from its reserve — and drains it, settling under the
same quiescence window as batch propagation (`-settle`). It reports
`healed / total`, `remaining`, and a span.

### Residual `remaining` is an outcome, not a failure

A settled episode can leave chunks unhealed, and that is not a bug. Dropping to
R-1 makes a node responsible for bin R-1, but the puller only pulls bin R-1 from
a peer at `PO >= R-1`, or the single bin `po` from a peer where
`radius - po <= 2`. A chunk whose only surviving holder is neither is simply not
reachable by pull-sync from that node. It is reported rather than hidden, and no
test asserts it is zero.

**In the clustered configuration above you will see `remaining 0`, and that is
expected too.** `-clusters` places addresses at proximity `bins+4` from their
cluster base, which caps them all at the maximum PO, so every surviving
cluster-mate is a full-sync peer at any radius and the unreachable case cannot
arise inside a cluster. Seeing the residue needs a *sparse* neighborhood —
survivors sitting at a PO just above the new radius rather than far above it,
which is what `-topology k-nearest` or a low `-degree` kademlia run gives you.
The clustered setup is the one that shows the backfill working; the sparse one
is what shows its limits.

Expect the heal to be abrupt rather than gradual. The radius drop makes the
puller disconnect its peers and reset intervals in one go, so the whole deficit
tends to arrive in a single re-offer round — around one second, the same
`pageTimeout` floor the propagation section describes.

### Goroutine budget

Roughly `N·(N-1)·Bins·2` live sync workers for a full mesh (N=50, Bins=8 ≈ 39k).
Prefer `-topology k-nearest -degree 6` for large N.

## Architecture

- `internal/sim` — engine: `SimReserve` (in-memory `storer.Reserve`), a custom
  in-memory `p2p.Streamer` transport with a protobuf wire tap, address/chunk
  generation, topologies, the instrumented syncer decorator, the batch
  propagation tracker (quiescence-based settle detection with per-batch
  timing metrics), churn plus the deficit/heal tracker, and the `Network`
  orchestrator. Knows nothing about HTTP.
- `internal/event` — wire schema + fan-out `Bus` that folds wire messages into
  per-directed-edge state and emits authoritative 250 ms snapshots. Knows
  nothing about the protocol.
- `internal/web` — mux routes, REST control, websocket hub, embedded static UI
  (vanilla JS + canvas).

## Testing

```bash
go test ./cmd/pullsim/...           # full suite incl. propagation milestones
go test -race ./cmd/pullsim/...     # excludes the multi-peer integration tests
```

The multi-peer integration tests in `internal/sim/network_test.go` are tagged
`//go:build !race`. They faithfully drive concurrent pullsync handlers, which
exposes a **benign, pre-existing data race in the `resenje.org/singleflight`
dependency**: `call.shared` is written under the group mutex
(`singleflight.go:43`) but read lock-free after unlock (`singleflight.go:86`).
`pullsync` discards that `shared` bool, so behaviour is unaffected — but the
race detector reports it whenever two peers coalesce the same `(bin, start)` on
one server Syncer. The race is in the dependency, not in `cmd/pullsim`, and
fixing it would require changing a production module, so those tests are
excluded from `-race` runs. The HTTP/websocket tests use a 2-node network (one
client per server → no coalescing) and run cleanly under `-race`.
