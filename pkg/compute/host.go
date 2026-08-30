// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compute

import (
	"context"
	"errors"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"golang.org/x/net/http/httpguts"
)

// Host serves the calls a running module makes back into the node.
//
// A nil Host means the swarm module is not instantiated at all, so a module
// importing it is rejected as invalid rather than trapping mid-run.
//
// Implementations report guest-caused outcomes with the ErrNotFound, ErrDenied
// and ErrInvalid sentinels. Any other error is treated as a node-local failure:
// it aborts the whole execution as StatusHostError and is never laundered into
// a program verdict.
type Host interface {
	// BytesGet reassembles data of arbitrary length addressed by addr.
	BytesGet(ctx context.Context, addr swarm.Address) ([]byte, error)
	// BytesPut splits data of arbitrary length, stamps it with batchID and
	// returns the root reference.
	BytesPut(ctx context.Context, batchID, data []byte) (swarm.Address, error)
	// ChunkGet retrieves a single chunk verbatim.
	ChunkGet(ctx context.Context, addr swarm.Address) ([]byte, error)
	// ChunkPut stores a single content-addressed chunk verbatim.
	ChunkPut(ctx context.Context, batchID, data []byte) (swarm.Address, error)
}

// Sentinel errors a Host returns to describe a guest-caused failure. They are
// the only errors a module can observe; everything else ends the run.
var (
	// ErrNotFound reports that the addressed data does not exist.
	ErrNotFound = errors.New("compute: not found")
	// ErrDenied reports that the node refused the call, e.g. an unusable
	// postage batch or a second batch in one execution.
	ErrDenied = errors.New("compute: denied")
	// ErrInvalid reports malformed guest-supplied arguments.
	ErrInvalid = errors.New("compute: invalid argument")
	// ErrTooLarge reports a payload the execution's byte budget cannot cover.
	// A Host uses it to refuse work before materialising it, which is why the
	// budget alone is not enough: it is charged only on delivery.
	ErrTooLarge = errors.New("compute: payload too large")
)

// swarmModuleName is the host module through which a guest reaches the node.
const swarmModuleName = "swarm"

// Guest-visible result codes returned by every swarm host function.
const (
	errnoOK              uint32 = 0
	errnoNotFound        uint32 = 1
	errnoDenied          uint32 = 2
	errnoBudgetExhausted uint32 = 3
	errnoBufferTooSmall  uint32 = 4
	errnoInvalid         uint32 = 5
	errnoExecFailed      uint32 = 6
)

// exitCodeHostAbort stops a module whose host call hit a node-local failure.
// It is only a mechanism to unwind: hostState.err is the source of truth, so a
// guest calling proc_exit with the same code is never mistaken for a host error.
const exitCodeHostAbort uint32 = 0xBEE0

// The swarm host module is defined in two halves, because they have different
// preconditions. checkImports rejects anything outside the applicable set before
// instantiation, so an unknown swarm import is a deterministic
// StatusInvalidModule rather than a link trap.
// TestSwarmExportsMatchBuilder keeps both in step with buildSwarmModule.
var (
	// swarmResponseExports shape the HTTP response. They cause no node work and
	// are therefore always available, even when the node runs with node access
	// switched off: setting a Content-Type is not a reason to need a Host.
	swarmResponseExports = map[string]struct{}{
		"swarm_response_status": {},
		"swarm_response_header": {},
	}
	// swarmHostExports reach the node's data and exist only when a Host does.
	swarmHostExports = map[string]struct{}{
		"swarm_bytes_get": {},
		"swarm_bytes_put": {},
		"swarm_chunk_get": {},
		"swarm_chunk_put": {},
		"swarm_execute":   {},
	}
)

