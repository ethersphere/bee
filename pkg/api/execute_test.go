// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/compute"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// mockEngine is a compute.Engine that records what it was asked to run and
// replays a canned outcome.
type mockEngine struct {
	result  compute.Result
	err     error
	request compute.Request
	calls   int
}

func (m *mockEngine) Execute(_ context.Context, req compute.Request) (compute.Result, error) {
	m.calls++
	m.request = req
	return m.result, m.err
}

func (m *mockEngine) Close() error { return nil }

// uploadModule stores content through the bytes endpoint and returns its address.
func uploadModule(t *testing.T, client *http.Client, content []byte) swarm.Address {
	t.Helper()

	var resp api.BytesPostResponse
	jsonhttptest.Request(t, client, http.MethodPost, "/bytes", http.StatusCreated,
		jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		jsonhttptest.WithRequestBody(bytes.NewReader(content)),
		jsonhttptest.WithUnmarshalJSONResponse(&resp),
	)
	return resp.Reference
}

func newExecuteTestServer(t *testing.T, engine compute.Engine, cfg api.ExecuteConfig) *http.Client {
	t.Helper()

	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer:        mockstorer.New(),
		Logger:        log.Noop,
		Post:          mockpost.New(mockpost.WithAcceptAll()),
		Compute:       engine,
		ExecuteConfig: cfg,
	})
	return client
}

// TestExecute checks that the module is downloaded from Swarm, the request body
// is handed to the engine as input and the result comes back as raw bytes.
func TestExecute(t *testing.T) {
	t.Parallel()

	module := []byte("this stands in for a wasm module")
	engine := &mockEngine{result: compute.Result{
		Status:       compute.StatusOK,
		Output:       []byte("computed output"),
		FuelConsumed: 4711,
	}}

	client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
	addr := uploadModule(t, client, module)

	jsonhttptest.Request(t, client, http.MethodPost, "/@/"+addr.String(), http.StatusOK,
		jsonhttptest.WithRequestHeader(api.AcceptHeader, "application/octet-stream"),
		jsonhttptest.WithRequestBody(bytes.NewReader([]byte("the input"))),
		jsonhttptest.WithExpectedResponse([]byte("computed output")),
		jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmStatusHeader, "ok"),
		jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmFuelConsumedHeader, "4711"),
	)

	if engine.calls != 1 {
		t.Fatalf("got %d engine calls, want 1", engine.calls)
	}
	if !bytes.Equal(engine.request.Module, module) {
		t.Errorf("got module %q, want %q", engine.request.Module, module)
	}
	if string(engine.request.Input) != "the input" {
		t.Errorf("got input %q, want %q", engine.request.Input, "the input")
	}
}

func TestExecuteDisabled(t *testing.T) {
	t.Parallel()

	client := newExecuteTestServer(t, nil, api.ExecuteConfig{})

	jsonhttptest.Request(t, client, http.MethodPost, "/@/"+swarm.RandAddress(t).String(), http.StatusForbidden,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: "WASM execution is disabled. This endpoint is unavailable.",
			Code:    http.StatusForbidden,
		}),
	)
}

func TestExecuteModuleNotFound(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{}
	client := newExecuteTestServer(t, engine, api.ExecuteConfig{})

	jsonhttptest.Request(t, client, http.MethodPost, "/@/"+swarm.RandAddress(t).String(), http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: "module not found",
			Code:    http.StatusNotFound,
		}),
	)

	if engine.calls != 0 {
		t.Errorf("got %d engine calls, want none for a missing module", engine.calls)
	}
}

func TestExecuteModuleTooLarge(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{}
	client := newExecuteTestServer(t, engine, api.ExecuteConfig{MaxModuleSize: 8})
	addr := uploadModule(t, client, []byte("well over eight bytes"))

	jsonhttptest.Request(t, client, http.MethodPost, "/@/"+addr.String(), http.StatusRequestEntityTooLarge,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: "module exceeds maximum size",
			Code:    http.StatusRequestEntityTooLarge,
		}),
	)

	if engine.calls != 0 {
		t.Errorf("got %d engine calls, want none for an oversized module", engine.calls)
	}
}

func TestExecuteBusy(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{err: compute.ErrBusy}
	client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
	addr := uploadModule(t, client, []byte("module"))

	jsonhttptest.Request(t, client, http.MethodPost, "/@/"+addr.String(), http.StatusTooManyRequests,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: "execution workers busy",
			Code:    http.StatusTooManyRequests,
		}),
	)
}

func TestExecuteEngineFailure(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{err: errors.New("engine exploded")}
	client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
	addr := uploadModule(t, client, []byte("module"))

	jsonhttptest.Request(t, client, http.MethodPost, "/@/"+addr.String(), http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: "execution failed",
			Code:    http.StatusInternalServerError,
		}),
	)
}

