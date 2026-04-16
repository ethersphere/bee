# AGENTS.md

Project instructions for **AI coding assistants and agents** (OpenAI Codex, Cursor, GitHub Copilot, Claude Code, and similar tools). This file is meant to be **self-contained** so any agent that discovers `AGENTS.md` gets enough context without another product-specific file.

Codex users: see [Custom instructions with AGENTS.md](https://developers.openai.com/codex/guides/agents-md/) for how global and project instructions merge and for the default combined size limit (`project_doc_max_bytes`, often 32 KiB).

This repo also has **`CLAUDE.md`**, tuned for **Claude Code** (shorter session prompt, `@` imports, `.claude/rules/`). Keep factual content aligned when you change workflows or versions.

## Project overview

Bee is the reference Go implementation of an Ethereum Swarm node. It implements decentralized storage and communication: content-addressed chunk storage, Kademlia-based routing, postage stamp accounting, push/pull syncing, PSS messaging, feeds, and storage incentives (redistribution game).

**Module**: `github.com/ethersphere/bee/v2`  
**Go version**: 1.26 (see `go.mod`)  
**License**: BSD 3-clause (see `LICENSE`)  
**Default branch**: `master`

Human-oriented contributing docs: `CONTRIBUTING.md`, `CODING.md`, `CODINGSTYLE.md`, `README.md`.

## Guidelines

Keep changes **minimal and focused**. Only touch code that belongs to the task. Do not refactor unrelated code, rename symbols for style only, or mix unrelated fixes in one commit or PR.

Read **`CONTRIBUTING.md`**, **`CODING.md`**, and **`CODINGSTYLE.md`** for process, patterns, and style. Prefer matching existing naming, types, imports, and log style in the files you edit.

Do **not** add, remove, or update `go.mod` dependencies unless the task **explicitly** requires it or the person asking for the work **explicitly** requests a dependency change.

Handle errors and logging the way this repo does: propagate errors with context (`fmt.Errorf("…: %w", err)`), avoid logging and returning the same error, and use structured logging with clear operator vs developer levels (see `CODING.md`).

Prefer **`package foo_test`** tests, **`export_test.go`** when you must export internals, and **`t.Parallel()`** only where it is safe. Add or update tests when behavior changes. Integration tests use **`-tags=integration`**.

## Pre-commit checklist

Before you finish a change set (especially before a commit or PR), run these and fix failures:

1. **Formatting** — `make format` (gofumpt + gci; see `CODING.md`).
2. **Compile** — `make build` (all packages) and, when you need the binary artifact, `make binary` (`dist/bee`, `CGO_ENABLED=0`).
3. **Tests** — `make test` (unit tests, `-failfast`). Use `make test-race` when concurrency is central to the change. Use `make test-integration` only when you touch integration-tagged code.
4. **Static checks** — `make lint` and `make vet` (see `.golangci.yml`).

CI pipelines may use `make test-ci` / `make test-ci-race` (see `Makefile` for flags).

## Dev commands (quick reference)

```bash
make binary     # dist/bee
make build      # compile all packages
make test       # unit tests
make test-race  # unit tests + race detector
make lint       # golangci-lint (see .golangci.yml)
make vet        # go vet
```

## Commit message format and PR titles

This repo uses **[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)** with **`commitlint.config.js`**: allowed types are `build`, `chore`, `ci`, `docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `test`. Header **max 100** characters; footer lines **max 72**. Use **imperative** mood and **no trailing period** on the subject.

Use an optional **scope** (often a top-level area such as `api`, `storer`, `node`) when it clarifies impact:

```text
<type>(<scope>): <short lowercase description>

fix(api): reject directory upload without content-type
test(storer): cover session cleanup on error
docs: clarify agent pre-commit steps
```

**Pull request titles** follow the same idea: same type and scope style, concise lowercase description (match what you will squash or merge).

## Architecture

### Entry point and CLI

Binary built from `cmd/bee/main.go`. CLI uses Cobra + Viper:

- `bee start` — full or light node (`cmd/bee/cmd/start.go`)
- `bee init` — initialize data directory
- `bee deploy` — deploy smart contracts
- `bee db` — database management
- `bee version` — print version info

Configuration: option constants in `cmd/bee/cmd/cmd.go`. Viper reads CLI flags, environment variables (`BEE_` prefix), and YAML config.

### Node bootstrap

`pkg/node/node.go` is the main orchestrator. `NewBee()` wires subsystems via dependency injection; avoid global mutable state. The `Bee` struct holds service references and provides `Shutdown()` for teardown.

### HTTP API

- Router: `gorilla/mux` in `pkg/api/router.go`
- Route groups in `Mount()`:
  - `mountTechnicalDebug()` — `/node`, `/addresses`, `/health`, `/readiness`, `/metrics`, `/loggers`, pprof
  - `mountBusinessDebug()` — topology, accounting, settlements, stamps management
  - `mountAPI()` — `/bytes`, `/chunks`, `/bzz`, `/feeds`, `/soc`, `/stamps`, `/tags`, `/pins`, `/pss`, `/grantee`
- `checkRouteAvailability` can block endpoints during sync
- OpenAPI: `openapi/Swarm.yaml` (API versioning follows SemVer there; the main Bee release version does not)
- Endpoints exist at root (e.g. `/bytes`) and under `/v1/` (e.g. `/v1/bytes`)

### P2P networking

- Transport: libp2p (`pkg/p2p/libp2p/`)
- Wire formats: protobuf (gogo) — each protocol area has a `pb/` directory with `.proto` and `doc.go` (`go:generate` calling `protoc` + `--gogofaster_out`)
- Important protocol packages: `pushsync`, `pullsync`, `retrieval`, `pingpong`, `hive`, `pricing`

### Storage

- Chunk types: CAC (`pkg/cac/`), SOC (`pkg/soc/`)
- Interfaces: `pkg/storage/` (`Putter`, `Getter`, `Hasser`, `Deleter`)
- Local store: `pkg/storer/` (reserve, cache, upload, pinning)
- Blob engine: `pkg/sharky/`
- BMT: `pkg/bmt/`
- State: `pkg/statestore/` (LevelDB); `pkg/shed/` (typed LevelDB layer)

### Postage and incentives

- `pkg/postage/` — batches, stamps, services
- `pkg/postage/listener/` — on-chain events
- `pkg/postage/postagecontract/` — contract interaction
- Stamps: batch ID, depth (capacity), amount (per-chunk value)
- `pkg/storageincentives/` — redistribution / storage incentive game

## Key domain concepts

- **Address** — 32-byte hash (`pkg/swarm/`). Chunk and overlay addresses; proximity is XOR-based (more shared prefix bits = closer), not numeric ordering.
- **Chunk** — 4096 bytes of data (`ChunkSize = SectionSize * Branches = 32 * 128`), plus 8-byte span (`SpanSize`); `ChunkWithSpanSize = 4104`.
- **CAC** — content-addressed chunk; address from BMT root of data.
- **SOC** — single owner chunk; address from owner + id, with signature.
- **PO** — proximity order (shared prefix bits). `MaxPO = 31`, `ExtendedPO = 36`.
- **Neighborhood** — prefix / responsibility region for storage and sync.
- **Kademlia** — routing table over XOR distance (`pkg/topology/`).
- **Postage stamp** — payment signal attached to chunks.
- **Push sync / pull sync** — push new data toward neighborhood; pull historical sync between peers.
- **Redistribution** — incentive game proving reserve storage.

## Coding conventions (summary)

Authoritative detail: `CODING.md` and `CODINGSTYLE.md`.

### Copyright (goheader)

Every `.go` file starts with:

```go
// Copyright <year> The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
```

### Errors, logging, concurrency

- Propagate errors; do not log and return the same error. Use `fmt.Errorf("context: %w", err)`. Avoid stacking "failed to" prefixes.
- Sentinel errors: `var ErrFoo = errors.New("package: description")` when appropriate.
- Logging: separate operator-facing (`Error`/`Warning`) from developer detail (`Debug`, V-levels). Keys: `lower_snake_case`, specific names. Runtime log tuning: `/loggers` API.
- Every goroutine needs a clear shutdown path. Channels: prefer unbuffered or size 1 unless strongly justified; an owning goroutine sends or closes.

### Testing

- Prefer external test packages: `package foo_test` not `package foo`.
- `export_test.go` in the real package to export symbols only for tests.
- Use `t.Parallel()` where safe. Avoid the word `fail` in test names. Integration: `-tags=integration`. Prefer `t.Fatal` / `t.FailNow` over `panic` in tests.

### Style and tooling

- American English (e.g. marshaling, canceled).
- Avoid `init()` where possible (`gochecknoinits`).
- Enums often start at `iota + 1` when zero should mean "unset".
- Use `time.Time` / `time.Duration`, not raw ints for time.
- `var _ Interface = (*Impl)(nil)` where useful.
- Dependency injection over mutable globals. Exit only from `main()`.

### Commits

See **[Commit message format and PR titles](#commit-message-format-and-pr-titles)** above.

### Linting

`golangci-lint` v2 per `.golangci.yml`. Notable: `goheader`, `paralleltest`, `misspell`, `errorlint`, `gochecknoinits`, `prealloc`, `forbidigo` (no `fmt.Print` except under `cmd/bee/cmd/`), `govet` with `enable-all` (minus `fieldalignment`, `shadow`). Run `make lint` and `make format`.

## Directory structure (high level)

```
cmd/bee/              CLI and Cobra commands
openapi/              OpenAPI specs (Swarm.yaml, SwarmCommon.yaml)
packaging/            deb, rpm, homebrew, scoop, docker, systemd
pkg/
  api/                HTTP API
  node/               composition root
  swarm/              Address, Chunk, core constants
  p2p/                libp2p transport
  topology/           Kademlia
  storage/            storage interfaces
  storer/             local storer implementation
  sharky/             blob slots
  postage/            stamps and batches
  storageincentives/  redistribution
  pushsync/ pullsync/ retrieval/ pingpong/ hive/ pricing/  # protocols
  feeds/ pss/ soc/ cac/ bmt/ manifest/ accesscontrol/ redundancy/ replicas/
  settlement/ accounting/ crypto/ keystore/ log/ metrics/ tracing/
  ...                 ~60 packages total
```

## Common pitfalls

- Do not confuse `ChunkSize` (4096 data bytes) with `ChunkWithSpanSize` (4104 including span).
- XOR distance: "closer" is more shared prefix bits, not smaller integers.
- Do not both log and return the same error.
- Tests: `foo_test` + `export_test.go` pattern.
- Goroutines must be stoppable (context cancel, quit channel, etc.).
- Full node vs light node: reserve and storage incentives are full-node concerns.
- Postage batches can be unusable (expired, depleted, unsynced); check before relying on stamps.
- `*.pb.go` files are committed; regenerate with `make protobuf` after `.proto` changes.
- Default branch name is `master`, not `main`.
