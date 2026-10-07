// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestLookaheadBufferSizeIsBounded guards against Swarm-Lookahead-Buffer-Size
// driving an unbounded allocation: the value reaches bufio.NewReaderSize, which
// allocates the whole buffer before any data is read.
func TestLookaheadBufferSizeIsBounded(t *testing.T) {
	t.Parallel()

	client, _, _, _, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Post:   mockpost.New(mockpost.WithAcceptAll()),
	})

	var resp api.BytesPostResponse
	jsonhttptest.Request(t, client, http.MethodPost, "/bytes", http.StatusCreated,
		jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		jsonhttptest.WithRequestBody(bytes.NewReader(make([]byte, swarm.ChunkSize))),
		jsonhttptest.WithUnmarshalJSONResponse(&resp),
	)

	const limit = 4 * 1024 * 1024

	for _, tc := range []struct {
		name string
		size int
		want int
	}{
		{"zero disables buffering", 0, http.StatusOK},
		{"negative", -1, http.StatusBadRequest},
		{"at limit", limit, http.StatusOK},
		{"over limit", limit + 1, http.StatusBadRequest},
		{"gigabyte", 1 << 30, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/bytes/"+resp.Reference.String(), nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(api.SwarmLookAheadBufferSizeHeader, strconv.Itoa(tc.size))

			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()

			if res.StatusCode != tc.want {
				t.Fatalf("lookahead %d: got status %d, want %d", tc.size, res.StatusCode, tc.want)
			}
		})
	}
}
