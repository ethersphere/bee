// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compute

// Limits bound a single execution. They are supplied per request (clamped by the
// API layer to the operator-configured maxima) and passed to the engine.
type Limits struct {
	// Memory is the maximum linear memory in bytes the module may allocate.
	Memory uint64
	// Entrypoint is the exported function to invoke. Empty selects the module's
	// WASI command entry (`_start`).
	Entrypoint string
	// MaxHostCalls bounds how many swarm host calls one execution tree may
	// make. Zero means defaultMaxHostCalls.
	MaxHostCalls uint32
	// MaxHostBytes bounds the total payload, in both directions, that the
	// swarm host calls of one execution tree may move. Zero means
	// defaultMaxHostBytes.
	MaxHostBytes uint64
	// MaxDepth bounds the number of execution levels a swarm_execute call tree
	// may reach, the outermost execution included, so 1 permits no nesting at
	// all. Zero means defaultMaxDepth.
	MaxDepth uint32
	// MaxResponseHeaders bounds how many response headers a guest may set.
	// Zero means defaultMaxResponseHeaders.
	MaxResponseHeaders uint32
	// MaxResponseHeaderBytes bounds the total size of the response headers a
	// guest may set, counting name and value. Zero means
	// defaultMaxResponseHeaderBytes.
	//
	// Unlike the host budgets these are not exposed as per-request headers. That
	// pattern exists so a caller can lower risk it is exposed to, and a caller is
	// not exposed to this one: setting a header costs the caller nothing, the
	// node's exposure is a few kilobytes, and lowering it can only break the
	// module.
	MaxResponseHeaderBytes uint32
}

// Defaults applied when a limit is left unset. They are deliberately modest:
// this engine has no work-based bound, so the host budgets are what stop a
// module from making the node fetch or store without end.
const (
	defaultMaxHostCalls           uint32 = 64
	defaultMaxHostBytes           uint64 = 32 << 20
	defaultMaxDepth               uint32 = 4
	defaultMaxResponseHeaders     uint32 = 32
	defaultMaxResponseHeaderBytes uint32 = 8 << 10
)

// Hard caps on a single response header, not configurable. They bound the work a
// rejected call can cause: both are checked before any guest memory is read, so a
// value length of 2^32-1 never allocates.
const (
	// MaxResponseHeaderNameLen is the longest response header name accepted.
	MaxResponseHeaderNameLen uint32 = 128
	// MaxResponseHeaderValueLen is the longest response header value accepted.
	MaxResponseHeaderValueLen uint32 = 4 << 10
)

// HostCalls, HostBytes and Depth resolve a limit against its default. They are
// exported because a Host has to agree with the engine on the numbers: it
// refuses oversized work before materialising it, which it can only do if it
// sees the same effective limit the budget was built from.

// HostCalls is the effective host-call limit.
func (l Limits) HostCalls() uint32 {
	if l.MaxHostCalls == 0 {
		return defaultMaxHostCalls
	}
	return l.MaxHostCalls
}

// HostBytes is the effective host-byte limit.
func (l Limits) HostBytes() uint64 {
	if l.MaxHostBytes == 0 {
		return defaultMaxHostBytes
	}
	return l.MaxHostBytes
}

// Depth is the effective limit on execution levels, the outermost included.
func (l Limits) Depth() uint32 {
	if l.MaxDepth == 0 {
		return defaultMaxDepth
	}
	return l.MaxDepth
}

// ResponseHeaders is the effective limit on how many headers a guest may set.
func (l Limits) ResponseHeaders() uint32 {
	if l.MaxResponseHeaders == 0 {
		return defaultMaxResponseHeaders
	}
	return l.MaxResponseHeaders
}

// ResponseHeaderBytes is the effective limit on the total size of a guest's
// response headers, counting name and value.
func (l Limits) ResponseHeaderBytes() uint32 {
	if l.MaxResponseHeaderBytes == 0 {
		return defaultMaxResponseHeaderBytes
	}
	return l.MaxResponseHeaderBytes
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
