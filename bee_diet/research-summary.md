# Research summary

To explore splitting bee into two processes, we need to analyze packages, identify the dependency cone, and propose a natural “seam” where a split is feasible.
As supporting evidence, see the dependency graph: [merged.json](./endpoint-graph/demo_graph/merged.json) ([SVG](./endpoint-graph/demo_graph/merged.svg)).

For more about the tool and how to use it, see the [endpoint-graph README](./endpoint-graph/README.en.md).

After manually analyzing the resulting dependency graph, packages fall into three categories as expected: **essential**, **envelope**, and **shared**. The package list below is incomplete; there are more than a hundred packages in total.

## Essential

Packages without which bee stops being a full network node. On the dependency graph these packages are often (but not always) terminal nodes where many paths converge.
They can be highlighted in red when exporting to GraphML: `export-yed -essential-color=true`.

For easier analysis they are grouped into subcategories.

### storage

Persistent node storage.

- `pkg/storer`
- `pkg/statestore/storeadapter`
- `pkg/postage`
- `pkg/storage/leveldbstore`
- `pkg/sharky`
- `pkg/shed`

### network

Transport and p2p connectivity.

- `pkg/p2p/libp2p`
- `pkg/hive`
- `pkg/pingpong`
- `pkg/skipper`
- `pkg/bzz`

### routing

Overlay routing (topology).

- `pkg/topology/kademlia`
- `pkg/stabilization`
- `pkg/discovery`

### chain

L1, contracts, on-chain sync.

- `pkg/transaction`
- `pkg/settlement/swap`
- `pkg/settlement/swap/chequebook`
- `pkg/settlement/swap/erc20`
- `pkg/postage/postagecontract`
- `pkg/postage/listener`

### protocol-chunks

Chunk movement and sync over the network layer.

- `pkg/pushsync`
- `pkg/pullsync`
- `pkg/retrieval`
- `pkg/pusher`
- `pkg/puller`

### economics

Off-chain settlement between peers.

- `pkg/accounting`
- `pkg/settlement/pseudosettle`

### incentives

Storage incentive game.

- `pkg/storageincentives`
- `pkg/storageincentives/redistribution`
- `pkg/storageincentives/staking`

### operator

Node introspection and health.

- `pkg/status`
- `pkg/salud`

### Packages which can be extracted, but bee is the only owner

- `pkg/util`
- `pkg/spinlock`
- `pkg/keystore`
- `pkg/rate`
- `pkg/ratelimit`

## Shared

Bee library packages and contract packages (often interfaces). These can be moved to a separate library repository.

Library packages on the graph are also often terminal nodes. Contract packages are “mid-path” nodes that frequently depend on essential packages.

- `pkg/bigint`
- `pkg/bitvector`
- `pkg/bmt`
- `pkg/bmtpool`
- `pkg/crypto`
- `pkg/crypto/eip712`
- `pkg/crypto/swarm`
- `pkg/log`
- `pkg/sctx`
- `pkg/resolver`
- `pkg/tracing`
- `pkg/cac`
- `pkg/metrics` (small package, can be duplicated on both sides or removed)
- `pkg/soc`

## Envelope

The hardest layer of packages to classify.

These form the “user-facing” layer for interacting with a bee node. They can be moved to a separate repository, but they depend heavily on essential packages (especially storage).

- `pkg/file`
- `pkg/manifest`
- `pkg/feed`
- `pkg/replicas`
- `pkg/gsoc`
- `pkg/jsonhttp`
- `pkg/accesscontrol`

## Proposed split

Interfaces for talking to essential packages already exist. A good example is the interfaces in `pkg/storage/chunkstore.go` and `pkg/storage/statestore.go`. These packages are contracts by nature and can serve as a natural split boundary. Where such a package is missing, we propose adding one.

### Storage-level split experiment

The user-facing layer stops talking to node storage directly and uses the interfaces above instead. A gRPC server and client were implemented: the calling package uses a gRPC client on storage access, and the gRPC server wraps storage methods. Everything still runs inside a single binary.

