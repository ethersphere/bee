// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestHasChunkHandler(t *testing.T) {
	mockStorer := mock.NewStorer()
	testServer := newTestServer(t, testServerOptions{
		Storer: mockStorer,
	})
	defer testServer.Cleanup()

	key := swarm.MustParseHexAddress("aabbcc")
	value := []byte("data data data")

	_, err := mockStorer.Put(context.Background(), storage.ModePutUpload, swarm.NewChunk(key, value))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/chunks/"+key.String(), nil, http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		})
	})

	t.Run("not found", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/chunks/abbbbb", nil, http.StatusNotFound, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusNotFound),
			Code:    http.StatusNotFound,
		})
	})

	t.Run("bad address", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/chunks/abcd1100zz", nil, http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "bad address",
			Code:    http.StatusBadRequest,
		})
	})
}
