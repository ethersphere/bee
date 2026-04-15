# CLAUDE.md

Instructions for [Claude Code](https://docs.anthropic.com/en/docs/claude-code/overview) in this repository. This file is loaded as project context at session start. For the full, tool-neutral project guide (Codex, Cursor, Copilot, etc.), see `AGENTS.md`.

Official reference: [How Claude remembers your project](https://docs.anthropic.com/en/docs/claude-code/claude-md).

## Claude Code–specific notes

- Prefer **concise, verifiable** bullets; long prose belongs in repo docs, not here.
- **Personal overrides** (not committed): `CLAUDE.local.md` at repo root (add to `.gitignore` if you create it).
- **Path-scoped rules**: `.claude/rules/` for conventions tied to directories or file patterns.
- **Pull in more context** when needed (imports resolve relative to this file):
  - @CODING.md @CODINGSTYLE.md @CONTRIBUTING.md
  - @README.md
- Run **`/init`** in Claude Code to refresh or extend this file from the codebase; merge by hand rather than losing team edits.

## Project snapshot

Bee is the reference Go implementation of an Ethereum Swarm node: chunk storage, Kademlia routing, postage, push/pull sync, PSS, feeds, storage incentives.

| Item | Value |
|------|--------|
| Module | `github.com/ethersphere/bee/v2` |
| Go | 1.26 (`go.mod`) |
| Default branch | `master` |
| License | BSD 3-clause (`LICENSE`) |

## Commands (verify before shipping)

```bash
make binary            # dist/bee, CGO_ENABLED=0 + ldflags
make build             # compile all packages
make test              # unit tests (-failfast)
make test-race         # + race detector
make test-integration  # -tags=integration
make lint && make vet  # golangci-lint v2.11.3 + go vet
make format            # gofumpt + gci
make protobuf          # needs protoc + gogofaster
```

CI: `make test-ci` / `make test-ci-race` skip `*FLAKY*` tests; `make test-ci-flaky` runs only those. Beekeeper: `make beekeeper`, `make beelocal`, `make deploylocal`, `make testlocal`.

## Where things live

| Area | Location |
|------|-----------|
| CLI | `cmd/bee/` (Cobra + Viper; options in `cmd/bee/cmd/cmd.go`, env `BEE_*`) |
| Bootstrap | `pkg/node/node.go` — `NewBee()`, DI, `Shutdown()` |
| HTTP API | `pkg/api/router.go` — technical debug, business debug, public API; `openapi/Swarm.yaml` |
| P2P | `pkg/p2p/libp2p/`; protobuf per protocol under `*/pb/` (`doc.go` + `go:generate`) |
| Core types | `pkg/swarm/` — `Address`, `Chunk`, `ChunkSize` (4096) + span (8) → `ChunkWithSpanSize` 4104 |
| Store | `pkg/storer/`, `pkg/storage/`, `pkg/sharky/`, `pkg/statestore/`, `pkg/shed/` |
| Chunks | `pkg/cac/`, `pkg/soc/` |
| Postage | `pkg/postage/` (+ `listener/`, `postagecontract/`) |
| Incentives | `pkg/storageincentives/` |

Protocols to remember by name: `pushsync`, `pullsync`, `retrieval`, `pingpong`, `hive`, `pricing`.

## Rules Claude should not forget

- **Errors:** propagate; do not log and return the same error. Wrap with `fmt.Errorf("…: %w", err)`. Skip noisy "failed to" chains.
- **Tests:** prefer `package foo_test`; `export_test.go` for test-only exports; `t.Parallel()` where safe; flaky tests end with `FLAKY`; integration uses `-tags=integration`.
- **Go files:** Swarm copyright header (see `goheader` in `.golangci.yml`). American English. No `init()` unless unavoidable. No `fmt.Print` outside `cmd/bee/cmd/` (forbidigo).
- **Commits:** Conventional Commits; types in `commitlint.config.js`; header ≤100 chars, footer lines ≤72.
- **Product facts:** Bee release version is not strict SemVer; HTTP API version in `openapi/Swarm.yaml` is. XOR proximity, not numeric order. `MaxPO` 31, `ExtendedPO` 36.

When in doubt, read `AGENTS.md` for the long-form map and pitfalls, or import the coding docs above.
