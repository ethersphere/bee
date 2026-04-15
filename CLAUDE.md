# CLAUDE.md

This file provides guidance to AI coding assistants (Claude Code, Cursor, GitHub Copilot, Codex) when working with code in this repository.

## Project overview

Bee is the reference Go implementation of an Ethereum Swarm node. It implements decentralized storage and communication protocols: content-addressed chunk storage, Kademlia-based routing, postage stamp accounting, push/pull syncing, PSS messaging, feeds, and storage incentives (redistribution game).

**Module**: `github.com/ethersphere/bee/v2`
**Go version**: 1.26 (see `go.mod`)
**License**: BSD 3-clause (see `LICENSE`)
**Default branch**: `master`

## Build and test commands

```bash
make binary            # build dist/bee (CGO_ENABLED=0, version ldflags injected)
make build             # compile all packages
make test              # unit tests (-failfast)
make test-race         # unit tests with race detector
make test-integration  # integration tests (requires -tags=integration)
make lint              # golangci-lint v2.11.3 (see .golangci.yml)
make vet               # go vet
make format            # gofumpt + gci
make protobuf          # regenerate protobuf files (requires protoc + gogofaster)
make clean             # remove dist/ and go clean
make docker-build      # build Docker image via Dockerfile.dev
```

CI-specific targets (skip tests with names ending in `FLAKY`):
- `make test-ci` / `make test-ci-race` — exclude flaky tests
- `make test-ci-flaky` — run only flaky tests

Beekeeper integration testing:
- `make beekeeper` — install beekeeper tool
- `make beelocal` — start local k3s cluster
- `make deploylocal` — deploy bee cluster
- `make testlocal` — run integration checks (pingpong, connectivity, pushsync, retrieval, etc.)

## Architecture

### Entry point and CLI

Binary built from `cmd/bee/main.go`. CLI uses Cobra + Viper:
- `bee start` — full or light node (`cmd/bee/cmd/start.go`)
- `bee init` — initialize data directory
- `bee deploy` — deploy smart contracts
- `bee db` — database management
- `bee version` — print version info

Configuration: option constants defined in `cmd/bee/cmd/cmd.go`. Viper reads from CLI flags, env vars (`BEE_` prefix), and YAML config.

### Node bootstrap

`pkg/node/node.go` is the main orchestrator. `NewBee()` wires all subsystems via dependency injection — no global mutable state. The `Bee` struct holds references to every service and provides `Shutdown()` for clean teardown.

### HTTP API

- Router: `gorilla/mux` in `pkg/api/router.go`
- Three route groups in `Mount()`:
  - `mountTechnicalDebug()` — `/node`, `/addresses`, `/health`, `/readiness`, `/metrics`, `/loggers`, pprof
  - `mountBusinessDebug()` — topology, accounting, settlements, stamps management
  - `mountAPI()` — `/bytes`, `/chunks`, `/bzz`, `/feeds`, `/soc`, `/stamps`, `/tags`, `/pins`, `/pss`, `/grantee`
- Route gating: `checkRouteAvailability` blocks endpoints during sync
- OpenAPI spec: `openapi/Swarm.yaml` (follows SemVer independently from the bee version)
- Endpoints available at both root (`/bytes`) and versioned (`/v1/bytes`)

### P2P networking

- Transport: libp2p (`pkg/p2p/libp2p/`)
- Protocols use protobuf (gogo/protobuf with `--gogofaster_out`) — each protocol package has a `pb/` subdirectory with `.proto` and `doc.go` containing the `go:generate` directive
- Key protocols:
  - `pushsync` — push chunks to their neighborhood
  - `pullsync` — pull chunks from peers during syncing
  - `retrieval` — retrieve chunks by address
  - `pingpong` — liveness checking
  - `hive` — peer discovery and address broadcasting
  - `pricing` — price announcements between peers

### Storage

- Chunk types: Content-Addressed Chunks (`pkg/cac/`) and Single Owner Chunks (`pkg/soc/`)
- Core interfaces: `Putter`, `Getter`, `Hasser`, `Deleter` in `pkg/storage/`
- Storer: `pkg/storer/` manages local store (reserve, cache, upload store, pinning)
- Sharky: `pkg/sharky/` — blob storage engine (fixed-size slots)
- BMT: `pkg/bmt/` — Binary Merkle Tree hasher for chunk integrity
- State store: `pkg/statestore/` — LevelDB-backed key-value store
- Shed: `pkg/shed/` — typed LevelDB abstraction layer

### Postage stamps

- `pkg/postage/` — batch store, batch service, stamp types
- `pkg/postage/listener/` — listens to on-chain stamp events
- `pkg/postage/postagecontract/` — interacts with the postage stamp smart contract
- Stamps have batch ID, depth (log2 capacity), and amount (per-chunk value)

### Storage incentives

- `pkg/storageincentives/` — redistribution game agent
- Nodes in the correct neighborhood prove they store chunks and earn rewards

## Key domain concepts

- **Address** — 32-byte hash (`pkg/swarm/`). Used for both chunk and node overlay addresses. Proximity measured by XOR distance.
- **Chunk** — fundamental storage unit. Data is 4096 bytes (`ChunkSize = SectionSize * Branches = 32 * 128`), with an 8-byte span prefix (`SpanSize`), making `ChunkWithSpanSize = 4104`.
- **CAC** — Content Address Chunk. Address = BMT hash of data.
- **SOC** — Single Owner Chunk. Address derived from owner + identifier, signed by the owner.
- **Proximity order (PO)** — number of leading bits two addresses share. `MaxPO = 31`, `ExtendedPO = 36`.
- **Neighborhood** — addresses sharing a common prefix. Determines which chunks a node is responsible for.
- **Kademlia** — XOR-distance routing overlay. Peers organized in bins by PO.
- **Postage stamp** — proof of payment attached to chunks. Batch has depth (capacity) and amount (value per chunk).
- **Push sync** — forwards newly uploaded chunks to their neighborhood.
- **Pull sync** — syncs chunks between peers in overlapping neighborhoods.
- **Redistribution** — storage incentive game: prove storage, earn rewards.

