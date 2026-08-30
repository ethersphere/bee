// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/compute"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
)

// Real Accept headers browsers send. A subresource request never names
// text/html, so before the format upgrade every one of these came back as a
// base64 JSON envelope and no module could serve a stylesheet or an image.
const (
	acceptStylesheet = "text/css,*/*;q=0.1"
	acceptImage      = "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8"
	acceptFetch      = "*/*"
	acceptNavigation = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"
)

// okWith builds a clean result carrying the guest's response metadata.
func okWith(output string, status int, headers ...compute.Header) compute.Result {
	return compute.Result{
		Status:   compute.StatusOK,
		Output:   []byte(output),
		Response: compute.ResponseMeta{Status: status, Headers: headers},
	}
}

func TestExecuteGuestContentType(t *testing.T) {
	t.Parallel()

	css := "body{color:red}"

	for _, tc := range []struct {
		name     string
		accept   string
		wantType string
	}{
		{"browser stylesheet request", acceptStylesheet, "text/css"},
		{"browser image request", acceptImage, "text/css"},
		{"default fetch", acceptFetch, "text/css"},
		{"no accept header at all", "", "text/css"},
		{"top-level navigation", acceptNavigation, "text/css"},
		{"explicit octet-stream", "application/octet-stream", "text/css"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			engine := &mockEngine{result: okWith(css, 0,
				compute.Header{Name: "Content-Type", Value: "text/css"},
				compute.Header{Name: "Cache-Control", Value: "max-age=60"},
			)}
			client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
			addr := uploadModule(t, client, []byte("module"))

			opts := []jsonhttptest.Option{
				jsonhttptest.WithExpectedResponse([]byte(css)),
				jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, tc.wantType),
				jsonhttptest.WithExpectedResponseHeader("Cache-Control", "max-age=60"),
				jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmStatusHeader, "ok"),
			}
			if tc.accept != "" {
				opts = append(opts, jsonhttptest.WithRequestHeader(api.AcceptHeader, tc.accept))
			}

			jsonhttptest.Request(t, client, http.MethodGet, "/@/"+addr.String(), http.StatusOK, opts...)
		})
	}
}

// A module that sets nothing must behave exactly as it did before the response
// functions existed. This is the regression that keeps the change additive.
func TestExecuteWildcardWithoutMetadataStillEnvelope(t *testing.T) {
	t.Parallel()

	output := []byte("plain result")

	for _, accept := range []string{"", acceptFetch, "application/*"} {
		engine := &mockEngine{result: compute.Result{Status: compute.StatusOK, Output: output}}
		client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
		addr := uploadModule(t, client, []byte("module"))

		opts := []jsonhttptest.Option{}
		var body []byte
		opts = append(opts, jsonhttptest.WithPutResponseBody(&body))
		if accept != "" {
			opts = append(opts, jsonhttptest.WithRequestHeader(api.AcceptHeader, accept))
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/@/"+addr.String(), http.StatusOK, opts...)

		var resp struct {
			Status string `json:"status"`
			Output []byte `json:"output"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("accept %q: unmarshal %q: %v", accept, body, err)
		}
		if resp.Status != "ok" || !bytes.Equal(resp.Output, output) {
			t.Errorf("accept %q: got %+v, want the unchanged envelope", accept, resp)
		}
	}
}

// An explicit application/json is a request to be told about the run, so the
// guest's metadata is reported as fields and never applied to the transport.
func TestExecuteExplicitJSONReportsMetadata(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{result: okWith("<h1>hi</h1>", 404,
		compute.Header{Name: "Content-Type", Value: "text/html"},
		compute.Header{Name: "Link", Value: "</a>; rel=next"},
		compute.Header{Name: "Link", Value: "</b>; rel=prev"},
	)}
	client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
	addr := uploadModule(t, client, []byte("module"))

	var body []byte
	jsonhttptest.Request(t, client, http.MethodGet, "/@/"+addr.String(), http.StatusOK,
		jsonhttptest.WithRequestHeader(api.AcceptHeader, "application/json"),
		// The envelope is JSON regardless of what the module asked for.
		jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/json; charset=utf-8"),
		jsonhttptest.WithPutResponseBody(&body),
	)

	var resp struct {
		Status     string              `json:"status"`
		HTTPStatus int                 `json:"httpStatus"`
		Headers    map[string][]string `json:"headers"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal %q: %v", body, err)
	}
	if resp.Status != "ok" {
		t.Errorf("status: got %q, want ok", resp.Status)
	}
	if resp.HTTPStatus != 404 {
		t.Errorf("httpStatus: got %d, want 404", resp.HTTPStatus)
	}
	if got := resp.Headers["Link"]; len(got) != 2 {
		t.Errorf("Link: got %v, want both values in order", got)
	}
	if got := resp.Headers["Content-Type"]; len(got) != 1 || got[0] != "text/html" {
		t.Errorf("Content-Type: got %v", got)
	}
}

func TestExecuteGuestStatusCode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		status   int
		wantHTTP int
	}{
		{"module reports not found", 404, http.StatusNotFound},
		{"module redirects", 303, http.StatusSeeOther},
		{"module reports its own failure", 500, http.StatusInternalServerError},
		{"unset falls back to 200", 0, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			engine := &mockEngine{result: okWith("body", tc.status,
				compute.Header{Name: "Content-Type", Value: "text/plain"},
			)}
			client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
			addr := uploadModule(t, client, []byte("module"))

			jsonhttptest.Request(t, client, http.MethodGet, "/@/"+addr.String(), tc.wantHTTP,
				jsonhttptest.WithRequestHeader(api.AcceptHeader, acceptFetch),
				jsonhttptest.WithExpectedResponse([]byte("body")),
				// The verdict header stays truthful even when the module reports
				// a 500 of its own: ok here, host-error if the node had failed.
				jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmStatusHeader, "ok"),
			)
		})
	}
}

