// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/compute"
	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/gorilla/mux"
)

// ExecuteConfig holds the operator-configured bounds for the execute endpoint.
// Per-request headers may only lower a limit below its configured maximum.
type ExecuteConfig struct {
	MaxModuleSize uint64
	DefaultMemory uint64
	MaxMemory     uint64
	// Bounds on what a module may make the node do through the swarm host
	// module. wazero has no gas metering, so these are what stop a module
	// fetching or storing without end.
	DefaultHostCalls uint64
	MaxHostCalls     uint64
	DefaultHostBytes uint64
	MaxHostBytes     uint64
	DefaultDepth     uint64
	MaxDepth         uint64
	// Bounds on the response metadata a module may set. These carry no
	// default/maximum pair and no per-request header: that pattern exists so a
	// caller can lower risk it is exposed to, and a caller is not exposed to
	// this one.
	MaxResponseHeaders     uint64
	MaxResponseHeaderBytes uint64
	// RequestHeaders names the request headers forwarded to a module as CGI
	// HTTP_* variables. Empty means defaultRequestHeaders; a non-empty value
	// replaces that list outright, so an operator who widens the surface owns
	// the decision. forbiddenRequestHeaders is enforced regardless.
	RequestHeaders []string
	// MaxEnvBytes bounds the derived environment. Zero means
	// defaultMaxEnvBytes.
	MaxEnvBytes uint64
}

// defaultMaxEnvBytes bounds the CGI environment when the operator has not set a
// limit. Overflow is a 431 rather than a truncation: silently shortening the
// environment would hand the guest a lie it cannot detect.
const defaultMaxEnvBytes = 16 << 10

