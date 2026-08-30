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
	"strings"

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

// wazeroEngine is the in-process execution engine.
//
// WARNING: it does NOT meter execution deterministically, it wires WASI
// stdin/stdout for I/O, and a module reaching the node through the swarm host
// module observes node-local state. Its output is not reproducible across
// nodes. See the package documentation.
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
	// The response state is created once, here, and handed only to the outermost
	// execution. Nested modules see a nil one and are refused.
	return e.execute(ctx, req, newBudget(req.Limits), newResponseState(req.Limits), 0)
}

// execute runs one module at the given nesting depth. The budget is shared by
// pointer across the whole call tree, so a module cannot multiply its host-call
// allowance by recursing through swarm_execute.
func (e *wazeroEngine) execute(ctx context.Context, req Request, b *budget, resp *responseState, depth uint32) (res Result, err error) {
	// Attached on the way out rather than at each return site: a run can finish
	// through classifyRunError (a WASI exit 0 is a clean run reported as an error
	// by wazero), and a return-site edit would miss that path.
	defer func() {
		if res.Status == StatusOK {
			res.Response = resp.snapshot()
		}
	}()

	e.logger.Debug("execute: starting", "module_size", len(req.Module), "input_size", len(req.Input), "method", req.Method, "entrypoint", req.Limits.Entrypoint, "memory_limit", req.Limits.Memory, "depth", depth)

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

	// The swarm module is built per execution, closing over this tree's budget
	// and depth. Nothing is shared between untrusted programs.
	// The swarm module is built per execution, closing over this tree's budget and
	// depth, and is always instantiated: the response half needs no Host.
	hs := &hostState{
		host:     req.Host,
		budget:   b,
		depth:    depth,
		maxDepth: req.Limits.Depth(),
		logger:   e.logger,
	}
	// Only the outermost execution owns the response. Nested modules get a nil
	// responseState, which is what makes swarm_response_* return DENIED.
	if depth == 0 {
		hs.resp = resp
	}
	if req.Host != nil {
		hs.nested = func(ctx context.Context, module, input []byte) (Result, error) {
			nested := req
			nested.Module = module
			nested.Input = input
			// A nested module is always run as a WASI command: the caller's
			// entrypoint header describes the outermost module only.
			nested.Limits.Entrypoint = ""
			return e.execute(ctx, nested, b, resp, depth+1)
		}
	}
	if err := buildSwarmModule(ctx, r, hs); err != nil {
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
	// hs is always non-nil now that the response half needs no Host, so node
	// availability is req.Host, not the presence of a hostState.
	if err := checkImports(compiled, req.Host != nil); err != nil {
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
	for _, v := range req.Env {
		// Method owns REQUEST_METHOD; a duplicate would give the guest two
		// entries for one name.
		if v.Name == envRequestMethod {
			continue
		}
		// A name carrying "=" or NUL would corrupt the WASI environ block, which
		// is a flat run of NUL-terminated "name=value" strings. The API layer
		// sanitises too; this is the engine refusing to emit a malformed block
		// whatever it is handed.
		if strings.ContainsAny(v.Name, "=\x00") || strings.ContainsRune(v.Value, 0) {
			continue
		}
		modCfg = modCfg.WithEnv(v.Name, v.Value)
	}

	// With an explicit entrypoint, disable the automatic `_start` invocation and
	// call the named export ourselves after instantiation.
	if req.Limits.Entrypoint != "" {
		modCfg = modCfg.WithStartFunctions()
	}

	// For a WASI command module `_start` runs during instantiation, so a host
	// call — and a host abort — can happen here.
	mod, err := r.InstantiateModule(ctx, compiled, modCfg)
	if err != nil {
		e.logger.Debug("execute: instantiate failed", "error", err)
		if res, aborted := hostAbort(hs); aborted {
			return res, hs.err
		}
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
			if res, aborted := hostAbort(hs); aborted {
				return res, hs.err
			}
			if res, ok := classifyRunError(err, stdout.Bytes()); ok {
				e.logger.Debug("execute: entrypoint error classified", "status", res.Status)
				return res, nil
			}
			return Result{Status: StatusHostError, TrapMessage: err.Error()}, err
		}
	}

	// Defence in depth: a recorded host failure outranks an apparently clean run.
	if res, aborted := hostAbort(hs); aborted {
		return res, hs.err
	}

	e.logger.Debug("execute: ok", "output_size", stdout.Len())
	return Result{Status: StatusOK, Output: stdout.Bytes()}, nil
}

// hostAbort reports whether the run was torn down by a node-local host failure
// and, if so, the verdict to return. A host abort unwinds the guest as a trap,
// but a trap is a verdict on the program and this was not one: it must surface
// as StatusHostError with a non-nil error, never as StatusTrap.
func hostAbort(hs *hostState) (Result, bool) {
	if hs == nil || hs.err == nil {
		return Result{}, false
	}
	return Result{Status: StatusHostError, TrapMessage: hs.err.Error()}, true
}

// checkImports verifies the module only imports from the host environment the
// sandbox instantiates. Importing memory is not supported at all.
//
// Rejecting up front means an unsatisfiable import is a deterministic verdict on
// the module (StatusInvalidModule) rather than a link failure surfacing as a
// trap. The WASI namespace is accepted wholesale — wasi_snapshot_preview1
// provides all of preview1 — while the swarm namespace is checked name by name
// against what buildSwarmModule actually defines.
func checkImports(compiled wazero.CompiledModule, hostAvailable bool) error {
	for _, f := range compiled.ImportedFunctions() {
		moduleName, name, ok := f.Import()
		if !ok {
			continue
		}
		switch moduleName {
		case wasiModuleName:
		case swarmModuleName:
			if _, ok := swarmResponseExports[name]; ok {
				// Shaping the response causes no node work, so it needs no Host.
				continue
			}
			if !hostAvailable {
				return fmt.Errorf("import %q from module %q: node access is not available", name, moduleName)
			}
			if _, ok := swarmHostExports[name]; !ok {
				return fmt.Errorf("unknown import %q from module %q", name, moduleName)
			}
		default:
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
