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

	"github.com/ethersphere/bee/pkg/tags"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestChunkUploadDownload uploads a chunk to an API that verifies the chunk according
// to a given validator, then tries to download the uploaded data.
func TestChunkUploadDownload(t *testing.T) {

	var (
		targets         = "0x222"
		chunksEndpoint  = "/chunks"
		chunksResource  = func(a swarm.Address) string { return "/chunks/" + a.String() }
		resourceTargets = func(addr swarm.Address) string { return "/chunks/" + addr.String() + "?targets=" + targets }
		chunk           = testingc.GenerateTestRandomChunk()
		mockStatestore  = statestore.NewStateStore()
		logger          = logging.New(ioutil.Discard, 0)
		tag             = tags.NewTags(mockStatestore, logger)
		mockStorer      = mock.NewStorer()
		client, _, _    = newTestServer(t, testServerOptions{
			Storer: mockStorer,
			Tags:   tag,
		})
	)

	t.Run("empty chunk", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "data length",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
		)

		// try to fetch the same chunk
		resp := request(t, client, http.MethodGet, chunksResource(chunk.Address()), nil, http.StatusOK)
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(chunk.Data(), data) {
			t.Fatal("data retrieved doesnt match uploaded content")
		}
	})

	t.Run("pin-invalid-value", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "invalid-pin"),
		)

		// Also check if the chunk is NOT pinned
		if mockStorer.GetModeSet(chunk.Address()) == storage.ModeSetPin {
			t.Fatal("chunk should not be pinned")
		}
	})
	t.Run("pin-header-missing", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
		)

		// Also check if the chunk is NOT pinned
		if mockStorer.GetModeSet(chunk.Address()) == storage.ModeSetPin {
			t.Fatal("chunk should not be pinned")
		}
	})
	t.Run("pin-ok", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "True"),
		)

		// Also check if the chunk is pinned
		if mockStorer.GetModePut(chunk.Address()) != storage.ModePutUploadPin {
			t.Fatal("chunk is not pinned")
		}

	})
	t.Run("retrieve-targets", func(t *testing.T) {
		resp := request(t, client, http.MethodGet, resourceTargets(chunk.Address()), nil, http.StatusOK)

		// Check if the target is obtained correctly
		if resp.Header.Get(api.TargetsRecoveryHeader) != targets {
			t.Fatalf("targets mismatch. got %s, want %s", resp.Header.Get(api.TargetsRecoveryHeader), targets)
		}
	})
}