// executeResponse is the structured (JSON) representation of an execution result.
type executeResponse struct {
	Status      string `json:"status"`
	Output      []byte `json:"output"`
	TrapMessage string `json:"trapMessage,omitempty"`
	// HTTPStatus and Headers report what the module asked for through
	// swarm_response_*. In this representation they are reported, never applied:
	// a client that asked for the envelope asked to be told about the run, not to
	// have its own transport reshaped by it. Applying them would also make "the
	// module says 404" indistinguishable from "the node says module not found".
	HTTPStatus int                 `json:"httpStatus,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
}

// envelopeFor renders the result as the JSON envelope both a 200 and a
// program-fault 400 carry.
func envelopeFor(res compute.Result) executeResponse {
	return executeResponse{
		Status:      res.Status.String(),
		Output:      res.Output,
		TrapMessage: res.TrapMessage,
		HTTPStatus:  res.Response.Status,
		Headers:     headerMap(res.Response),
	}
}

// headerMap groups the guest's headers by name, preserving the order of repeats.
func headerMap(meta compute.ResponseMeta) map[string][]string {
	if len(meta.Headers) == 0 {
		return nil
	}
	out := make(map[string][]string, len(meta.Headers))
	for _, h := range meta.Headers {
		name := http.CanonicalHeaderKey(h.Name)
		out[name] = append(out[name], h.Value)
	}
	return out
}

// executeHandler downloads the WASM module addressed by {address}, runs it in the
// sandbox with the request body as input, and renders the result negotiated on
// the Accept header.
//
// The route accepts every HTTP method and hands the method to the module (see
// compute.Request.Method), so the program decides how to react to it. OPTIONS is
// the one exception: it is answered by the node so CORS preflight never reaches
// untrusted code.
func (s *Service) executeHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("execute").Build())

	// We don't allow the OPTIONS method, as browsers use it for CORS preflight checks.
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	vars := mux.Vars(r)

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(vars, &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	// PATH_INFO is empty on the bare route and starts with "/" on the
	// trailing-path one, which is the CGI rule: /@/a -> "", /@/a/ -> "/",
	// /@/a/x/y -> "/x/y". mux hands back the decoded path, matching /bzz.
	pathInfo := ""
	if raw, ok := vars["path"]; ok {
		pathInfo = "/" + raw
	}

	env := s.executeEnv(r, pathInfo)
	maxEnv := s.executeConfig.MaxEnvBytes
	if maxEnv == 0 {
		maxEnv = defaultMaxEnvBytes
	}
	if uint64(envSize(env)) > maxEnv {
		jsonhttp.RequestHeaderFieldsTooLarge(w, "request metadata exceeds maximum size")
		return
	}

	headers := struct {
		Memory     *uint64 `map:"Swarm-Wasm-Memory-Limit"`
		Entrypoint string  `map:"Swarm-Wasm-Entrypoint"`
		HostCalls  *uint64 `map:"Swarm-Wasm-Host-Calls-Limit"`
		HostBytes  *uint64 `map:"Swarm-Wasm-Host-Bytes-Limit"`
		Depth      *uint64 `map:"Swarm-Wasm-Depth-Limit"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	// Negotiate the response representation up front so we can reject an
	// unsupported Accept before doing any work.
	format, explicitJSON, ok := negotiateExecuteFormat(r.Header.Get(AcceptHeader))
	if !ok {
		jsonhttp.NotAcceptable(w, "unsupported Accept media type")
		return
	}

	lim := compute.Limits{
		Memory:       clampLimit(headers.Memory, s.executeConfig.DefaultMemory, s.executeConfig.MaxMemory),
		Entrypoint:   headers.Entrypoint,
		MaxHostCalls: uint32(clampLimit(headers.HostCalls, s.executeConfig.DefaultHostCalls, s.executeConfig.MaxHostCalls)),
		MaxHostBytes: clampLimit(headers.HostBytes, s.executeConfig.DefaultHostBytes, s.executeConfig.MaxHostBytes),
		MaxDepth:     uint32(clampLimit(headers.Depth, s.executeConfig.DefaultDepth, s.executeConfig.MaxDepth)),

		MaxResponseHeaders:     uint32(s.executeConfig.MaxResponseHeaders),
		MaxResponseHeaderBytes: uint32(s.executeConfig.MaxResponseHeaderBytes),
	}

	// Download and reassemble the module bytes, capped at the configured maximum.
	reader, l, err := joiner.New(r.Context(), s.storer.Download(true), s.storer.Cache(), paths.Address, redundancy.DefaultDownloadLevel)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, topology.ErrNotFound) {
			logger.Debug("execute: module not found", "address", paths.Address, "error", err)
			jsonhttp.NotFound(w, "module not found")
			return
		}
		logger.Debug("execute: joiner failed", "address", paths.Address, "error", err)
		logger.Error(nil, "execute: joiner failed")
		jsonhttp.InternalServerError(w, "could not read module")
		return
	}

	maxModule := s.executeConfig.MaxModuleSize
	if maxModule > 0 && l >= 0 && uint64(l) > maxModule {
		jsonhttp.RequestEntityTooLarge(w, "module exceeds maximum size")
		return
	}

	module, err := readCapped(reader, maxModule)
	if err != nil {
		if errors.Is(err, errTooLarge) {
			jsonhttp.RequestEntityTooLarge(w, "module exceeds maximum size")
			return
		}
		logger.Debug("execute: reading module failed", "address", paths.Address, "error", err)
		logger.Error(nil, "execute: reading module failed")
		jsonhttp.InternalServerError(w, "could not read module")
		return
	}

	input, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Debug("execute: reading request body failed", "error", err)
		jsonhttp.InternalServerError(w, "could not read input")
		return
	}

	// The host is per-request: any upload it opens belongs to this execution
	// alone and is committed or dropped below.
	host := s.newExecuteHost(r.Context(), logger, lim.HostBytes())

	result, err := s.compute.Execute(r.Context(), compute.Request{
		Module: module,
		Method: r.Method,
		Input:  input,
		Env:    env,
		Limits: lim,
		Host:   host,
	})
	if err != nil {
		if cerr := host.Close(false); cerr != nil {
			logger.Debug("execute: discarding upload session failed", "error", cerr)
		}
		if errors.Is(err, compute.ErrBusy) {
			jsonhttp.TooManyRequests(w, "execution workers busy")
			return
		}
		logger.Debug("execute: execution failed", "address", paths.Address, "error", err)
		logger.Error(nil, "execute: execution failed")
		jsonhttp.InternalServerError(w, "execution failed")
		return
	}

	// Only a clean run commits its uploads; a trapped or rejected module leaves
	// nothing behind.
	if err := host.Close(result.Status == compute.StatusOK); err != nil {
		logger.Debug("execute: closing upload session failed", "error", err)
		logger.Error(nil, "execute: closing upload session failed")
		jsonhttp.InternalServerError(w, "could not store uploaded data")
		return
	}

	// A wildcard Accept defaults to the envelope, but a module that shaped its own
	// response has expressed an opinion the wildcard has not contradicted. Honour
	// it, so browsers and fetch() get the bytes the module meant to serve. A
	// module that set nothing is untouched, which is what keeps this additive.
	if format == formatJSON && !explicitJSON && !result.Response.Empty() {
		format = formatModule
	}

	renderExecResult(w, format, result)
}

// renderExecResult writes the execution result in the negotiated representation.
// The HTTP status is the program verdict's unless the module set its own, which
// it can only do in the raw representations.
func renderExecResult(w http.ResponseWriter, format string, res compute.Result) {
	// Set first, so the guest's headers cannot end up overwriting it. The
	// denylist forbids the name anyway; this is the second lock on that door.
	w.Header().Set(SwarmWasmStatusHeader, res.Status.String())

	switch res.Status {
	case compute.StatusInvalidModule, compute.StatusTrap:
		// Program's fault: deterministic bad request. res.Response is empty on
		// these paths by construction, so nothing of the guest's is applied.
		renderExecError(w, format, res)
		return
	case compute.StatusHostError:
		jsonhttp.InternalServerError(w, "execution failed")
		return
	}

	if format == formatJSON {
		// Reported, not applied.
		jsonhttp.OK(w, envelopeFor(res))
		return
	}

	// A raw representation: the module owns the body, so it owns the headers
	// describing it. Its Content-Type replaces the negotiated default.
	switch format {
	case formatHTML:
		w.Header().Set(ContentTypeHeader, "text/html; charset=utf-8")
	case formatOctet:
		w.Header().Set(ContentTypeHeader, "application/octet-stream")
	default: // formatModule: the guest decides, with a conservative fallback.
		w.Header().Set(ContentTypeHeader, "application/octet-stream")
	}
	applyResponseHeaders(w, res.Response)

	status := http.StatusOK
	if res.Response.Status != 0 {
		status = res.Response.Status
	}
	w.WriteHeader(status)
	_, _ = w.Write(res.Output)
}

