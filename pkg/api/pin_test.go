// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	pinning "github.com/ethersphere/bee/pkg/pinning/mock"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/traversal"
)

func checkPinHandlers(t *testing.T, client *http.Client, rootHash string) {
	t.Helper()

	const pinsBasePath = "/pins"

	var (
		pinsAddressPath        = pinsBasePath + "/" + rootHash
		pinsInvalidAddressPath = pinsBasePath + "/" + "838d0a193ecd1152d1bb1432d5ecc02398533b2494889e23b8bd5ace30ac2zzz"
		pinsUnknownAddressPath = pinsBasePath + "/" + "838d0a193ecd1152d1bb1432d5ecc02398533b2494889e23b8bd5ace30ac2ccc"
	)

	jsonhttptest.Request(t, client, http.MethodGet, pinsInvalidAddressPath, http.StatusBadRequest)

	jsonhttptest.Request(t, client, http.MethodGet, pinsUnknownAddressPath, http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusNotFound),
			Code:    http.StatusNotFound,
		}),
	)

	jsonhttptest.Request(t, client, http.MethodPost, pinsAddressPath, http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}),
	)

	jsonhttptest.Request(t, client, http.MethodGet, pinsAddressPath, http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(struct {
			Address swarm.Address `json:"address"`
		}{
			Address: swarm.MustParseHexAddress(rootHash),
		}),
	)

	jsonhttptest.Request(t, client, http.MethodGet, pinsBasePath, http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(struct {
			Addresses []swarm.Address `json:"addresses"`
		}{
			Addresses: []swarm.Address{swarm.MustParseHexAddress(rootHash)},
		}),
	)

	jsonhttptest.Request(t, client, http.MethodDelete, pinsAddressPath, http.StatusOK)

	jsonhttptest.Request(t, client, http.MethodGet, pinsAddressPath, http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusNotFound),
			Code:    http.StatusNotFound,
		}),
	)
}

func TestPinHandlers(t *testing.T) {
	var (
		storerMock   = mock.NewStorer()
		client, _, _ = newTestServer(t, testServerOptions{
			Storer:    storerMock,
			Traversal: traversal.NewService(storerMock),
			Tags:      tags.NewTags(statestore.NewStateStore(), logging.New(ioutil.Discard, 0)),
			Pinning:   pinning.NewServiceMock(),
			Logger:    logging.New(ioutil.Discard, 5),
			Post:      mockpost.New(mockpost.WithAcceptAll()),
		})
	)

	t.Run("bytes", func(t *testing.T) {
		const rootHash = "838d0a193ecd1152d1bb1432d5ecc02398533b2494889e23b8bd5ace30ac2aeb"
		jsonhttptest.Request(t, client, http.MethodPost, "/bytes", http.StatusOK,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(strings.NewReader("this is a simple text")),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
		)
		checkPinHandlers(t, client, rootHash)
	})

	t.Run("bzz", func(t *testing.T) {
		tarReader := tarFiles(t, []f{{
			data: []byte("<h1>Swarm"),
			name: "index.html",
			dir:  "",
		}})
		rootHash := "9e178dbd1ed4b748379e25144e28dfb29c07a4b5114896ef454480115a56b237"
		jsonhttptest.Request(t, client, http.MethodPost, "/bzz", http.StatusOK,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(tarReader),
			jsonhttptest.WithRequestHeader("Content-Type", api.ContentTypeTar),
			jsonhttptest.WithRequestHeader(api.SwarmCollectionHeader, "True"),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
		)
		checkPinHandlers(t, client, rootHash)

		rootHash = "dd13a5a6cc9db3ef514d645e6719178dbfb1a90b49b9262cafce35b0d27cf245"
		jsonhttptest.Request(t, client, http.MethodPost, "/bzz?name=somefile.txt", http.StatusOK,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader("Content-Type", "text/plain"),
			jsonhttptest.WithRequestBody(strings.NewReader("this is a simple text")),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
		)
		checkPinHandlers(t, client, rootHash)
	})

	t.Run("chunk", func(t *testing.T) {
		var (
			chunk    = testingc.GenerateTestRandomChunk()
			rootHash = chunk.Address().String()
		)
		jsonhttptest.Request(t, client, http.MethodPost, "/chunks", http.StatusOK,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{
				Reference: chunk.Address(),
			}),
		)
		checkPinHandlers(t, client, rootHash)
	})
}
