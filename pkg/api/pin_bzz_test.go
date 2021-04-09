// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/traversal"
)

func TestPinBzzHandler(t *testing.T) {
	var (
		dirUploadResource     = "/dirs"
		pinBzzResource        = "/pin/bzz"
		pinBzzAddressResource = func(addr string) string { return pinBzzResource + "/" + addr }
		pinChunksResource     = "/pin/chunks"

		mockStorer       = mock.NewStorer()
		mockStatestore   = statestore.NewStateStore()
		traversalService = traversal.NewService(mockStorer)
		logger           = logging.New(ioutil.Discard, 0)
		client, _, _     = newTestServer(t, testServerOptions{
			Storer:    mockStorer,
			Traversal: traversalService,
			Tags:      tags.NewTags(mockStatestore, logger),
			Logger:    logger,
		})
	)

	t.Run("pin-bzz-1", func(t *testing.T) {
		files := []f{
			{
				data: []byte("<h1>Swarm"),
				name: "index.html",
				dir:  "",
			},
		}

		tarReader := tarFiles(t, files)

		rootHash := "9e178dbd1ed4b748379e25144e28dfb29c07a4b5114896ef454480115a56b237"

		// verify directory tar upload response
		jsonhttptest.Request(t, client, http.MethodPost, dirUploadResource, http.StatusOK,
			jsonhttptest.WithRequestBody(tarReader),
			jsonhttptest.WithRequestHeader("Content-Type", api.ContentTypeTar),
			jsonhttptest.WithExpectedJSONResponse(api.FileUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
		)

		jsonhttptest.Request(t, client, http.MethodPost, pinBzzAddressResource(rootHash), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		expectedChunkCount := 3

		// get the reference as everytime it will change because of random encryption key
		var resp api.ListPinnedChunksResponse

		jsonhttptest.Request(t, client, http.MethodGet, pinChunksResource, http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if expectedChunkCount != len(resp.Chunks) {
			t.Fatalf("expected to find %d pinned chunks, got %d", expectedChunkCount, len(resp.Chunks))
		}
	})

	t.Run("unpin-bzz-1", func(t *testing.T) {
		rootHash := "9e178dbd1ed4b748379e25144e28dfb29c07a4b5114896ef454480115a56b237"

		jsonhttptest.Request(t, client, http.MethodDelete, pinBzzAddressResource(rootHash), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		jsonhttptest.Request(t, client, http.MethodGet, pinChunksResource, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.ListPinnedChunksResponse{
				Chunks: []api.PinnedChunk{},
			}),
		)
	})

}
