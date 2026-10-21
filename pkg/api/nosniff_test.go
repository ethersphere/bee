// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
)

// TestNoSniffHeader checks that downloaded content is served with
// X-Content-Type-Options: nosniff, so a browser does not reinterpret uploaded
// bytes as a type the uploader did not declare.
func TestNoSniffHeader(t *testing.T) {
	t.Parallel()

	client, _, _, _, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Post:   mockpost.New(mockpost.WithAcceptAll()),
	})

	var resp api.BzzUploadResponse
	jsonhttptest.Request(t, client, http.MethodPost, "/bzz?name=a.txt", http.StatusCreated,
		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/plain"),
		jsonhttptest.WithRequestBody(bytes.NewReader([]byte("<script>alert(1)</script>"))),
		jsonhttptest.WithUnmarshalJSONResponse(&resp),
	)

	jsonhttptest.Request(t, client, http.MethodGet, "/bzz/"+resp.Reference.String()+"/", http.StatusOK,
		jsonhttptest.WithExpectedResponseHeader("X-Content-Type-Options", "nosniff"),
	)
}

// TestNoSniffOnlyWithContentType checks that nosniff is sent only for
// responses that declare a content type. Files uploaded in a collection with an
// unknown extension have none, and browsers must still be able to sniff them.
func TestNoSniffOnlyWithContentType(t *testing.T) {
	t.Parallel()

	client, _, _, _, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Post:   mockpost.New(mockpost.WithAcceptAll()),
	})

	var resp api.BzzUploadResponse
	jsonhttptest.Request(t, client, http.MethodPost, "/bzz", http.StatusCreated,
		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		jsonhttptest.WithRequestHeader(api.SwarmCollectionHeader, "True"),
		jsonhttptest.WithRequestHeader(api.ContentTypeHeader, api.ContentTypeTar),
		jsonhttptest.WithRequestBody(tarFiles(t, []f{
			{data: []byte("<p>page</p>"), name: "page.html"},
			{data: []byte("<p>index</p>"), name: "index"},
			{data: []byte("console.log(1)"), name: "app.mjs2"},
		})),
		jsonhttptest.WithUnmarshalJSONResponse(&resp),
	)

	for _, tc := range []struct {
		path    string
		nosniff bool
	}{
		{"page.html", true},
		{"index", false},
		{"app.mjs2", false},
	} {
		h := jsonhttptest.Request(t, client, http.MethodGet, "/bzz/"+resp.Reference.String()+"/"+tc.path, http.StatusOK)
		if got := h.Get("X-Content-Type-Options") == "nosniff"; got != tc.nosniff {
			t.Errorf("%s: got nosniff %v with Content-Type %q, want %v", tc.path, got, h.Get("Content-Type"), tc.nosniff)
		}
	}
}
