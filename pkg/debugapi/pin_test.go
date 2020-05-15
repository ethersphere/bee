// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestPinChunkHandler checks for pinning, unpinning and listing of chunks.
// It also check other edgw cases like chunk not present and checking for pinning,
// invalid chunk address case etc. This test case has to be run in sequence and
// it assumes some state of the DB before another case is run.
func TestPinChunkHandler(t *testing.T) {
	key := swarm.MustParseHexAddress("aabbcc")
	value := []byte("data data data")
	logger := logging.New(ioutil.Discard, 0)

	storer, err := localstore.New("", key.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	testServer := newTestServer(t, testServerOptions{
		Storer: storer,
	})
	defer testServer.Cleanup()

	_, err = storer.Put(context.Background(), storage.ModePutUpload, swarm.NewChunk(key, value))
	if err != nil {
		t.Fatal(err)
	}

	// bad chunk address
	t.Run("pin-bad-address", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodPost, "/chunks-pin/abcd1100zz", nil, http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "bad address",
			Code:    http.StatusBadRequest,
		})
	})

	// pin a chunk which is not existing
	t.Run("pin-absent-chunk", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodPost, "/chunks-pin/123456", nil, http.StatusNotFound, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusNotFound),
			Code:    http.StatusNotFound,
		})
	})

	// unpin on a chunk which is not pinned
	t.Run("unpin-while-not-pinned", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodDelete, "/chunks-pin/"+key.String(), nil, http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "chunk is not yet pinned",
			Code:    http.StatusBadRequest,
		})
	})

	// pin a existing chunk first time
	t.Run("pin-chunk-1", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodPost, "/chunks-pin/"+key.String(), nil, http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		})

		// Check is the chunk is pinned once
		jsonhttptest.ResponseDirectWithJson(t, testServer.Client, http.MethodGet, "/chunks-pin/"+key.String(), nil, http.StatusOK, jsonhttp.StatusResponse{
			Message: `{"Address":"aabbcc","PinCounter":1}`,
			Code:    http.StatusOK,
		})

	})

	// pin a existing chunk second time
	t.Run("pin-chunk-2", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodPost, "/chunks-pin/"+key.String(), nil, http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		})

		// Check is the chunk is pinned twice
		jsonhttptest.ResponseDirectWithJson(t, testServer.Client, http.MethodGet, "/chunks-pin/"+key.String(), nil, http.StatusOK, jsonhttp.StatusResponse{
			Message: `{"Address":"aabbcc","PinCounter":2}`,
			Code:    http.StatusOK,
		})
	})

	// unpin a chunk first time
	t.Run("unpin-chunk-1", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodDelete, "/chunks-pin/"+key.String(), nil, http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		})

		// Check is the chunk is pinned once
		jsonhttptest.ResponseDirectWithJson(t, testServer.Client, http.MethodGet, "/chunks-pin/"+key.String(), nil, http.StatusOK, jsonhttp.StatusResponse{
			Message: `{"Address":"aabbcc","PinCounter":1}`,
			Code:    http.StatusOK,
		})
	})

	// unpin a chunk second time
	t.Run("unpin-chunk-2", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodDelete, "/chunks-pin/"+key.String(), nil, http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		})

		// Check is the chunk is removed from the pinIndex
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/chunks-pin/"+key.String(), nil, http.StatusInternalServerError, jsonhttp.StatusResponse{
			Message: "pin chunks: leveldb: not found",
			Code:    http.StatusInternalServerError,
		})
	})

	// Add 2 chunks, pin it and check if they show up in the list
	t.Run("list-chunks", func(t *testing.T) {
		_, err = storer.Put(context.Background(), storage.ModePutUpload, swarm.NewChunk(key, value))
		if err != nil {
			t.Fatal(err)
		}

		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodPost, "/chunks-pin/"+key.String(), nil, http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		})

		key2 := swarm.MustParseHexAddress("ddeeff")
		value2 := []byte("data data data")
		_, err = storer.Put(context.Background(), storage.ModePutUpload, swarm.NewChunk(key2, value2))
		if err != nil {
			t.Fatal(err)
		}
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodPost, "/chunks-pin/"+key2.String(), nil, http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		})

		jsonhttptest.ResponseDirectWithJson(t, testServer.Client, http.MethodGet, "/chunks-pin/", nil, http.StatusOK, jsonhttp.StatusResponse{
			Message: `{"Address":"aabbcc","PinCounter":1},{"Address":"ddeeff","PinCounter":1}`,
			Code:    http.StatusOK,
		})

	})

}
