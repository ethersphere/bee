// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
)

func TestHealth(t *testing.T) {
	client, cleanup := newTestServer(t, testServerOptions{})
	defer cleanup()

	jsonhttptest.ResponseDirect(t, client, http.MethodGet, "/health", nil, http.StatusOK, statusResponse{
		Status: "ok",
	})
}

func TestReadiness(t *testing.T) {
	client, cleanup := newTestServer(t, testServerOptions{})
	defer cleanup()

	jsonhttptest.ResponseDirect(t, client, http.MethodGet, "/readiness", nil, http.StatusOK, statusResponse{
		Status: "ok",
	})
}
