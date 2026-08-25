// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
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
	DefaultFuel   uint64
	MaxFuel       uint64
	DefaultMemory uint64
	MaxMemory     uint64
}

// executeResponse is the structured (JSON) representation of an execution result.
type executeResponse struct {
	Status       string `json:"status"`
	Output       []byte `json:"output"`
	FuelConsumed uint64 `json:"fuelConsumed"`
	TrapMessage  string `json:"trapMessage,omitempty"`
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

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	headers := struct {
		Fuel       *uint64 `map:"Swarm-Wasm-Fuel-Limit"`
		Memory     *uint64 `map:"Swarm-Wasm-Memory-Limit"`
		Entrypoint string  `map:"Swarm-Wasm-Entrypoint"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	// Negotiate the response representation up front so we can reject an
	// unsupported Accept before doing any work.
	format, ok := negotiateExecuteFormat(r.Header.Get(AcceptHeader))
	if !ok {
		jsonhttp.NotAcceptable(w, "unsupported Accept media type")
		return
	}

	lim := compute.Limits{
		Fuel:       clampLimit(headers.Fuel, s.executeConfig.DefaultFuel, s.executeConfig.MaxFuel),
		Memory:     clampLimit(headers.Memory, s.executeConfig.DefaultMemory, s.executeConfig.MaxMemory),
		Entrypoint: headers.Entrypoint,
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

	result, err := s.compute.Execute(r.Context(), compute.Request{
		Module: module,
		Method: r.Method,
		Input:  input,
		Limits: lim,
	})
	if err != nil {
		if errors.Is(err, compute.ErrBusy) {
			jsonhttp.TooManyRequests(w, "execution workers busy")
			return
		}
		logger.Debug("execute: execution failed", "address", paths.Address, "error", err)
		logger.Error(nil, "execute: execution failed")
		jsonhttp.InternalServerError(w, "execution failed")
		return
	}

	renderExecResult(w, format, result)
}

// renderExecResult writes the execution result in the negotiated representation.
// The HTTP status is derived from the program verdict and is independent of the
// chosen format.
func renderExecResult(w http.ResponseWriter, format string, res compute.Result) {
	w.Header().Set(SwarmWasmStatusHeader, res.Status.String())
	w.Header().Set(SwarmWasmFuelConsumedHeader, strconv.FormatUint(res.FuelConsumed, 10))

	switch res.Status {
	case compute.StatusInvalidModule, compute.StatusTrap:
		// Program's fault: deterministic bad request.
		jsonhttp.BadRequest(w, execErrorBody(format, res))
		return
	case compute.StatusHostError:
		jsonhttp.InternalServerError(w, "execution failed")
		return
	}

	// StatusOK or StatusOutOfFuel: 200. For out-of-fuel there may be no output.
	switch format {
	case formatJSON:
		jsonhttp.OK(w, executeResponse{
			Status:       res.Status.String(),
			Output:       res.Output,
			FuelConsumed: res.FuelConsumed,
			TrapMessage:  res.TrapMessage,
		})
	case formatHTML:
		w.Header().Set(ContentTypeHeader, "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(res.Output)
	default: // formatOctet
		w.Header().Set(ContentTypeHeader, "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(res.Output)
	}
}

// execErrorBody builds the body for a deterministic program-fault response. For
// JSON it returns the structured envelope; otherwise a short message string.
func execErrorBody(format string, res compute.Result) interface{} {
	if format == formatJSON {
		return executeResponse{
			Status:       res.Status.String(),
			Output:       res.Output,
			FuelConsumed: res.FuelConsumed,
			TrapMessage:  res.TrapMessage,
		}
	}
	msg := res.Status.String()
	if res.TrapMessage != "" {
		msg += ": " + res.TrapMessage
	}
	return msg
}

const (
	formatOctet = "octet"
	formatJSON  = "json"
	formatHTML  = "html"
)

// negotiateExecuteFormat picks a response representation from the Accept header.
// It returns false when the client requires a media type we do not support.
func negotiateExecuteFormat(accept string) (string, bool) {
	accept = strings.TrimSpace(accept)
	if accept == "" {
		return formatJSON, true
	}
	for _, part := range strings.Split(accept, ",") {
		// Drop any parameters (e.g. q-values); we do not rank by quality.
		mediaType := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		switch mediaType {
		case "application/json", "*/*", "application/*":
			return formatJSON, true
		case "text/html", "application/xhtml+xml":
			return formatHTML, true
		case "application/octet-stream":
			return formatOctet, true
		}
	}
	return "", false
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
