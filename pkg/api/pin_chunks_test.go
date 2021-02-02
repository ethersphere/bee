// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/tags"
)

// TestPinChunkHandler checks for pinning, unpinning and listing of chunks.
// It also check other edgw cases like chunk not present and checking for pinning,
// invalid chunk address case etc. This test case has to be run in sequence and
// it assumes some state of the DB before another case is run.
func TestPinChunkHandler(t *testing.T) {
	var (
		chunksEndpoint = "/chunks"
		chunk          = testingc.GenerateTestRandomChunk()
		mockStorer     = mock.NewStorer()
		mockStatestore = statestore.NewStateStore()
		logger         = logging.New(ioutil.Discard, 0)
		tag            = tags.NewTags(mockStatestore, logger)

		client, _, _ = newTestServer(t, testServerOptions{
			Storer: mockStorer,
			Tags:   tag,
			Logger: logger,
		})
	)

	// bad chunk address
	t.Run("pin-bad-address", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/pin/chunks/abcd1100zz", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "bad address",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	// list pins without anything pinned
	t.Run("list-pins-zero-pins", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, "/pin/chunks", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.ListPinnedChunksResponse{
				Chunks: []api.PinnedChunk{},
			}),
		)
	})

	// pin a chunk which is not existing
	t.Run("pin-absent-chunk", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/pin/chunks/123456", http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusNotFound),
				Code:    http.StatusNotFound,
			}),
		)
	})

	// unpin on a chunk which is not pinned
	t.Run("unpin-while-not-pinned", func(t *testing.T) {
		// Post a chunk
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
		)

		jsonhttptest.Request(t, client, http.MethodDelete, "/pin/chunks/"+chunk.Address().String(), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "chunk is not yet pinned",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	// pin a existing chunk first time
	t.Run("pin-chunk-1", func(t *testing.T) {
		// Post a chunk
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
		)

		jsonhttptest.Request(t, client, http.MethodPost, "/pin/chunks/"+chunk.Address().String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		// Check is the chunk is pinned once
		jsonhttptest.Request(t, client, http.MethodGet, "/pin/chunks/"+chunk.Address().String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.PinnedChunk{
				Address:    chunk.Address(),
				PinCounter: 1,
			}),
		)

	})

	// pin a existing chunk second time
	t.Run("pin-chunk-2", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/pin/chunks/"+chunk.Address().String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		// Check is the chunk is pinned twice
		jsonhttptest.Request(t, client, http.MethodGet, "/pin/chunks/"+chunk.Address().String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.PinnedChunk{
				Address:    chunk.Address(),
				PinCounter: 2,
			}),
		)
	})

	// unpin a chunk first time
	t.Run("unpin-chunk-1", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodDelete, "/pin/chunks/"+chunk.Address().String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		// Check is the chunk is pinned once
		jsonhttptest.Request(t, client, http.MethodGet, "/pin/chunks/"+chunk.Address().String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.PinnedChunk{
				Address:    chunk.Address(),
				PinCounter: 1,
			}),
		)
	})

	// unpin a chunk second time
	t.Run("unpin-chunk-2", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodDelete, "/pin/chunks/"+chunk.Address().String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		// Check if the chunk is removed from the pinIndex
		jsonhttptest.Request(t, client, http.MethodGet, "/pin/chunks/"+chunk.Address().String(), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusNotFound),
				Code:    http.StatusNotFound,
			}),
		)
	})

	// Add 2 chunks, pin it and check if they show up in the list
	t.Run("list-chunks", func(t *testing.T) {
		// Post a chunk
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
		)

		jsonhttptest.Request(t, client, http.MethodPost, "/pin/chunks/"+chunk.Address().String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		// post another chunk
		chunk2 := testingc.GenerateTestRandomChunk()
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk2.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk2.Address()}),
		)
		jsonhttptest.Request(t, client, http.MethodPost, "/pin/chunks/"+chunk2.Address().String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		jsonhttptest.Request(t, client, http.MethodGet, "/pin/chunks", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.ListPinnedChunksResponse{
				Chunks: []api.PinnedChunk{
					{
						Address:    chunk.Address(),
						PinCounter: 1,
					},
					{
						Address:    chunk2.Address(),
						PinCounter: 1,
					},
				},
			}),
		)
	})

	t.Run("update-pin-counter-up", func(t *testing.T) {
		updatePinCounter := api.UpdatePinCounter{
			PinCounter: 7,
		}

		jsonhttptest.Request(t, client, http.MethodPut, "/pin/chunks/"+chunk.Address().String(), http.StatusOK,
			jsonhttptest.WithJSONRequestBody(updatePinCounter),
			jsonhttptest.WithExpectedJSONResponse(api.PinnedChunk{
				Address:    chunk.Address(),
				PinCounter: updatePinCounter.PinCounter,
			}),
		)

		jsonhttptest.Request(t, client, http.MethodGet, "/pin/chunks/"+chunk.Address().String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.PinnedChunk{
				Address:    chunk.Address(),
				PinCounter: updatePinCounter.PinCounter,
			}),
		)
	})

	t.Run("update-pin-counter-to-zero", func(t *testing.T) {
		updatePinCounter := api.UpdatePinCounter{
			PinCounter: 0,
		}

		jsonhttptest.Request(t, client, http.MethodPut, "/pin/chunks/"+chunk.Address().String(), http.StatusOK,
			jsonhttptest.WithJSONRequestBody(updatePinCounter),
			jsonhttptest.WithExpectedJSONResponse(api.PinnedChunk{
				Address:    chunk.Address(),
				PinCounter: updatePinCounter.PinCounter,
			}),
		)

		jsonhttptest.Request(t, client, http.MethodGet, "/pin/chunks/"+chunk.Address().String(), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusNotFound),
				Code:    http.StatusNotFound,
			}),
		)
	})

}
