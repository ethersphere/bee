// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	topmock "github.com/ethersphere/bee/pkg/topology/mock"
)

type topologyResponse struct {
	Topology string `json:"topology"`
}

func TestTopologyOK(t *testing.T) {
	marshalFunc := func() ([]byte, error) {
		return json.Marshal(topologyResponse{Topology: "abcd"})
	}
	testServer := newTestServer(t, testServerOptions{
		TopologyOpts: []topmock.Option{topmock.WithMarshalJSONFunc(marshalFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/topology", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(topologyResponse{
			Topology: "abcd",
		}),
	)
}

func TestTopologyError(t *testing.T) {
	marshalFunc := func() ([]byte, error) {
		return nil, errors.New("error")
	}
	testServer := newTestServer(t, testServerOptions{
		TopologyOpts: []topmock.Option{topmock.WithMarshalJSONFunc(marshalFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/topology", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: "error",
			Code:    http.StatusInternalServerError,
		}),
	)
}
