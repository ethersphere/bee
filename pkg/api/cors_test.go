// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"net/http"
	"testing"
)

func TestCORSHeaders(t *testing.T) {
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
		t.Run(tc.name, func(t *testing.T) {
			client, _, _, _ := newTestServer(t, testServerOptions{
				CORSAllowedOrigins: tc.allowedOrigins,
			})

			req, err := http.NewRequest(http.MethodGet, "/", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
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

	for _, tc := range []struct {
		expectedMethods string // expectedMethods contains HTTP methods like GET, POST, HEAD, PATCH, DELETE, OPTIONS. These are in alphabetical sorted order
		endpoint        string
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
			expectedMethods: "GET, PATCH",
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
		t.Run(tc.endpoint, func(t *testing.T) {
			client, _, _, _ := newTestServer(t, testServerOptions{})

			r := jsonhttptest.Request(t, client, http.MethodOptions, "/"+tc.endpoint, http.StatusNoContent)

			allowedMethods := r.Get("Allow")

			if allowedMethods != tc.expectedMethods {
				t.Fatalf("expects %s and got %s", tc.expectedMethods, allowedMethods)
			}
		})
	}
}
