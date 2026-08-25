// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compute

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

const (
	// wasiModuleName is the only host module the sandbox instantiates and
	// therefore the only one a guest may import from.
	wasiModuleName = wasi_snapshot_preview1.ModuleName
	// wasiEntrypoint is the entrypoint of a WASI command module.
	wasiEntrypoint = "_start"
	// envRequestMethod is the CGI-style environment variable carrying the HTTP
	// method the execute endpoint was called with.
	envRequestMethod = "REQUEST_METHOD"
)

// wazeroEngine is the phase-0, in-process execution engine.
//
// WARNING: it does NOT meter execution deterministically and it wires WASI
// stdin/stdout for I/O, so its output is not guaranteed reproducible across
// nodes. It exists to exercise the download/API/wiring path and to be swapped
// out for the deterministic out-of-process wasmtime worker.
type wazeroEngine struct {
	logger log.Logger
}

func newWazeroEngine(logger log.Logger) *wazeroEngine {
	return &wazeroEngine{logger: logger}
}

// Execute compiles and runs the module, feeding the input on stdin and returning
// whatever the module writes to stdout as the result. A fresh runtime is created
// per call so no state leaks between executions.
func (e *wazeroEngine) Execute(ctx context.Context, req Request) (Result, error) {
	e.logger.Debug("execute: starting", "module_size", len(req.Module), "input_size", len(req.Input), "method", req.Method, "entrypoint", req.Limits.Entrypoint, "memory_limit", req.Limits.Memory)

	cfg := wazero.NewRuntimeConfig().
		// Interrupt execution when the context (watchdog) is cancelled.
		WithCloseOnContextDone(true).
		// Enforce the memory ceiling in-engine.
		WithMemoryLimitPages(req.Limits.memoryPages())

	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		// Failing to provide the host environment is an infrastructure fault.
		return Result{Status: StatusHostError, TrapMessage: err.Error()}, err
	}

	compiled, err := r.CompileModule(ctx, req.Module)
	if err != nil {
		e.logger.Debug("execute: compile failed", "error", err)
		return Result{Status: StatusInvalidModule, TrapMessage: err.Error()}, nil
	}
	e.logger.Debug("execute: compiled", "exports", compiled.ExportedFunctions(), "imported_memories", len(compiled.ImportedMemories()))

	// Reject anything the sandbox does not provide up front, so an unsatisfiable
	// import is a deterministic verdict on the module rather than a link failure
	// surfacing as a trap.
	if err := checkImports(compiled); err != nil {
		e.logger.Debug("execute: rejected import", "error", err)
		return Result{Status: StatusInvalidModule, TrapMessage: err.Error()}, nil
	}

	// Without an explicit entrypoint the module is run as a WASI command, which
	// requires the conventional `_start` export.
	if req.Limits.Entrypoint == "" {
		if _, ok := compiled.ExportedFunctions()[wasiEntrypoint]; !ok {
			e.logger.Debug("execute: missing entrypoint export", "want", wasiEntrypoint)
			return Result{Status: StatusInvalidModule, TrapMessage: "module does not export " + wasiEntrypoint}, nil
		}
	}

	var stdout bytes.Buffer
	modCfg := wazero.NewModuleConfig().
		WithName("").
		WithStdin(bytes.NewReader(req.Input)).
		WithStdout(&stdout).
		WithStderr(io.Discard)

	// Request metadata is exposed CGI-style. Only values derived from the request
	// are passed; the host environment is never inherited, so the guest sees the
	// same environment on every node.
	if req.Method != "" {
		modCfg = modCfg.WithEnv(envRequestMethod, req.Method)
	}

	// With an explicit entrypoint, disable the automatic `_start` invocation and
	// call the named export ourselves after instantiation.
	if req.Limits.Entrypoint != "" {
		modCfg = modCfg.WithStartFunctions()
	}

	mod, err := r.InstantiateModule(ctx, compiled, modCfg)
	if err != nil {
		e.logger.Debug("execute: instantiate failed", "error", err)
		if res, ok := classifyRunError(err, stdout.Bytes()); ok {
			e.logger.Debug("execute: instantiate error classified", "status", res.Status)
			return res, nil
		}
		return Result{Status: StatusHostError, TrapMessage: err.Error()}, err
	}
	defer mod.Close(ctx)

	if req.Limits.Entrypoint != "" {
		fn := mod.ExportedFunction(req.Limits.Entrypoint)
		if fn == nil {
			e.logger.Debug("execute: entrypoint not exported", "entrypoint", req.Limits.Entrypoint)
			return Result{Status: StatusInvalidModule, TrapMessage: "entrypoint not exported: " + req.Limits.Entrypoint}, nil
		}
		if _, err := fn.Call(ctx); err != nil {
			e.logger.Debug("execute: entrypoint call failed", "entrypoint", req.Limits.Entrypoint, "error", err)
			if res, ok := classifyRunError(err, stdout.Bytes()); ok {
				e.logger.Debug("execute: entrypoint error classified", "status", res.Status)
				return res, nil
			}
			return Result{Status: StatusHostError, TrapMessage: err.Error()}, err
		}
	}

	e.logger.Debug("execute: ok", "output_size", stdout.Len())
	return Result{Status: StatusOK, Output: stdout.Bytes()}, nil
}

// checkImports verifies the module only imports from the host environment the
// sandbox instantiates. Importing memory is not supported at all.
func checkImports(compiled wazero.CompiledModule) error {
	for _, f := range compiled.ImportedFunctions() {
		moduleName, name, ok := f.Import()
		if !ok {
			continue
		}
		if moduleName != wasiModuleName {
			return fmt.Errorf("unsupported import %q from module %q", name, moduleName)
		}
	}
	if len(compiled.ImportedMemories()) > 0 {
		return errors.New("importing memory is not supported")
	}
	return nil
}

// classifyRunError maps a wazero execution error to a program verdict. The bool
// result is false when the error is not a program-level failure (i.e. it should
// be surfaced as an infrastructure error).
func classifyRunError(err error, out []byte) (Result, bool) {
	var exitErr *sys.ExitError
	if errors.As(err, &exitErr) {
		switch exitErr.ExitCode() {
		case 0:
			// Normal WASI exit.
			return Result{Status: StatusOK, Output: out}, true
		case sys.ExitCodeContextCanceled, sys.ExitCodeDeadlineExceeded:
			// The watchdog or the caller's context stopped the module. This is a
			// local, non-deterministic kill, not a verdict on the program.
			return Result{}, false
		}
		return Result{Status: StatusTrap, Output: out, TrapMessage: err.Error()}, true
	}
	// Any other execution error is a trap (unreachable, OOB access, ...).
	return Result{Status: StatusTrap, Output: out, TrapMessage: err.Error()}, true
}

func (e *wazeroEngine) Close() error { return nil }