A benchmark measures allocation size and count when moving chunks between two processes, to estimate the “cost” of splitting bee.

#### Benchmark

Compares four modes on a **single real** in-memory `storer.DB`:

- **Direct** — put a chunk into storage without gRPC; baseline. Each chunk is written separately.
- **Unary** — each chunk is sent over a newly opened gRPC connection.
- **Streaming** — one gRPC stream is opened; all chunks go through one connection.
- **BatchStreaming16** — one gRPC stream is opened; chunks are batched on the client (batch size 16).

Each gRPC `Put` inside the measurement repeats:

- `codec.ChunkToProto`
- `Stamp.MarshalBinary`
- creation of a new protobuf request
- marshal / unmarshal
- `storer.DirectUpload().Put`
- wait for pusher ack

A single protobuf request is **not** reused across `Put` calls.

Full matrix:

```bash
go test ./pkg/storer -run '^$' \
  -bench '^BenchmarkDirectUploadProductionPath$' \
  -benchmem -benchtime=1x -count=10
```

Targeted run (full chunk, all file sizes):

```bash
go test ./pkg/storer -run '^$' \
  -bench '^BenchmarkDirectUploadProductionPath$/^(Direct|Unary|Streaming|BatchStreaming16)$/^payload_4096$/^file_chunks_(1|16|128|1024|10000)$' \
  -benchmem -benchtime=1x -count=10
```

##### Measurement environment

- **CPU:** AMD Ryzen 7 PRO 4750U
- **OS:** linux/amd64
- **Go:** 1.26 (see `go.mod`)
- **Transport:** `google.golang.org/grpc/test/bufconn`
- **Repeats:** `-count=10`; median used for analysis

##### Measurement matrix

| Parameter        | Values                                      |
|------------------|---------------------------------------------|
| Payload (bytes)  | 1, 32, 128, 512, 1024, 2048, 4096           |
| Chunks per file  | 1, 16, 128, 1024, 10000                     |
| Batch size       | 16 (`BatchStreaming16` only)                |
| Fixture ring     | up to 1024 unique stamped CAC chunks        |

##### Results: file = 10,000 chunks, each chunk 4096 B

One benchmark operation (`op`) = **uploading one file**:

```
Open session → Put N chunks → flush/close stream → Done
```

Standard `ns/op`, `B/op`, `allocs/op` refer to the **file**.

The benchmark also normalizes per chunk:

- `B/chunk` — total heap allocations per PUT for one chunk
- `B/alloc` — average size of one allocation
- `allocs/chunk` — heap allocations per PUT for one chunk
- `ns/chunk` — amortized time

`B/chunk` and `allocs/chunk` are computed via `runtime.MemStats` around the full session lifecycle.

Median of 10 runs.

| Mode                 | B/chunk | allocs/chunk | B/alloc | ns/chunk | vs Direct                 |
|----------------------|---------|--------------|---------|----------|---------------------------|
| **Direct**           | 346 B   | 6.0          | 57.5 B  | 7.1 µs   | —                         |
| **Unary**            | 14.7 KB | 161          | 91.6 B  | 152 µs   | ~27× allocs, ~22× time    |
| **Streaming**        | 6.6 KB  | 27.4         | 240 B   | 14.2 µs  | ~4.5× allocs, ~2× time    |
| **BatchStreaming16** | 6.4 KB  | 20.9         | 336 B   | 15.9 µs  | ~3.5× allocs, ~2.2× time   |

Why `B/chunk` differs so much between direct upload and gRPC upload.

With direct upload, the chunk payload is already in memory before measurement starts; payload memory is not reallocated afterward.
With any gRPC upload, marshal/unmarshal allocates a buffer of at least chunk payload size (4096 B) even though the chunk is already in memory, plus extra metadata and connection overhead.

##### Next steps

1. Find a more efficient way to move data between processes.
2. Decide which component owns which package; that may change whether a package should be extracted.
