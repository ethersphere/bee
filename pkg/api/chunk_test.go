// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	pinning "github.com/ethersphere/bee/pkg/pinning/mock"
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
		statestoreMock  = statestore.NewStateStore()
		logger          = logging.New(ioutil.Discard, 0)
		tag             = tags.NewTags(statestoreMock, logger)
		storerMock      = mock.NewStorer()
		pinningMock     = pinning.NewServiceMock()
		client, _, _    = newTestServer(t, testServerOptions{
			Storer:  storerMock,
			Pinning: pinningMock,
			Tags:    tag,
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
		if storerMock.GetModeSet(chunk.Address()) == storage.ModeSetPin {
			t.Fatal("chunk should not be pinned")
		}
	})
	t.Run("pin-header-missing", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
		)

		// Also check if the chunk is NOT pinned
		if storerMock.GetModeSet(chunk.Address()) == storage.ModeSetPin {
			t.Fatal("chunk should not be pinned")
		}
	})
	t.Run("pin-ok", func(t *testing.T) {
		address := chunk.Address()
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: address}),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "True"),
		)

		has, err := storerMock.Has(context.Background(), address)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatal("storer check root chunk address: have none; want one")
		}

		if have, want := len(pinningMock.Entries()), 1; have != want {
			t.Fatalf("root pin count mismatch: have %d; want %d", have, want)
		}
		addrs, err := pinningMock.Pins()
		if err != nil {
			t.Fatal(err)
		}
		if have, want := addrs[0], address; !have.Equal(want) {
			t.Fatalf("root pin reference mismatch: have %q; want %q", have, want)
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