// TestExecuteStatusMapping checks that a program verdict maps to a stable HTTP
// status: a bad module or a trap is the caller's fault, a host error is ours.
func TestExecuteStatusMapping(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		result     compute.Result
		wantHTTP   int
		wantStatus string
	}{
		{
			name:       "ok",
			result:     compute.Result{Status: compute.StatusOK, Output: []byte("out")},
			wantHTTP:   http.StatusOK,
			wantStatus: "ok",
		},
		{
			name:       "out of fuel",
			result:     compute.Result{Status: compute.StatusOutOfFuel},
			wantHTTP:   http.StatusOK,
			wantStatus: "out-of-fuel",
		},
		{
			name:       "trap",
			result:     compute.Result{Status: compute.StatusTrap, TrapMessage: "unreachable"},
			wantHTTP:   http.StatusBadRequest,
			wantStatus: "trap",
		},
		{
			name:       "invalid module",
			result:     compute.Result{Status: compute.StatusInvalidModule, TrapMessage: "invalid magic number"},
			wantHTTP:   http.StatusBadRequest,
			wantStatus: "invalid-module",
		},
		{
			name:       "host error",
			result:     compute.Result{Status: compute.StatusHostError},
			wantHTTP:   http.StatusInternalServerError,
			wantStatus: "host-error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			engine := &mockEngine{result: tc.result}
			client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
			addr := uploadModule(t, client, []byte("module"))

			jsonhttptest.Request(t, client, http.MethodPost, "/@/"+addr.String(), tc.wantHTTP,
				jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmStatusHeader, tc.wantStatus),
			)
		})
	}
}

// TestExecuteContentNegotiation checks the representation chosen for each Accept
// header, and that an unsupported one is rejected before the module is run.
func TestExecuteContentNegotiation(t *testing.T) {
	t.Parallel()

	output := []byte("<b>result</b>")

	for _, tc := range []struct {
		name        string
		accept      string
		wantHTTP    int
		wantType    string
		wantBody    []byte
		wantJSON    bool
		wantExecute bool
	}{
		{
			name:        "no accept header defaults to json",
			wantHTTP:    http.StatusOK,
			wantJSON:    true,
			wantExecute: true,
		},
		{
			name:        "octet-stream",
			accept:      "application/octet-stream",
			wantHTTP:    http.StatusOK,
			wantType:    "application/octet-stream",
			wantBody:    output,
			wantExecute: true,
		},
		{
			name:        "wildcard defaults to json",
			accept:      "*/*",
			wantHTTP:    http.StatusOK,
			wantJSON:    true,
			wantExecute: true,
		},
		{
			name:        "html",
			accept:      "text/html",
			wantHTTP:    http.StatusOK,
			wantType:    "text/html; charset=utf-8",
			wantBody:    output,
			wantExecute: true,
		},
		{
			name:        "json",
			accept:      "application/json",
			wantHTTP:    http.StatusOK,
			wantJSON:    true,
			wantExecute: true,
		},
		{
			name:        "first supported type wins",
			accept:      "image/png, application/json;q=0.9",
			wantHTTP:    http.StatusOK,
			wantJSON:    true,
			wantExecute: true,
		},
		{
			name:     "unsupported type",
			accept:   "image/png",
			wantHTTP: http.StatusNotAcceptable,
			wantJSON: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			engine := &mockEngine{result: compute.Result{
				Status:       compute.StatusOK,
				Output:       output,
				FuelConsumed: 42,
			}}
			client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
			addr := uploadModule(t, client, []byte("module"))

			opts := []jsonhttptest.Option{}
			if tc.accept != "" {
				opts = append(opts, jsonhttptest.WithRequestHeader(api.AcceptHeader, tc.accept))
			}

			var body []byte
			if tc.wantJSON || tc.wantHTTP == http.StatusNotAcceptable {
				opts = append(opts, jsonhttptest.WithPutResponseBody(&body))
			} else {
				opts = append(opts,
					jsonhttptest.WithExpectedResponse(tc.wantBody),
					jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, tc.wantType),
				)
			}

			jsonhttptest.Request(t, client, http.MethodPost, "/@/"+addr.String(), tc.wantHTTP, opts...)

			if tc.wantJSON {
				var resp struct {
					Status       string `json:"status"`
					Output       []byte `json:"output"`
					FuelConsumed uint64 `json:"fuelConsumed"`
				}
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("unmarshal response %q: %v", body, err)
				}
				if resp.Status != "ok" {
					t.Errorf("got status %q, want %q", resp.Status, "ok")
				}
				if !bytes.Equal(resp.Output, output) {
					t.Errorf("got output %q, want %q", resp.Output, output)
				}
				if resp.FuelConsumed != 42 {
					t.Errorf("got fuel consumed %d, want 42", resp.FuelConsumed)
				}
			}

			if got := engine.calls > 0; got != tc.wantExecute {
				t.Errorf("engine called: %v, want %v", got, tc.wantExecute)
			}
		})
	}
}

