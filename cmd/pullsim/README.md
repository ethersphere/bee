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
`-radius 0 -latency 5ms -maxpage 64 -clusters 1 -seed 0 -v`

### Goroutine budget

Roughly `N·(N-1)·Bins·2` live sync workers for a full mesh (N=50, Bins=8 ≈ 39k).
Prefer `-topology k-nearest -degree 6` for large N.

## Architecture

- `internal/sim` — engine: `SimReserve` (in-memory `storer.Reserve`), a custom
  in-memory `p2p.Streamer` transport with a protobuf wire tap, address/chunk
  generation, topologies, the instrumented syncer decorator, and the `Network`
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
