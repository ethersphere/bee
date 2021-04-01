// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
)

func TestTopologyOK(t *testing.T) {
	testServer := newTestServer(t, testServerOptions{})

	var body []byte
	opts := jsonhttptest.WithPutResponseBody(&body)
	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/topology", http.StatusOK, opts)

	if len(body) == 0 {
		t.Error("empty response")
	}
}
