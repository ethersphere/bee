// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
)

type testIndexDebugger struct {
	indicesFunc func() (map[string]int, error)
}

var _ api.StorageIndexDebugger = (*testIndexDebugger)(nil)

func (t *testIndexDebugger) DebugIndices() (map[string]int, error) {
	return t.indicesFunc()
}

func TestDBIndices(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		expectedIndices := map[string]int{
			"a": 1,
			"b": 100,
			"c": 10000,
		}
		testServer, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI: true,
			IndexDebugger: &testIndexDebugger{
				indicesFunc: func() (map[string]int, error) { return expectedIndices, nil },
			},
		})

		// We expect a list of items unordered
		var got map[string]int
		jsonhttptest.Request(t, testServer, http.MethodGet, "/dbindices", http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		for k, v := range expectedIndices {
			if got[k] != v {
				t.Fatalf("expected index value %s, expected %d found %d", k, v, got[k])
			}
		}
	})
	t.Run("internal error returned", func(t *testing.T) {
		t.Parallel()
		testServer, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI: true,
			IndexDebugger: &testIndexDebugger{
				indicesFunc: func() (map[string]int, error) { return nil, errors.New("dummy error") },
			},
		})

		jsonhttptest.Request(t, testServer, http.MethodGet, "/dbindices", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "cannot get storage indices",
				Code:    http.StatusInternalServerError,
			}),
		)
	})
	t.Run("not implemented error returned", func(t *testing.T) {
		t.Parallel()
		testServer, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI: true,
		})

		jsonhttptest.Request(t, testServer, http.MethodGet, "/dbindices", http.StatusNotImplemented,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "storage indices not available",
				Code:    http.StatusNotImplemented,
			}),
		)
	})
}
