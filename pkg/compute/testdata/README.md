# compute test fixtures

Each `*.wasm` module in this directory has its WebAssembly text source next to it
as `*.wat`. The `.wat` file is the source of record; regenerate a module after
editing it with:

    wat2wasm <name>.wat -o <name>.wasm

The modules are deliberately tiny and hand-written so the sandbox behaviour they
exercise (output, traps, exits, rejected imports, memory limits, non-termination,
request metadata) stays obvious.