// TestExecuteLimits checks that per-request limits are clamped to the operator
// configured maxima and fall back to the configured defaults.
func TestExecuteLimits(t *testing.T) {
	t.Parallel()

	cfg := api.ExecuteConfig{
		DefaultFuel:   1000,
		MaxFuel:       5000,
		DefaultMemory: 2048,
		MaxMemory:     8192,
	}

	for _, tc := range []struct {
		name       string
		fuel       string
		memory     string
		entrypoint string
		want       compute.Limits
	}{
		{
			name: "defaults",
			want: compute.Limits{Fuel: 1000, Memory: 2048},
		},
		{
			name:   "request below the maximum is honored",
			fuel:   "200",
			memory: "1024",
			want:   compute.Limits{Fuel: 200, Memory: 1024},
		},
		{
			name:   "request above the maximum is clamped",
			fuel:   "999999",
			memory: "999999",
			want:   compute.Limits{Fuel: 5000, Memory: 8192},
		},
		{
			name:       "entrypoint is passed through",
			entrypoint: "run",
			want:       compute.Limits{Fuel: 1000, Memory: 2048, Entrypoint: "run"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			engine := &mockEngine{result: compute.Result{Status: compute.StatusOK}}
			client := newExecuteTestServer(t, engine, cfg)
			addr := uploadModule(t, client, []byte("module"))

			opts := []jsonhttptest.Option{
				jsonhttptest.WithRequestHeader(api.AcceptHeader, "application/octet-stream"),
				jsonhttptest.WithNoResponseBody(),
			}
			if tc.fuel != "" {
				opts = append(opts, jsonhttptest.WithRequestHeader(api.SwarmWasmFuelLimitHeader, tc.fuel))
			}
			if tc.memory != "" {
				opts = append(opts, jsonhttptest.WithRequestHeader(api.SwarmWasmMemoryLimitHeader, tc.memory))
			}
			if tc.entrypoint != "" {
				opts = append(opts, jsonhttptest.WithRequestHeader(api.SwarmWasmEntrypointHeader, tc.entrypoint))
			}

			jsonhttptest.Request(t, client, http.MethodPost, "/@/"+addr.String(), http.StatusOK, opts...)

			if engine.request.Limits != tc.want {
				t.Errorf("got limits %+v, want %+v", engine.request.Limits, tc.want)
			}
		})
	}
}

func TestExecuteInvalidRequest(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{}
	client := newExecuteTestServer(t, engine, api.ExecuteConfig{})

	t.Run("invalid address", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/@/not-an-address", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid path params",
				Code:    http.StatusBadRequest,
				Reasons: []jsonhttp.Reason{{
					Field: "address",
					Error: api.HexInvalidByteError('n').Error(),
				}},
			}),
		)
	})

	t.Run("invalid fuel limit", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/@/"+swarm.RandAddress(t).String(), http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmWasmFuelLimitHeader, "not a number"),
		)
	})

	if engine.calls != 0 {
		t.Errorf("got %d engine calls, want none for an invalid request", engine.calls)
	}
}

// TestExecuteMethods checks that every HTTP method reaches the module and that
// the module is told which one it was called with.
func TestExecuteMethods(t *testing.T) {
	t.Parallel()

	for _, method := range []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		"PROPFIND",
	} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			module := []byte("this stands in for a wasm module")
			engine := &mockEngine{result: compute.Result{
				Status: compute.StatusOK,
				Output: []byte("computed output"),
			}}

			client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
			addr := uploadModule(t, client, module)

			opts := []jsonhttptest.Option{
				jsonhttptest.WithRequestHeader(api.AcceptHeader, "application/octet-stream"),
				jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmStatusHeader, "ok"),
			}
			// A HEAD response carries no body, so only assert it elsewhere.
			if method != http.MethodHead {
				opts = append(opts, jsonhttptest.WithExpectedResponse([]byte("computed output")))
			}
			jsonhttptest.Request(t, client, method, "/@/"+addr.String(), http.StatusOK, opts...)

			if engine.calls != 1 {
				t.Fatalf("got %d engine calls, want 1", engine.calls)
			}
			if engine.request.Method != method {
				t.Errorf("got method %q, want %q", engine.request.Method, method)
			}
			if !bytes.Equal(engine.request.Module, module) {
				t.Errorf("got module %q, want %q", engine.request.Module, module)
			}
		})
	}
}

// TestExecuteOptions checks that a CORS preflight is answered by the node and
// never reaches the module.
func TestExecuteOptions(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{result: compute.Result{Status: compute.StatusOK}}
	client := newExecuteTestServer(t, engine, api.ExecuteConfig{})

	jsonhttptest.Request(t, client, http.MethodOptions, "/@/"+swarm.RandAddress(t).String(), http.StatusNoContent)

	if engine.calls != 0 {
		t.Errorf("got %d engine calls, want none for a preflight", engine.calls)
	}
}
