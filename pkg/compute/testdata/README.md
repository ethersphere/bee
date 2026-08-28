# compute test fixtures

Each `*.wasm` module in this directory has its WebAssembly text source next to it
as `*.wat`. The `.wat` file is the source of record; regenerate a module after
editing it with either assembler:

    wasm-tools parse <name>.wat -o <name>.wasm
    wat2wasm <name>.wat -o <name>.wasm

The modules are deliberately tiny and hand-written so the sandbox behaviour they
exercise stays obvious.

## Sandbox fixtures

Output, traps, exits, rejected imports, memory limits, non-termination and
request metadata: `writer`, `echo`, `entrypoint`, `exit1`, `trap`, `infloop`,
`bigmem`, `badimport`, `method`.

## Host fixtures

These import the `swarm` module and exercise the guest ABI (see
`../README.md`). They read their arguments from stdin and write fixed-width
little-endian fields to stdout so a test can assert the exact result code a
module observes:

| Fixture | stdin | stdout |
|---|---|---|
| `hostbytesget` | `[32-byte address][4-byte buffer length]` | `[errno][required length][payload]` |
| `hostbytesput` | `[32-byte batch id][payload]` | `[errno][32-byte reference]` |
| `hostchunk` | `[32-byte batch id][chunk data]` | `[put errno][get errno][retrieved chunk]` |
| `hostnested` | `[32-byte module address][nested input]` | `[errno][output length][nested output]` |
| `hostcalls` | `[32-byte address]` | `[successful calls][errno that stopped the loop]` |
| `hostbadptr` | — | `[errno]` |
| `hostputtrap` | `[32-byte batch id][payload]` | `[errno][32-byte reference]`, then traps |
| `hostunknown` | — | — (imports a function the host module does not define) |

Each fixture writes its payload field only when the call succeeded, so a
non-zero result code yields the fixed-width fields alone.
