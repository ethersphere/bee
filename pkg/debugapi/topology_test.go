// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	topmock "github.com/ethersphere/bee/pkg/topology/mock"
)

func TestTopologyOK(t *testing.T) {
	marshalFunc := func() ([]byte, error) {
		return []byte("abcd"), nil
	}
	testServer := newTestServer(t, testServerOptions{
		TopologyOpts: []topmock.Option{topmock.WithMarshalJSONFunc(marshalFunc)},
	})
	defer testServer.Cleanup()

	jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/topology", nil, http.StatusOK, debugapi.TopologyResponse{
		Topology: "abcd",
	})
}

func TestTopologyError(t *testing.T) {
	marshalFunc := func() ([]byte, error) {
		return nil, errors.New("error")
	}
	testServer := newTestServer(t, testServerOptions{
		TopologyOpts: []topmock.Option{topmock.WithMarshalJSONFunc(marshalFunc)},
	})
	defer testServer.Cleanup()

	jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/topology", nil, http.StatusInternalServerError, jsonhttp.StatusResponse{
		Message: "error",
		Code:    http.StatusInternalServerError,
	})
}