// applyResponseHeaders writes the guest's headers onto the response.
//
// The first occurrence of a name replaces whatever the node negotiated, and
// later repeats accumulate, so a guest can both override Content-Type and send
// several Link headers. The denylist is re-checked here: the engine already
// enforces it, and an engine bug should not become a header leak.
func applyResponseHeaders(w http.ResponseWriter, meta compute.ResponseMeta) {
	seen := make(map[string]struct{}, len(meta.Headers))
	for _, h := range meta.Headers {
		if compute.DeniedResponseHeader(h.Name) {
			continue
		}
		name := http.CanonicalHeaderKey(h.Name)
		if _, ok := seen[name]; !ok {
			w.Header().Del(name)
			seen[name] = struct{}{}
		}
		w.Header().Add(name, h.Value)
	}
}

// renderExecError writes a deterministic program-fault response in the
// representation the client negotiated.
//
// The JSON envelope is the same one a 200 carries. For the raw representations a
// client that asked for HTML gets HTML: answering a negotiated text/html request
// with a JSON body, as this used to, is a content-type lie.
func renderExecError(w http.ResponseWriter, format string, res compute.Result) {
	if format == formatJSON {
		jsonhttp.BadRequest(w, envelopeFor(res))
		return
	}

	msg := res.Status.String()
	if res.TrapMessage != "" {
		msg += ": " + res.TrapMessage
	}

	if format == formatHTML {
		w.Header().Set(ContentTypeHeader, "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "<!doctype html><title>execution failed</title><h1>execution failed</h1><p>%s</p>\n", html.EscapeString(msg))
		return
	}

	w.Header().Set(ContentTypeHeader, "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = io.WriteString(w, msg+"\n")
}

const (
	formatOctet = "octet"
	formatJSON  = "json"
	formatHTML  = "html"
	// formatModule is raw output whose Content-Type comes entirely from the
	// module. It is never negotiated directly: it is what a wildcard Accept
	// becomes once the module has said something about its own response.
	formatModule = "module"
)

// negotiateExecuteFormat picks a response representation from the Accept header.
//
// The second result reports whether the client named application/json outright,
// as opposed to reaching JSON through a wildcard. That distinction matters
// because a wildcard is not a request for the envelope, it is the absence of an
// opinion — and a browser fetching a subresource sends one. A stylesheet request
// is "Accept: text/css,*/*;q=0.1", an image request ends in "*/*;q=0.8", and a
// default fetch() sends "*/*", so without this a module could never serve
// anything but a top-level HTML page.
//
// The third result is false when the client requires a media type we cannot
// produce.
func negotiateExecuteFormat(accept string) (format string, explicit bool, ok bool) {
	accept = strings.TrimSpace(accept)
	if accept == "" {
		return formatJSON, false, true
	}
	for _, part := range strings.Split(accept, ",") {
		// Drop any parameters (e.g. q-values); we do not rank by quality.
		mediaType := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		switch mediaType {
		case "application/json":
			return formatJSON, true, true
		case "*/*", "application/*":
			return formatJSON, false, true
		case "text/html", "application/xhtml+xml":
			return formatHTML, false, true
		case "application/octet-stream":
			return formatOctet, false, true
		}
	}
	return "", false, false
}

// clampLimit resolves a per-request override against the configured default and
// maximum. A nil override uses the default; any value is capped at the maximum.
func clampLimit(override *uint64, def, maximum uint64) uint64 {
	v := def
	if override != nil {
		v = *override
	}
	if maximum > 0 && v > maximum {
		v = maximum
	}
	return v
}

var errTooLarge = errors.New("data exceeds maximum size")

// readCapped reads all bytes from r, failing with errTooLarge if the content
// exceeds maximum. A maximum of 0 means unlimited.
func readCapped(r io.Reader, maximum uint64) ([]byte, error) {
	if maximum == 0 {
		return io.ReadAll(r)
	}
	buf, err := io.ReadAll(io.LimitReader(r, int64(maximum)+1))
	if err != nil {
		return nil, err
	}
	if uint64(len(buf)) > maximum {
		return nil, errTooLarge
	}
	return buf, nil
}
