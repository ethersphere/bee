# The `swarm` guest ABI

A module executed through `/@/{address}` — any method, with an optional trailing
path — runs in a WASI sandbox. Alongside
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
(import "swarm" "swarm_response_status"
  (func (param i32) (result i32)))                    ;; code
(import "swarm" "swarm_response_header"
  (func (param i32 i32 i32 i32) (result i32)))        ;; name_ptr, name_len, val_ptr, val_len
```

The module is defined in two halves. The five data functions above exist only
when the node has node access; the two response functions always do, because
shaping a response causes no node work. A module that only sets a Content-Type
therefore runs on a node with node access switched off.

`bytes_*` moves data of arbitrary length through the same splitter and joiner
the `/bytes` endpoints use. `chunk_*` is the raw single-chunk pair: `chunk_put`
takes at most 4104 bytes (an 8-byte span followed by up to 4096 bytes of data)
and `chunk_get` yields a chunk's data verbatim.

Addresses, references and batch ids are always 32 bytes, so they carry no length
argument. `out_addr_ptr` must have 32 writable bytes.

Importing a name the host module does not define, or any module other than
`swarm` and `wasi_snapshot_preview1`, is rejected before the module runs — the
result is `invalid-module`, never a trap partway through. Importing a data
function on a node without node access is rejected the same way.

Note what is *not* checked: a module need not export its memory. It will not get
far without one — every call that moves bytes answers `INVALID` when there is no
memory to read — but that is a result code, not a rejection.

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
bounded by the depth limit; a cycle simply runs out of depth. The nested 
module also inherits the Request metadata, but it is not allowed to shape the
response.

A nested module is always run as a WASI command: the caller's
`Swarm-Wasm-Entrypoint` applies to the outermost module only.

## Shaping the response

By default the node decides how a module's output is rendered: the status comes
from the verdict and the content type from the caller's `Accept`. A module that
serves a web page needs to decide both itself, so it may set them:

```wat
(call $response_status (i32.const 404))
(call $response_header (local.get $name) (i32.const 12) (local.get $value) (i32.const 8))
```

Both return a result code and neither charges the host-call budget, because
neither causes the node to do any work. They have their own bounds instead —
32 headers and 8 KiB of name-plus-value by default, `BUDGET_EXHAUSTED` beyond
that — and hard per-field caps of 128 bytes for a name and 4 KiB for a value.
Lengths are checked before any guest memory is read, so an absurd `val_len` costs
nothing.

Rules worth knowing:

- **Only the outermost execution has a response.** A module reached through
  `swarm_execute` is a library call, not an HTTP request, so both functions
  answer `DENIED` there rather than letting a fetched module rewrite its caller's
  content type.
- **Only a clean run commits.** A module that traps sets no headers, exactly as
  it stores nothing. Its partial *output* does survive — that is evidence about
  what went wrong, while a header would be an instruction to follow.
- **A status must be 200–599.** 1xx is refused because an informational status
  desynchronises the connection. 5xx is allowed: a module must be able to report
  its own failure, and `Swarm-Wasm-Status` still says `ok`, which is what
  distinguishes the module's 500 from the node's.
- **Some names are refused** with `DENIED`: `Swarm-Wasm-*` (the node's verdict
  channel must not be forgeable), `Access-Control-*` (the node sets
  `Access-Control-Allow-Credentials`, so a guest widening CORS would be a real
  cross-origin credential leak), `Set-Cookie` and the origin-wide security
  headers (every module shares the node's origin with its authenticated API), and
  the hop-by-hop and framing headers. A malformed name or a value containing a
  control character is `INVALID`, which subsumes CR/LF injection.

How the metadata is rendered depends on what the caller asked for. `Accept:
application/json` reports it in the envelope as `httpStatus` and `headers`
without applying it — a client that asked to be told about the run did not ask to
have its transport reshaped. Every other representation applies it, the guest's
`Content-Type` replacing the negotiated default. A wildcard `Accept` reports by
default and applies once the module has set something, which is what lets a
browser load a stylesheet: a subresource request never names `text/html`.

## Request metadata

Exposed CGI-style. The host environment is never inherited, so a module sees the
same variables on every node.

| Variable | Value |
|---|---|
| `REQUEST_METHOD` | the HTTP method the endpoint was called with |
| `SCRIPT_NAME` | the mount point, e.g. `/@/{address}` — what a module builds self-links from |
| `PATH_INFO` | the path after the address: empty for `/@/a`, `/` for `/@/a/`, `/x/y` for `/@/a/x/y` |
| `QUERY_STRING` | the raw, undecoded query |
| `REQUEST_URI` | the full request target |
| `CONTENT_TYPE`, `CONTENT_LENGTH` | of the request body |
| `HTTP_*` | allowlisted request headers, upper-cased with `-` replaced by `_` |

Both `/@/{address}` and `/@/{address}/{path}` serve. Unlike `/bzz`, the bare form
is not redirected to the trailing-slash form: `POST /@/{address}` is how a module
is invoked, and most modules are pure compute for which a trailing slash means
nothing. A module that wants that redirect issues it itself, from `SCRIPT_NAME`.

Request headers are an **allowlist**, the mirror image of the response denylist:
a response header carries only what the guest already knows, while a request
header carries what the operator's clients send. The default list is `Accept`,
`Accept-Language`, `Host`, `If-None-Match`, `If-Modified-Since`, `Range`,
`Referer`, `User-Agent`, `X-Requested-With` and `Swarm-Postage-Batch-Id`.
`--wasm-request-headers` replaces it. `Authorization`, `Proxy-Authorization` and
`Cookie` can never be forwarded whatever is configured, and a configuration
naming one stops the node at startup.

The whole environment is capped (16 KiB by default). Overflow is a `431`, not a
truncation: a shortened environment would be a lie the module cannot detect.

## Budgets

| Bound | Default | Header | Stops |
|---|---|---|---|
| host calls | 64 | `Swarm-Wasm-Host-Calls-Limit` | fetch amplification |
| host bytes | 32 MiB | `Swarm-Wasm-Host-Bytes-Limit` | memory and bandwidth blowup |
| depth | 4 | `Swarm-Wasm-Depth-Limit` | runaway recursion |
| response headers | 32 | — | unbounded response metadata |
| response header bytes | 8 KiB | — | the same, by size |

The two response bounds carry no request header. The per-request overrides exist
so a *caller* can lower risk it is exposed to, and a caller is not exposed to
these: they cost it nothing, and lowering them could only break the module.

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

Request metadata is exposed through the environment, as described above.

## Examples

Hand-written fixtures covering every call and result code live in
[`testdata/`](testdata/), with a table of their stdin and stdout layouts in
[`testdata/README.md`](testdata/README.md).
