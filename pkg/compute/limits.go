// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compute

// Limits bound a single execution. They are supplied per request (clamped by the
// API layer to the operator-configured maxima) and passed to the engine.
type Limits struct {
	// Fuel is the deterministic gas budget (instruction count). Zero means the
	// engine default. Not enforced by the phase-0 wazero engine.
	Fuel uint64
	// Memory is the maximum linear memory in bytes the module may allocate.
	Memory uint64
	// Entrypoint is the exported function to invoke. Empty selects the module's
	// WASI command entry (`_start`).
	Entrypoint string
}

const (
	// wasmPageSize is the size of a single WebAssembly memory page.
	wasmPageSize = 65536
	// maxWasmPages is the maximum number of pages a 32-bit WASM memory can address.
	maxWasmPages = 65536
)

// memoryPages converts a byte memory limit into a number of WASM pages, rounding
// up and clamping to the 32-bit maximum. A zero limit returns the maximum.
func (l Limits) memoryPages() uint32 {
	if l.Memory == 0 {
		return maxWasmPages
	}
	pages := (l.Memory + wasmPageSize - 1) / wasmPageSize
	if pages > maxWasmPages {
		return maxWasmPages
	}
	return uint32(pages)
}
