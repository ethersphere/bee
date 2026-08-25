// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package compute runs untrusted WebAssembly modules downloaded from Swarm in a
// sandboxed execution engine and returns the deterministic result of the
// computation.
//
// This is the phase-0 skeleton: it executes modules in-process with wazero and
// does NOT yet enforce deterministic gas metering. It is intended to validate
// the API, download and wiring path end-to-end and must not be relied upon for
// reproducible-across-nodes output. A later phase replaces the engine with an
// out-of-process wasmtime worker that meters execution by deterministic fuel.
package compute

import "context"

// Status classifies the outcome of a WASM execution.
//
// StatusOK, StatusOutOfFuel, StatusTrap and StatusInvalidModule are program
// verdicts and are intended to be deterministic across nodes. StatusHostError
// signals an infrastructure failure local to this node (spawn failure, watchdog
// kill, IPC error) and must never be treated as a program result.
type Status uint8

const (
	// StatusOK indicates the module ran to completion and produced output.
	StatusOK Status = iota + 1
	// StatusOutOfFuel indicates the module exceeded its deterministic gas budget.
	StatusOutOfFuel
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
	case StatusOutOfFuel:
		return "out-of-fuel"
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
	Status       Status
	Output       []byte
	FuelConsumed uint64
	TrapMessage  string
}

// Request describes a single execution: the module to run, the caller-supplied
// input and the request metadata the module is allowed to observe.
//
// Every field is derived from the incoming HTTP request, never from the host, so
// the same Request produces the same Result on every node.
type Request struct {
	// Module is the WASM binary to execute.
	Module []byte
	// Method is the HTTP method the endpoint was called with. It is exposed to
	// the guest as the REQUEST_METHOD environment variable, following CGI
	// convention. An empty value means no method is exposed.
	Method string
	// Input is the request body, handed to the guest on stdin.
	Input []byte
	// Limits bound the execution.
	Limits Limits
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