// Response headers a guest may not set. Everything else is allowed: a response
// header carries only what the guest already knows, so an allowlist here would be
// endless friction (Cache-Control, ETag, Location, Link, Vary, ...). The request
// direction is the opposite — see the allowlist in pkg/api, which guards the
// operator's secrets.
var (
	// A guest must not forge the node's own protocol namespace, and must not
	// touch CORS: the node sets Access-Control-Allow-Credentials, so a guest
	// setting Allow-Origin would be a genuine cross-origin credential leak.
	deniedResponseHeaderPrefixes = []string{"swarm-wasm-", "access-control-"}

	deniedResponseHeaders = map[string]struct{}{
		// Hop-by-hop and framing: Go owns the wire format.
		"connection":          {},
		"keep-alive":          {},
		"proxy-authenticate":  {},
		"proxy-authorization": {},
		"te":                  {},
		"trailer":             {},
		"transfer-encoding":   {},
		"upgrade":             {},
		"content-length":      {},
		"host":                {},
		// Every module shares the node's origin with the node's own authenticated
		// API, so a guest-set cookie is session fixation across all of them.
		// Revisit only behind a per-module origin.
		"set-cookie": {},
		// Origin-wide and persistent: a guest must not be able to reconfigure or
		// brick the operator's origin.
		"strict-transport-security": {},
		"public-key-pins":           {},
		"clear-site-data":           {},
	}
)

// DeniedResponseHeader reports whether a guest is forbidden from setting name.
// It is exported so the API layer can re-check before applying, keeping an engine
// bug from becoming a leak.
func DeniedResponseHeader(name string) bool {
	lower := strings.ToLower(name)
	for _, prefix := range deniedResponseHeaderPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	_, denied := deniedResponseHeaders[lower]
	return denied
}

// responseState accumulates what the guest asked for through swarm_response_*.
// It belongs to the outermost execution alone; see hostState.resp.
type responseState struct {
	status     int
	headers    []Header
	bytes      uint32
	maxHeaders uint32
	maxBytes   uint32
}

func newResponseState(l Limits) *responseState {
	return &responseState{
		maxHeaders: l.ResponseHeaders(),
		maxBytes:   l.ResponseHeaderBytes(),
	}
}

// add records an accepted header, charging it against the two caps.
func (r *responseState) add(name, value string) uint32 {
	if uint32(len(r.headers)) >= r.maxHeaders {
		return errnoBudgetExhausted
	}
	// Subtract rather than add: bytes never exceeds maxBytes, so this cannot
	// overflow the way bytes+size could for a large configured maximum.
	size := uint32(len(name) + len(value))
	if size > r.maxBytes-r.bytes {
		return errnoBudgetExhausted
	}
	r.bytes += size
	r.headers = append(r.headers, Header{Name: name, Value: value})
	return errnoOK
}

// snapshot renders what the guest set. A nil receiver is the no-metadata case.
func (r *responseState) snapshot() ResponseMeta {
	if r == nil {
		return ResponseMeta{}
	}
	return ResponseMeta{Status: r.status, Headers: r.headers}
}

// validResponseHeaderValue accepts HTAB and printable ASCII only.
//
// This is deliberately stricter than RFC 9110, which also permits obs-text
// (0x80-0xFF): those bytes have no agreed encoding in a header value and are a
// smuggling surface. Rejecting control characters here subsumes CR/LF injection —
// Go's net/http would silently strip newlines on write, and a silent mangling is
// worse for a guest than a result code it can branch on.
func validResponseHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		if c := value[i]; c != '\t' && (c < 0x20 || c > 0x7e) {
			return false
		}
	}
	return true
}

// budget bounds the node work one execution tree may cause. It is shared by
// pointer across nested executions so a module cannot multiply its allowance by
// recursing.
type budget struct {
	calls uint32
	bytes uint64
}

func newBudget(l Limits) *budget {
	return &budget{calls: l.HostCalls(), bytes: l.HostBytes()}
}

// useCall charges one host call, reporting whether the budget allowed it.
func (b *budget) useCall() bool {
	if b.calls == 0 {
		return false
	}
	b.calls--
	return true
}

// useBytes charges n bytes, reporting whether the budget allowed it. The charge
// is all-or-nothing so a rejected call consumes nothing.
func (b *budget) useBytes(n uint64) bool {
	if n > b.bytes {
		return false
	}
	b.bytes -= n
	return true
}

// nestedFunc runs a module fetched by swarm_execute, re-entering the engine.
type nestedFunc func(ctx context.Context, module, input []byte) (Result, error)

