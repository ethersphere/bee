# Bee areas and how to test them

This file maps changed packages to what to exercise and which beekeeper check
**types** cover them. Beekeeper's `--checks` flag takes check **entry names**
from its config, not types. To find the entries for a type, run
`grep -n -B30 'type: <type>$' $BEEKEEPER_DIR/config/local.yaml` and take the
two-space-indented key above it (for example `ci-pushsync-chunks` has type
`pushsync`). The authoritative list of types is the `Checks` map in
`$BEEKEEPER_DIR/pkg/config/check.go`.

Known types: act, autotls, balances, cashout, datadurability, feed, feed-v1,
file-retrieval, full-connectivity, gc, gsoc, kademlia, load, longavailability,
manifest, manifest-v1, networkavailability, peer-count, pingpong, postage, pss,
pullsync, pushsync, redundancy, retrieval, settlements, smoke, soc, stake,
storage-radius, withdraw.

| Changed area | What to exercise | Beekeeper types |
|---|---|---|
| `pkg/pushsync`, `pkg/pusher` | Upload, then confirm the chunk is stored in its neighborhood. Include light-node uploads and stamp rejection. | pushsync, smoke, retrieval |
| `pkg/pullsync`, `pkg/puller` | Historical sync: a new or restarted node in a neighborhood catches up, and radius changes trigger resync. | pullsync, storage-radius, datadurability |
| `pkg/retrieval`, `pkg/storer/netstore.go` | Download from a node that does not store the chunk: forwarding, caching, and 404 versus timeout behavior. | retrieval, file-retrieval, smoke |
| `pkg/topology/kademlia`, `pkg/hive`, `pkg/addressbook`, `pkg/discovery` | Peer discovery, depth, neighborhood membership, reconnects, and convergence time after restarts. | kademlia, full-connectivity, peer-count |
| `pkg/p2p/libp2p` | Connect, handshake, stream handling, NAT and AutoTLS, and connection limits. | full-connectivity, peer-count, pingpong, autotls |
| `pkg/postage`, `pkg/postage/listener`, batch store | Buying, topping up, and diluting batches; listener catch-up from chain or snapshot; usable and expired batch transitions. | postage |
| `pkg/storageincentives` (agent, redistribution) | Participation over several rounds: commit, reveal, claim; sample timing; frozen or skipped rounds. No dedicated check exists, so watch `/redistributionstate` and logs on a staging or testnet full node. | stake |
| `pkg/storageincentives/staking` | Deposit, withdraw, minimum deposit, height changes, and the overlay and stake relationship. | stake, withdraw |
| `pkg/accounting`, `pkg/settlement`, `pkg/pricing`, `pkg/pricer` | Balances between peers, cheque issuance and cashout, and pseudosettle refresh. | balances, settlements, cashout |
| `pkg/pss` | Send and receive with topics, and targeting. | pss |
| `pkg/gsoc` | GSOC subscribe and send, including the websocket lifecycle. | gsoc |
| `pkg/soc` | SOC upload and validation, including invalid signatures. | soc |
| `pkg/feeds` | Feed update and lookup (sequence and epoch), and the v1 compatibility path. | feed, feed-v1 |
| `pkg/file` (joiner, splitter, pipeline, redundancy), `pkg/replicas` | Large and odd-sized files (boundary sizes such as 4096×128ⁿ±1), every redundancy level, and range requests. | file-retrieval, redundancy, smoke, load |
| `pkg/manifest` | `/bzz` collections, index and error documents, and path lookup. | manifest, manifest-v1 |
| `pkg/storer` (reserve, cache, sample, compact) | Reserve fill and eviction, radius decrease, cache GC, reserve sample correctness and duration, and restart with a full reserve. | gc, storage-radius, datadurability, longavailability |
| `pkg/storer/migration`, `pkg/statestore`, `pkg/shed`, `pkg/sharky` | Upgrade from BASE: start BASE, upload and pin data, stop, start HEAD on the same data directory, then verify that the data, pins, and stamps survive. Also check `bee db` commands. | (manual) then smoke |
| `pkg/accesscontrol` | ACT upload, download, grantee management. | act |
| `pkg/api` | Hit the endpoint directly: status codes, headers, error bodies. Diff against `openapi/`. Then run the check type for the subsystem behind the endpoint. | per area |
| `cmd/bee/cmd`, `pkg/node`, `packaging/` | New or changed flags, defaults, env (`BEE_*`) and YAML config parity, startup and shutdown, and full versus light startup. | smoke |
| `metrics.go` files, `pkg/metrics` | `curl :1633/metrics` shows the new series, with sane values under load. | (manual) |
| `pkg/salud`, `pkg/status`, `pkg/stabilization` | `/status`, `/health`, `/readiness`, and startup stabilization timing. | (manual) |

## Go test commands (only what CI does not run)

CI already runs every existing unit and race test once on every PR. Do not put
those in a test plan. Use `go test` only for:

- Flake hunt, when flakiness is a known risk: `go test -race -count=200 -run '^TestName$' ./pkg/<name>/`
- Benchmark BASE vs HEAD: `go test -run='^$' -bench='^BenchmarkX$' -benchmem -count=10 ./pkg/<name>/`, then `benchstat base.txt head.txt`
- Real fuzzing (CI replays seed corpora only): `go test -run='^$' -fuzz='^FuzzName$' -fuzztime=60s ./pkg/<name>/`
- Integration-tagged code, if CI does not run it: `go test -tags=integration ./pkg/<name>/...`
- A proposed **new** test: describe its setup and assertion in "Gaps not covered by CI".

## Cross-cutting triggers

- `.proto` changed: run a mixed-version cluster (some nodes on BASE, some on
  HEAD) and exercise that protocol in both directions.
- `openapi/` changed: confirm that `info.version` in `Swarm.yaml` was bumped,
  and that existing clients (bee-js, swarm-cli) still parse the responses.
- Default config value changed: test an upgraded node that has no explicit
  setting, because its behavior changes silently.
- On-chain logic changed: test against a real chain (staging or testnet), not
  only mocks.
