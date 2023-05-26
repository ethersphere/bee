// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
)

func TestCORSHeaders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		origin         string
		allowedOrigins []string
		wantCORS       bool
	}{
		{
			name: "none",
		},
		{
			name:           "no origin",
			allowedOrigins: []string{"https://gateway.ethswarm.org"},
			wantCORS:       false,
		},
		{
			name:           "single explicit",
			origin:         "https://gateway.ethswarm.org",
			allowedOrigins: []string{"https://gateway.ethswarm.org"},
			wantCORS:       true,
		},
		{
			name:           "single explicit blocked",
			origin:         "http://a-hacker.me",
			allowedOrigins: []string{"https://gateway.ethswarm.org"},
			wantCORS:       false,
		},
		{
			name:           "multiple explicit",
			origin:         "https://staging.gateway.ethswarm.org",
			allowedOrigins: []string{"https://gateway.ethswarm.org", "https://staging.gateway.ethswarm.org"},
			wantCORS:       true,
		},
		{
			name:           "multiple explicit blocked",
			origin:         "http://a-hacker.me",
			allowedOrigins: []string{"https://gateway.ethswarm.org", "https://staging.gateway.ethswarm.org"},
			wantCORS:       false,
		},
		{
			name:           "wildcard",
			origin:         "http://localhost:1234",
			allowedOrigins: []string{"*"},
			wantCORS:       true,
		},
		{
			name:           "wildcard",
			origin:         "https://gateway.ethswarm.org",
			allowedOrigins: []string{"*"},
			wantCORS:       true,
		},
		{
			name:           "with origin only",
			origin:         "https://gateway.ethswarm.org",
			allowedOrigins: nil,
			wantCORS:       false,
		},
		{
			name:           "with origin only not nil",
			origin:         "https://gateway.ethswarm.org",
			allowedOrigins: []string{},
			wantCORS:       false,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			client, _, _, _ := newTestServer(t, testServerOptions{
				CORSAllowedOrigins: tc.allowedOrigins,
			})

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.origin != "" {
				req.Header.Set(api.OriginHeader, tc.origin)
			}

			r, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}

			got := r.Header.Get("Access-Control-Allow-Origin")

			if tc.wantCORS {
				if got != tc.origin {
					t.Errorf("got Access-Control-Allow-Origin %q, want %q", got, tc.origin)
				}
			} else {
				if got != "" {
					t.Errorf("got Access-Control-Allow-Origin %q, want none", got)
				}
			}
		})
	}

}

// TestCors tests whether CORs work correctly with OPTIONS method
func TestCors(t *testing.T) {
	t.Parallel()

	const origin = "example.com"
	for _, tc := range []struct {
		endpoint        string
		expectedMethods string // expectedMethods contains HTTP methods like GET, POST, HEAD, PATCH, DELETE, OPTIONS. These are in alphabetical sorted order
	}{
		{
			endpoint:        "tags",
			expectedMethods: "GET, POST",
		},
		{
			endpoint:        "bzz",
			expectedMethods: "POST",
		}, {
			endpoint:        "bzz/0101011",
			expectedMethods: "GET",
		},
		{
			endpoint:        "chunks",
			expectedMethods: "POST",
		},
		{
			endpoint:        "chunks/123213",
			expectedMethods: "DELETE, GET, HEAD",
		},
		{
			endpoint:        "bytes",
			expectedMethods: "POST",
		},
		{
			endpoint:        "bytes/0121012",
			expectedMethods: "GET, HEAD",
		},
	} {
		tc := tc
		t.Run(tc.endpoint, func(t *testing.T) {
			t.Parallel()

			client, _, _, _ := newTestServer(t, testServerOptions{
				CORSAllowedOrigins: []string{origin},
			})

			r := jsonhttptest.Request(t, client, http.MethodOptions, "/"+tc.endpoint, http.StatusNoContent,
				jsonhttptest.WithRequestHeader(api.OriginHeader, origin))

			allowedMethods := r.Get("Access-Control-Allow-Methods")

			if allowedMethods != tc.expectedMethods {
				t.Fatalf("expects %s and got %s", tc.expectedMethods, allowedMethods)
			}
		})
	}
}

// TestCorsStatus tests whether CORs returns correct allowed method if wrong method is called
func TestCorsStatus(t *testing.T) {
	t.Parallel()
	const origin = "example.com"
	for _, tc := range []struct {
		endpoint          string
		notAllowedMethods string // notAllowedMethods contains HTTP methods like GET, POST, HEAD, PATCH, DELETE, OPTIONS. These are method which is not supported by endpoint
		allowedMethods    string // expectedMethods contains HTTP methods like GET, POST, HEAD, PATCH, DELETE, OPTIONS. These are in alphabetical sorted order
	}{
		{
			endpoint:          "tags",
			notAllowedMethods: http.MethodDelete,
			allowedMethods:    "GET, POST",
		},
		{
			endpoint:          "bzz",
			notAllowedMethods: http.MethodDelete,
			allowedMethods:    "POST",
		},
		{
			endpoint:          "chunks",
			notAllowedMethods: http.MethodDelete,
			allowedMethods:    "POST",
		},
		{
			endpoint:          "chunks/0101011",
			notAllowedMethods: http.MethodPost,
			allowedMethods:    "DELETE, GET, HEAD",
		},
		{
			endpoint:          "bytes",
			notAllowedMethods: http.MethodDelete,
			allowedMethods:    "POST",
		},
		{
			endpoint:          "bytes/0121012",
			notAllowedMethods: http.MethodDelete,
			allowedMethods:    "GET, HEAD",
		},
	} {
		tc := tc
		t.Run(tc.endpoint, func(t *testing.T) {
			t.Parallel()

			client, _, _, _ := newTestServer(t, testServerOptions{
				CORSAllowedOrigins: []string{origin},
			})

			r := jsonhttptest.Request(t, client, tc.notAllowedMethods, "/"+tc.endpoint, http.StatusMethodNotAllowed,
				jsonhttptest.WithRequestHeader(api.OriginHeader, origin))

			allowedMethods := r.Get("Access-Control-Allow-Methods")
			if allowedMethods != tc.allowedMethods {
				t.Fatalf("expects %s and got %s", tc.notAllowedMethods, allowedMethods)
			}
		})
	}
}
