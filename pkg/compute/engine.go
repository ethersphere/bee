// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package compute runs untrusted WebAssembly modules downloaded from Swarm in a
// sandboxed execution engine and returns the result of the computation.
//
// This is an experimental prototype. Modules run in-process with wazero and may
// call back into the node through the swarm host module to read and write Swarm
// data (see Host). Two properties the production design calls for are therefore
// absent, deferred rather than rejected:
//
//   - Output is NOT reproducible across nodes. A host call reads what this node
//     happens to hold, and wazero has no gas metering, so there is no
//     deterministic budget bounding the work a module may do.
//   - There is no process boundary. An engine escape lands in the node's own
//     address space, which is tolerable only because wazero is pure Go and
//     memory-safe. Keep the endpoint off public gateways.
//
// What does hold: node work is bounded (a concurrency semaphore, a wall-clock
// watchdog and the host-call budgets in Limits), and a node-local failure is
// never laundered into a program verdict — it surfaces as StatusHostError.
package compute

import "context"

// Status classifies the outcome of a WASM execution.
//
// StatusOK, StatusTrap and StatusInvalidModule are program verdicts and are intended to be deterministic across nodes. StatusHostError
// signals an infrastructure failure local to this node (spawn failure, watchdog
// kill, IPC error) and must never be treated as a program result.
type Status uint8

const (
	// StatusOK indicates the module ran to completion and produced output.
	StatusOK Status = iota + 1
	// StatusTrap indicates the module trapped (unreachable, out-of-bounds, non-zero exit, ...).
	StatusTrap
	// StatusInvalidModule indicates the bytes failed validation/compilation or import checks.
	StatusInvalidModule
	// StatusHostError indicates a non-deterministic infrastructure failure on this node.
	StatusHostError
)

// String returns a stable, lower-kebab representation used in responses and headers.
func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusTrap:
		return "trap"
	case StatusInvalidModule:
		return "invalid-module"
	case StatusHostError:
		return "host-error"
	default:
		return "unknown"
	}
}

// Result is the outcome of executing a module.
type Result struct {
	Status      Status
	Output      []byte
	TrapMessage string
	// Response is the HTTP metadata the guest set through swarm_response_status
	// and swarm_response_header. It is populated only on StatusOK: a module that
	// trapped has no say in how its failure is rendered, exactly as a module that
	// trapped commits no upload.
	//
	// Note the deliberate asymmetry with Output, which a trapped module does keep
	// (see classifyRunError). Partial output is evidence about what went wrong;
	// partial response metadata would be an instruction the node should not follow.
	Response ResponseMeta
}

// Header is one response header the guest set. Duplicates are kept in the order
// they were set, because Link and Vary legitimately repeat.
type Header struct {
	Name  string
	Value string
}

// ResponseMeta is the HTTP status and headers a guest asked for. Its zero value
// means the guest asked for nothing, which is the pre-existing behaviour and is
// what Empty reports.
type ResponseMeta struct {
	// Status is the HTTP status code, or 0 when the guest did not set one.
	Status int
	// Headers are the accepted headers, in the order the guest set them.
	Headers []Header
}

// Empty reports whether the guest set no response metadata at all. The API layer
// uses it to decide whether a wildcard Accept still means "give me the envelope".
func (r ResponseMeta) Empty() bool {
	return r.Status == 0 && len(r.Headers) == 0
}

// Request describes a single execution: the module to run, the caller-supplied
// input and the request metadata the module is allowed to observe.
//
// Every field but Host is derived from the incoming HTTP request. Host calls
// read node-local state, so a module using them is not reproducible across
// nodes; see the package documentation.
type Request struct {
	// Module is the WASM binary to execute.
	Module []byte
	// Method is the HTTP method the endpoint was called with. It is exposed to
	// the guest as the REQUEST_METHOD environment variable, following CGI
	// convention. An empty value means no method is exposed.
	Method string
	// Input is the request body, handed to the guest on stdin.
	Input []byte
	// Env carries the rest of the request metadata, CGI-style: PATH_INFO,
	// QUERY_STRING, the allowlisted HTTP_* headers and so on.
	//
	// It is an ordered slice rather than a map because the guest can observe the
	// order through environ_get, and Go's map iteration is random: a map would
	// make a module's view of its own environment differ between two runs on the
	// same node for no reason.
	//
	// REQUEST_METHOD comes from Method, not from here; a duplicate entry is
	// ignored. The host environment is never inherited.
	Env []EnvVar
	// Limits bound the execution.
	Limits Limits
	// Host serves the calls the module makes back into the node. It is
	// per-request: uploads it performs belong to one execution and no more.
	//
	// A nil Host leaves the swarm module uninstantiated, so a module importing
	// it is StatusInvalidModule rather than trapping mid-run.
	Host Host
}

// EnvVar is one environment variable the endpoint derived from the request.
type EnvVar struct {
	Name  string
	Value string
}

// Engine executes a single WASM module in isolation and returns its Result.
//
// A non-nil error is reserved for infrastructure failures (the engine could not
// run the module at all); program-level outcomes such as traps or invalid
// modules are reported through Result.Status with a nil error so callers can
// treat them as deterministic verdicts.
type Engine interface {
	Execute(ctx context.Context, req Request) (Result, error)
	Close() error
}