// hostState backs the swarm host module for a single execution tree. A fresh
// one is built per execution, so nothing is shared between untrusted programs.
type hostState struct {
	host   Host
	budget *budget
	nested nestedFunc
	// depth is the current nesting level; 0 is the outermost execution.
	depth uint32
	// maxDepth bounds the number of execution levels, the outermost included.
	maxDepth uint32
	logger   log.Logger
	// resp accumulates the guest's HTTP response metadata. It is non-nil only at
	// depth 0: a module reached through swarm_execute is a library call, not an
	// HTTP request, and must not be able to rewrite its caller's response. A nil
	// resp is what makes the response calls return DENIED when nested.
	resp *responseState
	// err records a node-local failure. When set, the run is aborted and its
	// verdict is StatusHostError regardless of how wazero reports the unwind.
	err error
}

// abort records a node-local failure and stops the module. The errno it returns
// is never observed: the guest is being torn down.
func (h *hostState) abort(ctx context.Context, mod api.Module, err error) uint32 {
	if h.err == nil {
		h.err = err
	}
	h.logger.Debug("host call failed", "error", err)
	_ = mod.CloseWithExitCode(ctx, exitCodeHostAbort)
	return errnoInvalid
}

// classify maps a Host error to a guest-visible errno. The bool is false when
// the error is node-local and the run must be aborted instead.
func classifyHostErr(err error) (uint32, bool) {
	switch {
	case errors.Is(err, ErrNotFound):
		return errnoNotFound, true
	case errors.Is(err, ErrDenied):
		return errnoDenied, true
	case errors.Is(err, ErrInvalid):
		return errnoInvalid, true
	case errors.Is(err, ErrTooLarge):
		return errnoBudgetExhausted, true
	default:
		return 0, false
	}
}

// buildSwarmModule instantiates the swarm host module against the runtime.
//
// The response functions are always defined; the data functions only when a Host
// is present, which is what makes an unavailable node a deterministic
// invalid-module for a data import while still letting any module set a
// Content-Type.
func buildSwarmModule(ctx context.Context, r wazero.Runtime, h *hostState) error {
	b := r.NewHostModuleBuilder(swarmModuleName).
		NewFunctionBuilder().
		WithFunc(h.responseStatus).
		WithParameterNames("code").
		Export("swarm_response_status").
		NewFunctionBuilder().
		WithFunc(h.responseHeader).
		WithParameterNames("name_ptr", "name_len", "val_ptr", "val_len").
		Export("swarm_response_header")

	if h.host == nil {
		_, err := b.Instantiate(ctx)
		return err
	}

	_, err := b.
		NewFunctionBuilder().
		WithFunc(h.bytesGet).
		WithParameterNames("addr_ptr", "buf_ptr", "buf_len", "out_len_ptr").
		Export("swarm_bytes_get").
		NewFunctionBuilder().
		WithFunc(h.bytesPut).
		WithParameterNames("batch_ptr", "data_ptr", "data_len", "out_addr_ptr").
		Export("swarm_bytes_put").
		NewFunctionBuilder().
		WithFunc(h.chunkGet).
		WithParameterNames("addr_ptr", "buf_ptr", "buf_len", "out_len_ptr").
		Export("swarm_chunk_get").
		NewFunctionBuilder().
		WithFunc(h.chunkPut).
		WithParameterNames("batch_ptr", "data_ptr", "data_len", "out_addr_ptr").
		Export("swarm_chunk_put").
		NewFunctionBuilder().
		WithFunc(h.execute).
		WithParameterNames("addr_ptr", "input_ptr", "input_len", "buf_ptr", "buf_len", "out_len_ptr").
		Export("swarm_execute").
		Instantiate(ctx)
	return err
}

// responseStatus sets the HTTP status the node will answer with.
//
// The last call wins. 1xx is refused because writing an informational status
// through an http.ResponseWriter desynchronises the connection; 5xx is allowed,
// because a module must be able to report its own failure and Swarm-Wasm-Status
// already distinguishes the guest's 500 (ok) from the node's (host-error).
func (h *hostState) responseStatus(_ context.Context, _ api.Module, code uint32) uint32 {
	if h.resp == nil {
		return errnoDenied
	}
	if code < 200 || code > 599 {
		return errnoInvalid
	}
	h.resp.status = int(code)
	return errnoOK
}

