// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"errors"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/auth/mock"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
)

func TestAuth(t *testing.T) {
	var (
		resource      = "/auth"
		logger        = logging.New(ioutil.Discard, 0)
		authenticator = &mock.Auth{
			AuthorizeFunc: func(s1, s2 string) bool { return true },
			AddKeyFunc:    func(s string) (string, error) { return "123", nil },
		}
		client, _, _ = newTestServer(t, testServerOptions{
			Logger:        logger,
			Restricted:    true,
			Authenticator: authenticator,
		})
	)

	t.Run("missing authorization header", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusUnauthorized,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Unauthorized",
				Code:    http.StatusUnauthorized,
			}),
		)
	})
	t.Run("missing role", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader("Authorization", "Basic dGVzdDp0ZXN0"),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Unmarshal json body",
				Code:    http.StatusBadRequest,
			}),
		)
	})
	t.Run("bad authorization header", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusUnauthorized,
			jsonhttptest.WithRequestHeader("Authorization", "Basic dGV"),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Unauthorized",
				Code:    http.StatusUnauthorized,
			}),
		)
	})
	t.Run("bad request body", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader("Authorization", "Basic dGVzdDp0ZXN0"),

			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Unmarshal json body",
				Code:    http.StatusBadRequest,
			}),
		)
	})
	t.Run("unauthorized", func(t *testing.T) {
		original := authenticator.AuthorizeFunc
		authenticator.AuthorizeFunc = func(s1, s2 string) bool {
			return false
		}
		defer func() {
			authenticator.AuthorizeFunc = original
		}()
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusUnauthorized,
			jsonhttptest.WithRequestHeader("Authorization", "Basic dGVzdDp0ZXN0"),

			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Unauthorized",
				Code:    http.StatusUnauthorized,
			}),
		)
	})
	t.Run("failed to add key", func(t *testing.T) {
		original := authenticator.AddKeyFunc
		authenticator.AddKeyFunc = func(s string) (string, error) {
			return "", errors.New("error adding key")
		}
		defer func() {
			authenticator.AddKeyFunc = original
		}()
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusInternalServerError,
			jsonhttptest.WithRequestHeader("Authorization", "Basic dGVzdDp0ZXN0"),
			jsonhttptest.WithJSONRequestBody(api.RoleRequest{
				Role: "role0",
			}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "error adding key",
				Code:    http.StatusInternalServerError,
			}),
		)
	})
	t.Run("success", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusCreated,
			jsonhttptest.WithRequestHeader("Authorization", "Basic dGVzdDp0ZXN0"),
			jsonhttptest.WithJSONRequestBody(api.RoleRequest{
				Role: "role0",
			}),
			jsonhttptest.WithExpectedJSONResponse(api.AuthKeyResponse{
				Key: "123",
			}),
		)
	})
}
