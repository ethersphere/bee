# Follow-up TF: fuzz targets (deferred)

**Status:** deferred on 2026-10-15 to save tokens during implementation. Not in the serial order of
[EXECUTION.md](EXECUTION.md) §5. Run it as a separate task once the phases it covers have landed,
and at the latest before any mainnet rollout. Until then the code does **not** meet the AGENTS.md
rule that every parser/decoder of untrusted input has a fuzz target.

**Rule while deferred (README "Ground rules"):** tasks write no `*_fuzz_test.go` files and run
neither `make fuzz` nor `make fuzz-regression`. A task that adds a parser, decoder or validator of
peer input appends one line to the log below: the task, the function, the file, and what a fuzz
target should assert. Existing fuzz targets stay, and `go test` runs their seeds.

## Fuzz targets that already exist (written before the deferral)

`pkg/rangesync`: `FuzzParseRange`, `FuzzParseFingerprint`, `FuzzParseItems`, `FuzzParseStatus`
(`wire_fuzz_test.go`), `FuzzSessionHandle`, `FuzzSessionConverges` (`session_fuzz_test.go`).
`pkg/rangesync/fingerprint`: `FuzzFenwick`, `FuzzIndex`, `FuzzLtHashAddSub`, `FuzzParseSymbolData`,
`FuzzDecoderAdd`, `FuzzParseID`, `FuzzRange`. `pkg/rangesync/pb`: `FuzzUnmarshal`.
`pkg/storer/internal/reserve`: `FuzzChunkAddrItemUnmarshal`, `FuzzChunkAddrValidItemUnmarshal`.

T1.9 wrote `pkg/rangesync/service_fuzz_test.go` before the deferral. It stays if it passes as an
ordinary test; nobody extends or debugs it until TF.

## Planned targets (from task texts, removed from the tasks)

| From | Target | Notes |
|---|---|---|
| T1.9 | `status` / `reconcile` stream handlers fed malformed messages | Extend `service_fuzz_test.go` |
| T1.10 | `fetch` handler fed malformed `Want` | Delivery validation on the requester side too |
| T1.13 | Client and handler fuzz targets modelled on `pkg/pullsync/client_fuzz_test.go` and `handler_fuzz_test.go` | Copied patterns, new files |
| T2.2 | Extend `FuzzIndex` to the symbol caches (root, level-1, bucket Fenwick vs `Encode` over brute force) | T2.2 keeps a seeded randomized test |
| T2.4 | Extend `FuzzSessionHandle` / `FuzzSessionConverges` to `Symbols` / `More` parts (sessions with `Config.Status`/`PeerStatus` set, else symbols are a violation), plus `ParseSymbols` / `ParseMore` in `wire.go` | T2.4 keeps table tests (`TestSessionSymbolsValidation`, `TestScopeCheckMessage`) |
| T2.5 | `More` past cap / past budget / unknown range, > 64 scan-path ranges, endless `More` as seeds | T2.5 keeps them as table tests |
| T2.7 | `FuzzParseSymbolData` / `FuzzDecoderAdd` with `IntegerMapper` | Only if T2.7 runs |
| all | `make fuzz` (mutation, not just seeds) over every `rangesync` target, with a time budget | Record findings here |

## Log: decoders added while fuzzing is deferred

_(tasks append here: `- Tn.m: Func in path/file.go — what to assert`)_
- T1.9: handleStatus in pkg/rangesync/status.go — any bytes: no panic, Status recorded only if ParseStatus accepts, else reset + Fail
- T1.9: handleReconcile / readMessage (strictMessage) in pkg/rangesync/reconcile.go, stream.go — any byte stream: no panic, oversize/undecodable → ErrInvalidMessage + reset, slots released
- T1.10: handleFetch / parseWant in pkg/rangesync/fetch.go — any byte stream: no panic, oversize/undecodable/empty/>250/invalid or duplicate ID → ErrInvalidMessage + reset, stream slot released
- T1.10: Service.fetch / deliver (Delivery reader) in pkg/rangesync/fetch.go — any Delivery stream: no panic, only requested valid chunks stored, every claim ended on return, violations → ErrInvalidMessage + disconnect
- T2.4: Session.handleSymbols / handleMore in pkg/rangesync/session_symbols.go — any Symbols/More parts: no panic, errors wrap ErrInvalidMessage (or ErrMappingMismatch), wants ⊆ IDs inside the part's range, never more symbols kept than the UpTo this side requested
- T2.5: Session.handleSymbols / handleMore / served.emit in pkg/rangesync/session_symbols.go — seeds: More past cap / past budget / unknown range, > 64 scan-path ranges, endless More, Count lies; assert sent and received symbols ≤ SessionSymbolBudget, ≤ MaxScansPerMessage scan batches per Message, More.UpTo ≤ cap(R)
- T4.5 part 2b (rules as of fix round 3): Service.FetchWithResult (shortfall, resets) and handleStatus (spam) in pkg/rangesync/fetch.go, shortfall.go, status.go — no new parser; add to the T1.10 and T1.9 targets: at most one Fail per fetch call, none while the peer is marked (violationGrace after a violation, marked only while it has a Status), and no shortfall or reset Fail without the peer's Status; a Fail only for a blatant shortfall (≥ half of ≥ 64 requested IDs missing, after root-move excuses, plus invalid-on-every-node stamps), an offense (IDs the peer left missing again within shortfallWindow plus invalid stamps above max(4, requested/100); the first a Fail wrapping only ErrFetchShortfall, a repeat a violation), a fetch stream that resets or fails to open (not p2p.ErrPeerNotFound, not this node's cancellation) past maxFetchResets then one per fetchResetInterval, or another violation; over-rate Statuses Fail only past PeerStatusDropBurst drops
- T4.5 part 2b fix round 1: handleStatus root check in pkg/rangesync/status.go — any Status: a root that does not contain the sender's overlay → reset + Fail + disconnect, never recorded; one that does is recorded as before
- T4.5 part 2c: Service.FetchWithResult in pkg/rangesync/fetch.go — no new parser; this updates the part 2b line for the T1.10 target: a fetch stream that fails or stalls while it opens is never a Fail (no claim held, no fetch rate taken while it opens, nothing Undelivered); only a stream whose Want claimed IDs and then reset counts towards maxFetchResets; a stall (no deliveries within FetchTimeout of the claims) is a Fail only while the peer has a Status
- T4.5 part 2c fix round 1: Service.FetchWithResult in pkg/rangesync/fetch.go, streamlimit.go — no new parser; add to the T1.10 target: a failed or timed-out fetch open backs off the peer's opens (openBackoffBase doubling to maxOpenBackoff, one probe at a time, cleared by a successful open or a disconnect), calls during it fail with errOpenBackoff without a claim or a Fail; past maxFetchWaiters waiting calls a call fails with errFetchQueueFull; neither is ever a Fail
