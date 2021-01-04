// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"sort"
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

func TestPinBytesHandler(t *testing.T) {
	var (
		bytesUploadResource     = "/bytes"
		pinBytesResource        = "/pin/bytes"
		pinBytesAddressResource = func(addr string) string { return pinBytesResource + "/" + addr }
		pinChunksResource       = "/pin/chunks"

		simpleData = []byte("this is a simple text")

		mockStorer       = mock.NewStorer()
		mockStatestore   = statestore.NewStateStore()
		traversalService = traversal.NewService(mockStorer)
		logger           = logging.New(ioutil.Discard, 0)
		client, _, _     = newTestServer(t, testServerOptions{
			Storer:    mockStorer,
			Traversal: traversalService,
			Tags:      tags.NewTags(mockStatestore, logger),
		})
	)

	t.Run("pin-bytes-1", func(t *testing.T) {
		rootHash := "838d0a193ecd1152d1bb1432d5ecc02398533b2494889e23b8bd5ace30ac2aeb"

		jsonhttptest.Request(t, client, http.MethodPost, bytesUploadResource, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithExpectedJSONResponse(api.FileUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
		)

		jsonhttptest.Request(t, client, http.MethodPost, pinBytesAddressResource(rootHash), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		hashes := []string{rootHash}
		sort.Strings(hashes)

		expectedResponse := api.ListPinnedChunksResponse{
			Chunks: []api.PinnedChunk{},
		}

		for _, h := range hashes {
			expectedResponse.Chunks = append(expectedResponse.Chunks, api.PinnedChunk{
				Address:    swarm.MustParseHexAddress(h),
				PinCounter: 1,
			})
		}

		jsonhttptest.Request(t, client, http.MethodGet, pinChunksResource, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(expectedResponse),
		)
	})

	t.Run("unpin-bytes-1", func(t *testing.T) {
		rootHash := "838d0a193ecd1152d1bb1432d5ecc02398533b2494889e23b8bd5ace30ac2aeb"

		jsonhttptest.Request(t, client, http.MethodDelete, pinBytesAddressResource(rootHash), http.StatusOK,
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

	t.Run("pin-bytes-2", func(t *testing.T) {
		var b []byte
		for {
			b = append(b, simpleData...)
			if len(b) > swarm.ChunkSize {
				break
			}
		}

		rootHash := "42ee01ae3a50663ca0903f2d5c3b55fc5ef4faf98368b74cf9a24c75955fa388"
		data1Hash := "933db58bbd119e5d3a8eb4fc7d4a923d5e7cfc7ca35b07832ced495d05721b6d"
		data2Hash := "430274f4e6d2af72b5491ad0dc7707e45892fb3a166c54b6ac9cee3c14149757"

		jsonhttptest.Request(t, client, http.MethodPost, bytesUploadResource, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(b)),
			jsonhttptest.WithExpectedJSONResponse(api.FileUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
		)

		jsonhttptest.Request(t, client, http.MethodPost, pinBytesAddressResource(rootHash), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		hashes := []string{rootHash, data1Hash, data2Hash}
		sort.Strings(hashes)

		// NOTE: all this because we cannot rely on sort from response

		var resp api.ListPinnedChunksResponse

		jsonhttptest.Request(t, client, http.MethodGet, pinChunksResource, http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if len(hashes) != len(resp.Chunks) {
			t.Fatalf("expected to find %d pinned chunks, got %d", len(hashes), len(resp.Chunks))
		}

		respChunksHashes := make([]string, 0)

		for _, rc := range resp.Chunks {
			respChunksHashes = append(respChunksHashes, rc.Address.String())
		}

		sort.Strings(respChunksHashes)

		for i, h := range hashes {
			if h != respChunksHashes[i] {
				t.Fatalf("expected to find %s address, found %s", h, respChunksHashes[i])
			}
		}
	})

}