// responseHeader appends a header to the response.
//
// The checks run in a fixed order and the SDK harness mirrors it exactly, so a
// module gets the same result code locally and on a node. Lengths are validated
// before any guest memory is read, so an absurd val_len never allocates, and a
// rejected call charges nothing.
func (h *hostState) responseHeader(_ context.Context, mod api.Module, namePtr, nameLen, valPtr, valLen uint32) uint32 {
	if h.resp == nil {
		return errnoDenied
	}
	mem := mod.Memory()
	if mem == nil {
		return errnoInvalid
	}
	if nameLen == 0 || nameLen > MaxResponseHeaderNameLen || valLen > MaxResponseHeaderValueLen {
		return errnoInvalid
	}

	nameBytes, ok := mem.Read(namePtr, nameLen)
	if !ok {
		return errnoInvalid
	}
	valueBytes, ok := mem.Read(valPtr, valLen)
	if !ok {
		return errnoInvalid
	}
	// Memory.Read aliases guest memory; string() copies.
	name, value := string(nameBytes), string(valueBytes)

	if !httpguts.ValidHeaderFieldName(name) || !validResponseHeaderValue(value) {
		return errnoInvalid
	}
	if DeniedResponseHeader(name) {
		return errnoDenied
	}
	return h.resp.add(name, value)
}

func (h *hostState) bytesGet(ctx context.Context, mod api.Module, addrPtr, bufPtr, bufLen, outLenPtr uint32) uint32 {
	return h.get(ctx, mod, addrPtr, bufPtr, bufLen, outLenPtr, h.host.BytesGet)
}

func (h *hostState) chunkGet(ctx context.Context, mod api.Module, addrPtr, bufPtr, bufLen, outLenPtr uint32) uint32 {
	return h.get(ctx, mod, addrPtr, bufPtr, bufLen, outLenPtr, h.host.ChunkGet)
}

func (h *hostState) bytesPut(ctx context.Context, mod api.Module, batchPtr, dataPtr, dataLen, outAddrPtr uint32) uint32 {
	return h.put(ctx, mod, batchPtr, dataPtr, dataLen, outAddrPtr, 0, h.host.BytesPut)
}

func (h *hostState) chunkPut(ctx context.Context, mod api.Module, batchPtr, dataPtr, dataLen, outAddrPtr uint32) uint32 {
	return h.put(ctx, mod, batchPtr, dataPtr, dataLen, outAddrPtr, swarm.ChunkWithSpanSize, h.host.ChunkPut)
}

// get is the shared body of the address-in, data-out calls.
//
// A call is charged whether or not the data fits the guest buffer; bytes are
// charged only when they are actually delivered, so the probe-then-fetch
// pattern (buf_len 0, read out_len, retry) costs two calls but pays for the
// payload once.
func (h *hostState) get(
	ctx context.Context,
	mod api.Module,
	addrPtr, bufPtr, bufLen, outLenPtr uint32,
	fn func(context.Context, swarm.Address) ([]byte, error),
) uint32 {
	if !h.budget.useCall() {
		return errnoBudgetExhausted
	}

	mem := mod.Memory()
	if mem == nil {
		return errnoInvalid
	}
	addr, ok := readAddress(mem, addrPtr)
	if !ok {
		return errnoInvalid
	}

	data, err := fn(ctx, addr)
	if err != nil {
		if code, guest := classifyHostErr(err); guest {
			return code
		}
		return h.abort(ctx, mod, err)
	}

	return h.deliver(ctx, mod, bufPtr, bufLen, outLenPtr, data)
}

// deliver writes data into the guest buffer, always reporting the required
// length through outLenPtr so a too-small buffer can be retried.
func (h *hostState) deliver(ctx context.Context, mod api.Module, bufPtr, bufLen, outLenPtr uint32, data []byte) uint32 {
	mem := mod.Memory()
	if mem == nil {
		return errnoInvalid
	}
	if !mem.WriteUint32Le(outLenPtr, uint32(len(data))) {
		return errnoInvalid
	}
	if uint64(len(data)) > uint64(bufLen) {
		return errnoBufferTooSmall
	}
	if !h.budget.useBytes(uint64(len(data))) {
		return errnoBudgetExhausted
	}
	if !mem.Write(bufPtr, data) {
		return errnoInvalid
	}
	return errnoOK
}

