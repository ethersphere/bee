# Storage contracts (bee-diet-experiments)

MVP gRPC seam based on `pkg/storage/chunkstore.go` and `pkg/storage/statestore.go`.

## Generate protobuf

```bash
make -C contracts protobuf
```

## Direct upload allocation benchmark

The benchmark measures stamped chunks with payloads of 1, 32, 128, 512, 1024,
2048, and 4096 bytes. It reports separate results for protobuf encoding and
decoding, a direct storage interface call, unary `DirectUpload.Put`, one reused
client stream, and batches of 16 chunks. The transport cases use generated
protobuf clients, in-memory gRPC, a server with RPC logging disabled, and a
no-op storage session. This keeps logging, client adapter, storage, pushsync,
fixture creation, connection setup, session setup, and warm-up calls outside
the measured interval.

Run the complete matrix:

```bash
go test ./contracts/client -run '^$' \
  -bench '^BenchmarkDirectUploadChunkAllocations$' -benchmem
```

For stable `B/op` and `allocs/op`, use a fixed iteration count and compare
several independent runs:

```bash
go test ./contracts/client -run '^$' \
  -bench '^BenchmarkDirectUploadChunkAllocations$' \
  -benchmem -benchtime=10000x -count=5
```

Capture every allocation for one layer and one payload size per process. This
avoids mixing allocation profiles from different cases:

```bash
go test ./contracts/client -run '^$' \
  -bench '^BenchmarkDirectUploadChunkAllocations$/^Unary$/^payload_4096$' \
  -benchtime=10000x \
  -memprofile /tmp/direct-upload-4096.mem.pprof -memprofilerate=1
go tool pprof -top -alloc_objects /tmp/direct-upload-4096.mem.pprof
go tool pprof -top -alloc_space /tmp/direct-upload-4096.mem.pprof
```

Use `payload_1` instead of `payload_4096` for the minimum-size profile. Replace
`Unary` with `ClientStreaming`, `Batch16`, `DirectInterface`, `CodecEncode`, or
`CodecDecode` to isolate another transfer strategy, the no-op interface
baseline, or serialization stage. For a given call stack, dividing its
`alloc_space` by `alloc_objects` gives its average allocation size.

The benchmark reports `B/op` and `allocs/op` together with payload, chunk
(payload plus span), protobuf wire size, and chunks per RPC or stream. Standard
metrics are normalized per chunk: the streaming case sends all `b.N` chunks
through one stream, while the batch case uses up to 16 chunks per unary RPC.
Consequently, comparisons must use the same fixed `-benchtime` iteration count.
Heap profiles still contain one-time Go runtime and test setup allocations;
the fixed, large iteration count makes their contribution negligible. Run
without `-race` and without disabling compiler optimizations so the result
represents the production code path.

The benchmark above intentionally isolates individual layers with a no-op
session and prebuilt protobuf messages. It is a lower-bound transport
measurement, not the production endpoint path.

Use the production-path benchmark to compare direct storage, unary gRPC,
client-streaming gRPC, and client-streaming batches of 16. All modes use the
same real `storer.DB.DirectUpload` behavior and deterministic source chunks.
Each gRPC `Put` performs client-side chunk conversion and stamp marshaling.
The matrix covers payloads of 1 through 4096 bytes and files containing 1, 16,
128, 1024, or 10000 chunks:

```bash
go test ./pkg/storer -run '^$' \
  -bench '^BenchmarkDirectUploadProductionPath$' \
  -benchmem -benchtime=10x -count=10
```

Standard `B/op`, `allocs/op`, and `ns/op` values are per file. The benchmark
also reports normalized `B/chunk`, `allocs/chunk`, `B/alloc`, and `ns/chunk`.
Connection setup, deterministic chunk generation, and one warm-up upload are
outside the timer. Session Open, Put, stream flush/close, and Done are inside
the timer. RPC logging is disabled for every gRPC mode.

`bufconn` deliberately excludes TCP and process-boundary costs while retaining
protobuf and gRPC client/server work. A separate-process benchmark is required
to measure those additional costs.

## Runtime

- gRPC server listens on `:17337` (bee process, `contracts/server`).
- Envelope HTTP paths use `contracts/client` (`127.0.0.1:17337`) instead of in-process `storer.Download` / `Cache` / `ChunkStore`.
- Set `--storage-contract-disable` to keep the gRPC implementation available
  but route envelope, feeds, and stewardship through storage interfaces
  directly. gRPC remains enabled by default.

## Layout

- `proto/storage.proto` — contract spec
- `pb/` — generated `protoc` output (do not edit)
- `client/` — `storage.Getter` / `Putter` / `StateStorer` over gRPC
- `server/` — delegates to real storer + state store
