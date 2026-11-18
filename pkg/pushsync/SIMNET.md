# Pushsync and retrieval network simulator

The simulator runs the real `pushsync.PushSync` and `retrieval.Service` code
of every node in one process. The network between the nodes, the peer lists
and the health marks are artificial. Time is virtual (`testing/synctest`), so
a run of many minutes of network time takes seconds to minutes.

It exists to measure changes of the forwarder code, which cannot be installed
on the mainnet nodes of other operators.

## Files

| File | Content |
|---|---|
| `pkg/pushsync/simnet_test.go` | Network, topology, stream transport, upload run and its report |
| `pkg/pushsync/simnet_run_test.go` | Upload scenarios and `TestSimUpload` |
| `pkg/pushsync/simnet_switch_test.go` | Test of the switches |
| `pkg/retrieval/simnet_scaffold_test.go` | Copy of the network part of `pkg/pushsync/simnet_test.go` (from the start of the file to `// simStore`). Keep the two copies the same. |
| `pkg/retrieval/simnet_test.go` | Download runs |

## Switches

The switches are fields that are not exported and are set only through
`export_test.go`. Each one selects an old code path.

| Switch | Package | Effect when set |
|---|---|---|
| `noHopMemory` | pushsync | The origin selects the first hop without first-hop memory |
| `legacyForwarder` | pushsync | The forwarder acts as in v2.8.3 (written from the code, the v2.8.3 binary is not run) |
| `healthFirst` | retrieval | The origin asks healthy peers at any distance first |
| `forwarderHealthFirst` | retrieval | A forwarder asks healthy peers at any distance first |

## How to run

All simulator tests are skipped without the `SIMNET` environment variable,
except the small smoke tests.

```sh
# upload: one or more scenario names of simScenarios(), or "all" (about 10 minutes)
SIMNET=popmainnet go test ./pkg/pushsync/ -run TestSimUpload -count=1 -v -timeout 30m

# download: nodes1000, nodes4500, peerhealth or all
SIMNET=peerhealth go test ./pkg/retrieval/ -run 'TestSimDownload$' -count=1 -v -timeout 30m

# download with a model of a peer that is not healthy; SIMMODE is optional
SIMNET=unhealthy SIMMODE=slow,stall,missing go test ./pkg/retrieval/ -run TestSimDownloadUnhealthy -count=1 -v -timeout 60m

# download when a share of the forwarders has the change in closestPeer
SIMNET=mixed go test ./pkg/retrieval/ -run TestSimDownloadMixed -count=1 -v -timeout 30m
```

## Reference numbers

`popmainnet` (4,500 nodes, storage radius 10): 31.2 shallow receipts per 100
chunks without first-hop memory and 2.2 with it, no chunk that did not sync.
Mainnet uploads gave 23–65 and 1–6.

## Limits

- No accounting limits.
- The peer lists do not change during a run.
- The health marks are artificial. With `peerHealth` every node sees the same
  mark for a peer, which is the most difficult case.
- Use the `populated` scenarios for comparisons. Without `populated`, some
  neighborhoods have no node.
- The absolute retrieval numbers do not apply to mainnet. Only the sequence
  of the variants applies.
- The parameters of the models of a peer that is not healthy (300 ms, 30 %,
  50 %) are a selection, not a measurement.
