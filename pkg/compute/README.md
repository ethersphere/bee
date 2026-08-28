# The `swarm` guest ABI

A module executed through `POST /@/{address}` runs in a WASI sandbox. Alongside
`wasi_snapshot_preview1` it may import a host module named `swarm`, through which
it reaches the node it is running on.

> **Experimental.** Output is not reproducible across nodes: a host call reads
> what *this* node happens to hold. There is no gas metering and no process
> boundary. Node work is bounded by budgets, not by a work-based limit. Do not
> enable the endpoint on a public gateway.

## Functions

```wat
(import "swarm" "swarm_bytes_get"
  (func (param i32 i32 i32 i32) (result i32)))       ;; addr_ptr, buf_ptr, buf_len, out_len_ptr
(import "swarm" "swarm_bytes_put"
  (func (param i32 i32 i32 i32) (result i32)))       ;; batch_ptr, data_ptr, data_len, out_addr_ptr
(import "swarm" "swarm_chunk_get"
  (func (param i32 i32 i32 i32) (result i32)))       ;; addr_ptr, buf_ptr, buf_len, out_len_ptr
(import "swarm" "swarm_chunk_put"
  (func (param i32 i32 i32 i32) (result i32)))       ;; batch_ptr, data_ptr, data_len, out_addr_ptr
(import "swarm" "swarm_execute"
  (func (param i32 i32 i32 i32 i32 i32) (result i32)));; addr_ptr, input_ptr, input_len, buf_ptr, buf_len, out_len_ptr
```

`bytes_*` moves data of arbitrary length through the same splitter and joiner
the `/bytes` endpoints use. `chunk_*` is the raw single-chunk pair: `chunk_put`
takes at most 4104 bytes (an 8-byte span followed by up to 4096 bytes of data)
and `chunk_get` yields a chunk's data verbatim.

Addresses, references and batch ids are always 32 bytes, so they carry no length
argument. `out_addr_ptr` must have 32 writable bytes.

Importing a name the host module does not define, or any module other than
`swarm` and `wasi_snapshot_preview1`, is rejected before the module runs — the
result is `invalid-module`, never a trap partway through.

## Result codes

Every function returns a code rather than trapping, so a module can react to a
refusal instead of dying:

| Code | Name | Meaning |
|---|---|---|
| 0 | `OK` | the call succeeded |
| 1 | `NOT_FOUND` | nothing is stored at that address |
| 2 | `DENIED` | the node refused: an unusable postage batch, or a second batch in one execution |
| 3 | `BUDGET_EXHAUSTED` | the call, byte or depth budget is spent |
| 4 | `BUFFER_TOO_SMALL` | the payload does not fit `buf_len`; the required length is at `out_len_ptr` |
| 5 | `INVALID` | a pointer is out of bounds, or the arguments are malformed |
| 6 | `EXEC_FAILED` | `swarm_execute` ran the nested module and it trapped or was invalid |

A pointer outside linear memory is `INVALID`, not a trap: the host bounds-checks
every offset it is given.

Failures **inside the node** — the storer erroring, the watchdog firing — are
never reported through these codes. They end the execution with status
`host-error` and a 500, because a node-local failure is not a verdict on the
program.

## Reading data: the two-call pattern

The caller provides the buffer, so the host never grows guest memory mid-call.
`out_len_ptr` always receives the required length, including when the buffer was
too small, which gives the usual probe-then-fetch pattern without a second entry
point:

```wat
;; probe with no buffer to learn the size
(call $bytes_get (local.get $addr) (i32.const 0) (i32.const 0) (i32.const 36))
;; ... allocate (i32.load (i32.const 36)) bytes, then ask again
(call $bytes_get (local.get $addr) (local.get $buf) (local.get $len) (i32.const 36))
```

A probe costs one host call but no bytes: the byte budget is charged only on
delivery.

## Uploads

The guest supplies the postage batch id, which it can only have received as
input — it has no way to enumerate the node's batches. The node resolves it
exactly as `POST /chunks` does, so the WASM path grants no authority the HTTP
API does not already grant. An unusable or unknown batch is `DENIED`.

One execution gets **one upload session and therefore one batch**: a put naming a
different batch than the first is `DENIED`.

Uploads are **deferred**. A put returns once the chunk is stored locally and the
pusher syncs it afterwards, so a host call never blocks on network round trips.
Two consequences:

- When the HTTP response returns, the data is in the local upload store but not
  yet acknowledged by the network.
- The session is committed only if the execution succeeds. A module that traps,
  or is cut off, leaves nothing behind.

Encryption and redundancy are not exposed: `bytes_put` always writes
unencrypted at the default redundancy level, which is what keeps a reference
32 bytes wide.

## Nested execution

`swarm_execute` fetches a module from Swarm and runs it, handing it `input` on
stdin and returning its stdout. The budgets are **shared across the whole call
tree**, so a module cannot multiply its allowance by recursing. Nesting is
bounded by the depth limit; a cycle simply runs out of depth.

A nested module is always run as a WASI command: the caller's
`Swarm-Wasm-Entrypoint` applies to the outermost module only.

## Budgets

| Bound | Default | Header | Stops |
|---|---|---|---|
| host calls | 64 | `Swarm-Wasm-Host-Calls-Limit` | fetch amplification |
| host bytes | 32 MiB | `Swarm-Wasm-Host-Bytes-Limit` | memory and bandwidth blowup |
| depth | 4 | `Swarm-Wasm-Depth-Limit` | runaway recursion |

Headers may only lower a limit; the operator's configured maximum wins. The byte
budget is one pool counting both directions — what the node hands the guest and
what it accepts from it. An upload is charged its declared length before the
splitter runs, so an oversized put is refused without the node ever chunking it.

The depth limit counts execution levels including the outermost, so `1` permits
no nesting at all.

## WASI

The whole of `wasi_snapshot_preview1` is available, `random_get` and
`clock_time_get` included, which is what lets modules built by Rust std, TinyGo
and Go run without special builds. That is a prototype convenience, not a
portability guarantee: a deterministic engine would restrict this surface.

Request metadata is exposed CGI-style: `REQUEST_METHOD` carries the HTTP method
the endpoint was called with. The host environment is never inherited.

## Examples

Hand-written fixtures covering every call and result code live in
[`testdata/`](testdata/), with a table of their stdin and stdout layouts in
[`testdata/README.md`](testdata/README.md).