## Coding conventions

Refer to `CODING.md` and `CODINGSTYLE.md` for the full rules. Summary of the most important ones:

### Copyright header (enforced by goheader linter)

Every `.go` file must start with:
```go
// Copyright <current year> The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
```

### Error handling

- Propagate errors up; never log and return the same error
- Wrap with `fmt.Errorf("context: %w", err)` — produces stack-trace-like messages
- Avoid "failed to" prefixes: `"new store: %w"` not `"failed to create new store: %w"`
- Sentinel errors: `var ErrFoo = errors.New("package: description")`

### Testing

- Separate test packages preferred: `package foo_test`, not `package foo`
- Use `export_test.go` (in the real package) to expose internals for tests only
- Run tests in parallel (`t.Parallel()`) where possible
- Compact test names; use godoc for scenario description
- Avoid "fail" in test names (conflicts with test runner output)
- Flaky tests: suffix name with `FLAKY` so CI can separate them
- Integration tests: use `-tags=integration` build tag
- Use `t.Fatal`/`t.FailNow`, not `panic`

### Concurrency

- Every goroutine must have a defined termination path
- Channels: size 0 (unbuffered) or 1 — anything else needs justification
- Channels have an owning goroutine; prefer directional types

### Style

- American English (marshaling, not marshalling; canceled, not cancelled)
- Avoid `init()` functions
- Start enums at `iota + 1`
- Use `time.Duration` and `time.Time`, not raw integers
- Verify interface compliance: `var _ Interface = (*Impl)(nil)`
- No mutable globals — use dependency injection
- Exit only in `main()`

### Commit messages

[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) format. Allowed types: `build`, `chore`, `ci`, `docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `test`. Header max 100 chars, footer max 72 chars. Imperative mood. Enforced by commitlint (see `commitlint.config.js`).

### Logging

- Levels: `Error`, `Warning`, `Info`, `Debug` (with V-levels)
- `Error`/`Warning` for node operators — no internal implementation details
- `Debug` for developers — include technical details
- Keys: `lower_snake_case`, specific (`peer_address` not `address`)
- Loggers are runtime-configurable via the `/loggers` API endpoint

## Linting

golangci-lint v2 with `.golangci.yml`. Notable enabled linters:
- `goheader` — enforces copyright header
- `paralleltest` — detects missing `t.Parallel()`
- `misspell` — catches British spellings
- `errorlint` — ensures proper error wrapping
- `gochecknoinits` — flags `init()` functions
- `prealloc` — suggests pre-allocating slices
- `forbidigo` — restricts `fmt.Print` usage (allowed only in `cmd/bee/cmd/`)
- `govet` with `enable-all: true` (disables `fieldalignment`, `shadow`)

Run `make lint` to check. Run `make format` to auto-format.

## Directory structure

```
cmd/bee/              CLI entry point and Cobra commands
openapi/              OpenAPI 3.0 specs (Swarm.yaml, SwarmCommon.yaml)
packaging/            system packaging (deb, rpm, homebrew, scoop, docker, systemd)
pkg/
  api/                HTTP API handlers and router
  node/               node bootstrap and dependency wiring
  swarm/              core types (Address, Chunk, constants)
  p2p/                P2P transport (libp2p)
  topology/           Kademlia routing table
  storage/            storage interfaces
  storer/             local store (reserve, cache, upload, pinning)
  sharky/             blob storage engine
  postage/            postage stamp system
  storageincentives/  redistribution game
  pushsync/           push sync protocol
  pullsync/           pull sync protocol
  retrieval/          chunk retrieval protocol
  feeds/              mutable feed references
  pss/                Postal Service over Swarm
  soc/                Single Owner Chunks
  cac/                Content Address Chunks
  bmt/                Binary Merkle Tree hasher
  accesscontrol/      ACT encryption
  redundancy/         erasure coding (Reed-Solomon)
  replicas/           dispersed replicas
  manifest/           trie-based directory structures (Mantaray)
  settlement/         payment channels (SWAP chequebook + pseudosettle)
  accounting/         per-peer bandwidth accounting
  crypto/             signing and key management
  keystore/           encrypted key storage
  log/                structured logging with V-levels
  metrics/            Prometheus metrics
  tracing/            OpenTracing / Jaeger
  ...                 ~60 packages total
```

## Common pitfalls

- `ChunkSize` is 4096 bytes (data only). `ChunkWithSpanSize` is 4104 (data + 8-byte span). Don't confuse them.
- Addresses are XOR-distance based — "closer" means more shared prefix bits, not numerically smaller.
- Don't log and return the same error — pick one. Errors propagate up, logging happens at handlers.
- Tests go in `package foo_test`, not `package foo`. Use `export_test.go` for internal access.
- Every goroutine needs a shutdown path — typically via context cancellation or a quit channel.
- Full node vs light node — reserve, storage incentives are only available on full nodes.
- Stamps can be unusable (expired, depleted, unsynced batch). Always check usability.
- The main bee version does NOT follow strict SemVer. The API version in `openapi/Swarm.yaml` does.
- Generated protobuf files (`*.pb.go`) are committed to the repo. Regenerate with `make protobuf`.
- Default branch is `master`, not `main`.
