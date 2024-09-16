// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	mockbatchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
)

func TestPostEnvelope(t *testing.T) {
	t.Parallel()

	zeroHex := "0000000000000000000000000000000000000000000000000000000000000000"
	envelopeEndpoint := func(chunkAddress string) string { return fmt.Sprintf("/envelope/%s", chunkAddress) }
	client, _, _, _ := newTestServer(t, testServerOptions{
		Post: mockpost.New(mockpost.WithAcceptAll()),
	})

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, client, http.MethodPost, envelopeEndpoint(zeroHex), http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		)
	})

	t.Run("wrong chunk address", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, client, http.MethodPost, envelopeEndpoint("invalid"), http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		)
	})

	t.Run("postage does not exist", func(t *testing.T) {
		t.Parallel()
		client, _, _, _ := newTestServer(t, testServerOptions{})

		jsonhttptest.Request(t, client, http.MethodPost, envelopeEndpoint(zeroHex), http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, zeroHex),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{Message: "batch with id not found", Code: http.StatusNotFound}),
		)
	})

	t.Run("batch unusable", func(t *testing.T) {
		t.Parallel()
		client, _, _, _ := newTestServer(t, testServerOptions{
			Post:       mockpost.New(mockpost.WithAcceptAll()),
			BatchStore: mockbatchstore.New(),
		})

		jsonhttptest.Request(t, client, http.MethodPost, envelopeEndpoint(zeroHex), http.StatusUnprocessableEntity,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{Message: "batch not usable yet or does not exist", Code: http.StatusUnprocessableEntity}),
		)
	})
}
