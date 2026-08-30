// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/compute"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
)

// envOf runs a request and returns the environment the engine was handed.
func envOf(t *testing.T, engine *mockEngine) map[string]string {
	t.Helper()

	env := make(map[string]string, len(engine.request.Env))
	for _, v := range engine.request.Env {
		env[v.Name] = v.Value
	}
	return env
}

func TestExecutePathInfo(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		suffix         string
		wantPathInfo   string
		wantScriptTail string
	}{
		// CGI's rule: the bare form has no PATH_INFO at all, which is how a
		// module tells /@/{addr} from /@/{addr}/ and can redirect between them.
		{"bare address", "", "", ""},
		{"trailing slash", "/", "/", ""},
		{"single segment", "/style.css", "/style.css", ""},
		{"nested path", "/blob/abc/def", "/blob/abc/def", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			engine := &mockEngine{result: compute.Result{Status: compute.StatusOK}}
			client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
			addr := uploadModule(t, client, []byte("module"))

			var body []byte
			jsonhttptest.Request(t, client, http.MethodGet, "/@/"+addr.String()+tc.suffix,
				http.StatusOK, jsonhttptest.WithPutResponseBody(&body))

			env := envOf(t, engine)
			if got := env["PATH_INFO"]; got != tc.wantPathInfo {
				t.Errorf("PATH_INFO: got %q, want %q", got, tc.wantPathInfo)
			}
			// SCRIPT_NAME is the mount point: the request path minus PATH_INFO.
			wantScript := "/@/" + addr.String() + tc.wantScriptTail
			if got := env["SCRIPT_NAME"]; got != wantScript {
				t.Errorf("SCRIPT_NAME: got %q, want %q", got, wantScript)
			}
			// Concatenating them must reproduce the path the client asked for,
			// which is the property a module relies on to build self-links.
			if got := env["SCRIPT_NAME"] + env["PATH_INFO"]; got != "/@/"+addr.String()+tc.suffix {
				t.Errorf("SCRIPT_NAME+PATH_INFO: got %q, want %q", got, "/@/"+addr.String()+tc.suffix)
			}
		})
	}
}

func TestExecuteRequestEnv(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{result: compute.Result{Status: compute.StatusOK}}
	client := newExecuteTestServer(t, engine, api.ExecuteConfig{})
	addr := uploadModule(t, client, []byte("module"))

	var body []byte
	jsonhttptest.Request(t, client, http.MethodPost,
		"/@/"+addr.String()+"/upload?name=photo.png&size=3", http.StatusOK,
		jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "application/octet-stream"),
		jsonhttptest.WithRequestHeader(api.AcceptHeader, "application/json"),
		jsonhttptest.WithRequestHeader("User-Agent", "test-agent"),
		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		jsonhttptest.WithRequestBody(strings.NewReader("abc")),
		jsonhttptest.WithPutResponseBody(&body),
	)

	env := envOf(t, engine)
	for _, tc := range []struct{ name, want string }{
		{"QUERY_STRING", "name=photo.png&size=3"},
		{"REQUEST_URI", "/@/" + addr.String() + "/upload?name=photo.png&size=3"},
		{"CONTENT_TYPE", "application/octet-stream"},
		{"CONTENT_LENGTH", "3"},
		{"HTTP_USER_AGENT", "test-agent"},
		{"HTTP_ACCEPT", "application/json"},
		// The batch id is allowlisted on purpose: a module cannot enumerate the
		// node's batches, so this is how a caller hands it one.
		{"HTTP_SWARM_POSTAGE_BATCH_ID", batchOkStr},
	} {
		if got := env[tc.name]; got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Forwarding request headers must never hand a module the operator's
// credentials, whatever the configuration says.
func TestExecuteRequestHeaderAllowlist(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		configure []string
	}{
		{"default list", nil},
		{"operator tries to add them", []string{"Accept", "Authorization", "Cookie"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			engine := &mockEngine{result: compute.Result{Status: compute.StatusOK}}
			client := newExecuteTestServer(t, engine, api.ExecuteConfig{
				RequestHeaders: tc.configure,
			})
			addr := uploadModule(t, client, []byte("module"))

			var body []byte
			jsonhttptest.Request(t, client, http.MethodGet, "/@/"+addr.String()+"/", http.StatusOK,
				jsonhttptest.WithRequestHeader("Authorization", "Bearer operator-token"),
				jsonhttptest.WithRequestHeader("Cookie", "session=secret"),
				jsonhttptest.WithRequestHeader(api.AcceptHeader, "text/html"),
				jsonhttptest.WithPutResponseBody(&body),
			)

			env := envOf(t, engine)
			for _, name := range []string{"HTTP_AUTHORIZATION", "HTTP_COOKIE"} {
				if got, ok := env[name]; ok {
					t.Errorf("%s leaked to the guest: %q", name, got)
				}
			}
			if got := env["HTTP_ACCEPT"]; got != "text/html" {
				t.Errorf("HTTP_ACCEPT: got %q, want text/html", got)
			}
		})
	}
}

// A configuration naming a credential header is a startup error, not a warning.
func TestValidateRequestHeaders(t *testing.T) {
	t.Parallel()

	if err := api.ValidateRequestHeaders([]string{"Accept", "User-Agent"}); err != nil {
		t.Errorf("a sane list was rejected: %v", err)
	}
	for _, name := range []string{"Authorization", "authorization", " Cookie ", "Proxy-Authorization"} {
		if err := api.ValidateRequestHeaders([]string{"Accept", name}); err == nil {
			t.Errorf("%q was accepted", name)
		}
	}
}

// An oversized environment is refused outright rather than truncated: a
// truncated environment is a lie the module cannot detect.
func TestExecuteEnvTooLarge(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{result: compute.Result{Status: compute.StatusOK}}
	client := newExecuteTestServer(t, engine, api.ExecuteConfig{MaxEnvBytes: 128})
	addr := uploadModule(t, client, []byte("module"))

	var body []byte
	jsonhttptest.Request(t, client, http.MethodGet,
		"/@/"+addr.String()+"/"+strings.Repeat("a", 512), http.StatusRequestHeaderFieldsTooLarge,
		jsonhttptest.WithPutResponseBody(&body))

	if engine.calls != 0 {
		t.Errorf("engine ran despite an oversized environment")
	}
}
