// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/compute"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
)

// realEngineClient wires the actual wazero engine into the API test server, so
// this exercises the whole chain a request travels: HTTP handler -> negotiation
// -> engine -> swarm host module -> render. The mockEngine tests above pin the
// HTTP layer's behaviour; this pins that the layers agree.
func realEngineClient(t *testing.T, cfg api.ExecuteConfig) *http.Client {
	t.Helper()

	engine, err := compute.New(compute.Options{
		Workers:  1,
		Watchdog: 10 * time.Second,
		Logger:   log.Noop,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Errorf("close compute service: %v", err)
		}
	})
	return newExecuteTestServer(t, engine, cfg)
}

// computeFixture loads a hand-written module from the compute package's
// testdata, which is where the fixtures and their .wat sources live.
func computeFixture(t *testing.T, name string) []byte {
	t.Helper()

	module, err := os.ReadFile(filepath.Join("..", "compute", "testdata", name+".wasm"))
	if err != nil {
		t.Fatal(err)
	}
	return module
}

// A real module setting a real Content-Type must reach a browser as that type,
// through the real negotiation path. This is the end-to-end form of the bug that
// motivated the format upgrade: before it, a stylesheet request came back as a
// base64 JSON envelope.
func TestExecuteEndToEndGuestHeaders(t *testing.T) {
	t.Parallel()

	client := realEngineClient(t, api.ExecuteConfig{})
	addr := uploadModule(t, client, computeFixture(t, "respok"))

	jsonhttptest.Request(t, client, http.MethodGet, "/@/"+addr.String()+"/style.css",
		http.StatusCreated, // the module set 201
		jsonhttptest.WithRequestHeader(api.AcceptHeader, acceptStylesheet),
		jsonhttptest.WithExpectedResponse([]byte("hi")),
		jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "text/css"),
		jsonhttptest.WithExpectedResponseHeader("Cache-Control", "max-age=60"),
		jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmStatusHeader, "ok"),
	)
}

// The same module through an explicit application/json must report rather than
// apply, and answer 200 because the envelope itself was delivered.
func TestExecuteEndToEndEnvelopeReports(t *testing.T) {
	t.Parallel()

	client := realEngineClient(t, api.ExecuteConfig{})
	addr := uploadModule(t, client, computeFixture(t, "respok"))

	var body []byte
	jsonhttptest.Request(t, client, http.MethodGet, "/@/"+addr.String()+"/", http.StatusOK,
		jsonhttptest.WithRequestHeader(api.AcceptHeader, "application/json"),
		jsonhttptest.WithPutResponseBody(&body),
	)
	for _, want := range []string{`"httpStatus":201`, `"Content-Type"`, `"text/css"`} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("envelope %s does not contain %s", body, want)
		}
	}
}

// A real trapping module must not leave its headers or status behind.
func TestExecuteEndToEndTrapDropsMetadata(t *testing.T) {
	t.Parallel()

	client := realEngineClient(t, api.ExecuteConfig{})
	addr := uploadModule(t, client, computeFixture(t, "resptrap"))

	var body []byte
	jsonhttptest.Request(t, client, http.MethodGet, "/@/"+addr.String()+"/",
		// 400 from the verdict, not the 418 the module asked for.
		http.StatusBadRequest,
		jsonhttptest.WithRequestHeader(api.AcceptHeader, acceptNavigation),
		jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
		jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmStatusHeader, "trap"),
		jsonhttptest.WithPutResponseBody(&body),
	)
}

// A module that shapes its response needs no node access, while one that reaches
// for the node's data on such a node is still invalid.
func TestExecuteEndToEndResponseWithoutNodeAccess(t *testing.T) {
	t.Parallel()

	client := realEngineClient(t, api.ExecuteConfig{})
	addr := uploadModule(t, client, computeFixture(t, "respok"))

	jsonhttptest.Request(t, client, http.MethodGet, "/@/"+addr.String()+"/", http.StatusCreated,
		jsonhttptest.WithRequestHeader(api.AcceptHeader, acceptFetch),
		jsonhttptest.WithExpectedResponse([]byte("hi")),
		jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "text/css"),
	)
}
