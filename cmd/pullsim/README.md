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

`-listen :8080 -nodes 20 -bins 8 -topology full|ring|k-nearest|random -degree 6`
`-radius 0 -latency 5ms -maxpage 64 -clusters 1 -seed 0 -settle 3s -v`

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
`-topology ring` maximizes diameter for a given N, `-topology k-nearest
-degree 6` is a reasonable middle ground between hop count and goroutine
cost. Expect spans to quantize to roughly one second per hop.

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

### Goroutine budget

Roughly `N·(N-1)·Bins·2` live sync workers for a full mesh (N=50, Bins=8 ≈ 39k).
Prefer `-topology k-nearest -degree 6` for large N.

## Architecture

- `internal/sim` — engine: `SimReserve` (in-memory `storer.Reserve`), a custom
  in-memory `p2p.Streamer` transport with a protobuf wire tap, address/chunk
  generation, topologies, the instrumented syncer decorator, the batch
  propagation tracker (quiescence-based settle detection with per-batch
  timing metrics), and the `Network` orchestrator. Knows nothing about HTTP.
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
