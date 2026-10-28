// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
)

// TestCrossOriginWriteRejected checks that state-changing requests from origins
// the CORS configuration does not allow are rejected. A form POST is not
// preflighted, so the CORS response headers alone do not apply to it.
func TestCrossOriginWriteRejected(t *testing.T) {
	t.Parallel()

	const (
		allowed = "https://gateway.ethswarm.org"
		other   = "http://other.example"
	)

	for _, tc := range []struct {
		name           string
		method         string
		allowedOrigins []string
		headers        map[string]string
		wantStatus     int
	}{
		{
			name:       "no origin",
			method:     http.MethodPost,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "foreign origin post",
			method:     http.MethodPost,
			headers:    map[string]string{api.OriginHeader: other},
			wantStatus: http.StatusForbidden,
		},
		{
			name:           "foreign origin post with allow list",
			method:         http.MethodPost,
			allowedOrigins: []string{allowed},
			headers:        map[string]string{api.OriginHeader: other, "Sec-Fetch-Site": "cross-site"},
			wantStatus:     http.StatusForbidden,
		},
		{
			name:       "foreign fetch metadata only",
			method:     http.MethodPost,
			headers:    map[string]string{"Sec-Fetch-Site": "cross-site"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:           "listed origin post",
			method:         http.MethodPost,
			allowedOrigins: []string{allowed},
			headers:        map[string]string{api.OriginHeader: allowed, "Sec-Fetch-Site": "cross-site"},
			wantStatus:     http.StatusCreated,
		},
		{
			name:           "wildcard post",
			method:         http.MethodPost,
			allowedOrigins: []string{"*"},
			headers:        map[string]string{api.OriginHeader: other, "Sec-Fetch-Site": "cross-site"},
			wantStatus:     http.StatusCreated,
		},
		{
			name:       "same origin fetch metadata",
			method:     http.MethodPost,
			headers:    map[string]string{"Sec-Fetch-Site": "same-origin"},
			wantStatus: http.StatusCreated,
		},
		{
			// A TLS-terminating proxy in front of the node: the page is
			// https, the API sees plain HTTP.
			name:       "same origin through TLS proxy",
			method:     http.MethodPost,
			headers:    map[string]string{api.OriginHeader: "https://{host}", "Sec-Fetch-Site": "same-origin"},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "same host through TLS proxy without fetch metadata",
			method:     http.MethodPost,
			headers:    map[string]string{api.OriginHeader: "https://{host}"},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "other origin without fetch metadata",
			method:     http.MethodPost,
			headers:    map[string]string{api.OriginHeader: "https://other.example"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "foreign origin get",
			method:     http.MethodGet,
			headers:    map[string]string{api.OriginHeader: other, "Sec-Fetch-Site": "cross-site"},
			wantStatus: http.StatusOK,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, _, addr, _, _ := newTestServer(t, testServerOptions{
				Storer:             mockstorer.New(),
				CORSAllowedOrigins: tc.allowedOrigins,
			})

			opts := make([]jsonhttptest.Option, 0, len(tc.headers))
			for k, v := range tc.headers {
				opts = append(opts, jsonhttptest.WithRequestHeader(k, strings.ReplaceAll(v, "{host}", addr)))
			}
			jsonhttptest.Request(t, client, tc.method, "/tags", tc.wantStatus, opts...)
		})
	}
}

// TestCORSWildcardWithoutCredentials checks that a wildcard allow list does
// not grant credentialed access to every origin.
func TestCORSWildcardWithoutCredentials(t *testing.T) {
	t.Parallel()

	const origin = "http://other.example"

	client, _, _, _, _ := newTestServer(t, testServerOptions{
		Storer:             mockstorer.New(),
		CORSAllowedOrigins: []string{"*"},
	})

	h := jsonhttptest.Request(t, client, http.MethodGet, "/tags", http.StatusOK,
		jsonhttptest.WithRequestHeader(api.OriginHeader, origin),
		jsonhttptest.WithExpectedResponseHeader("Access-Control-Allow-Origin", origin),
	)
	if got := h.Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("got Access-Control-Allow-Credentials %q, want none", got)
	}
}