// put is the shared body of the data-in, address-out calls. maxLen caps the
// accepted payload; 0 means only the byte budget applies.
func (h *hostState) put(
	ctx context.Context,
	mod api.Module,
	batchPtr, dataPtr, dataLen, outAddrPtr uint32,
	maxLen uint32,
	fn func(context.Context, []byte, []byte) (swarm.Address, error),
) uint32 {
	if !h.budget.useCall() {
		return errnoBudgetExhausted
	}
	if maxLen > 0 && dataLen > maxLen {
		return errnoInvalid
	}
	// Charge the declared length before doing any work, so an oversized put is
	// refused without the node ever chunking it.
	if !h.budget.useBytes(uint64(dataLen)) {
		return errnoBudgetExhausted
	}

	mem := mod.Memory()
	if mem == nil {
		return errnoInvalid
	}
	batchID, ok := mem.Read(batchPtr, swarm.HashSize)
	if !ok {
		return errnoInvalid
	}
	data, ok := mem.Read(dataPtr, dataLen)
	if !ok {
		return errnoInvalid
	}

	// Memory.Read aliases the guest's memory; the storer keeps what it is given,
	// so hand it a copy the guest cannot mutate underneath it.
	addr, err := fn(ctx, bytesClone(batchID), bytesClone(data))
	if err != nil {
		if code, guest := classifyHostErr(err); guest {
			return code
		}
		return h.abort(ctx, mod, err)
	}

	if !mem.Write(outAddrPtr, addr.Bytes()) {
		return errnoInvalid
	}
	return errnoOK
}

// execute fetches a module from Swarm and runs it as a nested execution sharing
// this tree's budget.
func (h *hostState) execute(ctx context.Context, mod api.Module, addrPtr, inputPtr, inputLen, bufPtr, bufLen, outLenPtr uint32) uint32 {
	if !h.budget.useCall() {
		return errnoBudgetExhausted
	}
	// maxDepth counts execution levels, the outermost included, so a limit of 1
	// permits no nesting at all.
	if h.depth+1 >= h.maxDepth {
		return errnoBudgetExhausted
	}
	if !h.budget.useBytes(uint64(inputLen)) {
		return errnoBudgetExhausted
	}

	mem := mod.Memory()
	if mem == nil {
		return errnoInvalid
	}
	addr, ok := readAddress(mem, addrPtr)
	if !ok {
		return errnoInvalid
	}
	input, ok := mem.Read(inputPtr, inputLen)
	if !ok {
		return errnoInvalid
	}
	input = bytesClone(input)

	module, err := h.host.BytesGet(ctx, addr)
	if err != nil {
		if code, guest := classifyHostErr(err); guest {
			return code
		}
		return h.abort(ctx, mod, err)
	}
	if !h.budget.useBytes(uint64(len(module))) {
		return errnoBudgetExhausted
	}

	res, err := h.nested(ctx, module, input)
	if err != nil {
		return h.abort(ctx, mod, err)
	}
	switch res.Status {
	case StatusOK:
		return h.deliver(ctx, mod, bufPtr, bufLen, outLenPtr, res.Output)
	case StatusHostError:
		// A node-local failure inside the child is a node-local failure here.
		return h.abort(ctx, mod, errors.New("nested execution failed"))
	default:
		// The child trapped or was invalid. That is a verdict
		// on the child, which the caller may handle.
		return errnoExecFailed
	}
}

// readAddress reads a fixed-width Swarm address out of guest memory.
//
// Callers check mod.Memory() for nil first: a module may import the swarm
// functions and declare no memory at all (nothing in checkImports requires one),
// and api.Memory is a typed-nil interface in that case.
func readAddress(mem api.Memory, ptr uint32) (swarm.Address, bool) {
	b, ok := mem.Read(ptr, swarm.HashSize)
	if !ok {
		return swarm.ZeroAddress, false
	}
	return swarm.NewAddress(bytesClone(b)), true
}

// bytesClone copies a slice aliasing guest memory.
func bytesClone(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
