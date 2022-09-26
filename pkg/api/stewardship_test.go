// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/resolver"
	resolverMock "github.com/ethersphere/bee/pkg/resolver/mock"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/steward/mock"
	smock "github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

// nolint:paralleltest
func TestStewardship(t *testing.T) {
	var (
		logger         = log.Noop
		statestoreMock = statestore.NewStateStore()
		stewardMock    = &mock.Steward{}
		storer         = smock.NewStorer()
		addr           = swarm.NewAddress([]byte{31: 128})
	)
	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer:  storer,
		Tags:    tags.NewTags(statestoreMock, logger),
		Logger:  logger,
		Steward: stewardMock,
	})

	t.Run("re-upload", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPut, "/v1/stewardship/"+addr.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)
		if !stewardMock.LastAddress().Equal(addr) {
			t.Fatalf("\nhave address: %q\nwant address: %q", stewardMock.LastAddress().String(), addr.String())
		}
	})

	t.Run("is-retrievable", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, "/v1/stewardship/"+addr.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.IsRetrievableResponse{IsRetrievable: true}),
		)
		jsonhttptest.Request(t, client, http.MethodGet, "/v1/stewardship/"+hex.EncodeToString([]byte{}), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusNotFound,
				Message: http.StatusText(http.StatusNotFound),
			}),
		)
	})
}

func TestStewardshipInputValidations(t *testing.T) {
	t.Parallel()

	var (
		logger         = log.Noop
		statestoreMock = statestore.NewStateStore()
		stewardMock    = &mock.Steward{}
		storer         = smock.NewStorer()
	)
	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer:  storer,
		Tags:    tags.NewTags(statestoreMock, logger),
		Logger:  logger,
		Steward: stewardMock,
		Resolver: resolverMock.NewResolver(
			resolverMock.WithResolveFunc(
				func(string) (swarm.Address, error) {
					return swarm.Address{}, resolver.ErrParse
				},
			),
		),
	})
	for _, tt := range []struct {
		name            string
		reference       string
		expectedStatus  int
		expectedMessage string
	}{
		{
			name:            "correct reference",
			reference:       "1e477b015af480e387fbf5edd90f1685a30c0e3ba88eeb3871b326b816a542da",
			expectedStatus:  http.StatusOK,
			expectedMessage: http.StatusText(http.StatusOK),
		},
		{
			name:            "reference not found",
			reference:       "1e477b015af480e387fbf5edd90f1685a30c0e3ba88eeb3871b326b816a542d/",
			expectedStatus:  http.StatusNotFound,
			expectedMessage: http.StatusText(http.StatusNotFound),
		},
		{
			name:            "incorrect reference",
			reference:       "xc0f6",
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "invalid address",
		},
	} {
		tt := tt
		t.Run("input validation -"+tt.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodPut, "/v1/stewardship/"+tt.reference, tt.expectedStatus,
				jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
					Message: tt.expectedMessage,
					Code:    tt.expectedStatus,
				}),
			)
		})
	}
}
