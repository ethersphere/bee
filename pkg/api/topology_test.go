// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
)

func TestTopologyOK(t *testing.T) {
	t.Parallel()

	testServer, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})

	var body []byte
	opts := jsonhttptest.WithPutResponseBody(&body)
	jsonhttptest.Request(t, testServer, http.MethodGet, "/topology", http.StatusOK, opts)

	if len(body) == 0 {
		t.Error("empty response")
	}
}
