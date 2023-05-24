// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/util"
)

func TestShutdown(t *testing.T) {
	t.Parallel()

	shutdownSig := util.NewSignaler()

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		ShutdownSig: shutdownSig,
	})

	jsonhttptest.Request(t, testServer, http.MethodPost, "/shutdown", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.MsgShutdownStarted,
			Code:    http.StatusOK,
		}),
	)

	select {
	case <-shutdownSig.C:
	case <-time.After(time.Second):
		t.Fatal("shutdown signal not received")
	}

	// test endpoint idempotency
	jsonhttptest.Request(t, testServer, http.MethodPost, "/shutdown", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.MsgShutdownAlreadyStarted,
			Code:    http.StatusOK,
		}),
	)
}