// The engine enforces the denylist; the API re-checks it so an engine bug cannot
// become a header leak. Asserted on the raw response, because what matters here
// is that the headers are absent.
func TestExecuteDeniedHeadersFilteredAtAPI(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{result: okWith("body", 0,
		compute.Header{Name: "Content-Type", Value: "text/plain"},
		compute.Header{Name: "Access-Control-Allow-Origin", Value: "*"},
		compute.Header{Name: "Set-Cookie", Value: "sid=1"},
		compute.Header{Name: "Strict-Transport-Security", Value: "max-age=31536000"},
		compute.Header{Name: "Transfer-Encoding", Value: "chunked"},
		compute.Header{Name: api.SwarmWasmStatusHeader, Value: "trap"},
	)}
	client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
	addr := uploadModule(t, client, []byte("module"))

	req, err := http.NewRequest(http.MethodGet, "/@/"+addr.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(api.AcceptHeader, acceptFetch)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	for _, name := range []string{
		"Access-Control-Allow-Origin",
		"Set-Cookie",
		"Strict-Transport-Security",
		"Transfer-Encoding",
	} {
		if got := resp.Header.Values(name); len(got) != 0 {
			t.Errorf("%s leaked through: %v", name, got)
		}
	}
	// The one header it was allowed to set still applies, so the filter is not
	// simply dropping everything.
	if got := resp.Header.Get(api.ContentTypeHeader); got != "text/plain" {
		t.Errorf("content-type: got %q, want text/plain", got)
	}
	// And the verdict survived the guest's attempt to forge it.
	if got := resp.Header.Get(api.SwarmWasmStatusHeader); got != "ok" {
		t.Errorf("%s: got %q, want ok", api.SwarmWasmStatusHeader, got)
	}
}

// A client that negotiated HTML must not receive a JSON body when the module
// trapped: answering with the wrong content type is a lie regardless of status.
func TestExecuteTrapBodyMatchesNegotiatedType(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		accept   string
		wantType string
	}{
		{"html", acceptNavigation, "text/html; charset=utf-8"},
		{"octet-stream", "application/octet-stream", "text/plain; charset=utf-8"},
		{"json", "application/json", "application/json; charset=utf-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			engine := &mockEngine{result: compute.Result{
				Status:      compute.StatusTrap,
				Output:      []byte("partial"),
				TrapMessage: "unreachable",
			}}
			client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
			addr := uploadModule(t, client, []byte("module"))

			var body []byte
			jsonhttptest.Request(t, client, http.MethodGet, "/@/"+addr.String(), http.StatusBadRequest,
				jsonhttptest.WithRequestHeader(api.AcceptHeader, tc.accept),
				jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, tc.wantType),
				jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmStatusHeader, "trap"),
				jsonhttptest.WithPutResponseBody(&body),
			)
			if !bytes.Contains(body, []byte("unreachable")) {
				t.Errorf("body %q does not name the trap", body)
			}
		})
	}
}
