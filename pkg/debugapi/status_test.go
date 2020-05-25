// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
)

func TestHealth(t *testing.T) {
	testServer := newTestServer(t, testServerOptions{})

	jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/health", nil, http.StatusOK, debugapi.StatusResponse{
		Status: "ok",
	})
}

func TestReadiness(t *testing.T) {
	testServer := newTestServer(t, testServerOptions{})

	jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/readiness", nil, http.StatusOK, debugapi.StatusResponse{
		Status: "ok",
	})
}
