// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	topmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
)

type balancesResponse struct {
	balances string `json:"balances"`
}

func TestBalancesOK(t *testing.T) {

	/*
		marshalFunc := func() ([]byte, error) {
			return json.Marshal(balancesResponse{balances: "abcd"})
		}
		testServer := newTestServer(t, testServerOptions{
			balancesOpts: []topmock.Option{topmock.WithMarshalJSONFunc(marshalFunc)},
		})

		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/balances", nil, http.StatusOK, balancesResponse{
			balances: "abcd",
		})
	*/
}

func TestBalancesError(t *testing.T) {

	/*
		marshalFunc := func() ([]byte, error) {
			return nil, errors.New("error")
		}
		testServer := newTestServer(t, testServerOptions{
			balancesOpts: []topmock.Option{topmock.WithMarshalJSONFunc(marshalFunc)},
		})

		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/balances", nil, http.StatusInternalServerError, jsonhttp.StatusResponse{
			Message: "error",
			Code:    http.StatusInternalServerError,
		})
	*/
}

func TestBalancesPeersOK(t *testing.T) {

}

func TestBalancesPeersError(t *testing.T) {

}
