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
	"github.com/ethersphere/bee/pkg/storage/mock/validator"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

// TestPinChunkHandler checks for pinning, unpinning and listing of chunks.
// It also check other edgw cases like chunk not present and checking for pinning,
// invalid chunk address case etc. This test case has to be run in sequence and
// it assumes some state of the DB before another case is run.
func TestPinChunkHandler(t *testing.T) {
	var (
		resource             = func(addr swarm.Address) string { return "/chunks/" + addr.String() }
		hash                 = swarm.MustParseHexAddress("aabbcc")
		data                 = []byte("bbaatt")
		mockValidator        = validator.NewMockValidator(hash, data)
		mockValidatingStorer = mock.NewStorer(mock.WithValidator(mockValidator))
		mockStatestore       = statestore.NewStateStore()
		logger               = logging.New(ioutil.Discard, 0)
		tag                  = tags.NewTags(mockStatestore, logger)

		client, _, _ = newTestServer(t, testServerOptions{
			Storer: mockValidatingStorer,
			Tags:   tag,
			Logger: logger,
		})
	)

	// bad chunk address
	t.Run("pin-bad-address", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/pinning/chunks/abcd1100zz", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "bad address",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	// list pins without anything pinned
	t.Run("list-pins-zero-pins", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, "/pinning/chunks", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.ListPinnedChunksResponse{
				Chunks: []api.PinnedChunk{},
			}),
		)
	})

	// pin a chunk which is not existing
	t.Run("pin-absent-chunk", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/pinning/chunks/123456", http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusNotFound),
				Code:    http.StatusNotFound,
			}),
		)
	})

	// unpin on a chunk which is not pinned
	t.Run("unpin-while-not-pinned", func(t *testing.T) {
		// Post a chunk
		jsonhttptest.Request(t, client, http.MethodPost, resource(hash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(data)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		jsonhttptest.Request(t, client, http.MethodDelete, "/pinning/chunks/"+hash.String(), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "chunk is not yet pinned",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	// pin a existing chunk first time
	t.Run("pin-chunk-1", func(t *testing.T) {
		// Post a chunk
		jsonhttptest.Request(t, client, http.MethodPost, resource(hash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(data)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		jsonhttptest.Request(t, client, http.MethodPost, "/pinning/chunks/"+hash.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		// Check is the chunk is pinned once
		jsonhttptest.Request(t, client, http.MethodGet, "/pinning/chunks/"+hash.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.PinnedChunk{
				Address:    swarm.MustParseHexAddress("aabbcc"),
				PinCounter: 1,
			}),
		)

	})

	// pin a existing chunk second time
	t.Run("pin-chunk-2", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/pinning/chunks/"+hash.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		// Check is the chunk is pinned twice
		jsonhttptest.Request(t, client, http.MethodGet, "/pinning/chunks/"+hash.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.PinnedChunk{
				Address:    swarm.MustParseHexAddress("aabbcc"),
				PinCounter: 2,
			}),
		)
	})

	// unpin a chunk first time
	t.Run("unpin-chunk-1", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodDelete, "/pinning/chunks/"+hash.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		// Check is the chunk is pinned once
		jsonhttptest.Request(t, client, http.MethodGet, "/pinning/chunks/"+hash.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.PinnedChunk{
				Address:    swarm.MustParseHexAddress("aabbcc"),
				PinCounter: 1,
			}),
		)
	})

	// unpin a chunk second time
	t.Run("unpin-chunk-2", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodDelete, "/pinning/chunks/"+hash.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		// Check if the chunk is removed from the pinIndex
		jsonhttptest.Request(t, client, http.MethodGet, "/pinning/chunks/"+hash.String(), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusNotFound),
				Code:    http.StatusNotFound,
			}),
		)
	})

	// Add 2 chunks, pin it and check if they show up in the list
	t.Run("list-chunks", func(t *testing.T) {
		// Post a chunk
		jsonhttptest.Request(t, client, http.MethodPost, resource(hash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(data)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		jsonhttptest.Request(t, client, http.MethodPost, "/pinning/chunks/"+hash.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		// post another chunk
		hash2 := swarm.MustParseHexAddress("ddeeff")
		data2 := []byte("eagle")
		mockValidator.AddPair(hash2, data2)
		jsonhttptest.Request(t, client, http.MethodPost, resource(hash2), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(data2)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)
		jsonhttptest.Request(t, client, http.MethodPost, "/pinning/chunks/"+hash2.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		jsonhttptest.Request(t, client, http.MethodGet, "/pinning/chunks", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.ListPinnedChunksResponse{
				Chunks: []api.PinnedChunk{
					{
						Address:    swarm.MustParseHexAddress("aabbcc"),
						PinCounter: 1,
					},
					{
						Address:    swarm.MustParseHexAddress("ddeeff"),
						PinCounter: 1,
					},
				},
			}),
		)
	})
}
